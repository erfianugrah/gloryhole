// Package blocklist provides download/update management for blocklist sources
// plus lock-free matching utilities used by the DNS handler.
package blocklist

import (
	"context"
	"net/http"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"glory-hole/pkg/config"
	"glory-hole/pkg/logging"
	"glory-hole/pkg/pattern"
	"glory-hole/pkg/telemetry"

	mdns "github.com/miekg/dns"
)

const maxTrackedSources = 64

// BlockEntry stores metadata about a blocked domain.
// Kept for API compatibility (tests, handlers that inspect source provenance).
type BlockEntry struct {
	SourceMask uint64
	Overflow   bool
}

// Manager manages blocklist downloads and automatic updates
type Manager struct {
	cfg        *config.Config
	cfgMu      sync.RWMutex // Protects cfg access
	downloader *Downloader
	logger     *logging.Logger
	metrics    *telemetry.Metrics

	// Current blocklist — compact sorted structure, ~33 bytes/domain
	// vs ~140 bytes/domain for map[string]uint64. At 1.3M domains
	// this is ~43MB instead of ~180MB.
	current atomic.Pointer[FlatBlocklist]

	// Pattern-based blocklist (wildcard and regex)
	patterns atomic.Pointer[pattern.Matcher]

	lastUpdated atomic.Value
	sourceNames atomic.Value

	// updateMu serializes Update calls to prevent concurrent downloads
	// from overlapping (API reload + config watcher + auto-update ticker).
	// This prevents double memory usage from parallel downloads.
	updateMu sync.Mutex

	// lastSize tracks the domain count from the most recent update,
	// used to pre-allocate the merged map and avoid repeated growth.
	lastSize atomic.Int64

	// Lifecycle management
	updateTicker *time.Ticker
	stopChan     chan struct{}
	wg           sync.WaitGroup
	started      atomic.Bool
}

// NewManager creates a new blocklist manager with a custom HTTP client.
func NewManager(cfg *config.Config, logger *logging.Logger, metrics *telemetry.Metrics, httpClient *http.Client) *Manager {
	m := &Manager{
		cfg:        cfg,
		downloader: NewDownloader(logger, httpClient),
		logger:     logger,
		metrics:    metrics,
		stopChan:   make(chan struct{}),
	}

	// Initialize with empty blocklist
	empty := BuildFlatBlocklist(nil)
	m.current.Store(empty)
	m.lastUpdated.Store(time.Time{})
	m.sourceNames.Store([]string{})

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

	m.cfgMu.RLock()
	blocklists := m.cfg.Blocklists
	autoUpdate := m.cfg.AutoUpdateBlocklists
	updateInterval := m.cfg.UpdateInterval
	m.cfgMu.RUnlock()

	m.logger.Info("Starting blocklist manager",
		"sources", len(blocklists),
		"auto_update", autoUpdate,
		"interval", updateInterval)

	// Initial download
	if err := m.Update(ctx); err != nil {
		m.logger.Error("Initial blocklist download failed", "error", err)
	}

	// Start auto-update goroutine if enabled
	if autoUpdate && updateInterval > 0 {
		m.updateTicker = time.NewTicker(updateInterval)
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
	close(m.stopChan)

	if m.updateTicker != nil {
		m.updateTicker.Stop()
	}

	m.wg.Wait()
	m.logger.Info("Blocklist manager stopped")
}

// Update downloads all blocklists and updates the current blocklist.
func (m *Manager) Update(ctx context.Context) error {
	m.cfgMu.RLock()
	blocklists := m.cfg.Blocklists
	m.cfgMu.RUnlock()

	if len(blocklists) == 0 {
		m.logger.Debug("No blocklists configured")
		return nil
	}

	if !m.updateMu.TryLock() {
		m.logger.Info("Blocklist update already in progress, skipping")
		return nil
	}
	defer m.updateMu.Unlock()

	m.logger.Info("Updating blocklists", "sources", len(blocklists))
	startTime := time.Now()
	oldSize := int(m.lastSize.Load())

	// Download each list into a sorted slice, then k-way merge into FlatBlocklist.
	// This avoids the ~180MB temporary map[string]uint64 for 1.3M domains —
	// each per-list []string is sorted and released after merge.
	flat, err := m.downloadAndMerge(ctx)
	if err != nil {
		return err
	}

	m.logger.Info("Blocklist compacted",
		"domains", flat.Len(),
		"memory_bytes", flat.MemoryUsage(),
		"memory_mb", flat.MemoryUsage()/(1024*1024))

	newSize := flat.Len()
	delta := newSize - oldSize

	m.current.Store(flat)
	m.lastSize.Store(int64(newSize))

	// Force the Go runtime to return freed pages to the OS immediately.
	// Without this, the temporary per-list slices and sort buffers stay
	// mapped in RSS even though they're unreachable. On a 512MB Fly VM
	// this reclaims ~60-100MB of RSS after blocklist compaction.
	runtime.GC()
	debug.FreeOSMemory()
	m.lastUpdated.Store(time.Now())

	m.cfgMu.RLock()
	sourceCopy := make([]string, len(m.cfg.Blocklists))
	copy(sourceCopy, m.cfg.Blocklists)
	m.cfgMu.RUnlock()

	if len(sourceCopy) > maxTrackedSources {
		sourceCopy = sourceCopy[:maxTrackedSources]
	}
	m.sourceNames.Store(sourceCopy)

	if m.metrics != nil {
		m.metrics.BlocklistSize.Add(ctx, int64(delta))
	}

	elapsed := time.Since(startTime)
	if delta > 0 {
		m.logger.Info("Blocklists updated - domains increased",
			"total_domains", newSize, "added", delta,
			"duration", elapsed,
			"domains_per_second", float64(newSize)/elapsed.Seconds())
	} else if delta < 0 {
		m.logger.Info("Blocklists updated - domains decreased",
			"total_domains", newSize, "removed", -delta,
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

// downloadAndMerge downloads each blocklist into a sorted slice, then
// k-way merges them into a FlatBlocklist. This avoids the ~180MB temp
// map[string]uint64 that the old path needed for 1.3M domains.
//
// Memory profile during download:
//   - Each per-list []string holds ~25 bytes * N_list domains
//   - After sorting, it's passed to BuildFromSortedLists which streams
//     into the contiguous FlatBlocklist and the per-list slice is released
//   - Peak memory: sum of all per-list slices + final FlatBlocklist
//   - For 3 lists totaling 1.3M domains: ~50MB peak vs ~230MB with temp map
func (m *Manager) downloadAndMerge(ctx context.Context) (*FlatBlocklist, error) {
	m.cfgMu.RLock()
	urls := m.cfg.Blocklists
	m.cfgMu.RUnlock()

	if len(urls) == 0 {
		return &FlatBlocklist{}, nil
	}

	m.logger.Info("Downloading blocklists", "count", len(urls))
	startTime := time.Now()

	lists := make([]sortedList, 0, len(urls))

	for idx, url := range urls {
		m.logger.Info("Downloading blocklist", "index", idx+1, "total", len(urls), "url", url)

		// DownloadSorted returns a deduplicated, sorted []string directly —
		// no intermediate map[string]struct{} (saves ~60MB per 500K-domain list).
		sorted, err := m.downloader.DownloadSorted(ctx, url)
		if err != nil {
			m.logger.Error("Failed to download blocklist", "url", url, "error", err)
			continue
		}

		var mask uint64
		if idx < maxTrackedSources {
			mask = 1 << uint(idx)
		}

		lists = append(lists, sortedList{domains: sorted, mask: mask})
		m.logger.Info("Blocklist downloaded and sorted",
			"index", idx+1, "domains", len(sorted))
	}

	if len(urls) > maxTrackedSources {
		m.logger.Warn("Tracking metadata for first 64 blocklist sources only", "configured", len(urls))
	}

	m.logger.Info("Merging blocklists", "lists", len(lists))
	flat := BuildFromSortedLists(lists)

	// Release per-list slices
	lists = nil //nolint:ineffassign

	m.logger.Info("All blocklists downloaded and merged",
		"total_domains", flat.Len(),
		"duration", time.Since(startTime))

	return flat, nil
}

// SetHTTPClient updates the HTTP client used for downloads.
func (m *Manager) SetHTTPClient(client *http.Client) {
	m.downloader = NewDownloader(m.logger, client)
}

// UpdateConfig swaps the configuration reference used for future operations.
func (m *Manager) UpdateConfig(cfg *config.Config) {
	m.cfgMu.Lock()
	m.cfg = cfg
	m.cfgMu.Unlock()
}

// SetLogger updates the logger used by the manager and downloader.
func (m *Manager) SetLogger(logger *logging.Logger) {
	m.logger = logger
	m.downloader.logger = logger
}

// Get returns the blocklist as the legacy BlockEntry map.
// Converts from the compact representation on the fly.
// Used only by tests — the hot path uses Match()/IsBlocked().
func (m *Manager) Get() *map[string]BlockEntry {
	flat := m.current.Load()
	if flat == nil || flat.Len() == 0 {
		empty := make(map[string]BlockEntry)
		return &empty
	}
	result := make(map[string]BlockEntry, flat.Len())
	flat.ForEach(func(domain string, mask uint64) {
		result[domain] = BlockEntry{
			SourceMask: mask,
			Overflow:   mask == 0,
		}
	})
	return &result
}

// SetDomainsForTest replaces the blocklist with the given domains.
// Intended for benchmarks and tests in external packages.
func (m *Manager) SetDomainsForTest(domains []string) {
	tmp := make(map[string]uint64, len(domains))
	for _, d := range domains {
		tmp[d] = 1
	}
	flat := BuildFlatBlocklist(tmp)
	m.current.Store(flat)
	m.lastSize.Store(int64(flat.Len()))
}

// IsBlocked checks if a domain is blocked
func (m *Manager) IsBlocked(domain string) bool {
	return m.Match(domain).Blocked
}

// MatchResult describes how a domain was blocked.
type MatchResult struct {
	Blocked bool
	Kind    string   // exact, subdomain, wildcard, regex
	Pattern string   // for wildcard/regex
	Sources []string // blocklist sources
}

// Match returns detailed information about a blocked domain.
func (m *Manager) Match(domain string) MatchResult {
	if domain == "" {
		return MatchResult{}
	}

	normalized := strings.ToLower(domain)
	fqdn := mdns.Fqdn(normalized)
	short := strings.TrimSuffix(fqdn, ".")

	flat := m.current.Load()
	if flat != nil && flat.Len() > 0 {
		if mask, kind, ok := flat.LookupSubdomains(fqdn); ok {
			return MatchResult{
				Blocked: true,
				Kind:    kind,
				Sources: m.sourcesFromMask(mask),
			}
		}
	}

	if patterns := m.patterns.Load(); patterns != nil {
		if matched, ok := patterns.MatchPattern(short); ok && matched != nil {
			return MatchResult{
				Blocked: true,
				Kind:    matched.Type.String(),
				Pattern: matched.Raw,
				Sources: []string{"pattern"},
			}
		}
	}

	return MatchResult{}
}

// Size returns the number of blocked domains (exact matches only)
func (m *Manager) Size() int {
	flat := m.current.Load()
	if flat == nil {
		return 0
	}
	return flat.Len()
}

// SetPatterns sets the pattern-based blocklist (wildcard and regex)
func (m *Manager) SetPatterns(patternList []string) error {
	if len(patternList) == 0 {
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

// LastUpdated returns the timestamp of the most recent successful update.
func (m *Manager) LastUpdated() time.Time {
	if v := m.lastUpdated.Load(); v != nil {
		if ts, ok := v.(time.Time); ok {
			return ts
		}
	}
	return time.Time{}
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

// sourcesFromMask decodes a bitmask back to human-readable source URL strings.
func (m *Manager) sourcesFromMask(mask uint64) []string {
	value := m.sourceNames.Load()
	names, _ := value.([]string)

	overflow := mask == 0 && len(names) > 0
	if len(names) == 0 && !overflow {
		return nil
	}

	// Count bits to pre-allocate
	count := 0
	for b := mask; b != 0; b &= b - 1 {
		count++
	}
	result := make([]string, 0, count)
	for idx := 0; idx < len(names) && idx < maxTrackedSources; idx++ {
		if mask&(1<<uint(idx)) != 0 {
			result = append(result, names[idx])
		}
	}

	if overflow {
		result = append(result, "additional sources")
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

// updateLoop runs the automatic update loop
func (m *Manager) updateLoop(ctx context.Context) {
	defer m.wg.Done()

	m.cfgMu.RLock()
	updateInterval := m.cfg.UpdateInterval
	m.cfgMu.RUnlock()

	m.logger.Info("Blocklist auto-update loop started", "interval", updateInterval)

	for {
		select {
		case <-m.stopChan:
			m.logger.Info("Blocklist auto-update loop stopped")
			return

		case <-m.updateTicker.C:
			m.logger.Debug("Running scheduled blocklist update")
			updateCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			if err := m.Update(updateCtx); err != nil {
				m.logger.Error("Scheduled blocklist update failed", "error", err)
			}
			cancel()
		}
	}
}
