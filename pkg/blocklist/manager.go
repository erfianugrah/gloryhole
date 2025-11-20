package blocklist

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"glory-hole/pkg/config"
	"glory-hole/pkg/logging"
)

// Manager manages blocklist downloads and automatic updates
type Manager struct {
	cfg        *config.Config
	downloader *Downloader
	logger     *logging.Logger

	// Current blocklist (atomic pointer for zero-copy reads)
	current atomic.Pointer[map[string]struct{}]

	// Lifecycle management
	updateTicker *time.Ticker
	stopChan     chan struct{}
	wg           sync.WaitGroup
	started      atomic.Bool
}

// NewManager creates a new blocklist manager
func NewManager(cfg *config.Config, logger *logging.Logger) *Manager {
	m := &Manager{
		cfg:        cfg,
		downloader: NewDownloader(logger),
		logger:     logger,
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

	// Download all blocklists
	blocklist, err := m.downloader.DownloadAll(ctx, m.cfg.Blocklists)
	if err != nil {
		return err
	}

	// Atomically update current blocklist (zero-copy read for all DNS queries)
	m.current.Store(&blocklist)

	elapsed := time.Since(startTime)
	m.logger.Info("Blocklists updated",
		"domains", len(blocklist),
		"duration", elapsed,
		"domains_per_second", float64(len(blocklist))/elapsed.Seconds())

	return nil
}

// Get returns a pointer to the current blocklist (safe for concurrent reads)
func (m *Manager) Get() *map[string]struct{} {
	return m.current.Load()
}

// IsBlocked checks if a domain is blocked
func (m *Manager) IsBlocked(domain string) bool {
	blocklist := m.current.Load()
	if blocklist == nil {
		return false
	}
	_, blocked := (*blocklist)[domain]
	return blocked
}

// Size returns the number of blocked domains
func (m *Manager) Size() int {
	blocklist := m.current.Load()
	if blocklist == nil {
		return 0
	}
	return len(*blocklist)
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
