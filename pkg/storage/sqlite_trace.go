package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// GetTraceStatistics returns aggregated trace statistics for blocked queries since the specified time.
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

	// Query all block traces
	rows, err := s.db.QueryContext(ctx, `
		SELECT block_trace
		FROM queries
		WHERE timestamp >= ? AND blocked = 1 AND block_trace IS NOT NULL
	`, since)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close() // Close error is non-critical since rows.Err() is checked at return
	}()

	// Aggregate trace data
	for rows.Next() {
		var traceJSON string
		if err := rows.Scan(&traceJSON); err != nil {
			continue
		}

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

	return stats, rows.Err()
}

// GetQueriesWithTraceFilter returns queries filtered by trace attributes.
func (s *SQLiteStorage) GetQueriesWithTraceFilter(ctx context.Context, filter TraceFilter, limit, offset int) ([]*QueryLog, error) {
	// Start with base query
	query := `
		SELECT id, timestamp, client_ip, domain, query_type, response_code,
		       blocked, cached, response_time_ms, upstream, block_trace
		FROM queries
		WHERE blocked = 1 AND block_trace IS NOT NULL
	`

	// We need to filter in application code since SQLite doesn't have native JSON query functions
	// in the version we're using. First, get all blocked queries with traces.
	rows, err := s.db.QueryContext(ctx, query+` ORDER BY timestamp DESC`)
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

		// Apply limit
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
