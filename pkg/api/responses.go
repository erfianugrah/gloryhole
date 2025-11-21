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
	Status string            `json:"status"` // "ready" or "not_ready"
	Checks map[string]string `json:"checks"` // Component health status
}

// StatsResponse represents query statistics
type StatsResponse struct {
	TotalQueries   int64   `json:"total_queries"`
	BlockedQueries int64   `json:"blocked_queries"`
	CachedQueries  int64   `json:"cached_queries"`
	BlockRate      float64 `json:"block_rate"`      // Percentage
	CacheHitRate   float64 `json:"cache_hit_rate"`  // Percentage
	AvgResponseMs  float64 `json:"avg_response_ms"` // Average response time
	Period         string  `json:"period"`          // Time period for stats
	Timestamp      string  `json:"timestamp"`       // ISO 8601 format
}

// QueryResponse represents a single DNS query log entry
type QueryResponse struct {
	ID             int64  `json:"id"`
	Timestamp      string `json:"timestamp"` // ISO 8601 format
	ClientIP       string `json:"client_ip"`
	Domain         string `json:"domain"`
	QueryType      string `json:"query_type"`
	ResponseCode   int    `json:"response_code"`
	Blocked        bool   `json:"blocked"`
	Cached         bool   `json:"cached"`
	ResponseTimeMs int64  `json:"response_time_ms"`
	Upstream       string `json:"upstream,omitempty"`
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
	Domains int    `json:"domains"`
	Message string `json:"message,omitempty"`
}

// ErrorResponse represents an API error
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    int    `json:"code"`
	Message string `json:"message,omitempty"`
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
