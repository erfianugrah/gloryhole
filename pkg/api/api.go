package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"glory-hole/pkg/blocklist"
	"glory-hole/pkg/cache"
	"glory-hole/pkg/config"
	"glory-hole/pkg/policy"
	"glory-hole/pkg/ratelimit"
	"glory-hole/pkg/storage"
)

// Server represents the API server
type Server struct {
	handler          http.Handler
	storage          storage.Storage
	httpServer       *http.Server
	logger           *slog.Logger
	blocklistManager *blocklist.Manager
	policyEngine     *policy.Engine
	cache            cache.Interface    // DNS cache for purge operations
	configWatcher    *config.Watcher    // For kill-switch feature
	killSwitch       *KillSwitchManager // For duration-based temporary disabling
	configSnapshot   *config.Config     // Used when watcher is unavailable (tests, static configs)
	startTime        time.Time
	version          string
	configPath       string // Path to config file for persistence
	rateLimiter      *ratelimit.Manager
	authMu           sync.RWMutex
	authEnabled      bool
	authHeader       string
	apiKey           string
	basicUser        string
	basicPass        string
}

// Config holds API server configuration
type Config struct {
	Storage          storage.Storage
	BlocklistManager *blocklist.Manager
	PolicyEngine     *policy.Engine
	Cache            cache.Interface // DNS cache for purge operations
	Logger           *slog.Logger
	ConfigWatcher    *config.Watcher    // For kill-switch feature
	KillSwitch       *KillSwitchManager // For duration-based temporary disabling
	ListenAddress    string
	Version          string
	ConfigPath       string // Path to config file
	InitialConfig    *config.Config
	RateLimiter      *ratelimit.Manager
}

// New creates a new API server
func New(cfg *Config) *Server {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	// Initialize templates
	if err := initTemplates(); err != nil {
		cfg.Logger.Warn("Failed to initialize UI templates", "error", err)
	}

	s := &Server{
		storage:          cfg.Storage,
		blocklistManager: cfg.BlocklistManager,
		policyEngine:     cfg.PolicyEngine,
		cache:            cfg.Cache,
		logger:           cfg.Logger,
		version:          cfg.Version,
		configWatcher:    cfg.ConfigWatcher,
		configPath:       cfg.ConfigPath,
		killSwitch:       cfg.KillSwitch,
		configSnapshot:   cfg.InitialConfig,
		startTime:        time.Now(),
		rateLimiter:      cfg.RateLimiter,
	}

	if cfg.InitialConfig != nil {
		s.applyAuthConfig(cfg.InitialConfig.Auth)
	}

	// Setup routes
	mux := http.NewServeMux()

	// Health checks
	mux.HandleFunc("/api/health", s.handleHealth) // Detailed health with uptime/version
	mux.HandleFunc("/health", s.handleLiveness)   // Simple liveness check
	mux.HandleFunc("/ready", s.handleReadiness)   // Readiness check with components

	// Statistics
	mux.HandleFunc("/api/stats", s.handleStats)
	mux.HandleFunc("/api/stats/timeseries", s.handleStatsTimeSeries)
	mux.HandleFunc("/api/stats/query-types", s.handleQueryTypes)

	// Trace statistics
	mux.HandleFunc("/api/traces/stats", s.handleTraceStatistics)

	// Queries
	mux.HandleFunc("/api/queries", s.handleQueries)

	// Top domains
	mux.HandleFunc("/api/top-domains", s.handleTopDomains)

	// Blocklist management
	mux.HandleFunc("POST /api/blocklist/reload", s.handleBlocklistReload)

	// Cache management
	mux.HandleFunc("POST /api/cache/purge", s.handleCachePurge)
	mux.HandleFunc("POST /api/storage/reset", s.handleStorageReset)

	// Policy management
	mux.HandleFunc("GET /api/policies", s.handleGetPolicies)
	mux.HandleFunc("POST /api/policies", s.handleAddPolicy)
	mux.HandleFunc("GET /api/policies/{id}", s.handleGetPolicy)
	mux.HandleFunc("PUT /api/policies/{id}", s.handleUpdatePolicy)
	mux.HandleFunc("DELETE /api/policies/{id}", s.handleDeletePolicy)
	mux.HandleFunc("GET /api/policies/export", s.handleExportPolicies)
	mux.HandleFunc("POST /api/policies/test", s.handleTestPolicy)

	// Feature kill-switches
	mux.HandleFunc("GET /api/features", s.handleGetFeatures)
	mux.HandleFunc("PUT /api/features", s.handleUpdateFeatures)

	// Duration-based temporary disabling (Pi-hole style)
	mux.HandleFunc("POST /api/features/blocklist/disable", s.handleDisableBlocklist)
	mux.HandleFunc("POST /api/features/blocklist/enable", s.handleEnableBlocklist)
	mux.HandleFunc("POST /api/features/policies/disable", s.handleDisablePolicies)
	mux.HandleFunc("POST /api/features/policies/enable", s.handleEnablePolicies)

	// Configuration surface
	mux.HandleFunc("GET /api/config", s.handleGetConfig)
	mux.HandleFunc("PUT /api/config/upstreams", s.handleUpdateUpstreams)
	mux.HandleFunc("PUT /api/config/cache", s.handleUpdateCache)
	mux.HandleFunc("PUT /api/config/logging", s.handleUpdateLogging)

	// UI routes (add after API routes to avoid conflicts)
	mux.HandleFunc("GET /api/ui/stats", s.handleStatsPartial)
	mux.HandleFunc("GET /api/ui/queries", s.handleQueriesPartial)
	mux.HandleFunc("GET /api/ui/top-domains", s.handleTopDomainsPartial)
	mux.HandleFunc("GET /api/ui/clients", s.handleClientsPartial)
	mux.HandleFunc("GET /queries", s.handleQueriesPage)
	mux.HandleFunc("GET /policies", s.handlePoliciesPage)
	mux.HandleFunc("GET /settings", s.handleSettingsPage)
	mux.HandleFunc("GET /clients", s.handleClientsPage)
	mux.HandleFunc("GET /blocklists", s.handleBlocklistsPage)
	mux.HandleFunc("GET /{$}", s.handleDashboard) // {$} matches exact path only

	// Client management APIs
	mux.HandleFunc("GET /api/clients", s.handleGetClients)
	mux.HandleFunc("PUT /api/clients/{client}", s.handleUpdateClient)
	mux.HandleFunc("GET /api/client-groups", s.handleGetClientGroups)
	mux.HandleFunc("POST /api/client-groups", s.handleCreateClientGroup)
	mux.HandleFunc("PUT /api/client-groups/{group}", s.handleUpdateClientGroup)
	mux.HandleFunc("DELETE /api/client-groups/{group}", s.handleDeleteClientGroup)

	// Blocklist summary APIs
	mux.HandleFunc("GET /api/blocklists", s.handleGetBlocklists)
	mux.HandleFunc("GET /api/blocklists/check", s.handleCheckBlocklist)

	// Static files
	if staticFS, err := getStaticFS(); err == nil {
		mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(staticFS)))
	} else {
		cfg.Logger.Warn("Failed to initialize static file server", "error", err)
	}

	// Apply middleware
	handler := http.Handler(mux)
	handler = s.authMiddleware(handler)
	handler = s.rateLimitMiddleware(handler)
	handler = s.loggingMiddleware(handler)
	handler = s.corsMiddleware(handler)

	s.handler = handler
	s.httpServer = &http.Server{
		Addr:         cfg.ListenAddress,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return s
}

func (s *Server) applyAuthConfig(auth config.AuthConfig) {
	s.authMu.Lock()
	defer s.authMu.Unlock()

	header := strings.TrimSpace(auth.Header)
	if header == "" {
		header = "Authorization"
	}

	apiKey := strings.TrimSpace(auth.APIKey)
	username := strings.TrimSpace(auth.Username)
	password := auth.Password

	enabled := auth.Enabled && (apiKey != "" || (username != "" && password != ""))
	s.authEnabled = enabled
	if !enabled {
		s.apiKey = ""
		s.basicUser = ""
		s.basicPass = ""
		s.authHeader = ""
		return
	}

	s.apiKey = apiKey
	s.basicUser = username
	s.basicPass = password
	s.authHeader = strings.ToLower(header)
}

// SetAuthConfig hot-swaps authentication parameters (used by config watcher).
func (s *Server) SetAuthConfig(auth config.AuthConfig) {
	s.applyAuthConfig(auth)
}

// Start starts the API server
func (s *Server) Start(ctx context.Context) error {
	s.logger.Info("Starting API server", "address", s.httpServer.Addr)

	// Start kill-switch manager background worker
	if s.killSwitch != nil {
		s.killSwitch.Start(ctx)
	}

	errChan := make(chan error, 1)
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	// Wait for context cancellation or error
	select {
	case <-ctx.Done():
		return s.Shutdown(context.Background())
	case err := <-errChan:
		return err
	}
}

// Shutdown gracefully shuts down the API server
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down API server")

	// Stop kill-switch manager
	if s.killSwitch != nil {
		s.killSwitch.Stop()
	}

	return s.httpServer.Shutdown(ctx)
}

// SetCache updates the cache reference used by cache-related handlers.
func (s *Server) SetCache(c cache.Interface) {
	s.cache = c
}

// SetHTTPRateLimiter configures the HTTP rate limiter middleware.
func (s *Server) SetHTTPRateLimiter(rl *ratelimit.Manager) {
	s.rateLimiter = rl
}

// SetLogger updates the server logger reference.
func (s *Server) SetLogger(l *slog.Logger) {
	if l == nil {
		return
	}
	s.logger = l
}

// writeJSON writes a JSON response
func (s *Server) writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		s.logger.Error("Failed to encode JSON response", "error", err)
	}
}

// writeError writes an error response
func (s *Server) writeError(w http.ResponseWriter, statusCode int, message string) {
	s.writeJSON(w, statusCode, ErrorResponse{
		Error:   http.StatusText(statusCode),
		Code:    statusCode,
		Message: message,
	})
}

// parseDuration parses a duration string with default value
func parseDuration(s string, defaultDuration time.Duration) time.Duration {
	if s == "" {
		return defaultDuration
	}

	d, err := time.ParseDuration(s)
	if err != nil {
		return defaultDuration
	}

	return d
}

// getUptime returns the server uptime as a string
func (s *Server) getUptime() string {
	uptime := time.Since(s.startTime)

	hours := int(uptime.Hours())
	minutes := int(uptime.Minutes()) % 60
	seconds := int(uptime.Seconds()) % 60

	if hours > 0 {
		return fmt.Sprintf("%dh%dm%ds", hours, minutes, seconds)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm%ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}
