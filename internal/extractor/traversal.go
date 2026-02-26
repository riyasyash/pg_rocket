// Package extractor implements the core data extraction engine for pg_rocket.
// It performs breadth-first traversal of foreign key relationships to extract
// referentially complete data subsets from PostgreSQL databases.
package extractor

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/riyasyash/pg_rocket/internal/db"
	"github.com/riyasyash/pg_rocket/internal/graph"
)

// BatchSize defines the number of rows to fetch in a single query
// to balance memory usage and query performance.
const BatchSize = 500

// TraversalOptions configures the behavior of the data extraction traversal.
type TraversalOptions struct {
	ParentsOnly      bool     // Only traverse upward to parent records
	ChildrenOnly     bool     // Only traverse downward to child records
	SelectedChildren []string // Specific child tables to traverse (nil means all)
	MaxRows          int      // Maximum number of rows to extract
	Force            bool     // Override MaxRows limit
	Verbose          bool     // Enable detailed logging
}

// TraversalState maintains the state of an ongoing data extraction traversal.
// It tracks visited rows, collected data, and provides access to the database
// connection and FK graph.
type TraversalState struct {
	VisitedRows map[string]map[interface{}]bool     // Tracks visited rows by table and PK
	TableData   map[string][]map[string]interface{} // Collected data by table
	RowCount    int                                 // Total rows extracted
	Graph       *graph.Graph                        // Foreign key graph
	Connection  *db.Connection                      // Database connection pool
	Options     *TraversalOptions                   // Traversal configuration
	Progress    *ProgressTracker                    // Progress reporting
}

// NewTraversalState creates a new traversal state with the given graph, connection, and options.
func NewTraversalState(g *graph.Graph, conn *db.Connection, opts *TraversalOptions) *TraversalState {
	return &TraversalState{
		VisitedRows: make(map[string]map[interface{}]bool),
		TableData:   make(map[string][]map[string]interface{}),
		RowCount:    0,
		Graph:       g,
		Connection:  conn,
		Options:     opts,
		Progress:    NewProgressTracker(opts.Verbose),
	}
}

// Extract performs the main data extraction starting from the given query.
// It executes the root query, then traverses parent and/or child relationships
// based on the configured options. Returns an error if traversal fails or
// row limits are exceeded.
func (ts *TraversalState) Extract(ctx context.Context, queryInfo *QueryInfo) error {
	ts.Progress.StartPhase("Data Extraction")
	ts.Progress.Info("Starting from table: %s", queryInfo.BaseTable)

	if err := ts.executeRootQuery(ctx, queryInfo); err != nil {
		return err
	}

	// Track which tables we've already processed parents for
	processedParents := make(map[string]bool)

	if !ts.Options.ChildrenOnly {
		ts.Progress.Info("Traversing parent relationships...")
		if err := ts.traverseParents(ctx, queryInfo.BaseTable); err != nil {
			return err
		}
		processedParents[queryInfo.BaseTable] = true
	}

	if !ts.Options.ParentsOnly {
		ts.Progress.Info("Traversing child relationships...")
		if err := ts.traverseChildren(ctx, queryInfo.BaseTable); err != nil {
			return err
		}

		// After traversing children, traverse parents of ALL newly discovered child tables
		// This ensures we get configuration/reference tables that child tables reference
		if !ts.Options.ChildrenOnly {
			for tableName := range ts.TableData {
				if !processedParents[tableName] && tableName != queryInfo.BaseTable {
					ts.Progress.Info("Traversing parents of discovered table: %s", tableName)
					if err := ts.traverseParents(ctx, tableName); err != nil {
						return err
					}
					processedParents[tableName] = true
				}
			}
		}
	}

	ts.Progress.Complete(ts.RowCount, len(ts.TableData))

	return nil
}

func (ts *TraversalState) executeRootQuery(ctx context.Context, queryInfo *QueryInfo) error {
	// Get column info to detect JSONB columns
	columnQuery := `
		SELECT column_name, data_type
		FROM information_schema.columns
		WHERE table_schema = 'public' 
		  AND table_name = $1
		ORDER BY ordinal_position
	`

	colRows, err := ts.Connection.Pool.Query(ctx, columnQuery, queryInfo.BaseTable)
	if err != nil {
		return fmt.Errorf("failed to get column info for %s: %w", queryInfo.BaseTable, err)
	}

	jsonbCols := make(map[string]bool)
	hasJSONB := false
	for colRows.Next() {
		var colName, dataType string
		if err := colRows.Scan(&colName, &dataType); err != nil {
			colRows.Close()
			return fmt.Errorf("failed to scan column info: %w", err)
		}
		if dataType == "jsonb" || dataType == "json" {
			jsonbCols[colName] = true
			hasJSONB = true
		}
	}
	colRows.Close()

	// If there are JSONB columns and the query is SELECT *, rewrite it
	var queryToRun string
	if hasJSONB && strings.Contains(strings.ToUpper(queryInfo.Query), "SELECT *") {
		// Get all columns and build a query with JSONB casts
		allCols, err := ts.Connection.Pool.Query(ctx, columnQuery, queryInfo.BaseTable)
		if err != nil {
			return fmt.Errorf("failed to get columns: %w", err)
		}

		var selectCols []string
		for allCols.Next() {
			var colName, dataType string
			if err := allCols.Scan(&colName, &dataType); err != nil {
				allCols.Close()
				return fmt.Errorf("failed to scan column: %w", err)
			}

			if dataType == "jsonb" || dataType == "json" {
				selectCols = append(selectCols, fmt.Sprintf("%s::text AS %s", colName, colName))
			} else {
				selectCols = append(selectCols, colName)
			}
		}
		allCols.Close()

		// Replace SELECT * with explicit column list
		queryToRun = strings.Replace(queryInfo.Query, "SELECT *",
			fmt.Sprintf("SELECT %s", strings.Join(selectCols, ", ")), 1)
		queryToRun = strings.Replace(queryToRun, "select *",
			fmt.Sprintf("SELECT %s", strings.Join(selectCols, ", ")), 1)
	} else {
		queryToRun = queryInfo.Query
	}

	// Execute the (possibly rewritten) query
	rows, err := ts.Connection.Pool.Query(ctx, queryToRun)
	if err != nil {
		return fmt.Errorf("failed to execute root query: %w", err)
	}
	defer rows.Close()

	// If there are JSONB columns, process with JSONB info
	if hasJSONB {
		return ts.processRowsWithJSONBInfo(ctx, rows, queryInfo.BaseTable, jsonbCols)
	}

	return ts.processRows(ctx, rows, queryInfo.BaseTable)
}

func (ts *TraversalState) processRows(ctx context.Context, rows pgx.Rows, tableName string) error {
	fieldDescriptions := rows.FieldDescriptions()
	pkColumns := ts.Graph.GetPrimaryKeyColumns(tableName)

	if ts.VisitedRows[tableName] == nil {
		ts.VisitedRows[tableName] = make(map[interface{}]bool)
	}

	newRows := make([]map[string]interface{}, 0)

	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return fmt.Errorf("failed to scan row: %w", err)
		}

		rowMap := make(map[string]interface{})
		pkValues := make([]interface{}, 0, len(pkColumns))

		for i, value := range values {
			colName := string(fieldDescriptions[i].Name)
			rowMap[colName] = value

			// Collect values for all PK columns
			for _, pkCol := range pkColumns {
				if colName == pkCol {
					pkValues = append(pkValues, value)
				}
			}
		}

		if len(pkValues) != len(pkColumns) {
			return fmt.Errorf("primary key value(s) missing in table %s", tableName)
		}

		// Create composite PK key (for single PKs, this is just the single value)
		var pkKey interface{}
		if len(pkValues) == 1 {
			pkKey = pkValues[0]
		} else {
			// For composite PKs, create a string key
			pkKey = fmt.Sprintf("%v", pkValues)
		}

		if pkKey == nil {
			return fmt.Errorf("primary key value is NULL in table %s", tableName)
		}

		if ts.VisitedRows[tableName][pkKey] {
			continue
		}

		ts.VisitedRows[tableName][pkKey] = true
		newRows = append(newRows, rowMap)
		ts.RowCount++

		if !ts.Options.Force && ts.RowCount > ts.Options.MaxRows {
			return fmt.Errorf("row limit exceeded (%d rows). Use --force to override", ts.Options.MaxRows)
		}
	}

	if len(newRows) > 0 {
		ts.TableData[tableName] = append(ts.TableData[tableName], newRows...)
		ts.Progress.TableDiscovered(tableName, len(newRows))
		ts.Progress.Progress(ts.RowCount, ts.Options.MaxRows,
			fmt.Sprintf("Extracting data (%d/%d rows)", ts.RowCount, ts.Options.MaxRows))
	}

	return rows.Err()
}

// processRowsWithJSONBInfo processes rows where JSONB/JSON columns have been cast to text
func (ts *TraversalState) processRowsWithJSONBInfo(ctx context.Context, rows pgx.Rows, tableName string, jsonbCols map[string]bool) error {
	fieldDescriptions := rows.FieldDescriptions()
	pkColumns := ts.Graph.GetPrimaryKeyColumns(tableName)

	if ts.VisitedRows[tableName] == nil {
		ts.VisitedRows[tableName] = make(map[interface{}]bool)
	}

	newRows := make([]map[string]interface{}, 0)

	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return fmt.Errorf("failed to scan row: %w", err)
		}

		rowMap := make(map[string]interface{})
		pkValues := make([]interface{}, 0, len(pkColumns))

		for i, value := range values {
			colName := string(fieldDescriptions[i].Name)

			// JSONB columns are now text (from our ::text cast)
			// Store as-is - will be cast back to JSONB during insertion
			rowMap[colName] = value

			// Collect values for all PK columns
			for _, pkCol := range pkColumns {
				if colName == pkCol {
					pkValues = append(pkValues, value)
				}
			}
		}

		if len(pkValues) != len(pkColumns) {
			return fmt.Errorf("primary key value(s) missing in table %s", tableName)
		}

		// Create composite PK key
		var pkKey interface{}
		if len(pkValues) == 1 {
			pkKey = pkValues[0]
		} else {
			pkKey = fmt.Sprintf("%v", pkValues)
		}

		if pkKey == nil {
			return fmt.Errorf("primary key value is NULL in table %s", tableName)
		}

		if ts.VisitedRows[tableName][pkKey] {
			continue
		}

		ts.VisitedRows[tableName][pkKey] = true
		newRows = append(newRows, rowMap)
		ts.RowCount++

		if !ts.Options.Force && ts.RowCount > ts.Options.MaxRows {
			return fmt.Errorf("row limit exceeded (%d rows). Use --force to override", ts.Options.MaxRows)
		}
	}

	if len(newRows) > 0 {
		ts.TableData[tableName] = append(ts.TableData[tableName], newRows...)
		ts.Progress.TableDiscovered(tableName, len(newRows))
		ts.Progress.Progress(ts.RowCount, ts.Options.MaxRows,
			fmt.Sprintf("Extracting data (%d/%d rows)", ts.RowCount, ts.Options.MaxRows))
	}

	return rows.Err()
}

func (ts *TraversalState) traverseParents(ctx context.Context, startTable string) error {
	queue := []string{startTable}
	processed := make(map[string]bool)

	for len(queue) > 0 {
		currentTable := queue[0]
		queue = queue[1:]

		if processed[currentTable] {
			continue
		}
		processed[currentTable] = true

		ts.Progress.TraversingTable(currentTable, "parents")

		parentFKs := ts.Graph.GetParents(currentTable)
		for _, fk := range parentFKs {
			if err := ts.fetchParentRows(ctx, currentTable, fk); err != nil {
				return err
			}

			queue = append(queue, fk.ParentTable)
		}
	}

	return nil
}

func (ts *TraversalState) fetchParentRows(ctx context.Context, childTable string, fk db.ForeignKey) error {
	childRows := ts.TableData[childTable]
	if len(childRows) == 0 {
		return nil
	}

	parentValues := make([]interface{}, 0)
	for _, row := range childRows {
		value := row[fk.ChildColumn]
		if value != nil {
			parentValues = append(parentValues, value)
		}
	}

	if len(parentValues) == 0 {
		return nil
	}

	parentValues = removeDuplicates(parentValues)

	for i := 0; i < len(parentValues); i += BatchSize {
		end := i + BatchSize
		if end > len(parentValues) {
			end = len(parentValues)
		}

		batch := parentValues[i:end]
		if err := ts.fetchRowsByPK(ctx, fk.ParentTable, fk.ParentColumn, batch); err != nil {
			return err
		}
	}

	return nil
}

func (ts *TraversalState) traverseChildren(ctx context.Context, startTable string) error {
	queue := []string{startTable}
	processed := make(map[string]bool)

	selectedChildrenMap := make(map[string]bool)
	if len(ts.Options.SelectedChildren) > 0 {
		for _, child := range ts.Options.SelectedChildren {
			selectedChildrenMap[child] = true
		}
	}

	for len(queue) > 0 {
		currentTable := queue[0]
		queue = queue[1:]

		if processed[currentTable] {
			continue
		}
		processed[currentTable] = true

		ts.Progress.TraversingTable(currentTable, "children")

		childFKs := ts.Graph.GetChildren(currentTable)
		for _, fk := range childFKs {
			if len(selectedChildrenMap) > 0 && !selectedChildrenMap[fk.ChildTable] {
				continue
			}

			if err := ts.fetchChildRows(ctx, currentTable, fk); err != nil {
				return err
			}

			queue = append(queue, fk.ChildTable)
		}
	}

	return nil
}

func (ts *TraversalState) fetchChildRows(ctx context.Context, parentTable string, fk db.ForeignKey) error {
	parentRows := ts.TableData[parentTable]
	if len(parentRows) == 0 {
		return nil
	}

	pkColumn := ts.Graph.GetPrimaryKey(parentTable)
	parentPKs := make([]interface{}, 0)
	for _, row := range parentRows {
		value := row[pkColumn]
		if value != nil {
			parentPKs = append(parentPKs, value)
		}
	}

	if len(parentPKs) == 0 {
		return nil
	}

	parentPKs = removeDuplicates(parentPKs)

	for i := 0; i < len(parentPKs); i += BatchSize {
		end := i + BatchSize
		if end > len(parentPKs) {
			end = len(parentPKs)
		}

		batch := parentPKs[i:end]
		if err := ts.fetchRowsByFK(ctx, fk.ChildTable, fk.ChildColumn, batch); err != nil {
			return err
		}
	}

	return nil
}

func (ts *TraversalState) fetchRowsByPK(ctx context.Context, tableName, pkColumn string, values []interface{}) error {
	if len(values) == 0 {
		return nil
	}

	// Get column information to detect JSONB/JSON columns
	columnQuery := `
		SELECT column_name, data_type
		FROM information_schema.columns
		WHERE table_schema = 'public' 
		  AND table_name = $1
		ORDER BY ordinal_position
	`

	colRows, err := ts.Connection.Pool.Query(ctx, columnQuery, tableName)
	if err != nil {
		return fmt.Errorf("failed to get column info for %s: %w", tableName, err)
	}

	jsonbCols := make(map[string]bool)
	var selectCols []string

	for colRows.Next() {
		var colName, dataType string
		if err := colRows.Scan(&colName, &dataType); err != nil {
			colRows.Close()
			return fmt.Errorf("failed to scan column info: %w", err)
		}

		// For JSONB/JSON columns, cast to text to preserve exact representation
		if dataType == "jsonb" || dataType == "json" {
			jsonbCols[colName] = true
			// Use to_jsonb() to preserve the distinction between JSONB null and SQL NULL
			// to_jsonb(NULL) returns SQL NULL, but to_jsonb('null'::jsonb)::text returns "null"
			selectCols = append(selectCols, fmt.Sprintf("to_jsonb(%s)::text AS %s", colName, colName))
		} else {
			selectCols = append(selectCols, colName)
		}
	}
	colRows.Close()

	placeholders := make([]string, len(values))
	for i := range values {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}

	// Use explicit column list with JSONB columns cast to text
	query := fmt.Sprintf("SELECT %s FROM %s WHERE %s IN (%s) ORDER BY %s",
		strings.Join(selectCols, ", "), tableName, pkColumn, strings.Join(placeholders, ", "), pkColumn)

	rows, err := ts.Connection.Pool.Query(ctx, query, values...)
	if err != nil {
		return fmt.Errorf("failed to fetch rows from %s: %w", tableName, err)
	}
	defer rows.Close()

	return ts.processRowsWithJSONBInfo(ctx, rows, tableName, jsonbCols)
}

func (ts *TraversalState) fetchRowsByFK(ctx context.Context, tableName, fkColumn string, values []interface{}) error {
	if len(values) == 0 {
		return nil
	}

	// Get column information to detect JSONB/JSON columns
	columnQuery := `
		SELECT column_name, data_type
		FROM information_schema.columns
		WHERE table_schema = 'public' 
		  AND table_name = $1
		ORDER BY ordinal_position
	`

	colRows, err := ts.Connection.Pool.Query(ctx, columnQuery, tableName)
	if err != nil {
		return fmt.Errorf("failed to get column info for %s: %w", tableName, err)
	}

	jsonbCols := make(map[string]bool)
	var selectCols []string

	for colRows.Next() {
		var colName, dataType string
		if err := colRows.Scan(&colName, &dataType); err != nil {
			colRows.Close()
			return fmt.Errorf("failed to scan column info: %w", err)
		}

		// For JSONB/JSON columns, cast to text to preserve exact representation
		if dataType == "jsonb" || dataType == "json" {
			jsonbCols[colName] = true
			// Use to_jsonb() to preserve the distinction between JSONB null and SQL NULL
			// to_jsonb(NULL) returns SQL NULL, but to_jsonb('null'::jsonb)::text returns "null"
			selectCols = append(selectCols, fmt.Sprintf("to_jsonb(%s)::text AS %s", colName, colName))
		} else {
			selectCols = append(selectCols, colName)
		}
	}
	colRows.Close()

	placeholders := make([]string, len(values))
	for i := range values {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}

	pkColumn := ts.Graph.GetPrimaryKey(tableName)
	// Use explicit column list with JSONB columns cast to text
	query := fmt.Sprintf("SELECT %s FROM %s WHERE %s IN (%s) ORDER BY %s",
		strings.Join(selectCols, ", "), tableName, fkColumn, strings.Join(placeholders, ", "), pkColumn)

	rows, err := ts.Connection.Pool.Query(ctx, query, values...)
	if err != nil {
		return fmt.Errorf("failed to fetch rows from %s: %w", tableName, err)
	}
	defer rows.Close()

	return ts.processRowsWithJSONBInfo(ctx, rows, tableName, jsonbCols)
}

func removeDuplicates(values []interface{}) []interface{} {
	seen := make(map[interface{}]bool)
	result := make([]interface{}, 0)

	for _, value := range values {
		key := fmt.Sprintf("%v", value)
		if !seen[key] {
			seen[key] = true
			result = append(result, value)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return fmt.Sprintf("%v", result[i]) < fmt.Sprintf("%v", result[j])
	})

	return result
}

// GetAllTables returns a list of all tables that have extracted data.
func (ts *TraversalState) GetAllTables() []string {
	tables := make([]string, 0, len(ts.TableData))
	for table := range ts.TableData {
		tables = append(tables, table)
	}
	return tables
}
