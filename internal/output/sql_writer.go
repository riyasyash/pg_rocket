// Package output provides writers for generating SQL INSERTs, JSON output,
// and direct database execution. All writers maintain deterministic ordering
// and support composite primary keys.
package output

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/riyasyash/pg_rocket/internal/extractor"
	"github.com/riyasyash/pg_rocket/internal/graph"
)

// SQLWriter generates PostgreSQL INSERT statements from extracted data.
type SQLWriter struct {
	writer io.Writer
	graph  *graph.Graph
}

// NewSQLWriter creates a new SQL writer that outputs to the given writer.
func NewSQLWriter(writer io.Writer, g *graph.Graph) *SQLWriter {
	return &SQLWriter{
		writer: writer,
		graph:  g,
	}
}

func (w *SQLWriter) Write(ctx context.Context, state *extractor.TraversalState) error {
	tables := state.GetAllTables()

	sortedTables, err := w.graph.TopologicalSort(tables)
	if err != nil {
		return fmt.Errorf("failed to sort tables: %w", err)
	}

	state.Progress.OutputGeneration("SQL")

	fmt.Fprintln(w.writer, "-- pg_rocket data export")
	fmt.Fprintf(w.writer, "-- Generated at: %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintln(w.writer, "-- Total tables:", len(sortedTables))
	fmt.Fprintln(w.writer)

	for i, tableName := range sortedTables {
		rows := state.TableData[tableName]
		if len(rows) == 0 {
			continue
		}

		state.Progress.WritingTable(tableName, len(rows), i+1, len(sortedTables))

		if err := w.writeTable(tableName, rows); err != nil {
			return err
		}
	}

	state.Progress.FinishProgress()
	return nil
}

func (w *SQLWriter) writeTable(tableName string, rows []map[string]interface{}) error {
	if len(rows) == 0 {
		return nil
	}

	pkColumns := w.graph.GetPrimaryKeyColumns(tableName)

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
	if len(rows) > 0 {
		for col := range rows[0] {
			columns = append(columns, col)
		}
		sort.Strings(columns)
	}

	fmt.Fprintf(w.writer, "-- Table: %s (%d rows)\n", tableName, len(rows))
	fmt.Fprintf(w.writer, "INSERT INTO %s (%s)\nVALUES\n",
		tableName, strings.Join(columns, ", "))

	for i, row := range rows {
		values := make([]string, len(columns))
		for j, col := range columns {
			values[j] = formatValue(row[col])
		}

		fmt.Fprintf(w.writer, "  (%s)", strings.Join(values, ", "))

		if i < len(rows)-1 {
			fmt.Fprintln(w.writer, ",")
		} else {
			fmt.Fprintln(w.writer, ";")
		}
	}

	fmt.Fprintln(w.writer)
	return nil
}

func formatValue(value interface{}) string {
	if value == nil {
		return "NULL"
	}

	switch v := value.(type) {
	case string:
		escaped := strings.ReplaceAll(v, "'", "''")
		escaped = strings.ReplaceAll(escaped, "\\", "\\\\")
		return fmt.Sprintf("'%s'", escaped)

	case []byte:
		return fmt.Sprintf("'\\x%x'", v)

	case time.Time:
		return fmt.Sprintf("'%s'", v.Format(time.RFC3339Nano))

	case bool:
		if v {
			return "true"
		}
		return "false"

	case int, int8, int16, int32, int64:
		return fmt.Sprintf("%d", v)

	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", v)

	case float32, float64:
		return fmt.Sprintf("%v", v)

	case map[string]interface{}:
		return formatJSONB(v)

	case []interface{}:
		return formatArray(v)

	default:
		str := fmt.Sprintf("%v", v)
		if strings.HasPrefix(str, "{") || strings.HasPrefix(str, "[") {
			escaped := strings.ReplaceAll(str, "'", "''")
			return fmt.Sprintf("'%s'", escaped)
		}
		escaped := strings.ReplaceAll(str, "'", "''")
		return fmt.Sprintf("'%s'", escaped)
	}
}

func formatJSONB(m map[string]interface{}) string {
	parts := make([]string, 0, len(m))
	keys := make([]string, 0, len(m))

	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		v := m[k]
		valStr := formatJSONValue(v)
		parts = append(parts, fmt.Sprintf(`"%s":%s`, k, valStr))
	}

	return fmt.Sprintf("'{%s}'::jsonb", strings.Join(parts, ","))
}

func formatJSONValue(value interface{}) string {
	if value == nil {
		return "null"
	}

	switch v := value.(type) {
	case string:
		escaped := strings.ReplaceAll(v, "\\", "\\\\")
		escaped = strings.ReplaceAll(escaped, `"`, `\"`)
		return fmt.Sprintf(`"%s"`, escaped)
	case bool:
		if v {
			return "true"
		}
		return "false"
	case float64, int, int64:
		return fmt.Sprintf("%v", v)
	default:
		return fmt.Sprintf(`"%v"`, v)
	}
}

func formatArray(arr []interface{}) string {
	if len(arr) == 0 {
		return "ARRAY[]"
	}

	elements := make([]string, len(arr))
	for i, elem := range arr {
		elements[i] = formatValue(elem)
	}

	return fmt.Sprintf("ARRAY[%s]", strings.Join(elements, ", "))
}
