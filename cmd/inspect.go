package cmd

import (
	"context"
	"fmt"
	"os"
	"sort"

	"github.com/riyasyash/pg_rocket/internal/db"
	"github.com/spf13/cobra"
)

var (
	inspectSourceDSN string
)

var inspectCmd = &cobra.Command{
	Use:   "inspect",
	Short: "Display the foreign key graph of the database",
	Long:  `Inspect shows the foreign key relationships in the connected database.`,
	RunE:  runInspect,
}

func init() {
	inspectCmd.Flags().StringVar(&inspectSourceDSN, "source", "", "Source database DSN (default: PGROCKET_SOURCE env var)")
}

func runInspect(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Set source DSN with priority: --source flag > PGROCKET_SOURCE env
	if inspectSourceDSN == "" {
		inspectSourceDSN = os.Getenv("PGROCKET_SOURCE")
	}

	conn, err := db.NewConnection(ctx, inspectSourceDSN)
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer conn.Close()

	metadata, err := conn.ExtractMetadata(ctx)
	if err != nil {
		return fmt.Errorf("failed to extract metadata: %w", err)
	}

	allTables := make(map[string]bool)
	for table := range metadata.Parents {
		allTables[table] = true
	}
	for table := range metadata.Children {
		allTables[table] = true
	}

	tables := make([]string, 0, len(allTables))
	for table := range allTables {
		tables = append(tables, table)
	}
	sort.Strings(tables)

	fmt.Println("Database Foreign Key Graph:")
	fmt.Println()

	for _, table := range tables {
		fmt.Printf("%s\n", table)

		parents := metadata.Parents[table]
		if len(parents) > 0 {
			sort.Slice(parents, func(i, j int) bool {
				return parents[i].ParentTable < parents[j].ParentTable
			})

			for _, fk := range parents {
				fmt.Printf("  ↑ %s (via %s)\n", fk.ParentTable, fk.ChildColumn)
			}
		}

		children := metadata.Children[table]
		if len(children) > 0 {
			sort.Slice(children, func(i, j int) bool {
				return children[i].ChildTable < children[j].ChildTable
			})

			for _, fk := range children {
				fmt.Printf("  ↓ %s (via %s.%s)\n", fk.ChildTable, fk.ChildTable, fk.ChildColumn)
			}
		}

		fmt.Println()
	}

	return nil
}
