# Database Migrations

This directory contains SQL migration files for the Glory-Hole DNS server database schema.

## Overview

The migration system provides versioned, incremental schema changes with:
- **Automatic application**: Migrations run on startup
- **Transactional safety**: Each migration runs in a transaction (rollback on failure)
- **Idempotency**: Safe to run multiple times (only applies pending migrations)
- **Version tracking**: `schema_version` table records applied migrations

## Adding a New Migration

### 1. Create the SQL File

Create a new file in this directory with the naming pattern:
```
XXX_description.sql
```

Where:
- `XXX` is the migration version number (e.g., `002`, `003`)
- `description` is a brief description (e.g., `add_client_index`)

Example: `002_add_client_index.sql`

```sql
-- Add index on client_ip for faster client queries
CREATE INDEX IF NOT EXISTS idx_queries_client_ip_timestamp
    ON queries(client_ip, timestamp DESC);
```

### 2. Register the Migration

Edit `../migrations.go` and add your migration to the `migrations` slice:

```go
var migrations = []Migration{
    {
        Version:     1,
        Description: "Initial schema with queries, domain_stats, and statistics tables",
        SQL:         initialSchema,
    },
    {
        Version:     2,
        Description: "Add composite index on client_ip and timestamp",
        SQL:         migrationV2,  // Reference to embedded SQL
    },
}
```

### 3. Embed the SQL File

Add an embed directive at the top of `../migrations.go`:

```go
//go:embed migrations/002_add_client_index.sql
var migrationV2 string
```

### 4. Test the Migration

Run the migration tests to verify:

```bash
go test ./pkg/storage -run TestMigrations -v
```

### 5. Verify in Production

The migration will automatically apply on next server startup. Check logs for:
```
INFO: Applying migration v2: Add composite index on client_ip and timestamp
```

## Migration Guidelines

### DO:
-  Use `CREATE TABLE IF NOT EXISTS` for new tables
-  Use `CREATE INDEX IF NOT EXISTS` for new indexes
-  Use `ALTER TABLE ADD COLUMN` for new columns (with DEFAULT)
-  Keep migrations small and focused (one change per migration)
-  Test migrations on a copy of production data
-  Add comments explaining the purpose

### DON'T:
-  Don't modify existing migration files (create new ones instead)
-  Don't use `DROP TABLE` without data backup strategy
-  Don't add `NOT NULL` columns without DEFAULT values
-  Don't skip version numbers
-  Don't use database-specific features (stick to SQLite standard SQL)

## Rollback Strategy

The migration system does **not** support automatic rollbacks. If a migration causes issues:

1. **Option 1: Forward-fix**
   - Create a new migration to fix the problem
   - Example: If v2 added a bad index, v3 drops it and creates correct one

2. **Option 2: Restore from backup**
   - Restore database from pre-migration backup
   - Remove problematic migration from code
   - Restart server

## Example Migrations

### Adding a Column
```sql
-- Add client_name column for reverse DNS lookups
ALTER TABLE queries ADD COLUMN client_name TEXT DEFAULT NULL;
```

### Adding an Index
```sql
-- Speed up queries by blocked status and timestamp
CREATE INDEX IF NOT EXISTS idx_queries_blocked_timestamp
    ON queries(blocked, timestamp DESC)
    WHERE blocked = 1;
```

### Creating a Table
```sql
-- Store DNS query patterns for anomaly detection
CREATE TABLE IF NOT EXISTS query_patterns (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    pattern TEXT NOT NULL UNIQUE,
    query_count INTEGER NOT NULL DEFAULT 0,
    first_seen DATETIME DEFAULT CURRENT_TIMESTAMP,
    last_seen DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_query_patterns_count
    ON query_patterns(query_count DESC);
```

### Data Migration
```sql
-- Populate new client_name column from existing data
-- (This might be slow on large tables - consider batching in code)
UPDATE queries
SET client_name = (
    SELECT COALESCE(reverse_dns(client_ip), client_ip)
)
WHERE client_name IS NULL;
```

## Schema Version Table

The `schema_version` table tracks applied migrations:

| Column      | Type     | Description                    |
|-------------|----------|--------------------------------|
| version     | INTEGER  | Migration version (primary key)|
| applied_at  | DATETIME | When migration was applied     |

Example contents:
```
version | applied_at
--------|-------------------
1       | 2025-01-15 10:30:00
2       | 2025-01-20 14:15:00
3       | 2025-01-25 09:00:00
```

## Testing

### Unit Tests
```bash
# Run all migration tests
go test ./pkg/storage -run TestMigrations -v

# Run specific test
go test ./pkg/storage -run TestRunMigrations_Idempotent -v
```

### Integration Test
```bash
# Test with real database
go test ./pkg/storage -run TestMigrations_Integration -v
```

### Manual Testing
```bash
# 1. Create test database
sqlite3 test.db < migrations/001_initial.sql

# 2. Check version
echo "SELECT * FROM schema_version;" | sqlite3 test.db

# 3. Apply new migration manually
sqlite3 test.db < migrations/002_new_migration.sql

# 4. Verify schema
echo ".schema" | sqlite3 test.db
```

## Troubleshooting

### Migration fails with "constraint failed"
- Check if migration tries to insert duplicate data
- Verify unique constraints aren't violated
- Review foreign key constraints

### Migration fails with "no such table"
- Ensure dependent tables exist
- Check migration order (dependencies must come first)
- Verify table names are correct

### Migration appears to succeed but changes aren't visible
- Check if using transactions properly (COMMIT needed)
- Verify schema_version was updated
- Check for typos in table/column names

## Performance Considerations

- **Large tables**: Schema changes may lock table briefly
- **Production timing**: Apply during maintenance windows
- **Index creation**: Can be slow on large datasets
- **Data migrations**: Consider batching in application code

## Version History

| Version | Date       | Description                                  |
|---------|------------|----------------------------------------------|
| 1       | 2024-01-01 | Initial schema (queries, domain_stats, etc.) |

---

For more information, see:
- `../migrations.go` - Migration system implementation
- `../sqlite.go` - Database initialization
- SQLite documentation: https://www.sqlite.org/lang.html
