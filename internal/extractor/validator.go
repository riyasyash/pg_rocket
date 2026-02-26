package extractor

import (
	"context"

	"github.com/riyasyash/pg_rocket/internal/db"
)

// QueryInfo contains validated information about a SQL query
// including the base table it operates on.
type QueryInfo struct {
	Query     string // The original SQL SELECT query
	BaseTable string // The table this query extracts data from
}

// ValidateQuery analyzes a SQL query to determine the base table and validates
// that the query is suitable for data extraction (read-only, single table,
// includes primary key columns).
func ValidateQuery(ctx context.Context, conn *db.Connection, query string, metadata *db.Metadata) (*QueryInfo, error) {
	baseTable, err := conn.DetectBaseTable(ctx, query)
	if err != nil {
		return nil, err
	}

	if err := conn.ValidateQuery(ctx, query, baseTable, metadata); err != nil {
		return nil, err
	}

	return &QueryInfo{
		Query:     query,
		BaseTable: baseTable,
	}, nil
}
