package output

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"github.com/riyasyash/pg_rocket/internal/extractor"
	"github.com/riyasyash/pg_rocket/internal/graph"
)

// JSONWriter generates JSON output from extracted data with topological ordering.
type JSONWriter struct {
	writer io.Writer
	graph  *graph.Graph
}

// NewJSONWriter creates a new JSON writer that outputs to the given writer.
func NewJSONWriter(writer io.Writer, g *graph.Graph) *JSONWriter {
	return &JSONWriter{
		writer: writer,
		graph:  g,
	}
}

// Write outputs the extracted data as JSON with tables sorted topologically
// and rows sorted by primary key for deterministic output.
func (w *JSONWriter) Write(ctx context.Context, state *extractor.TraversalState) error {
	tables := state.GetAllTables()

	sortedTables, err := w.graph.TopologicalSort(tables)
	if err != nil {
		return fmt.Errorf("failed to sort tables: %w", err)
	}

	state.Progress.OutputGeneration("JSON")

	result := make(map[string][]map[string]interface{})

	for i, tableName := range sortedTables {
		rows := state.TableData[tableName]
		if len(rows) == 0 {
			result[tableName] = []map[string]interface{}{}
			continue
		}

		state.Progress.WritingTable(tableName, len(rows), i+1, len(sortedTables))

		pkColumns := w.graph.GetPrimaryKeyColumns(tableName)

		sortedRows := make([]map[string]interface{}, len(rows))
		copy(sortedRows, rows)

		// Sort by composite PK
		sort.Slice(sortedRows, func(i, j int) bool {
			for _, pkCol := range pkColumns {
				vi := fmt.Sprintf("%v", sortedRows[i][pkCol])
				vj := fmt.Sprintf("%v", sortedRows[j][pkCol])
				if vi != vj {
					return vi < vj
				}
			}
			return false
		})

		result[tableName] = sortedRows
	}

	state.Progress.FinishProgress()

	encoder := json.NewEncoder(w.writer)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(result); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}

	return nil
}
