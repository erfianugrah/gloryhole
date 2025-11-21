package storage

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/001_initial.sql
var initialSchema string

// SQLiteStorage implements the Storage interface using SQLite
type SQLiteStorage struct {
	db     *sql.DB
	cfg    *Config
	buffer chan *QueryLog
	wg     sync.WaitGroup
	mu     sync.RWMutex
	closed bool

	// Prepared statements (cached for performance)
	stmtInsertQuery      *sql.Stmt
	stmtGetRecentQueries *sql.Stmt
	stmtGetStatistics    *sql.Stmt
}

// NewSQLiteStorage creates a new SQLite storage backend
func NewSQLiteStorage(cfg *Config) (Storage, error) {
	if cfg == nil {
		return nil, ErrInvalidConfig
	}

	// Open database connection
	db, err := sql.Open("sqlite", cfg.SQLite.Path)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConnectionFailed, err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(1) // SQLite works best with single connection
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	// Test connection
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("%w: %v", ErrConnectionFailed, err)
	}

	// Apply SQLite pragmas for performance
	pragmas := []string{
		fmt.Sprintf("PRAGMA busy_timeout = %d", cfg.SQLite.BusyTimeout),
		fmt.Sprintf("PRAGMA cache_size = %d", -cfg.SQLite.CacheSize), // Negative means KB
		"PRAGMA synchronous = NORMAL", // Balance between safety and performance
		"PRAGMA temp_store = MEMORY",  // Use memory for temp tables
		"PRAGMA mmap_size = 30000000000", // 30GB mmap
	}

	if cfg.SQLite.WALMode {
		pragmas = append(pragmas, "PRAGMA journal_mode = WAL")
	}

	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("failed to set pragma: %w", err)
		}
	}

	// Apply migrations
	if err := applyMigrations(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to apply migrations: %w", err)
	}

	// Prepare statements
	stmtInsert, err := db.Prepare(`
		INSERT INTO queries
		(timestamp, client_ip, domain, query_type, response_code, blocked, cached, response_time_ms, upstream)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to prepare insert statement: %w", err)
	}

	storage := &SQLiteStorage{
		db:                  db,
		cfg:                 cfg,
		buffer:              make(chan *QueryLog, cfg.BufferSize),
		stmtInsertQuery:     stmtInsert,
	}

	// Start background flush worker
	storage.wg.Add(1)
	go storage.flushWorker()

	return storage, nil
}

// applyMigrations applies database migrations
func applyMigrations(db *sql.DB) error {
	// Check if schema_version table exists
	var tableExists bool
	err := db.QueryRow("SELECT 1 FROM sqlite_master WHERE type='table' AND name='schema_version'").Scan(&tableExists)
	if err == sql.ErrNoRows {
		// Schema doesn't exist, apply initial migration
		if _, err := db.Exec(initialSchema); err != nil {
			return fmt.Errorf("failed to apply initial schema: %w", err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to check schema version: %w", err)
	}

	// TODO: Add more migrations here as schema evolves
	return nil
}

// LogQuery logs a DNS query (async, buffered)
func (s *SQLiteStorage) LogQuery(ctx context.Context, query *QueryLog) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return ErrClosed
	}

	// Set timestamp if not provided
	if query.Timestamp.IsZero() {
		query.Timestamp = time.Now()
	}

	// Non-blocking write to buffer
	select {
	case s.buffer <- query:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		// Buffer full, drop query or return error
		return ErrBufferFull
	}
}

// flushWorker processes buffered queries in the background
func (s *SQLiteStorage) flushWorker() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.cfg.FlushInterval)
	defer ticker.Stop()

	batch := make([]*QueryLog, 0, s.cfg.BatchSize)

	flush := func() {
		if len(batch) == 0 {
			return
		}

		if err := s.flushBatch(batch); err != nil {
			// Log error but continue (we don't want to crash the server)
			// TODO: Add proper logging
			fmt.Printf("Error flushing batch: %v\n", err)
		}

		// Clear batch
		batch = batch[:0]
	}

	for {
		select {
		case query, ok := <-s.buffer:
			if !ok {
				// Channel closed, flush remaining and exit
				flush()
				return
			}

			batch = append(batch, query)

			// Flush if batch is full
			if len(batch) >= s.cfg.BatchSize {
				flush()
			}

		case <-ticker.C:
			// Periodic flush
			flush()
		}
	}
}

// flushBatch writes a batch of queries to the database
func (s *SQLiteStorage) flushBatch(queries []*QueryLog) error {
	if len(queries) == 0 {
		return nil
	}

	// Use transaction for batch insert
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrQueryFailed, err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt := tx.Stmt(s.stmtInsertQuery)

	for _, query := range queries {
		_, err := stmt.Exec(
			query.Timestamp,
			query.ClientIP,
			query.Domain,
			query.QueryType,
			query.ResponseCode,
			query.Blocked,
			query.Cached,
			query.ResponseTimeMs,
			query.Upstream,
		)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrQueryFailed, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%w: %v", ErrQueryFailed, err)
	}

	// Update domain stats asynchronously
	go s.updateDomainStats(queries)

	return nil
}

// updateDomainStats updates the domain_stats table
func (s *SQLiteStorage) updateDomainStats(queries []*QueryLog) {
	for _, query := range queries {
		_, err := s.db.Exec(`
			INSERT INTO domain_stats (domain, query_count, last_queried, first_queried, blocked)
			VALUES (?, 1, ?, ?, ?)
			ON CONFLICT(domain) DO UPDATE SET
				query_count = query_count + 1,
				last_queried = excluded.last_queried
		`, query.Domain, query.Timestamp, query.Timestamp, query.Blocked)

		if err != nil {
			// Log but don't fail
			fmt.Printf("Error updating domain stats: %v\n", err)
		}
	}
}

// GetRecentQueries returns the most recent queries
func (s *SQLiteStorage) GetRecentQueries(ctx context.Context, limit int) ([]*QueryLog, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrClosed
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, timestamp, client_ip, domain, query_type, response_code,
		       blocked, cached, response_time_ms, upstream
		FROM queries
		ORDER BY timestamp DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrQueryFailed, err)
	}
	defer func() { _ = rows.Close() }()

	return scanQueryLogs(rows)
}

// GetQueriesByDomain returns queries for a specific domain
func (s *SQLiteStorage) GetQueriesByDomain(ctx context.Context, domain string, limit int) ([]*QueryLog, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrClosed
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, timestamp, client_ip, domain, query_type, response_code,
		       blocked, cached, response_time_ms, upstream
		FROM queries
		WHERE domain = ?
		ORDER BY timestamp DESC
		LIMIT ?
	`, domain, limit)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrQueryFailed, err)
	}
	defer func() { _ = rows.Close() }()

	return scanQueryLogs(rows)
}

// GetQueriesByClientIP returns queries from a specific client
func (s *SQLiteStorage) GetQueriesByClientIP(ctx context.Context, clientIP string, limit int) ([]*QueryLog, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrClosed
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, timestamp, client_ip, domain, query_type, response_code,
		       blocked, cached, response_time_ms, upstream
		FROM queries
		WHERE client_ip = ?
		ORDER BY timestamp DESC
		LIMIT ?
	`, clientIP, limit)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrQueryFailed, err)
	}
	defer func() { _ = rows.Close() }()

	return scanQueryLogs(rows)
}

// GetStatistics returns query statistics since a given time
func (s *SQLiteStorage) GetStatistics(ctx context.Context, since time.Time) (*Statistics, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrClosed
	}

	stats := &Statistics{
		Since: since,
		Until: time.Now(),
	}

	err := s.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*) as total,
			SUM(CASE WHEN blocked THEN 1 ELSE 0 END) as blocked,
			SUM(CASE WHEN cached THEN 1 ELSE 0 END) as cached,
			COUNT(DISTINCT domain) as unique_domains,
			COUNT(DISTINCT client_ip) as unique_clients,
			AVG(response_time_ms) as avg_response_time
		FROM queries
		WHERE timestamp >= ?
	`, since).Scan(
		&stats.TotalQueries,
		&stats.BlockedQueries,
		&stats.CachedQueries,
		&stats.UniqueDomains,
		&stats.UniqueClients,
		&stats.AvgResponseTimeMs,
	)

	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrQueryFailed, err)
	}

	// Calculate rates
	if stats.TotalQueries > 0 {
		stats.BlockRate = float64(stats.BlockedQueries) / float64(stats.TotalQueries) * 100
		stats.CacheHitRate = float64(stats.CachedQueries) / float64(stats.TotalQueries) * 100
	}

	return stats, nil
}

// GetTopDomains returns the most queried domains
func (s *SQLiteStorage) GetTopDomains(ctx context.Context, limit int, blocked bool) ([]*DomainStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrClosed
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT domain, query_count, last_queried, first_queried, blocked
		FROM domain_stats
		WHERE blocked = ?
		ORDER BY query_count DESC
		LIMIT ?
	`, blocked, limit)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrQueryFailed, err)
	}
	defer func() { _ = rows.Close() }()

	var domains []*DomainStats
	for rows.Next() {
		var d DomainStats
		if err := rows.Scan(&d.Domain, &d.QueryCount, &d.LastQueried, &d.FirstQueried, &d.Blocked); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrQueryFailed, err)
		}
		domains = append(domains, &d)
	}

	return domains, nil
}

// GetBlockedCount returns the number of blocked queries since a given time
func (s *SQLiteStorage) GetBlockedCount(ctx context.Context, since time.Time) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return 0, ErrClosed
	}

	var count int64
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM queries WHERE blocked = 1 AND timestamp >= ?
	`, since).Scan(&count)

	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrQueryFailed, err)
	}

	return count, nil
}

// GetQueryCount returns the total number of queries since a given time
func (s *SQLiteStorage) GetQueryCount(ctx context.Context, since time.Time) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return 0, ErrClosed
	}

	var count int64
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM queries WHERE timestamp >= ?
	`, since).Scan(&count)

	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrQueryFailed, err)
	}

	return count, nil
}

// Cleanup removes old queries based on retention policy
func (s *SQLiteStorage) Cleanup(ctx context.Context, olderThan time.Time) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return ErrClosed
	}

	// Delete old queries
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM queries WHERE timestamp < ?
	`, olderThan)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrQueryFailed, err)
	}

	rows, _ := result.RowsAffected()

	// Clean up domain stats for domains that no longer have queries
	_, err = s.db.ExecContext(ctx, `
		DELETE FROM domain_stats
		WHERE domain NOT IN (SELECT DISTINCT domain FROM queries)
	`)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrQueryFailed, err)
	}

	// VACUUM to reclaim space (only if significant deletions)
	if rows > 10000 {
		if _, err := s.db.ExecContext(ctx, "VACUUM"); err != nil {
			// Log but don't fail
			fmt.Printf("VACUUM failed: %v\n", err)
		}
	}

	return nil
}

// Close closes the storage backend
func (s *SQLiteStorage) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()

	// Close buffer channel
	close(s.buffer)

	// Wait for flush worker to complete
	s.wg.Wait()

	// Close prepared statements
	if s.stmtInsertQuery != nil {
		_ = s.stmtInsertQuery.Close()
	}

	// Close database
	return s.db.Close()
}

// Ping checks if the storage is reachable
func (s *SQLiteStorage) Ping(ctx context.Context) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return ErrClosed
	}

	return s.db.PingContext(ctx)
}

// Helper function to scan query logs from rows
func scanQueryLogs(rows *sql.Rows) ([]*QueryLog, error) {
	var queries []*QueryLog

	for rows.Next() {
		var q QueryLog
		var upstream sql.NullString

		err := rows.Scan(
			&q.ID,
			&q.Timestamp,
			&q.ClientIP,
			&q.Domain,
			&q.QueryType,
			&q.ResponseCode,
			&q.Blocked,
			&q.Cached,
			&q.ResponseTimeMs,
			&upstream,
		)
		if err != nil {
			return nil, err
		}

		if upstream.Valid {
			q.Upstream = upstream.String
		}

		queries = append(queries, &q)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return queries, nil
}
