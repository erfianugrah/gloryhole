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
	db                  *sql.DB
	cfg                 *Config
	metrics             MetricsRecorder
	buffer              chan *QueryLog
	domainStatsCh       chan []*QueryLog // Channel for domain stats updates (avoids goroutine per batch)
	stmtInsertQuery     *sql.Stmt
	wg                  sync.WaitGroup
	mu                  sync.RWMutex
	closed              bool
	bufferHighWatermark int  // 80% of buffer capacity
	warningLogged       bool // Track if high watermark warning has been logged
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
		db:                  db,
		cfg:                 cfg,
		metrics:             metrics,
		buffer:              make(chan *QueryLog, cfg.BufferSize),
		domainStatsCh:       make(chan []*QueryLog, 100), // Buffer for domain stats batches
		stmtInsertQuery:     stmtInsert,
		bufferHighWatermark: int(float64(cfg.BufferSize) * 0.8), // 80% threshold
	}

	// Start background flush worker
	storage.wg.Add(1)
	go storage.flushWorker()

	// Start domain stats worker (avoids spawning goroutine per batch)
	storage.wg.Add(1)
	go storage.domainStatsWorker()

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

	// Check buffer utilization and warn if high
	currentSize := len(s.buffer)
	if currentSize > s.bufferHighWatermark && !s.warningLogged {
		utilization := float64(currentSize) / float64(cap(s.buffer)) * 100
		slog.Default().Warn("Query buffer high watermark exceeded",
			"current", currentSize,
			"capacity", cap(s.buffer),
			"utilization_pct", fmt.Sprintf("%.1f", utilization))
		s.warningLogged = true
	} else if currentSize < s.bufferHighWatermark/2 && s.warningLogged {
		// Reset warning flag when buffer drains below 40%
		s.warningLogged = false
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
		slog.Default().Error("Query buffer full - dropping entry",
			"domain", query.Domain,
			"client_ip", query.ClientIP)
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

		startTime := time.Now()
		batchSize := len(batch)

		if err := s.flushBatch(batch); err != nil {
			// Log error but continue (we don't want to crash the server)
			slog.Default().Error("Failed to flush query batch",
				"error", err,
				"batch_size", batchSize,
			)
		} else {
			flushDuration := time.Since(startTime)

			// Log successful flush with timing
			slog.Default().Debug("Flushed query batch",
				"batch_size", batchSize,
				"duration_ms", flushDuration.Milliseconds())

			// Alert if flush taking too long (>100ms is slow for batch writes)
			if flushDuration > 100*time.Millisecond {
				slog.Default().Warn("Slow batch flush detected",
					"batch_size", batchSize,
					"duration_ms", flushDuration.Milliseconds())
			}
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
			FormatTimestamp(query.Timestamp),
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

	// Send to domain stats worker instead of spawning goroutine
	// Make a copy to avoid data race with batch slice reuse
	queriesCopy := make([]*QueryLog, len(queries))
	copy(queriesCopy, queries)

	// Check if closed before sending to avoid panic on closed channel.
	// Hold the RLock during the non-blocking send to prevent Close() from
	// closing the channel between our check and send (fixes data race).
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		// Storage closing, update synchronously if needed
		s.updateDomainStats(queriesCopy)
	} else {
		// Non-blocking send to worker (safe: RLock held, channel can't be closed)
		select {
		case s.domainStatsCh <- queriesCopy:
			// Sent to worker
		default:
			// Channel full, update synchronously (rare)
			s.mu.RUnlock()
			s.updateDomainStats(queriesCopy)
			return nil
		}
		s.mu.RUnlock()
	}

	return nil
}

// domainStatsWorker processes domain stats updates from a channel.
// This avoids spawning a goroutine per batch and provides controlled concurrency.
func (s *SQLiteStorage) domainStatsWorker() {
	defer s.wg.Done()

	for batch := range s.domainStatsCh {
		s.updateDomainStats(batch)
	}
}

// domainStatUpdate tracks aggregated stats for a single domain in a batch.
type domainStatUpdate struct {
	count       int
	lastQueried time.Time
	blocked     bool
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
// Optimization: Groups queries by domain first to reduce SQL statements.
// Uses UPSERT (INSERT ... ON CONFLICT) for efficient updates.
// Errors are logged but don't propagate (non-critical data).
func (s *SQLiteStorage) updateDomainStats(queries []*QueryLog) {
	if len(queries) == 0 {
		return
	}

	// Group queries by domain to reduce SQL statements
	updates := make(map[string]*domainStatUpdate)
	for _, query := range queries {
		if existing, ok := updates[query.Domain]; ok {
			existing.count++
			if query.Timestamp.After(existing.lastQueried) {
				existing.lastQueried = query.Timestamp
			}
		} else {
			updates[query.Domain] = &domainStatUpdate{
				count:       1,
				lastQueried: query.Timestamp,
				blocked:     query.Blocked,
			}
		}
	}

	// Execute grouped updates in a transaction for efficiency
	tx, err := s.db.Begin()
	if err != nil {
		slog.Default().Error("Failed to begin domain stats transaction", "error", err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(`
		INSERT INTO domain_stats (domain, query_count, last_queried, first_queried, blocked)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(domain) DO UPDATE SET
			query_count = query_count + excluded.query_count,
			last_queried = MAX(last_queried, excluded.last_queried)
	`)
	if err != nil {
		slog.Default().Error("Failed to prepare domain stats statement", "error", err)
		return
	}
	defer func() { _ = stmt.Close() }()

	for domain, update := range updates {
		_, err = stmt.Exec(domain, update.count, update.lastQueried, update.lastQueried, update.blocked)
		if err != nil {
			slog.Default().Error("Failed to update domain statistics",
				"error", err,
				"domain", domain,
			)
		}
	}

	if err = tx.Commit(); err != nil {
		slog.Default().Error("Failed to commit domain stats transaction", "error", err)
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

// FormatTimestamp converts a time.Time to RFC3339Nano format for SQLite compatibility
func FormatTimestamp(t time.Time) string {
	return t.Format(time.RFC3339Nano)
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
	`, FormatTimestamp(since)).Scan(
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
func (s *SQLiteStorage) GetTopDomains(ctx context.Context, limit int, blocked bool, since time.Time) ([]*DomainStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrClosed
	}

	blockedValue := 0
	if blocked {
		blockedValue = 1
	}

	// Build query with optional time filter
	query := `
		WITH aggregated AS (
			SELECT
				domain,
				MIN(id) AS first_id,
				MAX(id) AS last_id,
				COUNT(*) AS total_queries
			FROM queries
			WHERE blocked = ?`

	args := []interface{}{blockedValue}

	// Add time filter if since is not zero
	if !since.IsZero() {
		query += ` AND timestamp >= ?`
		args = append(args, FormatTimestamp(since))
	}

	query += `
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
		LIMIT ?`

	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
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
	`, FormatTimestamp(since)).Scan(&count)

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
	`, FormatTimestamp(since)).Scan(&count)

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
				strftime('%Y-%m-%d %H:%M:%S', datetime((strftime('%s', timestamp) / ?) * ?, 'unixepoch')) AS bucket_start,
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
	`, bucketSeconds, bucketSeconds, FormatTimestamp(start))
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
		args = append(args, FormatTimestamp(since.UTC()))
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

	if filter.ClientIP != "" {
		conditions = append(conditions, "client_ip = ?")
		args = append(args, filter.ClientIP)
	}

	if filter.Upstream != "" {
		conditions = append(conditions, "LOWER(upstream) LIKE ?")
		args = append(args, "%"+strings.ToLower(filter.Upstream)+"%")
	}

	if filter.ResponseCode > 0 {
		conditions = append(conditions, "response_code = ?")
		args = append(args, filter.ResponseCode)
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
		args = append(args, FormatTimestamp(filter.Start))
	}

	if !filter.End.IsZero() {
		conditions = append(conditions, "timestamp <= ?")
		args = append(args, FormatTimestamp(filter.End))
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

// BufferStats represents storage buffer statistics
type BufferStats struct {
	Size        int     `json:"size"`        // Current buffer size
	Capacity    int     `json:"capacity"`    // Maximum capacity
	Utilization float64 `json:"utilization"` // Percentage (0-100)
	HighWater   int     `json:"high_water"`  // High watermark threshold
}

// GetBufferStats returns current buffer statistics
func (s *SQLiteStorage) GetBufferStats() BufferStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	size := len(s.buffer)
	capacity := cap(s.buffer)
	utilization := 0.0
	if capacity > 0 {
		utilization = float64(size) / float64(capacity) * 100
	}

	return BufferStats{
		Size:        size,
		Capacity:    capacity,
		Utilization: utilization,
		HighWater:   s.bufferHighWatermark,
	}
}

// Close closes the storage backend
func (s *SQLiteStorage) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	// Close domainStatsCh while holding the lock to prevent race with flushBatch
	// which checks s.closed and sends to the channel under RLock
	close(s.domainStatsCh)
	s.mu.Unlock()

	// Log buffer stats before closing
	stats := s.GetBufferStats()
	slog.Default().Info("Closing storage",
		"buffer_size", stats.Size,
		"buffer_capacity", stats.Capacity,
		"buffer_utilization_pct", fmt.Sprintf("%.1f", stats.Utilization))

	// Close buffer channel (flush worker will drain and exit)
	close(s.buffer)

	// Wait for workers to complete
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
