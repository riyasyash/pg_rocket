package db

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// ExplainPlan represents the top-level structure of PostgreSQL EXPLAIN JSON output.
type ExplainPlan struct {
	Plan ExplainNode `json:"Plan"`
}

// ExplainNode represents a node in the EXPLAIN query plan tree.
type ExplainNode struct {
	NodeType     string        `json:"Node Type"`
	RelationName string        `json:"Relation Name,omitempty"`
	Plans        []ExplainNode `json:"Plans,omitempty"`
}

// DetectBaseTable analyzes a SQL query using EXPLAIN to determine which table
// it operates on. Returns an error if the query is not read-only, references
// multiple tables, or cannot be analyzed.
func (c *Connection) DetectBaseTable(ctx context.Context, query string) (string, error) {
	query = strings.TrimSpace(query)

	upperQuery := strings.ToUpper(query)
	if strings.Contains(upperQuery, "INSERT") ||
		strings.Contains(upperQuery, "UPDATE") ||
		strings.Contains(upperQuery, "DELETE") ||
		strings.Contains(upperQuery, "DROP") ||
		strings.Contains(upperQuery, "CREATE") ||
		strings.Contains(upperQuery, "ALTER") {
		return "", fmt.Errorf("query must be read-only (SELECT only)")
	}

	explainQuery := fmt.Sprintf("EXPLAIN (FORMAT JSON) %s", query)

	var planJSON []byte
	err := c.Pool.QueryRow(ctx, explainQuery).Scan(&planJSON)
	if err != nil {
		return "", fmt.Errorf("failed to execute EXPLAIN: %w", err)
	}

	var plans []ExplainPlan
	if err := json.Unmarshal(planJSON, &plans); err != nil {
		return "", fmt.Errorf("failed to parse EXPLAIN output: %w", err)
	}

	if len(plans) == 0 {
		return "", fmt.Errorf("empty EXPLAIN output")
	}

	tables := make(map[string]bool)
	extractTables(&plans[0].Plan, tables)

	if len(tables) == 0 {
		return "", fmt.Errorf("no base table detected in query")
	}

	if len(tables) > 1 {
		tableList := make([]string, 0, len(tables))
		for table := range tables {
			tableList = append(tableList, table)
		}
		return "", fmt.Errorf("query references multiple base tables: %v. Please ensure your query returns rows from exactly one base table", tableList)
	}

	for table := range tables {
		return table, nil
	}

	return "", fmt.Errorf("no base table found")
}

// extractTables recursively walks the EXPLAIN plan tree to find all referenced tables.
func extractTables(node *ExplainNode, tables map[string]bool) {
	if node.RelationName != "" {
		tables[node.RelationName] = true
	}

	for i := range node.Plans {
		extractTables(&node.Plans[i], tables)
	}
}

func (c *Connection) ValidateQuery(ctx context.Context, query string, tableName string, metadata *Metadata) error {
	pkColumns, exists := metadata.PrimaryKey[tableName]
	if !exists || len(pkColumns) == 0 {
		fmt.Printf("DEBUG: Table '%s' not found in metadata. Available tables: %v\n", tableName, getTableNames(metadata.PrimaryKey))
		return fmt.Errorf("table '%s' does not have a primary key defined", tableName)
	}

	testQuery := fmt.Sprintf("%s LIMIT 0", query)
	rows, err := c.Pool.Query(ctx, testQuery)
	if err != nil {
		return fmt.Errorf("failed to validate query: %w", err)
	}
	defer rows.Close()

	fieldDescriptions := rows.FieldDescriptions()
	foundPKs := make(map[string]bool)

	for _, fd := range fieldDescriptions {
		colName := string(fd.Name)
		for _, pkCol := range pkColumns {
			if colName == pkCol {
				foundPKs[pkCol] = true
			}
		}
	}

	// Check all PK columns are present
	missingPKs := make([]string, 0)
	for _, pkCol := range pkColumns {
		if !foundPKs[pkCol] {
			missingPKs = append(missingPKs, pkCol)
		}
	}

	if len(missingPKs) > 0 {
		return fmt.Errorf("query must include all primary key columns. Missing: %v", missingPKs)
	}

	return nil
}

func getTableNames(pkMap map[string][]string) []string {
	tables := make([]string, 0, len(pkMap))
	for table := range pkMap {
		tables = append(tables, table)
	}
	return tables
}
