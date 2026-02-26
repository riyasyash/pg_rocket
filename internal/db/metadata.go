package db

import (
	"context"
	"fmt"
)

// ForeignKey represents a single foreign key relationship between two tables.
type ForeignKey struct {
	ChildTable   string // Table containing the foreign key
	ChildColumn  string // Column in child table
	ParentTable  string // Referenced parent table
	ParentColumn string // Referenced column in parent table
}

// Metadata contains the complete foreign key and primary key structure
// of a PostgreSQL database schema.
type Metadata struct {
	Parents    map[string][]ForeignKey // Parent relationships by child table
	Children   map[string][]ForeignKey // Child relationships by parent table
	PrimaryKey map[string][]string     // Primary key columns per table (supports composite PKs)
}

// ExtractMetadata queries the database to extract all foreign key relationships
// and primary key definitions from the public schema.
func (c *Connection) ExtractMetadata(ctx context.Context) (*Metadata, error) {
	metadata := &Metadata{
		Parents:    make(map[string][]ForeignKey),
		Children:   make(map[string][]ForeignKey),
		PrimaryKey: make(map[string][]string), // Changed: now slice of strings
	}

	// Extract primary keys FIRST (for all tables)
	if err := c.extractPrimaryKeys(ctx, metadata); err != nil {
		return nil, err
	}

	// Then extract foreign keys
	if err := c.extractForeignKeys(ctx, metadata); err != nil {
		return nil, err
	}

	// Validation is no longer needed since we have all PKs
	// Tables with FKs will automatically be validated

	return metadata, nil
}

func (c *Connection) extractForeignKeys(ctx context.Context, metadata *Metadata) error {
	// Use pg_catalog for better compatibility with restricted permissions
	query := `
		SELECT
			c.relname AS child_table,
			a.attname AS child_column,
			cp.relname AS parent_table,
			ap.attname AS parent_column
		FROM pg_constraint con
		JOIN pg_class c ON con.conrelid = c.oid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum = ANY(con.conkey)
		JOIN pg_class cp ON con.confrelid = cp.oid
		JOIN pg_attribute ap ON ap.attrelid = cp.oid AND ap.attnum = ANY(con.confkey)
		WHERE con.contype = 'f'
			AND n.nspname = 'public'
		ORDER BY c.relname, a.attname
	`

	rows, err := c.Pool.Query(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to query foreign keys: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var fk ForeignKey
		if err := rows.Scan(&fk.ChildTable, &fk.ChildColumn, &fk.ParentTable, &fk.ParentColumn); err != nil {
			return fmt.Errorf("failed to scan foreign key: %w", err)
		}

		metadata.Parents[fk.ChildTable] = append(metadata.Parents[fk.ChildTable], fk)
		metadata.Children[fk.ParentTable] = append(metadata.Children[fk.ParentTable], fk)
	}

	return rows.Err()
}

func (c *Connection) extractPrimaryKeys(ctx context.Context, metadata *Metadata) error {
	// Use pg_catalog instead of information_schema for better compatibility
	// with restricted permissions
	query := `
		SELECT 
			c.relname as table_name,
			a.attname as column_name,
			a.attnum as ordinal_position
		FROM pg_constraint con
		JOIN pg_class c ON con.conrelid = c.oid
		JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum = ANY(con.conkey)
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE con.contype = 'p'
			AND n.nspname = 'public'
		ORDER BY c.relname, a.attnum
	`

	rows, err := c.Pool.Query(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to query primary keys: %w", err)
	}
	defer rows.Close()

	pkCount := 0
	tableCount := 0

	for rows.Next() {
		var tableName, columnName string
		var ordinalPosition int
		if err := rows.Scan(&tableName, &columnName, &ordinalPosition); err != nil {
			return fmt.Errorf("failed to scan primary key: %w", err)
		}

		// Add column to the PK slice for this table (supports composite PKs)
		if _, exists := metadata.PrimaryKey[tableName]; !exists {
			metadata.PrimaryKey[tableName] = make([]string, 0)
			tableCount++
		}
		metadata.PrimaryKey[tableName] = append(metadata.PrimaryKey[tableName], columnName)
		pkCount++
	}

	// Silently support composite PKs - no verbose output needed

	return rows.Err()
}

// validatePrimaryKeys now only validates tables that are involved in FK relationships
func (c *Connection) validatePrimaryKeys(metadata *Metadata) error {
	allTables := make(map[string]bool)

	for table := range metadata.Parents {
		allTables[table] = true
	}
	for table := range metadata.Children {
		allTables[table] = true
	}

	for table := range allTables {
		if pks, exists := metadata.PrimaryKey[table]; !exists || len(pks) == 0 {
			return fmt.Errorf("table '%s' does not have a primary key. pg_rocket requires all tables to have a primary key", table)
		}
	}

	return nil
}
