package storage

import (
	"context"
	"fmt"
	"time"
)

// New creates a new storage instance based on the configuration
// Returns ErrNotEnabled if storage is disabled in config
func New(cfg *Config, metrics MetricsRecorder) (Storage, error) {
	if cfg == nil {
		cfg = &Config{}
		*cfg = DefaultConfig()
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// If storage is disabled, return a no-op storage
	if !cfg.Enabled {
		return NewNoOpStorage(), nil
	}

	// Create storage based on backend type
	switch cfg.Backend {
	case BackendSQLite:
		return NewSQLiteStorage(cfg, metrics)
	default:
		return nil, fmt.Errorf("%w: %s", ErrInvalidBackend, cfg.Backend)
	}
}

// NoOpStorage is a no-op storage that does nothing
// Used when storage is disabled
type NoOpStorage struct{}

// NewNoOpStorage creates a new no-op storage
func NewNoOpStorage() *NoOpStorage {
	return &NoOpStorage{}
}

// LogQuery does nothing
func (n *NoOpStorage) LogQuery(ctx context.Context, query *QueryLog) error {
	return nil
}

// GetRecentQueries returns an empty slice
func (n *NoOpStorage) GetRecentQueries(ctx context.Context, limit, offset int) ([]*QueryLog, error) {
	return []*QueryLog{}, nil
}

// GetQueriesByDomain returns an empty slice
func (n *NoOpStorage) GetQueriesByDomain(ctx context.Context, domain string, limit int) ([]*QueryLog, error) {
	return []*QueryLog{}, nil
}

// GetQueriesByClientIP returns an empty slice
func (n *NoOpStorage) GetQueriesByClientIP(ctx context.Context, clientIP string, limit int) ([]*QueryLog, error) {
	return []*QueryLog{}, nil
}

// GetStatistics returns empty statistics
func (n *NoOpStorage) GetStatistics(ctx context.Context, since time.Time) (*Statistics, error) {
	return &Statistics{
		Since: since,
		Until: time.Now(),
	}, nil
}

// GetTopDomains returns an empty slice
func (n *NoOpStorage) GetTopDomains(ctx context.Context, limit int, blocked bool) ([]*DomainStats, error) {
	return []*DomainStats{}, nil
}

// GetBlockedCount returns zero
func (n *NoOpStorage) GetBlockedCount(ctx context.Context, since time.Time) (int64, error) {
	return 0, nil
}

// GetQueryCount returns zero
func (n *NoOpStorage) GetQueryCount(ctx context.Context, since time.Time) (int64, error) {
	return 0, nil
}

// GetQueriesFiltered returns an empty slice
func (n *NoOpStorage) GetQueriesFiltered(ctx context.Context, filter QueryFilter, limit, offset int) ([]*QueryLog, error) {
	return []*QueryLog{}, nil
}

// GetTimeSeriesStats returns zero-filled time series data
func (n *NoOpStorage) GetTimeSeriesStats(ctx context.Context, bucket time.Duration, points int) ([]*TimeSeriesPoint, error) {
	if points < 1 {
		return []*TimeSeriesPoint{}, nil
	}

	now := time.Now().UTC()
	data := make([]*TimeSeriesPoint, 0, points)
	bucketSeconds := int64(bucket / time.Second)
	if bucketSeconds <= 0 {
		bucketSeconds = int64(time.Hour / time.Second)
	}

	aligned := time.Unix((now.Unix()/bucketSeconds)*bucketSeconds, 0).UTC()
	start := aligned.Add(-bucket * time.Duration(points-1))

	for i := 0; i < points; i++ {
		data = append(data, &TimeSeriesPoint{
			Timestamp:         start.Add(time.Duration(i) * bucket),
			TotalQueries:      0,
			BlockedQueries:    0,
			CachedQueries:     0,
			AvgResponseTimeMs: 0,
		})
	}

	return data, nil
}

// GetTraceStatistics returns empty trace statistics
func (n *NoOpStorage) GetTraceStatistics(ctx context.Context, since time.Time) (*TraceStatistics, error) {
	return &TraceStatistics{
		Since:    since,
		Until:    time.Now(),
		ByStage:  make(map[string]int64),
		ByAction: make(map[string]int64),
		ByRule:   make(map[string]int64),
		BySource: make(map[string]int64),
	}, nil
}

// GetQueriesWithTraceFilter returns an empty slice
func (n *NoOpStorage) GetQueriesWithTraceFilter(ctx context.Context, filter TraceFilter, limit, offset int) ([]*QueryLog, error) {
	return []*QueryLog{}, nil
}

// GetQueryTypeStats returns an empty slice.
func (n *NoOpStorage) GetQueryTypeStats(ctx context.Context, limit int, since time.Time) ([]*QueryTypeStats, error) {
	return []*QueryTypeStats{}, nil
}

func (n *NoOpStorage) GetClientSummaries(ctx context.Context, limit, offset int) ([]*ClientSummary, error) {
	return []*ClientSummary{}, nil
}

func (n *NoOpStorage) UpdateClientProfile(ctx context.Context, profile *ClientProfile) error {
	return ErrNotEnabled
}

func (n *NoOpStorage) GetClientGroups(ctx context.Context) ([]*ClientGroup, error) {
	return []*ClientGroup{}, nil
}

func (n *NoOpStorage) UpsertClientGroup(ctx context.Context, group *ClientGroup) error {
	return ErrNotEnabled
}

func (n *NoOpStorage) DeleteClientGroup(ctx context.Context, name string) error {
	return ErrNotEnabled
}

// Cleanup does nothing
func (n *NoOpStorage) Cleanup(ctx context.Context, olderThan time.Time) error {
	return nil
}

// Reset does nothing
func (n *NoOpStorage) Reset(ctx context.Context) error {
	return nil
}

// Close does nothing
func (n *NoOpStorage) Close() error {
	return nil
}

// Ping does nothing
func (n *NoOpStorage) Ping(ctx context.Context) error {
	return nil
}
