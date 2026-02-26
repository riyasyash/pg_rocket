// Package graph provides data structures and algorithms for working with
// foreign key relationship graphs. It includes graph representation,
// topological sorting with cycle detection, and support for composite primary keys.
package graph

import (
	"github.com/riyasyash/pg_rocket/internal/db"
)

// Graph represents the foreign key relationship structure of a database schema.
// It maintains bidirectional FK relationships and primary key information.
type Graph struct {
	Parents    map[string][]db.ForeignKey // FK relationships pointing to parent tables
	Children   map[string][]db.ForeignKey // FK relationships pointing to child tables
	PrimaryKey map[string][]string        // Primary key columns per table (supports composite PKs)
}

// NewGraph creates a new Graph from extracted database metadata.
func NewGraph(metadata *db.Metadata) *Graph {
	return &Graph{
		Parents:    metadata.Parents,
		Children:   metadata.Children,
		PrimaryKey: metadata.PrimaryKey,
	}
}

// GetParents returns all foreign keys where the given table is the child.
func (g *Graph) GetParents(table string) []db.ForeignKey {
	return g.Parents[table]
}

// GetChildren returns all foreign keys where the given table is the parent.
func (g *Graph) GetChildren(table string) []db.ForeignKey {
	return g.Children[table]
}

// GetPrimaryKey returns the first primary key column for backward compatibility.
// For composite primary keys, use GetPrimaryKeyColumns instead.
func (g *Graph) GetPrimaryKey(table string) string {
	pks := g.PrimaryKey[table]
	if len(pks) > 0 {
		return pks[0]
	}
	return ""
}

// GetPrimaryKeyColumns returns all primary key columns for the given table.
// This supports both single-column and composite primary keys.
func (g *Graph) GetPrimaryKeyColumns(table string) []string {
	return g.PrimaryKey[table]
}
