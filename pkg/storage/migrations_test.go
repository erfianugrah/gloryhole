package storage

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// setupTestDB creates a temporary SQLite database for testing
func setupTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()

	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "gloryhole-migrations-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")

	// Open database
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		t.Fatalf("failed to open database: %v", err)
	}

	// Cleanup function
	cleanup := func() {
		_ = db.Close()
		_ = os.RemoveAll(tmpDir)
	}

	return db, cleanup
}

func TestGetCurrentVersion_FreshDatabase(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	version, err := getCurrentVersion(db)
	if err != nil {
		t.Fatalf("getCurrentVersion failed: %v", err)
	}

	if version != 0 {
		t.Errorf("expected version 0 for fresh database, got %d", version)
	}
}

func TestGetCurrentVersion_AfterMigration(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Apply initial migration
	if err := runMigrations(db); err != nil {
		t.Fatalf("runMigrations failed: %v", err)
	}

	// Check version
	version, err := getCurrentVersion(db)
	if err != nil {
		t.Fatalf("getCurrentVersion failed: %v", err)
	}

	// Should be at least version 1 (initial schema)
	if version < 1 {
		t.Errorf("expected version >= 1 after migration, got %d", version)
	}
}

func TestRunMigrations_FreshDatabase(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Run migrations on fresh database
	if err := runMigrations(db); err != nil {
		t.Fatalf("runMigrations failed: %v", err)
	}

	// Verify schema_version table exists
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM schema_version").Scan(&count)
	if err != nil {
		t.Fatalf("failed to query schema_version: %v", err)
	}

	if count < 1 {
		t.Errorf("expected at least 1 migration record, got %d", count)
	}

	// Verify queries table exists
	err = db.QueryRow("SELECT COUNT(*) FROM queries").Scan(&count)
	if err != nil {
		t.Fatalf("queries table should exist: %v", err)
	}

	// Verify domain_stats table exists
	err = db.QueryRow("SELECT COUNT(*) FROM domain_stats").Scan(&count)
	if err != nil {
		t.Fatalf("domain_stats table should exist: %v", err)
	}

	// Verify statistics table exists
	err = db.QueryRow("SELECT COUNT(*) FROM statistics").Scan(&count)
	if err != nil {
		t.Fatalf("statistics table should exist: %v", err)
	}
}

func TestRunMigrations_Idempotent(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Run migrations first time
	if err := runMigrations(db); err != nil {
		t.Fatalf("first runMigrations failed: %v", err)
	}

	version1, err := getCurrentVersion(db)
	if err != nil {
		t.Fatalf("getCurrentVersion failed: %v", err)
	}

	// Run migrations second time (should be idempotent)
	err = runMigrations(db)
	if err != nil {
		t.Fatalf("second runMigrations failed: %v", err)
	}

	version2, err := getCurrentVersion(db)
	if err != nil {
		t.Fatalf("getCurrentVersion failed after second run: %v", err)
	}

	// Version should be the same
	if version1 != version2 {
		t.Errorf("version changed from %d to %d (should be idempotent)", version1, version2)
	}

	// Check migration count hasn't doubled
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM schema_version").Scan(&count)
	if err != nil {
		t.Fatalf("failed to query schema_version: %v", err)
	}

	// Should have exactly as many records as migrations (not doubled)
	expectedCount := len(getMigrations())
	if count != expectedCount {
		t.Errorf("expected %d migration records, got %d", expectedCount, count)
	}
}

func TestGetMigrations_Sorted(t *testing.T) {
	migs := getMigrations()

	if len(migs) == 0 {
		t.Fatal("expected at least one migration")
	}

	// Verify migrations are sorted by version
	for i := 1; i < len(migs); i++ {
		if migs[i].Version <= migs[i-1].Version {
			t.Errorf("migrations not sorted: v%d comes after v%d",
				migs[i].Version, migs[i-1].Version)
		}
	}
}

func TestApplyMigration_SingleMigration(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create a test migration
	testMigration := Migration{
		Version:     1,
		Description: "Test migration",
		SQL: `
			CREATE TABLE test_table (
				id INTEGER PRIMARY KEY,
				name TEXT
			);
			CREATE TABLE schema_version (
				version INTEGER PRIMARY KEY,
				applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
			);
		`,
	}

	// Apply the migration
	if err := applyMigration(db, testMigration); err != nil {
		t.Fatalf("applyMigration failed: %v", err)
	}

	// Verify table was created
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM test_table").Scan(&count)
	if err != nil {
		t.Fatalf("test_table should exist: %v", err)
	}

	// Verify migration was recorded
	var recordedVersion int
	err = db.QueryRow("SELECT version FROM schema_version WHERE version = ?", testMigration.Version).Scan(&recordedVersion)
	if err != nil {
		t.Fatalf("migration should be recorded: %v", err)
	}

	if recordedVersion != testMigration.Version {
		t.Errorf("expected version %d, got %d", testMigration.Version, recordedVersion)
	}
}

func TestApplyMigration_Rollback(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create schema_version table first
	_, err := db.Exec(`
		CREATE TABLE schema_version (
			version INTEGER PRIMARY KEY,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	`)
	if err != nil {
		t.Fatalf("failed to create schema_version: %v", err)
	}

	// Create a test migration with invalid SQL
	badMigration := Migration{
		Version:     1,
		Description: "Bad migration",
		SQL: `
			CREATE TABLE test_table (id INTEGER PRIMARY KEY);
			THIS IS INVALID SQL THAT WILL FAIL;
			CREATE TABLE another_table (id INTEGER PRIMARY KEY);
		`,
	}

	// Apply the migration (should fail)
	err = applyMigration(db, badMigration)
	if err == nil {
		t.Fatal("expected applyMigration to fail with invalid SQL")
	}

	// Verify no tables were created (transaction rolled back)
	err = db.QueryRow("SELECT COUNT(*) FROM test_table").Scan(new(int))
	if err == nil {
		t.Error("test_table should not exist after rollback")
	}

	// Verify migration was not recorded
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM schema_version WHERE version = ?", badMigration.Version).Scan(&count)
	if err != nil {
		t.Fatalf("failed to query schema_version: %v", err)
	}

	if count != 0 {
		t.Error("migration should not be recorded after failure")
	}
}

func TestMigrations_AllHaveUniqueVersions(t *testing.T) {
	migs := getMigrations()
	versions := make(map[int]bool)

	for _, mig := range migs {
		if versions[mig.Version] {
			t.Errorf("duplicate version found: %d", mig.Version)
		}
		versions[mig.Version] = true
	}
}

func TestMigrations_AllHaveDescriptions(t *testing.T) {
	migs := getMigrations()

	for _, mig := range migs {
		if mig.Description == "" {
			t.Errorf("migration v%d has no description", mig.Version)
		}
	}
}

func TestMigrations_AllHaveSQL(t *testing.T) {
	migs := getMigrations()

	for _, mig := range migs {
		if mig.SQL == "" {
			t.Errorf("migration v%d has no SQL", mig.Version)
		}
	}
}

// TestMigrations_Integration tests the full migration system with the actual migrations
func TestMigrations_Integration(t *testing.T) {
	// Create a config for SQLite storage
	cfg := &Config{
		Enabled: true,
		Backend: "sqlite",
		SQLite: SQLiteConfig{
			Path:        ":memory:",
			BusyTimeout: 5000,
			CacheSize:   10000,
			WALMode:     false,
		},
		BufferSize:    1000,
		FlushInterval: 1000000000, // 1 second
		BatchSize:     100,
		RetentionDays: 30,
	}

	// Create storage (which will run migrations)
	stor, err := NewSQLiteStorage(cfg)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer func() { _ = stor.Close() }()

	// Verify we can access the storage
	sqlStorage, ok := stor.(*SQLiteStorage)
	if !ok {
		t.Fatal("storage is not SQLiteStorage")
	}

	// Check version is correct
	version, err := getCurrentVersion(sqlStorage.db)
	if err != nil {
		t.Fatalf("failed to get version: %v", err)
	}

	expectedVersion := len(getMigrations())
	if version != expectedVersion {
		t.Errorf("expected version %d, got %d", expectedVersion, version)
	}
}
