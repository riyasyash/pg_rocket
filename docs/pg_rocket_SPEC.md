# pg_rocket -- V1 Specification

## 1. Overview

`pg_rocket` is a Postgres CLI tool that extracts a referentially
complete subset of data starting from a root SQL query.

It traverses foreign key relationships upward and/or downward and
outputs a deterministic dataset in SQL or JSON format.

### Primary Use Cases

-   Copy partial, referentially valid data from production to local
-   Extract tenant, cluster, or date-scoped subgraphs
-   Debug production issues locally without full database dumps
-   Safely extract relationally consistent subsets

------------------------------------------------------------------------

## 2. Design Principles

-   Deterministic output
-   Minimal CLI surface
-   Opinionated defaults
-   Fail fast on unsafe or unsupported cases
-   Strict V1 scope (no feature creep)

------------------------------------------------------------------------

## 3. CLI Contract (Frozen V1)

### Commands

    pg_rocket pull
    pg_rocket inspect
    pg_rocket version

------------------------------------------------------------------------

## 4. pull Command

Extract a referentially complete dataset starting from a root query.

### Required Flag

    --query "SELECT ..."

Requirements:

-   Must return rows from exactly one base table
-   Must not modify data
-   Must include the primary key column in the result set
-   Must not contain multiple unrelated base tables

------------------------------------------------------------------------

### Optional Flags

    --parents          Traverse upward only
    --children         Comma-separated child tables (downward traversal)
    --out              Output file (default: stdout)
    --json             Output JSON instead of SQL
    --dry-run          Print extraction plan only (no export)
    --max-rows         Hard row cap (default: 10000)
    --force            Override row cap
    --verbose          Print traversal logs

------------------------------------------------------------------------

## 5. Traversal Semantics

### 5.1 Default Behavior (Full Traversal)

If neither `--parents` nor `--children` is provided:

-   Traverse upward recursively
-   Traverse all downward relationships recursively
-   Extract full referential graph

------------------------------------------------------------------------

### 5.2 Upward Only

    --parents

Behavior:

-   Traverse only parent tables recursively
-   Do not fetch siblings
-   Stop when no more parents exist

------------------------------------------------------------------------

### 5.3 Downward Selective

    --children savings,events

Behavior:

-   Traverse only specified child tables
-   Recursive only within those tables
-   Do not include parents

------------------------------------------------------------------------

### 5.4 Combined Mode

    --parents --children savings

Behavior:

-   Fetch parents recursively
-   Fetch only specified children recursively

------------------------------------------------------------------------

## 6. inspect Command

Displays the foreign key graph of the connected database.

Example output:

    clusters
      ↑ accounts
      ↓ savings
      ↓ telemetry

No flags supported in V1.

------------------------------------------------------------------------

## 7. version Command

Print version string.

------------------------------------------------------------------------

## 8. Environment Configuration

Connection string is provided via environment variable:

    PGROCKET_DSN=postgres://user:pass@host:5432/dbname

No DSN flag in V1.

------------------------------------------------------------------------

## 9. V1 Constraints

-   Postgres only
-   Single schema: `public`
-   All tables must have a single-column primary key
-   Composite primary keys are NOT supported
-   Cyclic foreign keys result in error
-   No masking
-   No multi-schema support
-   No parallel extraction
-   No config file support
-   No direct DB-to-DB transfer

------------------------------------------------------------------------

## 10. Internal Architecture

    cmd/
      pull.go
      inspect.go
      version.go

    internal/
      db/
        connection.go
        metadata.go
      graph/
        builder.go
        topo.go
      extractor/
        engine.go
        traversal.go
      output/
        sql_writer.go
        json_writer.go

------------------------------------------------------------------------

## 11. Metadata Extraction

### 11.1 Foreign Keys

``` sql
SELECT
    tc.table_name AS child_table,
    kcu.column_name AS child_column,
    ccu.table_name AS parent_table,
    ccu.column_name AS parent_column
FROM information_schema.table_constraints tc
JOIN information_schema.key_column_usage kcu
  ON tc.constraint_name = kcu.constraint_name
JOIN information_schema.constraint_column_usage ccu
  ON ccu.constraint_name = tc.constraint_name
WHERE tc.constraint_type = 'FOREIGN KEY'
  AND tc.table_schema = 'public';
```

### 11.2 Primary Keys

``` sql
SELECT
    kcu.table_name,
    kcu.column_name
FROM information_schema.table_constraints tc
JOIN information_schema.key_column_usage kcu
  ON tc.constraint_name = kcu.constraint_name
WHERE tc.constraint_type = 'PRIMARY KEY'
  AND tc.table_schema = 'public';
```

Build in-memory structures:

    parents[child] -> []parent
    children[parent] -> []child
    primaryKey[table] -> column

------------------------------------------------------------------------

## 12. Root Query Validation

Steps:

1.  Execute query with `LIMIT 0`
2.  Detect base table
3.  Ensure exactly one base table exists
4.  Verify primary key column exists in result set

Fail if:

-   Multiple base tables detected
-   No primary key
-   Composite primary key
-   Unsupported query structure

------------------------------------------------------------------------

## 13. Traversal Engine

Maintain:

    visitedRows[table][primaryKey] = true
    tableData[table] = []Row

Algorithm:

1.  Execute root query
2.  Add rows to visitedRows
3.  Perform BFS traversal
    -   If upward: fetch parents via foreign keys
    -   If downward: fetch children via foreign keys
4.  Continue until no new rows discovered

All fetches must use batched queries:

    SELECT * FROM table WHERE id IN (...)

Never perform per-row queries.

------------------------------------------------------------------------

## 14. Row Limit Enforcement

-   Count total rows collected
-   Default maximum: 10000 rows
-   If exceeded:
    -   Abort unless `--force` is provided

------------------------------------------------------------------------

## 15. Topological Sorting

Before output:

-   Topologically sort tables so parents appear before children
-   Detect cycles during sorting
-   If cycle detected → exit with clear error

------------------------------------------------------------------------

## 16. Output Modes

### 16.1 SQL (Default)

Format:

    INSERT INTO table (col1, col2)
    VALUES
      (...),
      (...);

Rules:

-   Deterministic ordering by primary key
-   Stable table ordering
-   Proper NULL handling
-   Proper string escaping
-   JSONB marshalled correctly
-   Timestamps preserved

------------------------------------------------------------------------

### 16.2 JSON

Structure:

``` json
{
  "table1": [{}, {}],
  "table2": [{}]
}
```

-   Tables sorted topologically
-   Rows sorted by primary key

------------------------------------------------------------------------

## 17. Error Handling

Fail fast for:

-   Missing primary key
-   Composite primary key
-   Foreign key cycle
-   Multiple base tables in root query
-   Unknown table in `--children`
-   Row limit exceeded without `--force`
-   Missing `PGROCKET_DSN`

------------------------------------------------------------------------

## 18. Determinism Guarantee

Given:

-   Same database state
-   Same root query
-   Same flags

The output must be byte-for-byte identical.

------------------------------------------------------------------------

## 19. Non-Goals (V1)

-   Data masking
-   Composite key support
-   Cross-schema traversal
-   Direct DB-to-DB transfer
-   Parallel extraction
-   Configuration files
-   MySQL support
-   UI or web interface

------------------------------------------------------------------------

## 20. Definition of Done

-   All traversal modes work correctly
-   No foreign key violations on import
-   Deterministic output verified
-   Handles 10k rows comfortably
-   Clear, actionable error messages
-   Brew-installable binary
-   README includes basic usage examples
