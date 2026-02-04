package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// GetTraceStatistics returns aggregated trace statistics for blocked queries since the specified time.
// Uses batched processing to prevent memory exhaustion at scale.
func (s *SQLiteStorage) GetTraceStatistics(ctx context.Context, since time.Time) (*TraceStatistics, error) {
	stats := &TraceStatistics{
		Since:    since,
		Until:    time.Now(),
		ByStage:  make(map[string]int64),
		ByAction: make(map[string]int64),
		ByRule:   make(map[string]int64),
		BySource: make(map[string]int64),
	}

	// Get total blocked queries
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM queries
		WHERE timestamp >= ? AND blocked = 1 AND block_trace IS NOT NULL
	`, since).Scan(&stats.TotalBlocked)
	if err != nil {
		return nil, err
	}

	// Process traces in batches to prevent memory exhaustion at scale
	const batchSize = 1000
	var lastID int64 = 0

	for {
		// Check context cancellation between batches
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		// Query batch of block traces using cursor-based pagination
		rows, err := s.db.QueryContext(ctx, `
			SELECT id, block_trace
			FROM queries
			WHERE timestamp >= ? AND blocked = 1 AND block_trace IS NOT NULL AND id > ?
			ORDER BY id ASC
			LIMIT ?
		`, since, lastID, batchSize)
		if err != nil {
			return nil, err
		}

		var rowsProcessed int
		for rows.Next() {
			var id int64
			var traceJSON string
			if err := rows.Scan(&id, &traceJSON); err != nil {
				continue
			}
			lastID = id
			rowsProcessed++

			var traces []BlockTraceEntry
			if err := json.Unmarshal([]byte(traceJSON), &traces); err != nil {
				continue
			}

			// Aggregate by stage, action, rule, source
			for _, trace := range traces {
				if trace.Stage != "" {
					stats.ByStage[trace.Stage]++
				}
				if trace.Action != "" {
					stats.ByAction[trace.Action]++
				}
				if trace.Rule != "" {
					stats.ByRule[trace.Rule]++
				}
				if trace.Source != "" {
					stats.BySource[trace.Source]++
				}
			}
		}

		closeErr := rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}

		// Exit if we processed fewer rows than batch size (last batch)
		if rowsProcessed < batchSize {
			break
		}
	}

	return stats, nil
}

// GetQueriesWithTraceFilter returns queries filtered by trace attributes.
// Uses a bounded scan with early termination to prevent memory exhaustion at scale.
func (s *SQLiteStorage) GetQueriesWithTraceFilter(ctx context.Context, filter TraceFilter, limit, offset int) ([]*QueryLog, error) {
	// Maximum rows to scan to prevent memory exhaustion at high scale.
	// We scan more than limit+offset to account for filtered-out rows.
	// If filters are very selective, caller may need to adjust their query.
	const maxScanRows = 10000

	// Start with base query
	query := `
		SELECT id, timestamp, client_ip, domain, query_type, response_code,
		       blocked, cached, response_time_ms, upstream, upstream_time_ms, block_trace
		FROM queries
		WHERE blocked = 1 AND block_trace IS NOT NULL
		ORDER BY timestamp DESC
		LIMIT ?
	`

	// We need to filter in application code since SQLite doesn't have native JSON query functions
	// in the version we're using. Limit the scan to prevent unbounded memory usage.
	rows, err := s.db.QueryContext(ctx, query, maxScanRows)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close() // Close error is non-critical since rows.Err() is checked at return
	}()

	var queries []*QueryLog
	var skipped int

	for rows.Next() {
		var q QueryLog
		var upstream sql.NullString
		var traceJSON sql.NullString

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
			&traceJSON,
		)
		if err != nil {
			continue
		}

		if upstream.Valid {
			q.Upstream = upstream.String
		}

		// Decode trace
		var traces []BlockTraceEntry
		if traceJSON.Valid && traceJSON.String != "" {
			if err := json.Unmarshal([]byte(traceJSON.String), &traces); err != nil {
				continue
			}
		}
		q.BlockTrace = traces

		// Apply filters
		if !matchesFilter(traces, filter) {
			continue
		}

		// Apply offset
		if skipped < offset {
			skipped++
			continue
		}

		queries = append(queries, &q)

		// Apply limit - early termination once we have enough results
		if len(queries) >= limit {
			break
		}
	}

	return queries, rows.Err()
}

// matchesFilter checks if any trace entry matches the given filter.
func matchesFilter(traces []BlockTraceEntry, filter TraceFilter) bool {
	// If all filter fields are empty, match everything
	if filter.Stage == "" && filter.Action == "" && filter.Rule == "" && filter.Source == "" {
		return true
	}

	for _, trace := range traces {
		stageMatch := filter.Stage == "" || trace.Stage == filter.Stage
		actionMatch := filter.Action == "" || trace.Action == filter.Action
		ruleMatch := filter.Rule == "" || trace.Rule == filter.Rule
		sourceMatch := filter.Source == "" || trace.Source == filter.Source

		if stageMatch && actionMatch && ruleMatch && sourceMatch {
			return true
		}
	}

	return false
}
