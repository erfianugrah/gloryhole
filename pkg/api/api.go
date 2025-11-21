package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"glory-hole/pkg/blocklist"
	"glory-hole/pkg/policy"
	"glory-hole/pkg/storage"
)

// Server represents the API server
type Server struct {
	handler    http.Handler
	httpServer *http.Server
	logger     *slog.Logger

	// Dependencies
	storage          storage.Storage
	blocklistManager *blocklist.Manager
	policyEngine     *policy.Engine

	// Metadata
	version   string
	startTime time.Time
}

// Config holds API server configuration
type Config struct {
	ListenAddress    string
	Storage          storage.Storage
	BlocklistManager *blocklist.Manager
	PolicyEngine     *policy.Engine
	Logger           *slog.Logger
	Version          string
}

// New creates a new API server
func New(cfg *Config) *Server {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	s := &Server{
		storage:          cfg.Storage,
		blocklistManager: cfg.BlocklistManager,
		policyEngine:     cfg.PolicyEngine,
		logger:           cfg.Logger,
		version:          cfg.Version,
		startTime:        time.Now(),
	}

	// Setup routes
	mux := http.NewServeMux()

	// Health checks
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/healthz", s.handleHealthz) // Kubernetes liveness probe
	mux.HandleFunc("/readyz", s.handleReadyz)   // Kubernetes readiness probe

	// Statistics
	mux.HandleFunc("/api/stats", s.handleStats)

	// Queries
	mux.HandleFunc("/api/queries", s.handleQueries)

	// Top domains
	mux.HandleFunc("/api/top-domains", s.handleTopDomains)

	// Blocklist management
	mux.HandleFunc("POST /api/blocklist/reload", s.handleBlocklistReload)

	// Policy management
	mux.HandleFunc("GET /api/policies", s.handleGetPolicies)
	mux.HandleFunc("POST /api/policies", s.handleAddPolicy)
	mux.HandleFunc("GET /api/policies/{id}", s.handleGetPolicy)
	mux.HandleFunc("PUT /api/policies/{id}", s.handleUpdatePolicy)
	mux.HandleFunc("DELETE /api/policies/{id}", s.handleDeletePolicy)

	// Apply middleware
	handler := s.loggingMiddleware(mux)
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

// Start starts the API server
func (s *Server) Start(ctx context.Context) error {
	s.logger.Info("Starting API server", "address", s.httpServer.Addr)

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
	return s.httpServer.Shutdown(ctx)
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
