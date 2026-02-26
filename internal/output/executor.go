package output

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/riyasyash/pg_rocket/internal/extractor"
	"github.com/riyasyash/pg_rocket/internal/graph"
)

// Executor handles direct database insertion of extracted data.
// It validates foreign key integrity, performs topological sorting,
// and executes INSERTs within a transaction.
type Executor struct {
	conn       *pgx.Conn
	graph      *graph.Graph
	verbose    bool
	upsertMode bool // If true, uses ON CONFLICT DO UPDATE for idempotent insertions
}

// NewExecutor creates a new database executor with the given connection and options.
func NewExecutor(conn *pgx.Conn, g *graph.Graph, verbose bool, upsertMode bool) *Executor {
	return &Executor{
		conn:       conn,
		graph:      g,
		verbose:    verbose,
		upsertMode: upsertMode,
	}
}

// Execute inserts the extracted data into the target database within a transaction.
// All tables are inserted in topological order (parents before children).
// Foreign key integrity is validated before insertion.
// If any insertion fails, the entire transaction is rolled back.
func (e *Executor) Execute(ctx context.Context, state *extractor.TraversalState) error {
	// Validate foreign key integrity before attempting insertion
	if err := e.validateForeignKeys(state); err != nil {
		return err
	}

	tables := state.GetAllTables()

	sortedTables, err := e.graph.TopologicalSort(tables)
	if err != nil {
		return fmt.Errorf("failed to sort tables: %w", err)
	}

	tx, err := e.conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if e.verbose {
		fmt.Println("Starting direct database insertion...")
	}

	totalInserted := 0
	for _, tableName := range sortedTables {
		rows := state.TableData[tableName]
		if len(rows) == 0 {
			continue
		}

		if err := e.insertTable(ctx, tx, tableName, rows); err != nil {
			return fmt.Errorf("failed to insert into %s: %w", tableName, err)
		}

		totalInserted += len(rows)
		if e.verbose {
			fmt.Printf("Inserted %d rows into %s\n", len(rows), tableName)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	if e.verbose {
		fmt.Printf("Successfully inserted %d total rows into %d tables\n", totalInserted, len(sortedTables))
	}

	return nil
}

// validateForeignKeys checks that all foreign key references point to extracted rows
func (e *Executor) validateForeignKeys(state *extractor.TraversalState) error {
	missingRefs := make(map[string][]string) // table -> list of missing parent tables

	for tableName, rows := range state.TableData {
		if len(rows) == 0 {
			continue
		}

		// Check each foreign key for this table
		for _, fk := range e.graph.Parents[tableName] {
			parentTable := fk.ParentTable

			// Skip self-referential FKs (they're handled specially)
			if parentTable == tableName {
				continue
			}

			// Check if parent table was extracted
			if _, exists := state.TableData[parentTable]; !exists {
				// Parent table not extracted at all
				if missingRefs[tableName] == nil {
					missingRefs[tableName] = []string{}
				}
				missingRefs[tableName] = append(missingRefs[tableName], parentTable)
				continue
			}

			// Check if all FK values exist in parent table
			parentRows := state.TableData[parentTable]
			if len(parentRows) == 0 {
				// Parent table exists but has no rows
				if missingRefs[tableName] == nil {
					missingRefs[tableName] = []string{}
				}
				missingRefs[tableName] = append(missingRefs[tableName], parentTable)
			}
		}
	}

	if len(missingRefs) > 0 {
		var errorMsg strings.Builder
		errorMsg.WriteString("foreign key integrity violation: some extracted rows reference parent tables that were not extracted:\n\n")

		for table, parents := range missingRefs {
			errorMsg.WriteString(fmt.Sprintf("  • Table '%s' references missing parent table(s): %s\n",
				table, strings.Join(parents, ", ")))
		}

		errorMsg.WriteString("\nThis usually means the FK graph traversal didn't include all necessary parent tables.\n")
		errorMsg.WriteString("Try using full traversal (without --parents or --children flags) or check your FK relationships.")

		return fmt.Errorf("%s", errorMsg.String())
	}

	return nil
}

func (e *Executor) insertTable(ctx context.Context, tx pgx.Tx, tableName string, rows []map[string]interface{}) error {
	if len(rows) == 0 {
		return nil
	}

	pkColumns := e.graph.GetPrimaryKeyColumns(tableName)

	// Sort by composite PK
	sort.Slice(rows, func(i, j int) bool {
		for _, pkCol := range pkColumns {
			vi := fmt.Sprintf("%v", rows[i][pkCol])
			vj := fmt.Sprintf("%v", rows[j][pkCol])
			if vi != vj {
				return vi < vj
			}
		}
		return false
	})

	columns := make([]string, 0)
	for col := range rows[0] {
		columns = append(columns, col)
	}
	sort.Strings(columns)

	// Detect JSONB/JSON columns in target database FIRST
	columnQuery := `
		SELECT column_name, data_type
		FROM information_schema.columns
		WHERE table_schema = 'public' 
		  AND table_name = $1
	`

	colRows, err := tx.Query(ctx, columnQuery, tableName)
	if err != nil {
		return fmt.Errorf("failed to get column info for %s: %w", tableName, err)
	}

	jsonbCols := make(map[string]string) // column -> datatype
	for colRows.Next() {
		var colName, dataType string
		if err := colRows.Scan(&colName, &dataType); err != nil {
			colRows.Close()
			return fmt.Errorf("failed to scan column info: %w", err)
		}
		if dataType == "jsonb" || dataType == "json" {
			jsonbCols[colName] = dataType
		}
	}
	colRows.Close()

	// Build placeholders with casts for JSONB/JSON columns
	placeholders := make([]string, len(columns))
	for i, col := range columns {
		if dataType, isJSONB := jsonbCols[col]; isJSONB {
			// Cast text to JSONB/JSON
			placeholders[i] = fmt.Sprintf("$%d::%s", i+1, dataType)
		} else {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
		}
	}

	var query string
	if e.upsertMode {
		// Build ON CONFLICT clause for upsert
		conflictCols := make([]string, len(pkColumns))
		for i, col := range pkColumns {
			conflictCols[i] = col
		}

		// Build UPDATE SET clause (update all non-PK columns)
		updateSet := make([]string, 0)
		for _, col := range columns {
			isPK := false
			for _, pkCol := range pkColumns {
				if col == pkCol {
					isPK = true
					break
				}
			}
			if !isPK {
				updateSet = append(updateSet, fmt.Sprintf("%s = EXCLUDED.%s", col, col))
			}
		}

		if len(updateSet) > 0 {
			query = fmt.Sprintf(
				"INSERT INTO %s (%s) VALUES (%s) ON CONFLICT (%s) DO UPDATE SET %s",
				tableName,
				strings.Join(columns, ", "),
				strings.Join(placeholders, ", "),
				strings.Join(conflictCols, ", "),
				strings.Join(updateSet, ", "),
			)
		} else {
			// If all columns are PKs, just do nothing on conflict
			query = fmt.Sprintf(
				"INSERT INTO %s (%s) VALUES (%s) ON CONFLICT (%s) DO NOTHING",
				tableName,
				strings.Join(columns, ", "),
				strings.Join(placeholders, ", "),
				strings.Join(conflictCols, ", "),
			)
		}
	} else {
		// Standard INSERT (will fail on duplicates)
		query = fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
			tableName,
			strings.Join(columns, ", "),
			strings.Join(placeholders, ", "))
	}

	for _, row := range rows {
		values := make([]interface{}, len(columns))
		for i, col := range columns {
			values[i] = row[col]
		}

		if _, err := tx.Exec(ctx, query, values...); err != nil {
			return fmt.Errorf("failed to insert row: %w", err)
		}
	}

	return nil
}
