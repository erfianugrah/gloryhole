package storage

import (
	"database/sql"
	"fmt"
	"sort"
)

// Migration represents a database schema migration
type Migration struct {
	SQL         string
	Description string
	Version     int
}

// migrations is the registry of all database migrations in order
// Each migration must have a unique version number and will be applied
// in ascending order. Migrations are idempotent and transactional.
var migrations = []Migration{
	{
		Version:     1,
		Description: "Initial schema with queries, domain_stats, and statistics tables",
		SQL:         initialSchema,
	},
	{
		Version:     2,
		Description: "Add block trace column for query decision breadcrumbs",
		SQL: `
			ALTER TABLE queries ADD COLUMN block_trace TEXT;
		`,
	},
	{
		Version:     3,
		Description: "Change response_time_ms from INTEGER to REAL for sub-millisecond precision",
		SQL: `
			-- Create new table with REAL type for response_time_ms
			CREATE TABLE queries_new (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
				client_ip TEXT NOT NULL,
				domain TEXT NOT NULL,
				query_type TEXT NOT NULL,
				response_code INTEGER NOT NULL,
				blocked BOOLEAN NOT NULL,
				cached BOOLEAN NOT NULL,
				response_time_ms REAL NOT NULL,
				upstream TEXT,
				block_trace TEXT
			);

			-- Copy data from old table
			INSERT INTO queries_new
			SELECT id, timestamp, client_ip, domain, query_type, response_code,
			       blocked, cached, CAST(response_time_ms AS REAL), upstream, block_trace
			FROM queries;

			-- Drop old table
			DROP TABLE queries;

			-- Rename new table
			ALTER TABLE queries_new RENAME TO queries;

			-- Recreate indexes
			CREATE INDEX idx_queries_timestamp ON queries(timestamp);
			CREATE INDEX idx_queries_domain ON queries(domain);
			CREATE INDEX idx_queries_blocked ON queries(blocked);
			CREATE INDEX idx_queries_client_ip ON queries(client_ip);
			CREATE INDEX idx_queries_cached ON queries(cached);
		`,
	},
	{
		Version:     4,
		Description: "Add composite indexes for common query patterns and analytics",
		SQL: `
			-- Composite index for time-range queries on specific domains
			-- Speeds up: SELECT * FROM queries WHERE domain = ? AND timestamp BETWEEN ? AND ?
			CREATE INDEX idx_queries_domain_timestamp ON queries(domain, timestamp);

			-- Composite index for client-specific time-range queries
			-- Speeds up: SELECT * FROM queries WHERE client_ip = ? AND timestamp BETWEEN ? AND ?
			CREATE INDEX idx_queries_client_timestamp ON queries(client_ip, timestamp);

			-- Composite index for blocked query analytics over time
			-- Speeds up: SELECT * FROM queries WHERE blocked = 1 AND timestamp BETWEEN ? AND ?
			CREATE INDEX idx_queries_blocked_timestamp ON queries(blocked, timestamp);

			-- Composite index for cached query analytics
			-- Speeds up: SELECT * FROM queries WHERE cached = 1 AND timestamp BETWEEN ? AND ?
			CREATE INDEX idx_queries_cached_timestamp ON queries(cached, timestamp);

			-- Composite index for response time analysis by domain
			-- Speeds up: SELECT AVG(response_time_ms) FROM queries WHERE domain = ? GROUP BY timestamp
			CREATE INDEX idx_queries_domain_response_time ON queries(domain, response_time_ms);
		`,
	},
	// Future migrations will be added here with incrementing version numbers
	// Example:
	// {
	//     Version:     5,
	//     Description: "Add client_name column to queries table",
	//     SQL:         `ALTER TABLE queries ADD COLUMN client_name TEXT;`,
	// },
}

// getMigrations returns all migrations sorted by version
func getMigrations() []Migration {
	// Create a copy to avoid external modification
	result := make([]Migration, len(migrations))
	copy(result, migrations)

	// Sort by version to ensure correct order
	sort.Slice(result, func(i, j int) bool {
		return result[i].Version < result[j].Version
	})

	return result
}

// getCurrentVersion returns the current schema version from the database
// Returns 0 if schema_version table doesn't exist (fresh database)
func getCurrentVersion(db *sql.DB) (int, error) {
	// Check if schema_version table exists
	var tableExists bool
	err := db.QueryRow(`
		SELECT 1 FROM sqlite_master
		WHERE type='table' AND name='schema_version'
	`).Scan(&tableExists)

	if err == sql.ErrNoRows {
		// Table doesn't exist, this is a fresh database
		return 0, nil
	} else if err != nil {
		return 0, fmt.Errorf("failed to check schema_version table: %w", err)
	}

	// Table exists, get the highest version number
	var version int
	err = db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_version").Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("failed to query schema version: %w", err)
	}

	return version, nil
}

// applyMigration applies a single migration within a transaction
func applyMigration(db *sql.DB, migration Migration) error {
	// Start transaction
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Execute migration SQL
	_, err = tx.Exec(migration.SQL)
	if err != nil {
		return fmt.Errorf("failed to execute migration SQL: %w", err)
	}

	// Record migration in schema_version table
	_, err = tx.Exec(`
		INSERT INTO schema_version (version, applied_at)
		VALUES (?, CURRENT_TIMESTAMP)
	`, migration.Version)
	if err != nil {
		return fmt.Errorf("failed to record migration version: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit migration: %w", err)
	}

	return nil
}

// runMigrations applies all pending migrations in order
// This function is called during storage initialization and ensures
// the database schema is up to date.
//
// Migration process:
// 1. Get current database version
// 2. Find all migrations with version > current version
// 3. Apply each migration in order within a transaction
// 4. Record each migration in schema_version table
//
// Safety features:
// - Each migration runs in its own transaction (atomic)
// - Migrations are idempotent (safe to retry)
// - Version numbers prevent duplicate application
// - Failures rollback automatically
//
// Returns error if any migration fails, leaving database in
// the last successful migration state.
func runMigrations(db *sql.DB) error {
	// Get current database version
	currentVersion, err := getCurrentVersion(db)
	if err != nil {
		return fmt.Errorf("failed to get current version: %w", err)
	}

	// Get all available migrations
	allMigrations := getMigrations()

	// Find pending migrations (version > current)
	var pendingMigrations []Migration
	for _, migration := range allMigrations {
		if migration.Version > currentVersion {
			pendingMigrations = append(pendingMigrations, migration)
		}
	}

	// No pending migrations
	if len(pendingMigrations) == 0 {
		return nil
	}

	// Apply each pending migration
	for _, migration := range pendingMigrations {
		if err := applyMigration(db, migration); err != nil {
			return fmt.Errorf(
				"failed to apply migration v%d (%s): %w",
				migration.Version,
				migration.Description,
				err,
			)
		}
	}

	return nil
}
