package storage

import (
	"database/sql"
	_ "modernc.org/sqlite"
)

// DB is the storage interface
type DB struct {
	*sql.DB
}

// NewDB creates a new storage instance
func NewDB(path string) (*DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	return &DB{db}, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.DB.Close()
}

// LogQuery logs a DNS query
func (db *DB) LogQuery(domain, clientIP, status string) error {
	// Logic to insert a query log into the database will go here.
	return nil
}
