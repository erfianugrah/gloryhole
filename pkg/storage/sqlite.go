// Package storage contains the pluggable persistence layer; this file provides
// the SQLite implementation used for query logs and analytics.
package storage

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// MetricsRecorder defines the interface for recording storage metrics
// This interface breaks the import cycle between storage and telemetry packages
type MetricsRecorder interface {
	AddDroppedQuery(ctx context.Context, count int64)
}

//go:embed migrations/001_initial.sql
var initialSchema string

// SQLiteStorage implements the Storage interface using SQLite
type SQLiteStorage struct {
	db                   *sql.DB
	cfg                  *Config
	metrics              MetricsRecorder
	buffer               chan *QueryLog
	stmtInsertQuery      *sql.Stmt
	stmtGetRecentQueries *sql.Stmt
	stmtGetStatistics    *sql.Stmt
	wg                   sync.WaitGroup
	mu                   sync.RWMutex
	closed               bool
}

// NewSQLiteStorage creates a new SQLite storage backend
func NewSQLiteStorage(cfg *Config, metrics MetricsRecorder) (Storage, error) {
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
	if pingErr := db.Ping(); pingErr != nil {
		_ = db.Close()
		return nil, fmt.Errorf("%w: %v", ErrConnectionFailed, pingErr)
	}

	// Apply SQLite pragmas for performance
	pragmas := []string{
		fmt.Sprintf("PRAGMA busy_timeout = %d", cfg.SQLite.BusyTimeout),
		fmt.Sprintf("PRAGMA cache_size = %d", -cfg.SQLite.CacheSize), // Negative means KB
		"PRAGMA synchronous = NORMAL",                                // Balance between safety and performance
		"PRAGMA temp_store = MEMORY",                                 // Use memory for temp tables
	}

	if cfg.SQLite.MMapSize > 0 {
		pragmas = append(pragmas, fmt.Sprintf("PRAGMA mmap_size = %d", cfg.SQLite.MMapSize))
	}

	if cfg.SQLite.WALMode {
		pragmas = append(pragmas, "PRAGMA journal_mode = WAL")
	}

	for _, pragma := range pragmas {
		if _, pragmaErr := db.Exec(pragma); pragmaErr != nil {
			_ = db.Close()
			return nil, fmt.Errorf("failed to set pragma: %w", pragmaErr)
		}
	}

	// Apply migrations
	if migrationErr := applyMigrations(db); migrationErr != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to apply migrations: %w", migrationErr)
	}

	// Prepare statements
	stmtInsert, err := db.Prepare(`
		INSERT INTO queries
		(timestamp, client_ip, domain, query_type, response_code, blocked, cached, response_time_ms, upstream, upstream_time_ms, block_trace)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to prepare insert statement: %w", err)
	}

	storage := &SQLiteStorage{
		db:              db,
		cfg:             cfg,
		metrics:         metrics,
		buffer:          make(chan *QueryLog, cfg.BufferSize),
		stmtInsertQuery: stmtInsert,
	}

	// Start background flush worker
	storage.wg.Add(1)
	go storage.flushWorker()

	return storage, nil
}

// applyMigrations applies database schema migrations using the versioned migration system.
// This function delegates to runMigrations() which handles:
// - Detecting current database version
// - Applying only pending migrations in order
// - Recording each migration in schema_version table
// - Transactional safety (rollback on failure)
//
// New migrations should be added to migrations.go in the migrations registry.
// Each migration must have a unique version number and will be applied automatically
// when the database is opened.
//
// Migration files:
// - migrations.go: Migration registry and runner
// - migrations/001_initial.sql: Initial schema (embedded)
// - Future: Add new migration files and register in migrations.go
func applyMigrations(db *sql.DB) error {
	return runMigrations(db)
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
		// Buffer full, drop query and record metric
		if s.metrics != nil {
			s.metrics.AddDroppedQuery(ctx, 1)
		}
		return ErrBufferFull
	}
}

// flushWorker runs as a background goroutine that processes buffered DNS queries.
// It batches queries together for efficient database writes and flushes them either
// when the batch reaches cfg.BatchSize or when cfg.FlushInterval elapses.
//
// This worker ensures that query logging doesn't block DNS request processing:
// - Queries are received from s.buffer channel (async from DNS handler)
// - Batching reduces database write overhead (1 txn vs N txns)
// - Periodic flushes prevent queries from sitting in buffer too long
// - Graceful shutdown: flushes remaining queries when buffer closes
//
// The worker continues running until s.buffer is closed, at which point it
// flushes any remaining queries and exits.
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
			slog.Default().Error("Failed to flush query batch",
				"error", err,
				"batch_size", len(batch),
			)
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

// flushBatch writes a batch of queries to the database in a single transaction.
// This method is called by flushWorker and performs the actual database writes
// for accumulated queries. Using transactions significantly improves write
// performance (~50-100x faster than individual INSERTs).
//
// Performance characteristics:
// - Single transaction for entire batch (atomicity + speed)
// - Prepared statements reused for each query
// - Domain statistics updated asynchronously to avoid blocking
//
// Error handling:
// - Returns error if transaction fails (logged by caller)
// - Domain stats failures are logged but don't fail the batch
// - Transaction automatically rolled back on error (defer)
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
		traceValue, encodeErr := encodeBlockTrace(query.BlockTrace)
		if encodeErr != nil {
			return fmt.Errorf("%w: %v", ErrQueryFailed, encodeErr)
		}

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
			query.UpstreamTimeMs,
			traceValue,
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

// updateDomainStats updates the domain_stats table with aggregated statistics.
// This method maintains per-domain counters and timestamps for analytics purposes.
// It's called asynchronously from flushBatch to avoid blocking query inserts.
//
// The domain_stats table tracks:
// - query_count: Total queries for this domain
// - first_queried: Timestamp of first query (never updated)
// - last_queried: Timestamp of most recent query
// - blocked: Whether domain is in blocklist
//
// Uses UPSERT (INSERT ... ON CONFLICT) for efficient updates:
// - New domains: INSERT with initial values
// - Existing domains: Increment counter and update last_queried
//
// Errors are logged but don't propagate (non-critical data).
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
			slog.Default().Error("Failed to update domain statistics",
				"error", err,
				"domain", query.Domain,
			)
		}
	}
}

// GetRecentQueries returns the most recent queries with pagination support
func (s *SQLiteStorage) GetRecentQueries(ctx context.Context, limit, offset int) ([]*QueryLog, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrClosed
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, timestamp, client_ip, domain, query_type, response_code,
		       blocked, cached, response_time_ms, upstream, upstream_time_ms, block_trace
		FROM queries
		ORDER BY timestamp DESC
		LIMIT ? OFFSET ?
	`, limit, offset)
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
		       blocked, cached, response_time_ms, upstream, upstream_time_ms, block_trace
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
		       blocked, cached, response_time_ms, upstream, upstream_time_ms, block_trace
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

	blockedValue := 0
	if blocked {
		blockedValue = 1
	}

	rows, err := s.db.QueryContext(ctx, `
		WITH aggregated AS (
			SELECT
				domain,
				MIN(id) AS first_id,
				MAX(id) AS last_id,
				COUNT(*) AS total_queries
			FROM queries
			WHERE blocked = ?
			GROUP BY domain
		)
		SELECT
			a.domain,
			a.total_queries,
			first_q.timestamp AS first_seen_raw,
			last_q.timestamp AS last_seen_raw
		FROM aggregated a
		LEFT JOIN queries first_q ON first_q.id = a.first_id
		LEFT JOIN queries last_q ON last_q.id = a.last_id
		ORDER BY a.total_queries DESC
		LIMIT ?
	`, blockedValue, limit)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrQueryFailed, err)
	}
	defer func() { _ = rows.Close() }()

	var domains []*DomainStats
	for rows.Next() {
		var d DomainStats
		var lastRaw, firstRaw sql.NullString
		if err := rows.Scan(&d.Domain, &d.QueryCount, &firstRaw, &lastRaw); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrQueryFailed, err)
		}
		if firstRaw.Valid {
			d.FirstQueried = parseSQLiteTime(firstRaw.String)
		}
		if lastRaw.Valid {
			d.LastQueried = parseSQLiteTime(lastRaw.String)
		}
		d.Blocked = blocked
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

// GetTimeSeriesStats returns aggregated statistics grouped by the specified bucket duration.
func (s *SQLiteStorage) GetTimeSeriesStats(ctx context.Context, bucket time.Duration, points int) ([]*TimeSeriesPoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrClosed
	}

	if points <= 0 {
		return nil, fmt.Errorf("points must be greater than zero")
	}

	bucketSeconds := int64(bucket / time.Second)
	if bucketSeconds <= 0 {
		return nil, fmt.Errorf("bucket duration must be at least 1 second")
	}

	alignedEnd := truncateToBucket(time.Now().UTC(), bucket)
	start := alignedEnd.Add(-bucket * time.Duration(points-1))

	rows, err := s.db.QueryContext(ctx, `
		WITH bucketed AS (
			SELECT
				datetime((strftime('%s', replace(substr(timestamp, 1, 19), 'T', ' ')) / ?) * ?, 'unixepoch') AS bucket_start,
				blocked,
				cached,
				response_time_ms
			FROM queries
			WHERE timestamp >= ?
		)
		SELECT
			bucket_start,
			COUNT(*) as total,
			SUM(CASE WHEN blocked THEN 1 ELSE 0 END) as blocked,
			SUM(CASE WHEN cached THEN 1 ELSE 0 END) as cached,
			AVG(response_time_ms) as avg_response_time
		FROM bucketed
		GROUP BY bucket_start
		ORDER BY bucket_start ASC
	`, bucketSeconds, bucketSeconds, start)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrQueryFailed, err)
	}
	defer func() { _ = rows.Close() }()

	bucketLayout := "2006-01-02 15:04:05"
	pointsByBucket := make(map[int64]*TimeSeriesPoint)

	for rows.Next() {
		var (
			bucketStr string
			total     sql.NullInt64
			blocked   sql.NullInt64
			cached    sql.NullInt64
			avg       sql.NullFloat64
		)

		if err := rows.Scan(&bucketStr, &total, &blocked, &cached, &avg); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrQueryFailed, err)
		}

		bucketTime, parseErr := time.ParseInLocation(bucketLayout, bucketStr, time.UTC)
		if parseErr != nil {
			return nil, fmt.Errorf("failed to parse bucket timestamp: %w", parseErr)
		}

		point := &TimeSeriesPoint{
			Timestamp:         bucketTime,
			TotalQueries:      total.Int64,
			BlockedQueries:    blocked.Int64,
			CachedQueries:     cached.Int64,
			AvgResponseTimeMs: avg.Float64,
		}

		pointsByBucket[bucketTime.Unix()] = point
	}

	result := make([]*TimeSeriesPoint, 0, points)
	current := start
	for i := 0; i < points; i++ {
		if point, ok := pointsByBucket[current.Unix()]; ok {
			result = append(result, point)
		} else {
			result = append(result, &TimeSeriesPoint{
				Timestamp:         current,
				TotalQueries:      0,
				BlockedQueries:    0,
				CachedQueries:     0,
				AvgResponseTimeMs: 0,
			})
		}
		current = current.Add(bucket)
	}

	return result, nil
}

// GetQueryTypeStats returns aggregated counts grouped by DNS query type.
// If since is non-zero, only queries newer than or equal to that timestamp are considered.
func (s *SQLiteStorage) GetQueryTypeStats(ctx context.Context, limit int, since time.Time) ([]*QueryTypeStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrClosed
	}

	if limit <= 0 || limit > 100 {
		limit = 10
	}

	query := `
		SELECT
			COALESCE(NULLIF(UPPER(query_type), ''), 'UNKNOWN') AS type,
			COUNT(*) AS total,
			SUM(CASE WHEN blocked THEN 1 ELSE 0 END) AS blocked,
			SUM(CASE WHEN cached THEN 1 ELSE 0 END) AS cached
		FROM queries
	`

	args := make([]any, 0, 2)
	if !since.IsZero() {
		query += " WHERE timestamp >= ?"
		args = append(args, since.UTC())
	}

	query += `
		GROUP BY type
		ORDER BY total DESC
		LIMIT ?
	`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrQueryFailed, err)
	}
	defer func() { _ = rows.Close() }()

	var stats []*QueryTypeStats
	for rows.Next() {
		var stat QueryTypeStats
		if scanErr := rows.Scan(&stat.QueryType, &stat.Total, &stat.Blocked, &stat.Cached); scanErr != nil {
			return nil, fmt.Errorf("%w: %v", ErrQueryFailed, scanErr)
		}
		stats = append(stats, &stat)
	}

	return stats, nil
}

// GetQueriesFiltered returns queries matching the provided filter criteria.
func (s *SQLiteStorage) GetQueriesFiltered(ctx context.Context, filter QueryFilter, limit, offset int) ([]*QueryLog, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrClosed
	}

	query := `
		SELECT id, timestamp, client_ip, domain, query_type, response_code,
		       blocked, cached, response_time_ms, upstream, upstream_time_ms, block_trace
		FROM queries
	`
	conditions := make([]string, 0)
	args := make([]any, 0)

	if filter.Domain != "" {
		conditions = append(conditions, "LOWER(domain) LIKE ?")
		args = append(args, "%"+strings.ToLower(filter.Domain)+"%")
	}

	if filter.QueryType != "" {
		conditions = append(conditions, "UPPER(query_type) = ?")
		args = append(args, strings.ToUpper(filter.QueryType))
	}

	if filter.Blocked != nil {
		conditions = append(conditions, "blocked = ?")
		if *filter.Blocked {
			args = append(args, 1)
		} else {
			args = append(args, 0)
		}
	}

	if filter.Cached != nil {
		conditions = append(conditions, "cached = ?")
		if *filter.Cached {
			args = append(args, 1)
		} else {
			args = append(args, 0)
		}
	}

	if !filter.Start.IsZero() {
		conditions = append(conditions, "timestamp >= ?")
		args = append(args, filter.Start)
	}

	if !filter.End.IsZero() {
		conditions = append(conditions, "timestamp <= ?")
		args = append(args, filter.End)
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	query += `
		ORDER BY timestamp DESC
		LIMIT ? OFFSET ?
	`
	args = append(args, limit, offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrQueryFailed, err)
	}
	defer func() { _ = rows.Close() }()

	return scanQueryLogs(rows)
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
			slog.Default().Error("VACUUM operation failed",
				"error", err,
				"deleted_rows", rows,
			)
		}
	}

	return nil
}

// Reset wipes all stored query, statistics, and client metadata.
// Intended for troubleshooting or when the operator wants to start fresh.
func (s *SQLiteStorage) Reset(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrClosed
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("reset begin transaction failed: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	tables := []string{
		"queries",
		"statistics",
		"domain_stats",
		"client_profiles",
		"client_groups",
	}

	for _, table := range tables {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table); err != nil {
			return fmt.Errorf("reset failed clearing %s: %w", table, err)
		}
	}

	// Reset AUTOINCREMENT sequences so IDs start from 1 again.
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM sqlite_sequence WHERE name IN ('queries','statistics','domain_stats')
	`); err != nil {
		return fmt.Errorf("reset failed clearing sqlite_sequence: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("reset commit failed: %w", err)
	}

	if _, err := s.db.ExecContext(ctx, "VACUUM"); err != nil {
		slog.Default().Warn("SQLite vacuum after reset failed", "error", err)
	}

	return nil
}

func truncateToBucket(t time.Time, bucket time.Duration) time.Time {
	bucketSeconds := int64(bucket / time.Second)
	if bucketSeconds <= 0 {
		return t.UTC()
	}
	unix := t.Unix()
	truncated := (unix / bucketSeconds) * bucketSeconds
	return time.Unix(truncated, 0).UTC()
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

func encodeBlockTrace(entries []BlockTraceEntry) (interface{}, error) {
	if len(entries) == 0 {
		return nil, nil
	}

	data, err := json.Marshal(entries)
	if err != nil {
		return nil, err
	}

	return string(data), nil
}

func decodeBlockTrace(raw sql.NullString) ([]BlockTraceEntry, error) {
	if !raw.Valid || raw.String == "" {
		return nil, nil
	}

	var entries []BlockTraceEntry
	if err := json.Unmarshal([]byte(raw.String), &entries); err != nil {
		return nil, err
	}

	return entries, nil
}

// scanQueryLogs is a helper function that scans SQL rows into QueryLog structs.
// It's used by multiple query methods (GetRecentQueries, GetQueriesByDomain, etc.)
// to avoid code duplication in row scanning logic.
//
// The function handles:
// - Iterating through all rows
// - Scanning each column into QueryLog fields
// - NULL handling for optional fields (e.g., upstream)
// - Collecting all queries into a slice
//
// Returns an error if any row scan fails. The caller is responsible for
// closing the rows object.
func scanQueryLogs(rows *sql.Rows) ([]*QueryLog, error) {
	var queries []*QueryLog

	for rows.Next() {
		var q QueryLog
		var upstream sql.NullString
		var trace sql.NullString

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
			&q.UpstreamTimeMs,
			&trace,
		)
		if err != nil {
			return nil, err
		}

		if upstream.Valid {
			q.Upstream = upstream.String
		}

		entries, err := decodeBlockTrace(trace)
		if err != nil {
			return nil, err
		}
		q.BlockTrace = entries

		queries = append(queries, &q)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return queries, nil
}
