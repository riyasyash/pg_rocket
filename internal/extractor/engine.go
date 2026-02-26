package extractor

import (
	"context"
	"fmt"

	"github.com/riyasyash/pg_rocket/internal/db"
	"github.com/riyasyash/pg_rocket/internal/graph"
)

// Engine is the high-level orchestrator for data extraction operations.
// It manages the database connection, schema metadata, and FK graph.
type Engine struct {
	Connection *db.Connection // Database connection pool
	Metadata   *db.Metadata   // Foreign key and primary key metadata
	Graph      *graph.Graph   // FK relationship graph
}

// NewEngine creates a new extraction engine by connecting to the database
// and extracting schema metadata. Returns an error if connection or
// metadata extraction fails.
func NewEngine(ctx context.Context, conn *db.Connection) (*Engine, error) {
	metadata, err := conn.ExtractMetadata(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to extract metadata: %w", err)
	}

	g, err := graph.BuildGraph(metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to build graph: %w", err)
	}

	return &Engine{
		Connection: conn,
		Metadata:   metadata,
		Graph:      g,
	}, nil
}

// Extract performs data extraction starting from the given SQL query.
// It validates the query, performs FK traversal based on the provided options,
// and returns the traversal state containing all extracted data.
func (e *Engine) Extract(ctx context.Context, query string, opts *TraversalOptions) (*TraversalState, error) {
	queryInfo, err := ValidateQuery(ctx, e.Connection, query, e.Metadata)
	if err != nil {
		return nil, fmt.Errorf("query validation failed: %w", err)
	}

	state := NewTraversalState(e.Graph, e.Connection, opts)
	if err := state.Extract(ctx, queryInfo); err != nil {
		return nil, fmt.Errorf("extraction failed: %w", err)
	}

	return state, nil
}
