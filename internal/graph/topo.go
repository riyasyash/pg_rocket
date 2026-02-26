package graph

import (
	"fmt"
	"sort"
)

// TopologicalSort performs a topological sort on the given tables using Kahn's algorithm.
// Returns tables ordered such that parent tables appear before their children,
// ensuring INSERT statements can be executed in order without FK violations.
// Self-referential foreign keys are automatically excluded from the dependency graph.
func (g *Graph) TopologicalSort(tables []string) ([]string, error) {
	inDegree := make(map[string]int)
	adjList := make(map[string][]string)

	tableSet := make(map[string]bool)
	for _, table := range tables {
		tableSet[table] = true
		inDegree[table] = 0
	}

	for _, table := range tables {
		for _, fk := range g.Parents[table] {
			// Skip self-referential foreign keys (e.g., users.manager_id -> users.id)
			if fk.ParentTable == table {
				continue
			}
			if tableSet[fk.ParentTable] {
				adjList[fk.ParentTable] = append(adjList[fk.ParentTable], table)
				inDegree[table]++
			}
		}
	}

	queue := make([]string, 0)
	for _, table := range tables {
		if inDegree[table] == 0 {
			queue = append(queue, table)
		}
	}

	sort.Strings(queue)

	result := make([]string, 0, len(tables))

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		result = append(result, current)

		children := adjList[current]
		sort.Strings(children)

		for _, child := range children {
			inDegree[child]--
			if inDegree[child] == 0 {
				queue = append(queue, child)
				sort.Strings(queue)
			}
		}
	}

	if len(result) != len(tables) {
		remaining := make([]string, 0)
		for table := range inDegree {
			if inDegree[table] > 0 {
				remaining = append(remaining, table)
			}
		}
		sort.Strings(remaining)
		return nil, fmt.Errorf("cycle detected in table dependencies involving: %v", remaining)
	}

	return result, nil
}
