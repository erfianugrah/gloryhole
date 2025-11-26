package api

import (
	"time"

	"glory-hole/pkg/config"
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
	CPUUsagePercent    float64 `json:"cpu_usage_percent,omitempty"`
	MemoryUsageBytes   uint64  `json:"memory_usage_bytes,omitempty"`
	MemoryTotalBytes   uint64  `json:"memory_total_bytes,omitempty"`
	MemoryUsagePercent float64 `json:"memory_usage_percent,omitempty"`
	TemperatureCelsius float64 `json:"temperature_celsius,omitempty"`
	TemperatureAvailable bool  `json:"temperature_available,omitempty"`
}

// TimeSeriesResponse represents time-series statistics data
type TimeSeriesResponse struct {
	Period string                    `json:"period"`
	Points int                       `json:"points"`
	Data   []TimeSeriesPointResponse `json:"data"`
}

// TimeSeriesPointResponse represents a single aggregated bucket
type TimeSeriesPointResponse struct {
	Timestamp      string  `json:"timestamp"`
	TotalQueries   int64   `json:"total_queries"`
	BlockedQueries int64   `json:"blocked_queries"`
	CachedQueries  int64   `json:"cached_queries"`
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
	ResponseTimeMs float64                   `json:"response_time_ms"`
	UpstreamTimeMs float64                   `json:"upstream_response_ms"`
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

// QueryTypeStatsResponse represents aggregated counts per record type.
type QueryTypeStatsResponse struct {
	Limit int                     `json:"limit"`
	Types []QueryTypeStatResponse `json:"types"`
}

type QueryTypeStatResponse struct {
	QueryType string `json:"query_type"`
	Total     int64  `json:"total"`
	Blocked   int64  `json:"blocked"`
	Cached    int64  `json:"cached"`
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

// ConfigUpdateResponse represents a successful configuration mutation response.
type ConfigUpdateResponse struct {
	Status  string         `json:"status"`
	Message string         `json:"message"`
	Config  ConfigResponse `json:"config"`
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
		UpstreamTimeMs: q.UpstreamTimeMs,
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

func convertQueryTypeStats(items []*storage.QueryTypeStats, limit int) QueryTypeStatsResponse {
	resp := QueryTypeStatsResponse{
		Limit: limit,
		Types: make([]QueryTypeStatResponse, 0, len(items)),
	}
	for _, item := range items {
		resp.Types = append(resp.Types, QueryTypeStatResponse{
			QueryType: item.QueryType,
			Total:     item.Total,
			Blocked:   item.Blocked,
			Cached:    item.Cached,
		})
	}
	return resp
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

func convertTimeSeriesPoints(points []*storage.TimeSeriesPoint) []TimeSeriesPointResponse {
	result := make([]TimeSeriesPointResponse, 0, len(points))
	for _, p := range points {
		result = append(result, TimeSeriesPointResponse{
			Timestamp:      p.Timestamp.Format(time.RFC3339),
			TotalQueries:   p.TotalQueries,
			BlockedQueries: p.BlockedQueries,
			CachedQueries:  p.CachedQueries,
			AvgResponseMs:  p.AvgResponseTimeMs,
		})
	}
	return result
}

// ConfigResponse represents the public configuration payload used by the Settings UI
type ConfigResponse struct {
	Server               ConfigServerResponse    `json:"server"`
	Cache                ConfigCacheResponse     `json:"cache"`
	Policy               ConfigPolicyResponse    `json:"policy"`
	RateLimit            ConfigRateLimitResponse `json:"rate_limit"`
	Storage              ConfigStorageResponse   `json:"storage"`
	Logging              config.LoggingConfig    `json:"logging"`
	Telemetry            config.TelemetryConfig  `json:"telemetry"`
	UpstreamDNSServers   []string                `json:"upstream_dns_servers"`
	Blocklists           []string                `json:"blocklists"`
	Whitelist            []string                `json:"whitelist"`
	AutoUpdateBlocklists bool                    `json:"auto_update_blocklists"`
	UpdateInterval       string                  `json:"update_interval"`
}

type ConfigServerResponse struct {
	ListenAddress   string `json:"listen_address"`
	WebUIAddress    string `json:"web_ui_address"`
	TCPEnabled      bool   `json:"tcp_enabled"`
	UDPEnabled      bool   `json:"udp_enabled"`
	EnableBlocklist bool   `json:"enable_blocklist"`
	EnablePolicies  bool   `json:"enable_policies"`
	DecisionTrace   bool   `json:"decision_trace"`
}

type ConfigCacheResponse struct {
	Enabled     bool   `json:"enabled"`
	MaxEntries  int    `json:"max_entries"`
	MinTTL      string `json:"min_ttl"`
	MaxTTL      string `json:"max_ttl"`
	NegativeTTL string `json:"negative_ttl"`
	BlockedTTL  string `json:"blocked_ttl"`
	ShardCount  int    `json:"shard_count"`
}

type ConfigPolicyResponse struct {
	Enabled bool                     `json:"enabled"`
	Rules   []config.PolicyRuleEntry `json:"rules"`
}

type ConfigRateLimitResponse struct {
	Enabled           bool                       `json:"enabled"`
	RequestsPerSecond float64                    `json:"requests_per_second"`
	Burst             int                        `json:"burst"`
	Action            string                     `json:"on_exceed"`
	LogViolations     bool                       `json:"log_violations"`
	CleanupInterval   string                     `json:"cleanup_interval"`
	MaxTrackedClients int                        `json:"max_tracked_clients"`
	Overrides         []config.RateLimitOverride `json:"overrides"`
}

type ConfigStorageResponse struct {
	Backend       string `json:"backend"`
	BufferSize    int    `json:"buffer_size"`
	RetentionDays int    `json:"retention_days"`
}

func convertConfigResponse(cfg *config.Config) ConfigResponse {
	return ConfigResponse{
		Server: ConfigServerResponse{
			ListenAddress:   cfg.Server.ListenAddress,
			WebUIAddress:    cfg.Server.WebUIAddress,
			TCPEnabled:      cfg.Server.TCPEnabled,
			UDPEnabled:      cfg.Server.UDPEnabled,
			EnableBlocklist: cfg.Server.EnableBlocklist,
			EnablePolicies:  cfg.Server.EnablePolicies,
			DecisionTrace:   cfg.Server.DecisionTrace,
		},
		Cache: ConfigCacheResponse{
			Enabled:     cfg.Cache.Enabled,
			MaxEntries:  cfg.Cache.MaxEntries,
			MinTTL:      durationToString(cfg.Cache.MinTTL),
			MaxTTL:      durationToString(cfg.Cache.MaxTTL),
			NegativeTTL: durationToString(cfg.Cache.NegativeTTL),
			BlockedTTL:  durationToString(cfg.Cache.BlockedTTL),
			ShardCount:  cfg.Cache.ShardCount,
		},
		Policy: ConfigPolicyResponse{
			Enabled: cfg.Policy.Enabled,
			Rules:   cfg.Policy.Rules,
		},
		RateLimit: ConfigRateLimitResponse{
			Enabled:           cfg.RateLimit.Enabled,
			RequestsPerSecond: cfg.RateLimit.RequestsPerSecond,
			Burst:             cfg.RateLimit.Burst,
			Action:            string(cfg.RateLimit.Action),
			LogViolations:     cfg.RateLimit.LogViolations,
			CleanupInterval:   durationToString(cfg.RateLimit.CleanupInterval),
			MaxTrackedClients: cfg.RateLimit.MaxTrackedClients,
			Overrides:         cfg.RateLimit.Overrides,
		},
		Storage: ConfigStorageResponse{
			Backend:       string(cfg.Database.Backend),
			BufferSize:    cfg.Database.BufferSize,
			RetentionDays: cfg.Database.RetentionDays,
		},
		Logging:              cfg.Logging,
		Telemetry:            cfg.Telemetry,
		UpstreamDNSServers:   cfg.UpstreamDNSServers,
		Blocklists:           cfg.Blocklists,
		Whitelist:            cfg.Whitelist,
		AutoUpdateBlocklists: cfg.AutoUpdateBlocklists,
		UpdateInterval:       durationToString(cfg.UpdateInterval),
	}
}

func durationToString(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	return d.String()
}
