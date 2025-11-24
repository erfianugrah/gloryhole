package ratelimit

import (
	"net/netip"
	"sync"
	"time"

	"glory-hole/pkg/config"
	"glory-hole/pkg/logging"

	"golang.org/x/time/rate"
)

// Manager enforces simple per-client rate limiting using token buckets.
//nolint:fieldalignment // Layout favors logical grouping; padding impact is minimal.
type Manager struct {
	cfg        *config.RateLimitConfig
	logger     *logging.Logger
	overrides  []overrideMatcher
	ipOverride map[string]int

	mu      sync.Mutex
	clients map[string]*clientLimiter

	stopCh chan struct{}
	now    func() time.Time
}

type clientLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
	action   config.RateLimitAction
}

type overrideMatcher struct {
	name   string
	ips    map[string]struct{}
	cidrs  []netip.Prefix
	limit  rate.Limit
	burst  int
	action config.RateLimitAction
}

type limiterSettings struct {
	limit  rate.Limit
	burst  int
	action config.RateLimitAction
}

// NewManager creates a rate limit manager when rate limiting is enabled.
func NewManager(cfg *config.RateLimitConfig, logger *logging.Logger) *Manager {
	if cfg == nil || !cfg.Enabled {
		return nil
	}

	m := &Manager{
		cfg:        cfg,
		logger:     logger,
		clients:    make(map[string]*clientLimiter, 128),
		stopCh:     make(chan struct{}),
		now:        time.Now,
		ipOverride: make(map[string]int),
	}

	m.parseOverrides()

	if cfg.CleanupInterval > 0 {
		go m.cleanupLoop()
	}

	return m
}

// Allow returns whether the client may proceed or exceeded the rate limit.
func (m *Manager) Allow(clientIP string) (allowed bool, limited bool, action config.RateLimitAction) {
	if m == nil || clientIP == "" {
		return true, false, config.RateLimitActionDrop
	}

	entry := m.getLimiter(clientIP)
	if entry.limiter.Allow() {
		m.touch(clientIP, entry)
		return true, false, entry.action
	}

	m.touch(clientIP, entry)
	return false, true, entry.action
}

// LogViolations reports whether violations should be logged.
func (m *Manager) LogViolations() bool {
	if m == nil || m.cfg == nil {
		return false
	}
	return m.cfg.LogViolations
}

// Stop terminates background cleanup goroutines.
func (m *Manager) Stop() {
	if m == nil {
		return
	}
	select {
	case <-m.stopCh:
		return
	default:
		close(m.stopCh)
	}
}

func (m *Manager) cleanupLoop() {
	ticker := time.NewTicker(m.cfg.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.cleanup()
		case <-m.stopCh:
			return
		}
	}
}

func (m *Manager) cleanup() {
	now := m.now()
	m.mu.Lock()
	defer m.mu.Unlock()

	for ip, entry := range m.clients {
		if now.Sub(entry.lastSeen) > m.cfg.CleanupInterval {
			delete(m.clients, ip)
		}
	}
}

func (m *Manager) getLimiter(clientIP string) *clientLimiter {
	m.mu.Lock()
	defer m.mu.Unlock()

	if entry, ok := m.clients[clientIP]; ok {
		return entry
	}

	if m.cfg.MaxTrackedClients > 0 && len(m.clients) >= m.cfg.MaxTrackedClients {
		m.evictOldestLocked()
	}

	settings := m.settingsForIP(clientIP)
	entry := &clientLimiter{
		limiter:  rate.NewLimiter(settings.limit, settings.burst),
		lastSeen: m.now(),
		action:   settings.action,
	}
	m.clients[clientIP] = entry
	return entry
}

func (m *Manager) touch(clientIP string, entry *clientLimiter) {
	m.mu.Lock()
	entry.lastSeen = m.now()
	m.mu.Unlock()
}

func (m *Manager) evictOldestLocked() {
	var oldestIP string
	var oldestTime time.Time
	first := true

	for ip, entry := range m.clients {
		if first || entry.lastSeen.Before(oldestTime) {
			oldestIP = ip
			oldestTime = entry.lastSeen
			first = false
		}
	}

	if oldestIP != "" {
		delete(m.clients, oldestIP)
	}
}

func (m *Manager) settingsForIP(clientIP string) limiterSettings {
	if override := m.overrideForIP(clientIP); override != nil {
		return limiterSettings{
			limit:  override.limit,
			burst:  override.burst,
			action: override.action,
		}
	}

	return limiterSettings{
		limit:  rate.Limit(m.cfg.RequestsPerSecond),
		burst:  m.cfg.Burst,
		action: m.cfg.Action,
	}
}

func (m *Manager) overrideForIP(clientIP string) *overrideMatcher {
	if idx, ok := m.ipOverride[clientIP]; ok && idx < len(m.overrides) {
		return &m.overrides[idx]
	}

	addr, err := netip.ParseAddr(clientIP)
	if err != nil {
		return nil
	}

	for i := range m.overrides {
		for _, prefix := range m.overrides[i].cidrs {
			if prefix.Contains(addr) {
				return &m.overrides[i]
			}
		}
	}

	return nil
}

func (m *Manager) parseOverrides() {
	if m.cfg == nil {
		return
	}

	for idx, ov := range m.cfg.Overrides {
		settings := overrideMatcher{
			name:   ov.Name,
			ips:    make(map[string]struct{}),
			limit:  rate.Limit(m.cfg.RequestsPerSecond),
			burst:  m.cfg.Burst,
			action: m.cfg.Action,
		}

		if ov.RequestsPerSecond != nil {
			settings.limit = rate.Limit(*ov.RequestsPerSecond)
		}
		if ov.Burst != nil {
			settings.burst = *ov.Burst
		}
		if ov.Action != nil {
			settings.action = *ov.Action
		}

		for _, ip := range ov.Clients {
			settings.ips[ip] = struct{}{}
			m.ipOverride[ip] = len(m.overrides)
		}

		for _, cidr := range ov.CIDRs {
			prefix, err := netip.ParsePrefix(cidr)
			if err != nil {
				if m.logger != nil {
					m.logger.Warn("Invalid rate limit override CIDR",
						"override", ov.Name,
						"value", cidr,
						"index", idx,
						"error", err)
				}
				continue
			}
			settings.cidrs = append(settings.cidrs, prefix)
		}

		if len(settings.ips) == 0 && len(settings.cidrs) == 0 {
			continue
		}

		m.overrides = append(m.overrides, settings)
	}
}
