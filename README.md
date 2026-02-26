# pg_rocket 🚀

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![Version](https://img.shields.io/badge/version-0.0.1-blue.svg)](https://github.com/riyasyash/pg_rocket/releases)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](CONTRIBUTING.md)

**Extract referentially complete data subsets from PostgreSQL by traversing foreign key relationships.**

`pg_rocket` is a CLI tool that intelligently extracts data from PostgreSQL databases while maintaining referential integrity. Given a SQL query, it automatically discovers and fetches all related records across tables, producing a consistent subset you can use for testing, debugging, or data migration.

## Why pg_rocket?

- 🔍 **Debug production issues locally** - Pull a specific tenant's data without copying entire databases
- 🧪 **Realistic test data** - Extract production subsets with all relationships intact
- 📦 **Safe data migration** - Move related records across databases with referential integrity guaranteed
- 🚀 **Fast and deterministic** - Batched queries with consistent, repeatable output

## Features

- ✅ Automatic foreign key traversal (upward, downward, or bidirectional)
- ✅ Composite primary key support
- ✅ Self-referential foreign keys
- ✅ Multiple output formats (SQL INSERTs, JSON)
- ✅ Direct database-to-database transfer with `--exec`
- ✅ Upsert mode for successive runs (`--upsert`)
- ✅ JSONB null literal preservation
- ✅ Progress indicators and colored output
- ✅ Configurable row limits with safety checks
- ✅ Dry-run mode to preview extraction plan

## Installation

### Homebrew (macOS/Linux)

```bash
brew tap riyasyash/tap
brew install pg_rocket
```

### From Binary

Download the latest release for your platform from [GitHub Releases](https://github.com/riyasyash/pg_rocket/releases).

### From Source

```bash
git clone https://github.com/riyasyash/pg_rocket.git
cd pg_rocket
make build
sudo make install  # Optional: installs to /usr/local/bin
```

### Using Go Install

```bash
go install github.com/riyasyash/pg_rocket@latest
```

### Requirements

- Go 1.21 or later (if building from source)
- PostgreSQL 12+ (source database)

## Quick Start

```bash
# Set your source database
export PGROCKET_SOURCE="postgres://user:pass@localhost:5432/mydb"

# Inspect the foreign key graph
pg_rocket inspect

# Extract a user and all related data
pg_rocket pull --query "SELECT * FROM users WHERE id = 42" --out user_42.sql

# Direct database-to-database transfer
export PGROCKET_TARGET="postgres://admin:pass@localhost:5432/testdb"
pg_rocket pull --query "SELECT * FROM orders WHERE id = 1001" --exec --upsert
```

## Commands

### `pg_rocket pull`

Extract referentially complete data starting from a root SQL query.

#### Required
- `--query` - SQL SELECT statement returning rows from a single table

#### Connection
- `--source` - Source database DSN (default: `$PGROCKET_SOURCE`)
- `--target` - Target database DSN for `--exec` mode (default: `$PGROCKET_TARGET`)

#### Traversal
- `--parents` - Only traverse upward to parent records
- `--children table1,table2` - Only traverse specified child tables (comma-separated)
- *(default: full bidirectional traversal)*

#### Output
- `--out filename` - Write to file instead of stdout
- `--json` - Output JSON instead of SQL INSERTs
- `--exec` - Execute INSERTs directly to target database (requires confirmation)

#### Control
- `--upsert` - Use `ON CONFLICT DO UPDATE` for idempotent successive runs (requires `--exec`)
- `--dry-run` - Show extraction plan without executing
- `--max-rows N` - Maximum rows to extract (default: 10000)
- `--force` - Override row limit
- `--verbose` - Print detailed traversal logs

### `pg_rocket inspect`

Display the foreign key graph of your database.

```bash
pg_rocket inspect [--source DSN]
```

Example output:
```
users
  ↑ organizations (via org_id)
  ↓ posts (via author_id)
  ↓ comments (via user_id)
```

### `pg_rocket version`

Display version information.

```bash
pg_rocket version
```

## Usage Examples

### Extract User with All Related Data

```bash
pg_rocket pull \
  --query "SELECT * FROM users WHERE email = 'alice@example.com'" \
  --out alice.sql \
  --verbose
```

Fetches:
- The user record
- Parent records (organization, etc.)
- Child records (posts, comments, orders, etc.)
- All transitively related data

### Parents Only (Upward Traversal)

```bash
pg_rocket pull \
  --query "SELECT * FROM orders WHERE id = 1001" \
  --parents \
  --out order_1001.sql
```

Fetches order → user → organization (stops at root, no children).

### Selective Children (Downward Traversal)

```bash
pg_rocket pull \
  --query "SELECT * FROM projects WHERE id = 5" \
  --children tasks,comments \
  --out project_5.sql
```

Fetches project + only tasks and comments (skips other child tables).

### JSON Output

```bash
pg_rocket pull \
  --query "SELECT * FROM organizations WHERE id = 10" \
  --json \
  --out org_10.json
```

Produces structured JSON:
```json
{
  "organizations": [
    {"id": 10, "name": "Acme Corp"}
  ],
  "users": [
    {"id": 1, "org_id": 10, "name": "Alice"},
    {"id": 2, "org_id": 10, "name": "Bob"}
  ]
}
```

### Direct Database Transfer

```bash
# Production → Staging transfer
pg_rocket pull \
  --source "postgres://readonly@prod.db:5432/proddb?sslmode=require" \
  --target "postgres://admin@staging.db:5432/stagingdb?sslmode=require" \
  --query "SELECT * FROM tenants WHERE name = 'acme'" \
  --exec \
  --verbose
```

The tool will:
1. Extract data from production
2. Display summary and prompt for confirmation
3. Insert into staging with foreign key integrity validation

### Successive Runs with Upsert

```bash
# First run - inserts data
pg_rocket pull \
  --query "SELECT * FROM users WHERE id = 42" \
  --exec

# Second run - updates existing, inserts new (no duplicate key errors)
pg_rocket pull \
  --query "SELECT * FROM users WHERE id = 42" \
  --exec \
  --upsert
```

## Configuration

### Connection Strings

Format:
```
postgres://[user[:password]@][host][:port][/database][?param=value]
```

Examples:
```bash
# Local development
postgres://myuser:mypass@localhost:5432/mydb

# Production with SSL (recommended)
postgres://user:pass@prod.com:5432/db?sslmode=require

# AWS RDS
postgres://master:pass@mydb.abc.us-east-1.rds.amazonaws.com:5432/mydb?sslmode=require

# With certificate verification
postgres://user:pass@host:5432/db?sslmode=verify-full&sslrootcert=/path/to/ca.crt
```

### SSL/TLS Security

| Mode | Security | Description |
|------|----------|-------------|
| `disable` | None | No encryption |
| `allow` | Low | Tries non-SSL first |
| `prefer` | Medium | Tries SSL first (default) |
| `require` | **Good** | **Requires SSL** (recommended for production) |
| `verify-ca` | Better | SSL + certificate verification |
| `verify-full` | Best | SSL + cert + hostname verification |

**Production recommendation:** Always use `sslmode=require` or higher.

### Environment Variables

```bash
# Source database (for reading)
export PGROCKET_SOURCE="postgres://user:pass@host:5432/db?sslmode=require"

# Target database (for --exec mode)
export PGROCKET_TARGET="postgres://admin:pass@host:5432/targetdb?sslmode=require"
```

Priority: CLI flags (`--source`, `--target`) override environment variables.

## Safety Features

### Pre-Execution Validation

`pg_rocket` validates all commands before running:

- ✅ Source database must be specified (flag or env var)
- ✅ Target database required for `--exec` mode
- ✅ Source and target must be different (prevents accidental self-writes)
- ✅ `--exec` and `--out` are mutually exclusive
- ✅ `--upsert` requires `--exec`

### User Confirmation for Database Writes

When using `--exec`, you'll see:

```
⚠️  DATABASE WRITE OPERATION
============================================================
Source:
  postgres://***@prod-server:5432/proddb

Target (will be modified):
  postgres://***@staging-server:5432/stagingdb

Data to be inserted:
  Tables: 7
  Total rows: 1,423
============================================================

Are you sure you want to INSERT this data into the target database? (yes/no): 
```

Type `yes` or `y` to proceed. Any other response safely cancels.

### Row Limits

Default: 10,000 rows. Override with `--max-rows` or `--force`:

```bash
# Custom limit
pg_rocket pull --query "SELECT * FROM users" --max-rows 50000

# No limit (use with caution)
pg_rocket pull --query "SELECT * FROM users" --force
```

### Foreign Key Integrity Validation

Before inserting into the target database, `pg_rocket` validates that all foreign key references exist. If validation fails, no data is inserted.

## How It Works

1. **Query Analysis**: Parses your SQL query using `EXPLAIN` to identify the base table
2. **Schema Discovery**: Extracts foreign key and primary key metadata from `pg_catalog`
3. **Graph Traversal**: Performs breadth-first search following FK relationships
4. **Topological Sorting**: Orders tables to respect dependencies (parents before children)
5. **Output Generation**: Produces SQL INSERTs or JSON with deterministic ordering

### Traversal Algorithm

```
Root Query → [users WHERE id = 42]
    ↓
Fetch Parents (upward):
    organizations ← users.org_id
        ↓
    regions ← organizations.region_id
    
Fetch Children (downward):
    posts ← users.id (author_id)
        ↓
    comments ← posts.id (post_id)
    ↓
    orders ← users.id (user_id)
```

All discovered rows are:
- Deduplicated by primary key
- Sorted deterministically
- Validated for FK integrity

## Schema Requirements

### ✅ Supported

- Tables with primary keys (single or composite)
- Foreign key relationships in the `public` schema
- Self-referential foreign keys (e.g., `users.manager_id → users.id`)
- All standard PostgreSQL data types (including JSONB, arrays, UUIDs, etc.)

### ❌ Not Supported

- Tables without primary keys
- Multi-table cyclic foreign keys (A → B → C → A)
- Cross-schema relationships (only `public` schema)

### Compatibility Check

Run `pg_rocket inspect` to verify your schema is compatible.

## Troubleshooting

### "table does not have a primary key"

**Solution:** Add a primary key to the table:
```sql
ALTER TABLE my_table ADD PRIMARY KEY (id);
```

### "cycle detected in table dependencies"

**Problem:** Multi-table FK cycle exists (e.g., A → B → B → A).

**Solution:** Use `--parents` or `--children` to specify traversal direction and break the cycle:
```bash
pg_rocket pull --query "SELECT * FROM table_a WHERE id = 1" --parents
```

### "primary key column not included in SELECT"

**Problem:** Your query doesn't include the primary key column.

**Solution:** Include the PK or use `SELECT *`:
```sql
-- Bad
SELECT name FROM users WHERE id = 1

-- Good
SELECT * FROM users WHERE id = 1
```

### "foreign key integrity violation"

**Problem:** Extracted data references parent records that weren't extracted.

**Solution:** Use full traversal (remove `--parents` or `--children` flags) or check your FK relationships.

## Performance

- **Batching**: Queries are automatically batched (500 rows per query)
- **Memory**: All data is held in memory before output
- **Indexing**: Ensure FK columns are indexed for optimal performance
- **Network**: Uses connection pooling for efficient database communication

### Benchmarks

Typical extraction performance (on a modern workstation):
- Small datasets (<100 rows): <1 second
- Medium datasets (1,000-10,000 rows): 2-10 seconds
- Large datasets (50,000+ rows): 30+ seconds

## Development

### Building from Source

```bash
make build        # Compile binary
make install      # Install to /usr/local/bin
make clean        # Remove build artifacts
```

### Running Tests

```bash
make test         # Run integration tests (requires Docker)
```

Tests use Docker Compose to spin up a PostgreSQL container with test schemas.

### Project Structure

```
pg_rocket/
├── cmd/                    # CLI command definitions
│   ├── root.go            # Root cobra command
│   ├── pull.go            # Pull command (main logic)
│   ├── inspect.go         # Inspect command
│   └── version.go         # Version command
├── internal/
│   ├── db/                # Database layer
│   │   ├── connection.go  # Connection pooling
│   │   ├── metadata.go    # FK/PK extraction
│   │   └── explain.go     # Query analysis
│   ├── graph/             # FK graph structures
│   │   ├── types.go       # Graph data structures
│   │   └── topo.go        # Topological sorting
│   ├── extractor/         # Core extraction engine
│   │   ├── traversal.go   # BFS traversal
│   │   └── progress.go    # Progress tracking
│   └── output/            # Output writers
│       ├── sql_writer.go  # SQL INSERT generation
│       ├── json_writer.go # JSON output
│       └── executor.go    # Direct DB execution
├── test/
│   └── integration/       # Docker-based integration tests
│       ├── docker-compose.yml
│       ├── run_tests.sh
│       └── test_schemas/
└── Makefile
```

## Advanced Usage

### Handling Large Datasets

```bash
# Extract with higher limit and force flag
pg_rocket pull \
  --query "SELECT * FROM events WHERE created_at > '2024-01-01'" \
  --max-rows 100000 \
  --force \
  --out events_2024.sql
```

### Production → Staging Sync

```bash
#!/bin/bash
# sync_tenant.sh - Sync a tenant from prod to staging

TENANT_ID=$1

pg_rocket pull \
  --source "postgres://readonly@prod.db:5432/proddb?sslmode=require" \
  --target "postgres://admin@staging.db:5432/stagingdb?sslmode=require" \
  --query "SELECT * FROM tenants WHERE id = '$TENANT_ID'" \
  --exec \
  --upsert \
  --verbose
```

### Extracting Tenant Data

```bash
# Multi-tenant app: extract everything for tenant-123
pg_rocket pull \
  --query "SELECT * FROM tenants WHERE id = 'tenant-123'" \
  --out tenant_123.sql \
  --max-rows 50000 \
  --force
```

### Date-Scoped Extraction

```bash
# Extract last week's orders with all related data
pg_rocket pull \
  --query "SELECT * FROM orders WHERE created_at > NOW() - INTERVAL '7 days'" \
  --json \
  --out recent_orders.json
```

## How Foreign Key Traversal Works

`pg_rocket` uses breadth-first search to discover related records:

### Upward Traversal (Parents)

Follows FKs "up" to parent tables:

```
orders (id=1001) → users (id=42) → organizations (id=5)
```

### Downward Traversal (Children)

Follows FKs "down" to child tables:

```
users (id=42) → posts (author_id=42) → comments (post_id=...)
               → orders (user_id=42) → line_items (order_id=...)
```

### Full Traversal (Default)

Combines both directions to get the complete relational closure.

### Cycle Handling

- **Self-referential FKs**: Fully supported (e.g., `users.manager_id → users.id`)
- **Multi-table cycles**: Not supported - use `--parents` or `--children` to break the cycle

## Deterministic Output

Given identical inputs and database state, `pg_rocket` produces **byte-for-byte identical output**:

- Rows are sorted by primary key within each table
- Tables are sorted topologically (parents before children)
- Timestamps and output are consistent

This makes `pg_rocket` suitable for:
- Version control of test fixtures
- Regression testing
- Auditable data exports

## Limitations

- **Single schema**: Only traverses the `public` schema
- **PostgreSQL only**: No support for MySQL, SQLite, etc.
- **In-memory**: All data is loaded into memory before output
- **No data masking**: Use separate tools for anonymization
- **No parallel extraction**: Single-threaded traversal

## Security Considerations

⚠️ **Important Security Notes:**

1. **Credential Management**: Never commit DSNs with passwords to version control. Use environment variables or secure credential stores.

2. **Read-Only Connections**: Use read-only database users for `--source` when possible:
   ```sql
   CREATE USER pg_rocket_readonly WITH PASSWORD 'secure_password';
   GRANT CONNECT ON DATABASE mydb TO pg_rocket_readonly;
   GRANT USAGE ON SCHEMA public TO pg_rocket_readonly;
   GRANT SELECT ON ALL TABLES IN SCHEMA public TO pg_rocket_readonly;
   ```

3. **SSL/TLS**: Always use `sslmode=require` or higher for production:
   ```bash
   export PGROCKET_SOURCE="postgres://user:pass@prod:5432/db?sslmode=require"
   ```

4. **Row Limits**: Default 10,000-row limit prevents accidental large extractions. Use `--max-rows` or `--force` consciously.

5. **Confirmation Prompts**: `--exec` mode requires explicit user confirmation before database writes.

6. **Separate Environments**: The tool validates that source and target are different to prevent accidental overwrites.

## Roadmap

Planned for future releases:

- Multi-schema support
- Data masking/anonymization
- Parallel extraction for performance
- MySQL and SQLite support
- Configuration files (`.pg_rocket.yml`)
- Incremental extraction (delta mode)

## Contributing

Contributions are welcome! 

1. Fork the repository
2. Create a feature branch
3. Make your changes and add tests
4. Submit a pull request

See [CONTRIBUTING.md](CONTRIBUTING.md) for details.

## License

MIT License - see [LICENSE](LICENSE) file for details.

## Support

- **Issues**: [GitHub Issues](https://github.com/riyasyash/pg_rocket/issues)
- **Discussions**: [GitHub Discussions](https://github.com/riyasyash/pg_rocket/discussions)

---

**Built with Go and [pgx](https://github.com/jackc/pgx)**
