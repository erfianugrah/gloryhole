package ratelimit

import (
	"sync"
	"time"

	"glory-hole/pkg/config"
	"glory-hole/pkg/logging"

	"golang.org/x/time/rate"
)

// Manager enforces simple per-client rate limiting using token buckets.
type Manager struct {
	cfg    *config.RateLimitConfig
	logger *logging.Logger

	mu      sync.Mutex
	clients map[string]*clientLimiter

	stopCh chan struct{}
	now    func() time.Time
}

type clientLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// NewManager creates a rate limit manager when rate limiting is enabled.
func NewManager(cfg *config.RateLimitConfig, logger *logging.Logger) *Manager {
	if cfg == nil || !cfg.Enabled {
		return nil
	}

	m := &Manager{
		cfg:     cfg,
		logger:  logger,
		clients: make(map[string]*clientLimiter, 128),
		stopCh:  make(chan struct{}),
		now:     time.Now,
	}

	if cfg.CleanupInterval > 0 {
		go m.cleanupLoop()
	}

	return m
}

// Allow returns whether the client may proceed or exceeded the rate limit.
func (m *Manager) Allow(clientIP string) (allowed bool, limited bool) {
	if m == nil || clientIP == "" {
		return true, false
	}

	entry := m.getLimiter(clientIP)
	if entry.limiter.Allow() {
		m.touch(clientIP, entry)
		return true, false
	}

	m.touch(clientIP, entry)
	return false, true
}

// Action returns the configured exceed action.
func (m *Manager) Action() config.RateLimitAction {
	if m == nil || m.cfg == nil {
		return config.RateLimitActionDrop
	}
	return m.cfg.Action
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

	entry := &clientLimiter{
		limiter:  rate.NewLimiter(rate.Limit(m.cfg.RequestsPerSecond), m.cfg.Burst),
		lastSeen: m.now(),
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
