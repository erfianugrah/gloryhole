// +build ignore

package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

func main() {
	dbPath := flag.String("db", "", "Path to SQLite database file (required)")
	dryRun := flag.Bool("dry-run", false, "Show what would be migrated without making changes")
	flag.Parse()

	if *dbPath == "" {
		fmt.Println("Usage: go run scripts/migrate-timestamps.go -db /path/to/gloryhole.db [-dry-run]")
		os.Exit(1)
	}

	if _, err := os.Stat(*dbPath); os.IsNotExist(err) {
		log.Fatalf("Database file does not exist: %s", *dbPath)
	}

	db, err := sql.Open("sqlite3", *dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Count total records
	var total int64
	if err := db.QueryRow("SELECT COUNT(*) FROM queries").Scan(&total); err != nil {
		log.Fatalf("Failed to count records: %v", err)
	}

	log.Printf("Found %d total records\n", total)

	// Check how many need migration (contain " CET " or " UTC " or "m=+")
	var needsMigration int64
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM queries
		WHERE timestamp LIKE '% CET %'
		   OR timestamp LIKE '% UTC %'
		   OR timestamp LIKE '%m=+%'
	`).Scan(&needsMigration); err != nil {
		log.Fatalf("Failed to count records needing migration: %v", err)
	}

	log.Printf("Records needing migration: %d\n", needsMigration)

	if needsMigration == 0 {
		log.Println("No records need migration. All timestamps are already in RFC3339 format.")
		return
	}

	if *dryRun {
		log.Println("DRY RUN: Would migrate records but not making changes")
		return
	}

	// Confirm before proceeding
	fmt.Print("\nThis will modify the database. Continue? (yes/no): ")
	var confirm string
	fmt.Scanln(&confirm)
	if strings.ToLower(confirm) != "yes" {
		log.Println("Migration cancelled")
		return
	}

	// Begin transaction
	tx, err := db.Begin()
	if err != nil {
		log.Fatalf("Failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	log.Println("Starting migration...")

	// Fetch all records with old format
	rows, err := tx.Query(`
		SELECT id, timestamp FROM queries
		WHERE timestamp LIKE '% CET %'
		   OR timestamp LIKE '% UTC %'
		   OR timestamp LIKE '%m=+%'
		ORDER BY id
	`)
	if err != nil {
		log.Fatalf("Failed to query records: %v", err)
	}
	defer rows.Close()

	updateStmt, err := tx.Prepare("UPDATE queries SET timestamp = ? WHERE id = ?")
	if err != nil {
		log.Fatalf("Failed to prepare update statement: %v", err)
	}
	defer updateStmt.Close()

	var migrated, failed int64
	for rows.Next() {
		var id int64
		var oldTimestamp string

		if err := rows.Scan(&id, &oldTimestamp); err != nil {
			log.Printf("Failed to scan row %d: %v", id, err)
			failed++
			continue
		}

		// Parse the old Go time.Time format
		// Example: "2026-01-01 10:00:47.185322115 +0100 CET m=+20.711025655"
		newTimestamp, err := parseGoTimeString(oldTimestamp)
		if err != nil {
			log.Printf("Failed to parse timestamp for row %d (%s): %v", id, oldTimestamp, err)
			failed++
			continue
		}

		// Update with RFC3339Nano format
		if _, err := updateStmt.Exec(newTimestamp, id); err != nil {
			log.Printf("Failed to update row %d: %v", id, err)
			failed++
			continue
		}

		migrated++
		if migrated%10000 == 0 {
			log.Printf("Migrated %d / %d records...", migrated, needsMigration)
		}
	}

	if err := rows.Err(); err != nil {
		log.Fatalf("Error iterating rows: %v", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		log.Fatalf("Failed to commit transaction: %v", err)
	}

	log.Printf("\nMigration complete!")
	log.Printf("  Successfully migrated: %d", migrated)
	log.Printf("  Failed: %d", failed)
	log.Printf("  Total: %d", migrated+failed)

	// Verify migration
	var remaining int64
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM queries
		WHERE timestamp LIKE '% CET %'
		   OR timestamp LIKE '% UTC %'
		   OR timestamp LIKE '%m=+%'
	`).Scan(&remaining); err != nil {
		log.Printf("Warning: Failed to verify migration: %v", err)
	} else {
		log.Printf("  Remaining old format: %d", remaining)
	}
}

// parseGoTimeString parses Go's default time.Time.String() format
// Example: "2026-01-01 10:00:47.185322115 +0100 CET m=+20.711025655"
func parseGoTimeString(s string) (string, error) {
	// Remove the monotonic clock reading (m=+...)
	if idx := strings.Index(s, " m="); idx > 0 {
		s = s[:idx]
	}

	// Try parsing with various Go time layouts
	layouts := []string{
		"2006-01-02 15:04:05.999999999 -0700 MST",
		"2006-01-02 15:04:05.999999999 -0700",
		"2006-01-02 15:04:05 -0700 MST",
		"2006-01-02 15:04:05 -0700",
	}

	var t time.Time
	var err error
	for _, layout := range layouts {
		t, err = time.Parse(layout, s)
		if err == nil {
			return t.Format(time.RFC3339Nano), nil
		}
	}

	return "", fmt.Errorf("could not parse timestamp with any known layout: %s (last error: %v)", s, err)
}
