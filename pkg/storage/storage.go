package storage

import (
	"context"
	"strings"
	"time"
)

// Storage defines the interface for all storage backends
// Implementations must be thread-safe and support concurrent access
type Storage interface {
	// Query Logging
	LogQuery(ctx context.Context, query *QueryLog) error
	GetRecentQueries(ctx context.Context, limit, offset int) ([]*QueryLog, error)
	GetQueriesByDomain(ctx context.Context, domain string, limit int) ([]*QueryLog, error)
	GetQueriesByClientIP(ctx context.Context, clientIP string, limit int) ([]*QueryLog, error)
	GetQueriesFiltered(ctx context.Context, filter QueryFilter, limit, offset int) ([]*QueryLog, error)

	// Statistics
	GetStatistics(ctx context.Context, since time.Time) (*Statistics, error)
	GetTopDomains(ctx context.Context, limit int, blocked bool, since time.Time) ([]*DomainStats, error)
	GetBlockedCount(ctx context.Context, since time.Time) (int64, error)
	GetQueryCount(ctx context.Context, since time.Time) (int64, error)
	GetTimeSeriesStats(ctx context.Context, bucket time.Duration, points int) ([]*TimeSeriesPoint, error)
	GetQueryTypeStats(ctx context.Context, limit int, since time.Time) ([]*QueryTypeStats, error)

	// Trace Analytics
	GetTraceStatistics(ctx context.Context, since time.Time) (*TraceStatistics, error)
	GetQueriesWithTraceFilter(ctx context.Context, filter TraceFilter, limit, offset int) ([]*QueryLog, error)

	// Client Management
	GetClientSummaries(ctx context.Context, limit, offset int) ([]*ClientSummary, error)
	UpdateClientProfile(ctx context.Context, profile *ClientProfile) error
	GetClientGroups(ctx context.Context) ([]*ClientGroup, error)
	UpsertClientGroup(ctx context.Context, group *ClientGroup) error
	DeleteClientGroup(ctx context.Context, name string) error

	// Policy Rules (persistent dynamic state — survives redeploys)
	GetPolicyRules(ctx context.Context) ([]*PolicyRule, error)
	CreatePolicyRule(ctx context.Context, rule *PolicyRule) (int64, error)
	UpdatePolicyRule(ctx context.Context, id int64, rule *PolicyRule) error
	DeletePolicyRule(ctx context.Context, id int64) error

	// Dynamic Config (key-value store for ACL, feature flags, etc.)
	GetDynamicConfig(ctx context.Context, key string) (string, error)
	SetDynamicConfig(ctx context.Context, key, value string) error

	// Unbound Query Log (dnstap)
	LogUnboundQuery(ctx context.Context, query *UnboundQueryLog) error
	GetUnboundQueries(ctx context.Context, filter UnboundQueryFilter, limit, offset int) ([]*UnboundQueryLog, error)
	GetUnboundQueryStats(ctx context.Context, since time.Time) (*UnboundQueryStats, error)

	// Maintenance
	Cleanup(ctx context.Context, olderThan time.Time) error
	Reset(ctx context.Context) error
	Close() error
	Ping(ctx context.Context) error
}

// QueryLog represents a single DNS query log entry
type QueryLog struct {
	Timestamp       time.Time         `json:"timestamp"`
	ClientIP        string            `json:"client_ip"`
	Domain          string            `json:"domain"`
	QueryType       string            `json:"query_type"`
	Upstream        string            `json:"upstream,omitempty"`
	UpstreamError   string            `json:"upstream_error,omitempty"`
	ID              int64             `json:"id"`
	ResponseCode    int               `json:"response_code"`
	ResponseTimeMs  float64           `json:"response_time_ms"`
	UpstreamTimeMs  float64           `json:"upstream_response_ms"`
	Blocked         bool              `json:"blocked"`
	Cached          bool              `json:"cached"`
	DNSSECValidated bool              `json:"dnssec_validated,omitempty"`
	BlockTrace      []BlockTraceEntry `json:"block_trace,omitempty"`

	// Unbound enrichment (populated when upstream is Unbound via dnstap correlation)
	UnboundCached     *bool    `json:"unbound_cached,omitempty"`
	UnboundDurationMs *float64 `json:"unbound_duration_ms,omitempty"`
	UnboundRespSize   *int     `json:"unbound_resp_size,omitempty"`
}

// BlockTraceEntry captures a single decision step explaining how a query was handled.
type BlockTraceEntry struct {
	Stage    string            `json:"stage"`
	Action   string            `json:"action"`
	Rule     string            `json:"rule,omitempty"`
	Source   string            `json:"source,omitempty"`
	Detail   string            `json:"detail,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// UnboundQueryLog represents a single dnstap event from Unbound.
type UnboundQueryLog struct {
	ID              int64     `json:"id"`
	Timestamp       time.Time `json:"timestamp"`
	MessageType     string    `json:"message_type"`
	Domain          string    `json:"domain"`
	QueryType       string    `json:"query_type"`
	ResponseCode    string    `json:"response_code,omitempty"`
	DurationMs      float64   `json:"duration_ms,omitempty"`
	DNSSECValidated bool      `json:"dnssec_validated"`
	AnswerCount     int       `json:"answer_count,omitempty"`
	ResponseSize    int       `json:"response_size,omitempty"`
	ClientIP        string    `json:"client_ip"`
	ServerIP        string    `json:"server_ip,omitempty"`
	CachedInUnbound bool      `json:"cached_in_unbound"`
}

// UnboundQueryFilter holds filtering options for the unbound_queries table.
type UnboundQueryFilter struct {
	Domain      string `json:"domain,omitempty"`
	QueryType   string `json:"query_type,omitempty"`
	MessageType string `json:"message_type,omitempty"`
	RCode       string `json:"rcode,omitempty"`
	Cached      *bool  `json:"cached,omitempty"`
	Start       string `json:"start,omitempty"`
	End         string `json:"end,omitempty"`
}

// UnboundQueryStats holds aggregated statistics from the unbound_queries table.
type UnboundQueryStats struct {
	TotalQueries       int64            `json:"total_queries"`
	CacheHits          int64            `json:"cache_hits"`
	CacheHitRate       float64          `json:"cache_hit_rate"`
	RecursiveQueries   int64            `json:"recursive_queries"`
	AvgRecursiveMs     float64          `json:"avg_recursive_ms"`
	AvgCachedMs        float64          `json:"avg_cached_ms"`
	DNSSECValidatedPct float64          `json:"dnssec_validated_pct"`
	ResponseCodes      map[string]int64 `json:"response_codes"`
}

// Statistics represents aggregated query statistics
type Statistics struct {
	Since             time.Time `json:"since"`
	Until             time.Time `json:"until"`
	TotalQueries      int64     `json:"total_queries"`
	BlockedQueries    int64     `json:"blocked_queries"`
	CachedQueries     int64     `json:"cached_queries"`
	UniqueDomains     int64     `json:"unique_domains"`
	UniqueClients     int64     `json:"unique_clients"`
	AvgResponseTimeMs float64   `json:"avg_response_time_ms"`
	BlockRate         float64   `json:"block_rate"`     // Percentage of blocked queries
	CacheHitRate      float64   `json:"cache_hit_rate"` // Percentage of cached responses
}

// DomainStats represents statistics for a specific domain
type DomainStats struct {
	LastQueried  time.Time `json:"last_queried"`
	FirstQueried time.Time `json:"first_queried,omitempty"`
	Domain       string    `json:"domain"`
	QueryCount   int64     `json:"query_count"`
	Blocked      bool      `json:"blocked"`
}

// QueryTypeStats represents aggregated counts per DNS record type.
type QueryTypeStats struct {
	QueryType string `json:"query_type"`
	Total     int64  `json:"total"`
	Blocked   int64  `json:"blocked"`
	Cached    int64  `json:"cached"`
}

// TraceStatistics represents aggregated trace statistics
type TraceStatistics struct {
	Since        time.Time        `json:"since"`
	Until        time.Time        `json:"until"`
	TotalBlocked int64            `json:"total_blocked"`
	ByStage      map[string]int64 `json:"by_stage"`
	ByAction     map[string]int64 `json:"by_action"`
	ByRule       map[string]int64 `json:"by_rule"`
	BySource     map[string]int64 `json:"by_source"`
}

// TraceFilter represents filtering options for trace queries
type TraceFilter struct {
	Stage  string
	Action string
	Rule   string
	Source string
}

// QueryFilter represents filter options for fetching queries.
type QueryFilter struct {
	Domain       string
	QueryType    string
	ClientIP     string
	Upstream     string
	ResponseCode int
	Blocked      *bool
	Cached       *bool
	Start        time.Time
	End          time.Time
}

// ClientSummary aggregates per-client statistics for display.
type ClientSummary struct {
	ClientIP       string    `json:"client_ip"`
	DisplayName    string    `json:"display_name"`
	GroupName      string    `json:"group_name,omitempty"`
	GroupColor     string    `json:"group_color,omitempty"`
	Notes          string    `json:"notes,omitempty"`
	TotalQueries   int64     `json:"total_queries"`
	BlockedQueries int64     `json:"blocked_queries"`
	NXDomainCount  int64     `json:"nxdomain_queries"`
	LastSeen       time.Time `json:"last_seen"`
	FirstSeen      time.Time `json:"first_seen"`
}

// ClientProfile stores metadata maintained by operators.
type ClientProfile struct {
	ClientIP    string
	DisplayName string
	GroupName   string
	Notes       string
}

// ClientGroup represents a logical grouping of clients.
type ClientGroup struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Color       string `json:"color,omitempty"`
}

// HasFilters returns true if any filtering criteria is set.
func (f QueryFilter) HasFilters() bool {
	return f.Domain != "" || f.QueryType != "" || f.Blocked != nil || f.Cached != nil || !f.Start.IsZero() || !f.End.IsZero()
}

// BackendType represents the type of storage backend
type BackendType string

const (
	BackendSQLite BackendType = "sqlite"
)

// Config represents storage configuration
type Config struct {
	Backend       BackendType      `yaml:"backend"`
	SQLite        SQLiteConfig     `yaml:"sqlite"`
	Statistics    StatisticsConfig `yaml:"statistics"`
	BufferSize    int              `yaml:"buffer_size"`
	FlushInterval time.Duration    `yaml:"flush_interval"`
	BatchSize     int              `yaml:"batch_size"`
	RetentionDays int              `yaml:"retention_days"`
	Enabled       bool             `yaml:"enabled"`
}

// SQLiteConfig represents SQLite-specific configuration
type SQLiteConfig struct {
	Path        string `yaml:"path"`         // Database file path
	BusyTimeout int    `yaml:"busy_timeout"` // Busy timeout in milliseconds
	WALMode     bool   `yaml:"wal_mode"`     // Enable WAL mode
	CacheSize   int    `yaml:"cache_size"`   // Cache size in KB
	MMapSize    int64  `yaml:"mmap_size"`    // mmap window in bytes
}

// StatisticsConfig represents statistics aggregation configuration
type StatisticsConfig struct {
	Enabled             bool          `yaml:"enabled"`
	AggregationInterval time.Duration `yaml:"aggregation_interval"` // How often to aggregate
}

// DefaultConfig returns a default storage configuration
func DefaultConfig() Config {
	return Config{
		Enabled: true,
		Backend: BackendSQLite,
		SQLite: SQLiteConfig{
			Path:        "./glory-hole.db",
			BusyTimeout: 5000,
			WALMode:     true,
			CacheSize:   4096,
			MMapSize:    268435456,
		},
		BufferSize:    50000, // Increased from 500 to handle high QPS
		FlushInterval: 5 * time.Second,
		BatchSize:     100,
		RetentionDays: 7,
		Statistics: StatisticsConfig{
			Enabled:             true,
			AggregationInterval: 1 * time.Hour,
		},
	}
}

type clientSearchContextKey struct{}

// WithClientSearch attaches a case-insensitive search term for client summaries to the context.
func WithClientSearch(ctx context.Context, search string) context.Context {
	trimmed := strings.ToLower(strings.TrimSpace(search))
	if trimmed == "" {
		return ctx
	}
	return context.WithValue(ctx, clientSearchContextKey{}, trimmed)
}

// ClientSearchFromContext extracts the client search term from context if present.
func ClientSearchFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if value, ok := ctx.Value(clientSearchContextKey{}).(string); ok {
		return value
	}
	return ""
}

// Validate validates the storage configuration
func (c *Config) Validate() error {
	if !c.Enabled {
		return nil
	}

	if c.Backend != BackendSQLite {
		return ErrInvalidBackend
	}

	if c.BufferSize < 1 {
		c.BufferSize = 100
	}

	if c.BatchSize < 1 {
		c.BatchSize = 100
	}

	if c.RetentionDays < 1 {
		c.RetentionDays = 7
	}

	if c.SQLite.MMapSize < 0 {
		c.SQLite.MMapSize = 0
	}

	return nil
}

// PolicyRule represents a policy rule stored in SQLite.
// This is the persistent representation — survives container redeploys.
type PolicyRule struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Logic      string `json:"logic"`
	Action     string `json:"action"`
	ActionData string `json:"action_data"`
	SortOrder  int    `json:"sort_order"`
	Enabled    bool   `json:"enabled"`
}

// TimeSeriesPoint represents aggregated query statistics for a specific time bucket.
type TimeSeriesPoint struct {
	Timestamp         time.Time `json:"timestamp"`
	TotalQueries      int64     `json:"total_queries"`
	BlockedQueries    int64     `json:"blocked_queries"`
	CachedQueries     int64     `json:"cached_queries"`
	AvgResponseTimeMs float64   `json:"avg_response_time_ms"`
}
