package blocklist

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"glory-hole/pkg/config"
	"glory-hole/pkg/logging"
	"glory-hole/pkg/pattern"
	"glory-hole/pkg/telemetry"
)

// Manager manages blocklist downloads and automatic updates
type Manager struct {
	cfg        *config.Config
	downloader *Downloader
	logger     *logging.Logger
	metrics    *telemetry.Metrics

	// Current blocklist (atomic pointer for zero-copy reads)
	current atomic.Pointer[map[string]struct{}]

	// Pattern-based blocklist (wildcard and regex)
	patterns atomic.Pointer[pattern.Matcher]

	// Lifecycle management
	updateTicker *time.Ticker
	stopChan     chan struct{}
	wg           sync.WaitGroup
	started      atomic.Bool
}

// NewManager creates a new blocklist manager with a custom HTTP client.
// The HTTP client should use the application's configured DNS resolver (pkg/resolver)
// to ensure consistent DNS resolution across the application.
func NewManager(cfg *config.Config, logger *logging.Logger, metrics *telemetry.Metrics, httpClient *http.Client) *Manager {
	m := &Manager{
		cfg:        cfg,
		downloader: NewDownloader(logger, httpClient),
		logger:     logger,
		metrics:    metrics,
		stopChan:   make(chan struct{}),
	}

	// Initialize with empty blocklist
	empty := make(map[string]struct{})
	m.current.Store(&empty)

	return m
}

// Start begins the blocklist management goroutine
func (m *Manager) Start(ctx context.Context) error {
	if !m.started.CompareAndSwap(false, true) {
		m.logger.Warn("Blocklist manager already started")
		return nil
	}

	// Re-create stopChan if this is a restart
	m.stopChan = make(chan struct{})

	m.logger.Info("Starting blocklist manager",
		"sources", len(m.cfg.Blocklists),
		"auto_update", m.cfg.AutoUpdateBlocklists,
		"interval", m.cfg.UpdateInterval)

	// Initial download
	if err := m.Update(ctx); err != nil {
		m.logger.Error("Initial blocklist download failed", "error", err)
		// Continue anyway - we'll retry on next update
	}

	// Start auto-update goroutine if enabled
	if m.cfg.AutoUpdateBlocklists && m.cfg.UpdateInterval > 0 {
		m.updateTicker = time.NewTicker(m.cfg.UpdateInterval)
		m.wg.Add(1)
		go m.updateLoop(ctx)
	}

	return nil
}

// Stop gracefully stops the blocklist manager
func (m *Manager) Stop() {
	if !m.started.CompareAndSwap(true, false) {
		return
	}

	m.logger.Info("Stopping blocklist manager")

	// Signal stop
	close(m.stopChan)

	// Stop ticker
	if m.updateTicker != nil {
		m.updateTicker.Stop()
	}

	// Wait for goroutines
	m.wg.Wait()

	m.logger.Info("Blocklist manager stopped")
}

// Update downloads all blocklists and updates the current blocklist
func (m *Manager) Update(ctx context.Context) error {
	if len(m.cfg.Blocklists) == 0 {
		m.logger.Debug("No blocklists configured")
		return nil
	}

	m.logger.Info("Updating blocklists", "sources", len(m.cfg.Blocklists))
	startTime := time.Now()

	// Get old size for metrics delta
	oldBlocklist := m.current.Load()
	oldSize := 0
	if oldBlocklist != nil {
		oldSize = len(*oldBlocklist)
	}

	// Download all blocklists
	blocklist, err := m.downloader.DownloadAll(ctx, m.cfg.Blocklists)
	if err != nil {
		return err
	}

	newSize := len(blocklist)
	delta := newSize - oldSize

	// Atomically update current blocklist (zero-copy read for all DNS queries)
	m.current.Store(&blocklist)

	// Record blocklist size change to Prometheus metrics if available
	if m.metrics != nil {
		m.metrics.BlocklistSize.Add(ctx, int64(delta))
	}

	elapsed := time.Since(startTime)

	// Log update results with delta information
	if delta > 0 {
		m.logger.Info("Blocklists updated - domains increased",
			"total_domains", newSize,
			"added", delta,
			"duration", elapsed,
			"domains_per_second", float64(newSize)/elapsed.Seconds())
	} else if delta < 0 {
		m.logger.Info("Blocklists updated - domains decreased",
			"total_domains", newSize,
			"removed", -delta,
			"duration", elapsed,
			"domains_per_second", float64(newSize)/elapsed.Seconds())
	} else {
		m.logger.Info("Blocklists updated - no changes",
			"total_domains", newSize,
			"duration", elapsed,
			"domains_per_second", float64(newSize)/elapsed.Seconds())
	}

	return nil
}

// SetHTTPClient updates the HTTP client used for downloads.
func (m *Manager) SetHTTPClient(client *http.Client) {
	m.downloader = NewDownloader(m.logger, client)
}

// UpdateConfig swaps the configuration reference used for future operations.
func (m *Manager) UpdateConfig(cfg *config.Config) {
	m.cfg = cfg
}

// SetLogger updates the logger used by the manager and downloader.
func (m *Manager) SetLogger(logger *logging.Logger) {
	m.logger = logger
	m.downloader.logger = logger
}

// Get returns a pointer to the current blocklist (safe for concurrent reads)
func (m *Manager) Get() *map[string]struct{} {
	return m.current.Load()
}

// IsBlocked checks if a domain is blocked
// It uses a multi-tier matching strategy for optimal performance:
//  1. Try exact match first (fastest - O(1))
//  2. Try pattern match if no exact match (wildcard/regex)
func (m *Manager) IsBlocked(domain string) bool {
	// Try exact match first (fastest path)
	blocklist := m.current.Load()
	if blocklist != nil {
		if _, blocked := (*blocklist)[domain]; blocked {
			return true
		}
	}

	// Try pattern match (wildcard/regex)
	patterns := m.patterns.Load()
	if patterns != nil {
		return patterns.Match(domain)
	}

	return false
}

// Size returns the number of blocked domains (exact matches only)
func (m *Manager) Size() int {
	blocklist := m.current.Load()
	if blocklist == nil {
		return 0
	}
	return len(*blocklist)
}

// SetPatterns sets the pattern-based blocklist (wildcard and regex)
func (m *Manager) SetPatterns(patternList []string) error {
	if len(patternList) == 0 {
		// Clear patterns
		m.patterns.Store(nil)
		m.logger.Debug("Cleared blocklist patterns")
		return nil
	}

	matcher, err := pattern.NewMatcher(patternList)
	if err != nil {
		return err
	}

	m.patterns.Store(matcher)

	stats := matcher.Stats()
	m.logger.Info("Updated blocklist patterns",
		"exact", stats["exact"],
		"wildcard", stats["wildcard"],
		"regex", stats["regex"],
		"total", stats["total"])

	return nil
}

// Stats returns statistics about the blocklist
func (m *Manager) Stats() map[string]int {
	stats := map[string]int{
		"exact": m.Size(),
	}

	patterns := m.patterns.Load()
	if patterns != nil {
		patternStats := patterns.Stats()
		stats["pattern_exact"] = patternStats["exact"]
		stats["pattern_wildcard"] = patternStats["wildcard"]
		stats["pattern_regex"] = patternStats["regex"]
		stats["total"] = stats["exact"] + patternStats["total"]
	} else {
		stats["pattern_exact"] = 0
		stats["pattern_wildcard"] = 0
		stats["pattern_regex"] = 0
		stats["total"] = stats["exact"]
	}

	return stats
}

// updateLoop runs the automatic update loop
func (m *Manager) updateLoop(ctx context.Context) {
	defer m.wg.Done()

	m.logger.Info("Blocklist auto-update loop started", "interval", m.cfg.UpdateInterval)

	for {
		select {
		case <-m.stopChan:
			m.logger.Info("Blocklist auto-update loop stopped")
			return

		case <-m.updateTicker.C:
			m.logger.Debug("Running scheduled blocklist update")

			// Create a timeout context for this update
			updateCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			if err := m.Update(updateCtx); err != nil {
				m.logger.Error("Scheduled blocklist update failed", "error", err)
			}
			cancel()
		}
	}
}
