package graph

import (
	"fmt"
	"sort"
	"strings"

	"github.com/riyasyash/pg_rocket/internal/db"
)

// BuildGraph constructs a foreign key graph from database metadata and validates
// that no multi-table cycles exist. Returns an error if cycles are detected.
func BuildGraph(metadata *db.Metadata) (*Graph, error) {
	graph := NewGraph(metadata)

	if err := graph.DetectMultiTableCycles(); err != nil {
		return nil, err
	}

	return graph, nil
}

// DetectMultiTableCycles performs depth-first search to detect cycles in the FK graph.
// Self-referential foreign keys are excluded. Returns an error if a cycle is found,
// including the path of tables involved in the cycle.
func (g *Graph) DetectMultiTableCycles() error {
	allTables := make(map[string]bool)

	for table := range g.Parents {
		allTables[table] = true
	}
	for table := range g.Children {
		allTables[table] = true
	}

	color := make(map[string]int)
	parent := make(map[string]string)

	const (
		WHITE = 0
		GRAY  = 1
		BLACK = 2
	)

	for table := range allTables {
		color[table] = WHITE
	}

	var dfs func(string) error
	dfs = func(table string) error {
		color[table] = GRAY

		for _, fk := range g.Children[table] {
			childTable := fk.ChildTable

			if childTable == table {
				continue
			}

			if color[childTable] == GRAY {
				cycle := []string{childTable}
				current := table
				for current != childTable && current != "" {
					cycle = append(cycle, current)
					current = parent[current]
				}
				cycle = append(cycle, childTable)

				for i, j := 0, len(cycle)-1; i < j; i, j = i+1, j-1 {
					cycle[i], cycle[j] = cycle[j], cycle[i]
				}

				return fmt.Errorf("cyclic foreign keys detected: %s. Use --parents or --children to avoid cycles", strings.Join(cycle, " -> "))
			}

			if color[childTable] == WHITE {
				parent[childTable] = table
				if err := dfs(childTable); err != nil {
					return err
				}
			}
		}

		color[table] = BLACK
		return nil
	}

	tables := make([]string, 0, len(allTables))
	for table := range allTables {
		tables = append(tables, table)
	}
	sort.Strings(tables)

	for _, table := range tables {
		if color[table] == WHITE {
			if err := dfs(table); err != nil {
				return err
			}
		}
	}

	return nil
}
