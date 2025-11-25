package storage

import (
	"context"
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
	GetTopDomains(ctx context.Context, limit int, blocked bool) ([]*DomainStats, error)
	GetBlockedCount(ctx context.Context, since time.Time) (int64, error)
	GetQueryCount(ctx context.Context, since time.Time) (int64, error)
	GetTimeSeriesStats(ctx context.Context, bucket time.Duration, points int) ([]*TimeSeriesPoint, error)
	GetQueryTypeStats(ctx context.Context, limit int) ([]*QueryTypeStats, error)

	// Trace Analytics
	GetTraceStatistics(ctx context.Context, since time.Time) (*TraceStatistics, error)
	GetQueriesWithTraceFilter(ctx context.Context, filter TraceFilter, limit, offset int) ([]*QueryLog, error)

	// Maintenance
	Cleanup(ctx context.Context, olderThan time.Time) error
	Close() error
	Ping(ctx context.Context) error
}

// QueryLog represents a single DNS query log entry
type QueryLog struct {
	Timestamp      time.Time         `json:"timestamp"`
	ClientIP       string            `json:"client_ip"`
	Domain         string            `json:"domain"`
	QueryType      string            `json:"query_type"`
	Upstream       string            `json:"upstream,omitempty"`
	ID             int64             `json:"id"`
	ResponseCode   int               `json:"response_code"`
	ResponseTimeMs float64           `json:"response_time_ms"`
	Blocked        bool              `json:"blocked"`
	Cached         bool              `json:"cached"`
	BlockTrace     []BlockTraceEntry `json:"block_trace,omitempty"`
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
	Domain    string
	QueryType string
	Blocked   *bool
	Cached    *bool
	Start     time.Time
	End       time.Time
}

// HasFilters returns true if any filtering criteria is set.
func (f QueryFilter) HasFilters() bool {
	return f.Domain != "" || f.QueryType != "" || f.Blocked != nil || f.Cached != nil || !f.Start.IsZero() || !f.End.IsZero()
}

// BackendType represents the type of storage backend
type BackendType string

const (
	BackendSQLite BackendType = "sqlite"
	BackendD1     BackendType = "d1"
)

// Config represents storage configuration
type Config struct {
	D1            D1Config         `yaml:"d1"`
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
}

// D1Config represents D1-specific configuration
type D1Config struct {
	AccountID  string `yaml:"account_id"`  // Cloudflare account ID
	DatabaseID string `yaml:"database_id"` // D1 database ID
	APIToken   string `yaml:"api_token"`   // Cloudflare API token
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
			CacheSize:   10000,
		},
		BufferSize:    1000,
		FlushInterval: 5 * time.Second,
		BatchSize:     100,
		RetentionDays: 7,
		Statistics: StatisticsConfig{
			Enabled:             true,
			AggregationInterval: 1 * time.Hour,
		},
	}
}

// Validate validates the storage configuration
func (c *Config) Validate() error {
	if !c.Enabled {
		return nil
	}

	if c.Backend != BackendSQLite && c.Backend != BackendD1 {
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

	return nil
}

// TimeSeriesPoint represents aggregated query statistics for a specific time bucket.
type TimeSeriesPoint struct {
	Timestamp         time.Time `json:"timestamp"`
	TotalQueries      int64     `json:"total_queries"`
	BlockedQueries    int64     `json:"blocked_queries"`
	CachedQueries     int64     `json:"cached_queries"`
	AvgResponseTimeMs float64   `json:"avg_response_time_ms"`
}
