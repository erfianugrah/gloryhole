package storage

import (
	"os"
	"testing"
)

func TestNewDB(t *testing.T) {
	db, err := NewDB(":memory:")
	if err != nil {
		t.Fatalf("NewDB() failed: %v", err)
	}
	if db == nil {
		t.Fatal("NewDB() returned nil")
	}
	defer db.Close()
}

func TestLogQuery(t *testing.T) {
	db, err := NewDB(":memory:")
	if err != nil {
		t.Fatalf("NewDB() failed: %v", err)

	}
	defer db.Close()

	// This test will be expanded to test logging to the database.
	err = db.LogQuery("example.com", "127.0.0.1", "BLOCKED")
	if err != nil {
		t.Fatalf("LogQuery() failed: %v", err)
	}
}
