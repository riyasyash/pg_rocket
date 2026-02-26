package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/jackc/pgx/v5"
	"github.com/riyasyash/pg_rocket/internal/db"
	"github.com/riyasyash/pg_rocket/internal/extractor"
	"github.com/riyasyash/pg_rocket/internal/output"
	"github.com/spf13/cobra"
)

var (
	query        string
	sourceDSN    string
	targetDSN    string
	parentsOnly  bool
	childrenList string
	outFile      string
	jsonFormat   bool
	dryRun       bool
	maxRows      int
	force        bool
	verbose      bool
	execMode     bool
	upsertMode   bool
)

var pullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Extract referentially complete data subset",
	Long: `Pull extracts data starting from a root query and follows foreign key
relationships to create a referentially complete subset.`,
	RunE: runPull,
}

func init() {
	pullCmd.Flags().StringVar(&query, "query", "", "Root SQL query (required)")
	pullCmd.Flags().StringVar(&sourceDSN, "source", "", "Source database DSN (default: PGROCKET_SOURCE env var)")
	pullCmd.Flags().StringVar(&targetDSN, "target", "", "Target database DSN for --exec mode (default: PGROCKET_TARGET env var or same as source)")
	pullCmd.Flags().BoolVar(&parentsOnly, "parents", false, "Traverse upward only")
	pullCmd.Flags().StringVar(&childrenList, "children", "", "Comma-separated child tables for downward traversal")
	pullCmd.Flags().StringVar(&outFile, "out", "", "Output file (default: stdout)")
	pullCmd.Flags().BoolVar(&jsonFormat, "json", false, "Output JSON instead of SQL")
	pullCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print extraction plan only")
	pullCmd.Flags().IntVar(&maxRows, "max-rows", 10000, "Hard row cap")
	pullCmd.Flags().BoolVar(&force, "force", false, "Override row cap")
	pullCmd.Flags().BoolVar(&verbose, "verbose", false, "Print traversal logs")
	pullCmd.Flags().BoolVar(&execMode, "exec", false, "Execute INSERTs directly against target database")
	pullCmd.Flags().BoolVar(&upsertMode, "upsert", false, "Use ON CONFLICT DO UPDATE for successive runs (requires --exec)")

	pullCmd.MarkFlagRequired("query")
}

func runPull(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Set source DSN with priority: --source flag > PGROCKET_SOURCE env
	if sourceDSN == "" {
		sourceDSN = os.Getenv("PGROCKET_SOURCE")
	}

	// Validate: source must be specified
	if sourceDSN == "" {
		return fmt.Errorf("source database not specified. Use --source flag or set PGROCKET_SOURCE environment variable")
	}

	// Validate: if --exec is used, target must be different from source
	if execMode {
		targetDSN := getTargetDSN()
		if targetDSN == "" {
			return fmt.Errorf("--exec mode requires a target database. Use --target flag or set PGROCKET_TARGET environment variable")
		}
		if targetDSN == sourceDSN {
			return fmt.Errorf("--exec mode requires source and target databases to be different. Target is currently the same as source")
		}
	}

	// Validate: --exec and --out are mutually exclusive
	if execMode && outFile != "" {
		return fmt.Errorf("cannot use both --exec and --out flags together. Choose either direct execution or file output")
	}

	// Validate: --upsert requires --exec
	if upsertMode && !execMode {
		return fmt.Errorf("--upsert flag requires --exec mode")
	}

	conn, err := db.NewConnection(ctx, sourceDSN)
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer conn.Close()

	engine, err := extractor.NewEngine(ctx, conn)
	if err != nil {
		return err
	}

	opts := &extractor.TraversalOptions{
		ParentsOnly:  parentsOnly,
		ChildrenOnly: childrenList != "" && !parentsOnly,
		MaxRows:      maxRows,
		Force:        force,
		Verbose:      verbose,
	}

	if childrenList != "" {
		children := strings.Split(childrenList, ",")
		for i := range children {
			children[i] = strings.TrimSpace(children[i])
		}
		opts.SelectedChildren = children
	}

	if dryRun {
		fmt.Println("Dry run mode - extraction plan:")
		fmt.Printf("Query: %s\n", query)
		fmt.Printf("Parents only: %v\n", parentsOnly)
		fmt.Printf("Children filter: %v\n", opts.SelectedChildren)
		fmt.Printf("Max rows: %d\n", maxRows)
		return nil
	}

	state, err := engine.Extract(ctx, query, opts)
	if err != nil {
		return err
	}

	if execMode {
		return executeToDatabase(ctx, state, engine)
	}

	return writeOutput(ctx, state, engine)
}

func writeOutput(ctx context.Context, state *extractor.TraversalState, engine *extractor.Engine) error {
	var writer *os.File
	var err error

	if outFile != "" {
		writer, err = os.Create(outFile)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer writer.Close()
	} else {
		writer = os.Stdout
	}

	if jsonFormat {
		jsonWriter := output.NewJSONWriter(writer, engine.Graph)
		return jsonWriter.Write(ctx, state)
	}

	sqlWriter := output.NewSQLWriter(writer, engine.Graph)
	return sqlWriter.Write(ctx, state)
}

func getTargetDSN() string {
	// Priority: --target flag > PGROCKET_TARGET env
	dsn := targetDSN
	if dsn == "" {
		dsn = os.Getenv("PGROCKET_TARGET")
	}
	return dsn
}

func executeToDatabase(ctx context.Context, state *extractor.TraversalState, engine *extractor.Engine) error {
	dsn := getTargetDSN()

	// This should never happen due to validation in runPull, but double-check
	if dsn == "" {
		return fmt.Errorf("target database not specified. Use --target flag or PGROCKET_TARGET env var")
	}

	// Display summary and ask for confirmation
	totalRows := 0
	tableCount := len(state.TableData)
	for _, rows := range state.TableData {
		totalRows += len(rows)
	}

	// Extract host info from DSN (mask password)
	sourceInfo := maskDSN(sourceDSN)
	targetInfo := maskDSN(dsn)

	yellow := color.New(color.FgYellow, color.Bold)
	cyan := color.New(color.FgCyan, color.Bold)
	red := color.New(color.FgRed, color.Bold)

	fmt.Println()
	yellow.Println("⚠️  DATABASE WRITE OPERATION")
	fmt.Println(strings.Repeat("=", 60))
	cyan.Println("Source:")
	fmt.Printf("  %s\n", sourceInfo)
	fmt.Println()
	red.Println("Target (will be modified):")
	fmt.Printf("  %s\n", targetInfo)
	fmt.Println()
	cyan.Println("Data to be inserted:")
	fmt.Printf("  Tables: %d\n", tableCount)
	fmt.Printf("  Total rows: %d\n", totalRows)
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println()

	// Ask for confirmation
	fmt.Print("Are you sure you want to INSERT this data into the target database? (yes/no): ")

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read confirmation: %w", err)
	}

	response = strings.TrimSpace(strings.ToLower(response))
	if response != "yes" && response != "y" {
		yellow.Println("\n❌ Operation cancelled by user")
		return nil
	}

	fmt.Println()
	cyan.Println("✓ Confirmed. Proceeding with database insertion...")
	if upsertMode {
		cyan.Println("✓ Upsert mode enabled: existing rows will be updated")
	}
	fmt.Println()

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return fmt.Errorf("failed to connect to target database: %w", err)
	}
	defer conn.Close(ctx)

	executor := output.NewExecutor(conn, engine.Graph, verbose, upsertMode)
	return executor.Execute(ctx, state)
}

// maskDSN masks the password in a DSN for display purposes
func maskDSN(dsn string) string {
	// Simple masking: postgres://user:password@host:port/db -> postgres://user:***@host:port/db
	if strings.Contains(dsn, "@") {
		parts := strings.SplitN(dsn, "@", 2)
		if len(parts) == 2 {
			userPart := parts[0]
			hostPart := parts[1]

			// Mask password in user part
			if strings.Contains(userPart, ":") {
				userParts := strings.SplitN(userPart, ":", 2)
				if len(userParts) == 2 {
					return userParts[0] + ":***@" + hostPart
				}
			}
		}
	}
	return dsn
}
