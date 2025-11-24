package api

import (
	"time"

	"glory-hole/pkg/storage"
)

// HealthResponse represents the health check response
type HealthResponse struct {
	Status  string `json:"status"`
	Uptime  string `json:"uptime"`
	Version string `json:"version"`
}

// LivenessResponse represents the liveness probe response
type LivenessResponse struct {
	Status string `json:"status"` // "alive"
}

// ReadinessResponse represents the readiness probe response
type ReadinessResponse struct {
	Checks map[string]string `json:"checks"`
	Status string            `json:"status"`
}

// StatsResponse represents query statistics
type StatsResponse struct {
	Period         string  `json:"period"`
	Timestamp      string  `json:"timestamp"`
	TotalQueries   int64   `json:"total_queries"`
	BlockedQueries int64   `json:"blocked_queries"`
	CachedQueries  int64   `json:"cached_queries"`
	BlockRate      float64 `json:"block_rate"`
	CacheHitRate   float64 `json:"cache_hit_rate"`
	AvgResponseMs  float64 `json:"avg_response_ms"`
}

// QueryResponse represents a single DNS query log entry
type QueryResponse struct {
	Timestamp      string                    `json:"timestamp"`
	ClientIP       string                    `json:"client_ip"`
	Domain         string                    `json:"domain"`
	QueryType      string                    `json:"query_type"`
	Upstream       string                    `json:"upstream,omitempty"`
	ID             int64                     `json:"id"`
	ResponseCode   int                       `json:"response_code"`
	ResponseTimeMs int64                     `json:"response_time_ms"`
	Blocked        bool                      `json:"blocked"`
	Cached         bool                      `json:"cached"`
	BlockTrace     []storage.BlockTraceEntry `json:"block_trace,omitempty"`
}

// QueriesResponse represents paginated query results
type QueriesResponse struct {
	Queries []QueryResponse `json:"queries"`
	Total   int             `json:"total"`
	Limit   int             `json:"limit"`
	Offset  int             `json:"offset"`
}

// DomainStatsResponse represents statistics for a single domain
type DomainStatsResponse struct {
	Domain  string `json:"domain"`
	Queries int64  `json:"queries"`
	Blocked bool   `json:"blocked"`
}

// TopDomainsResponse represents top queried domains
type TopDomainsResponse struct {
	Domains []DomainStatsResponse `json:"domains"`
	Limit   int                   `json:"limit"`
}

// BlocklistReloadResponse represents blocklist reload result
type BlocklistReloadResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	Domains int    `json:"domains"`
}

// CachePurgeResponse represents cache purge result
type CachePurgeResponse struct {
	Status         string `json:"status"`
	Message        string `json:"message"`
	EntriesCleared int    `json:"entries_cleared,omitempty"`
}

// ErrorResponse represents an API error
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
	Code    int    `json:"code"`
}

// TraceStatisticsResponse represents aggregated trace statistics
type TraceStatisticsResponse struct {
	Since        string           `json:"since"`
	Until        string           `json:"until"`
	TotalBlocked int64            `json:"total_blocked"`
	ByStage      map[string]int64 `json:"by_stage"`
	ByAction     map[string]int64 `json:"by_action"`
	ByRule       map[string]int64 `json:"by_rule"`
	BySource     map[string]int64 `json:"by_source"`
}

// convertQueryLog converts storage.QueryLog to QueryResponse
func convertQueryLog(q *storage.QueryLog) QueryResponse {
	return QueryResponse{
		ID:             q.ID,
		Timestamp:      q.Timestamp.Format(time.RFC3339),
		ClientIP:       q.ClientIP,
		Domain:         q.Domain,
		QueryType:      q.QueryType,
		ResponseCode:   q.ResponseCode,
		Blocked:        q.Blocked,
		Cached:         q.Cached,
		ResponseTimeMs: q.ResponseTimeMs,
		Upstream:       q.Upstream,
		BlockTrace:     q.BlockTrace,
	}
}

// convertDomainStats converts storage.DomainStats to DomainStatsResponse
func convertDomainStats(d *storage.DomainStats) DomainStatsResponse {
	return DomainStatsResponse{
		Domain:  d.Domain,
		Queries: d.QueryCount,
		Blocked: d.Blocked,
	}
}

// convertTraceStatistics converts storage.TraceStatistics to TraceStatisticsResponse
func convertTraceStatistics(t *storage.TraceStatistics) TraceStatisticsResponse {
	return TraceStatisticsResponse{
		Since:        t.Since.Format(time.RFC3339),
		Until:        t.Until.Format(time.RFC3339),
		TotalBlocked: t.TotalBlocked,
		ByStage:      t.ByStage,
		ByAction:     t.ByAction,
		ByRule:       t.ByRule,
		BySource:     t.BySource,
	}
}
