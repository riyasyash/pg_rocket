// Package db provides database connection management and metadata extraction
// for PostgreSQL databases. It handles connection pooling, foreign key discovery,
// primary key extraction, and query validation.
package db

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connection wraps a pgx connection pool for database operations.
type Connection struct {
	Pool *pgxpool.Pool
}

// NewConnection creates a new database connection using the provided DSN.
// If dsn is empty, it falls back to the PGROCKET_SOURCE environment variable.
// Returns an error if connection fails or no DSN is provided.
func NewConnection(ctx context.Context, dsn string) (*Connection, error) {
	if dsn == "" {
		dsn = os.Getenv("PGROCKET_SOURCE")
	}
	if dsn == "" {
		return nil, fmt.Errorf("database connection not specified. Use --source flag or set PGROCKET_SOURCE environment variable")
	}

	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to parse DSN: %w", err)
	}

	config.MaxConns = 5

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	return &Connection{Pool: pool}, nil
}

// Close gracefully closes the database connection pool.
func (c *Connection) Close() {
	if c.Pool != nil {
		c.Pool.Close()
	}
}
