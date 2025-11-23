package api

import (
	"context"
	"net/http"
	"strconv"
	"time"
)

const (
	statusOK            = "ok"
	statusNotConfigured = "not_configured"
)

// handleHealth handles GET /api/health
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	response := HealthResponse{
		Status:  "ok",
		Uptime:  s.getUptime(),
		Version: s.version,
	}

	s.writeJSON(w, http.StatusOK, response)
}

// handleLiveness handles GET /health
// This endpoint indicates if the application is running and should be restarted if not responding
func (s *Server) handleLiveness(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Simple liveness check - if we can respond, we're alive
	response := LivenessResponse{
		Status: "alive",
	}

	s.writeJSON(w, http.StatusOK, response)
}

// handleReadiness handles GET /ready
// This endpoint indicates if the application is ready to accept traffic
func (s *Server) handleReadiness(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Check if critical dependencies are available
	checks := make(map[string]string)
	ready := true

	// Check storage (optional - degraded mode allowed)
	if s.storage != nil {
		// Try a quick ping to storage
		ctx, cancel := context.WithTimeout(r.Context(), 1*time.Second)
		defer cancel()

		// Just try to get statistics to verify storage is responsive
		if _, err := s.storage.GetStatistics(ctx, time.Now().Add(-1*time.Hour)); err != nil {
			checks["storage"] = "degraded"
			// Don't mark as not ready - we can operate without storage
		} else {
			checks["storage"] = statusOK
		}
	} else {
		checks["storage"] = statusNotConfigured
	}

	// Check blocklist manager (optional)
	if s.blocklistManager != nil {
		if s.blocklistManager.Size() > 0 {
			checks["blocklist"] = statusOK
		} else {
			checks["blocklist"] = "empty"
			// Don't mark as not ready - we can operate without blocklists
		}
	} else {
		checks["blocklist"] = statusNotConfigured
	}

	// Check policy engine (optional)
	if s.policyEngine != nil {
		checks["policy_engine"] = statusOK
	} else {
		checks["policy_engine"] = statusNotConfigured
	}

	status := "ready"
	statusCode := http.StatusOK

	if !ready {
		status = "not_ready"
		statusCode = http.StatusServiceUnavailable
	}

	response := ReadinessResponse{
		Status: status,
		Checks: checks,
	}

	s.writeJSON(w, statusCode, response)
}

// handleStats handles GET /api/stats
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Check if storage is available
	if s.storage == nil {
		s.writeError(w, http.StatusServiceUnavailable, "Storage not available")
		return
	}

	// Parse 'since' query parameter (default: 24 hours)
	sinceParam := r.URL.Query().Get("since")
	since := parseDuration(sinceParam, 24*time.Hour)
	sinceTime := time.Now().Add(-since)

	// Get statistics from storage
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	stats, err := s.storage.GetStatistics(ctx, sinceTime)
	if err != nil {
		s.logger.Error("Failed to get statistics", "error", err)
		s.writeError(w, http.StatusInternalServerError, "Failed to retrieve statistics")
		return
	}

	response := StatsResponse{
		TotalQueries:   stats.TotalQueries,
		BlockedQueries: stats.BlockedQueries,
		CachedQueries:  stats.CachedQueries,
		BlockRate:      stats.BlockRate,
		CacheHitRate:   stats.CacheHitRate,
		AvgResponseMs:  stats.AvgResponseTimeMs,
		Period:         since.String(),
		Timestamp:      time.Now().Format(time.RFC3339),
	}

	s.writeJSON(w, http.StatusOK, response)
}

// handleQueries handles GET /api/queries
func (s *Server) handleQueries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Check if storage is available
	if s.storage == nil {
		s.writeError(w, http.StatusServiceUnavailable, "Storage not available")
		return
	}

	// Parse query parameters
	limitParam := r.URL.Query().Get("limit")
	limit := 100 // Default
	if limitParam != "" {
		if l, err := strconv.Atoi(limitParam); err == nil && l > 0 && l <= 1000 {
			limit = l
		}
	}

	offsetParam := r.URL.Query().Get("offset")
	offset := 0
	if offsetParam != "" {
		if o, err := strconv.Atoi(offsetParam); err == nil && o >= 0 {
			offset = o
		}
	}

	// Get recent queries from storage
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	queries, err := s.storage.GetRecentQueries(ctx, limit, offset)
	if err != nil {
		s.logger.Error("Failed to get queries", "error", err)
		s.writeError(w, http.StatusInternalServerError, "Failed to retrieve queries")
		return
	}

	// Convert to response format
	queryResponses := make([]QueryResponse, 0, len(queries))
	for _, q := range queries {
		queryResponses = append(queryResponses, convertQueryLog(q))
	}

	response := QueriesResponse{
		Queries: queryResponses,
		Total:   len(queryResponses),
		Limit:   limit,
		Offset:  offset,
	}

	s.writeJSON(w, http.StatusOK, response)
}

// handleTopDomains handles GET /api/top-domains
func (s *Server) handleTopDomains(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Check if storage is available
	if s.storage == nil {
		s.writeError(w, http.StatusServiceUnavailable, "Storage not available")
		return
	}

	// Parse query parameters
	limitParam := r.URL.Query().Get("limit")
	limit := 10 // Default
	if limitParam != "" {
		if l, err := strconv.Atoi(limitParam); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	blockedParam := r.URL.Query().Get("blocked")
	blocked := blockedParam == "true"

	// Get top domains from storage
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	domains, err := s.storage.GetTopDomains(ctx, limit, blocked)
	if err != nil {
		s.logger.Error("Failed to get top domains", "error", err)
		s.writeError(w, http.StatusInternalServerError, "Failed to retrieve top domains")
		return
	}

	// Convert to response format
	domainResponses := make([]DomainStatsResponse, 0, len(domains))
	for _, d := range domains {
		domainResponses = append(domainResponses, convertDomainStats(d))
	}

	response := TopDomainsResponse{
		Domains: domainResponses,
		Limit:   limit,
	}

	s.writeJSON(w, http.StatusOK, response)
}

// handleBlocklistReload handles POST /api/blocklist/reload
func (s *Server) handleBlocklistReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Check if blocklist manager is available
	if s.blocklistManager == nil {
		s.writeError(w, http.StatusServiceUnavailable, "Blocklist manager not available")
		return
	}

	// Reload blocklists
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	if err := s.blocklistManager.Update(ctx); err != nil {
		s.logger.Error("Failed to reload blocklists", "error", err)
		s.writeError(w, http.StatusInternalServerError, "Failed to reload blocklists")
		return
	}

	domains := s.blocklistManager.Size()

	response := BlocklistReloadResponse{
		Status:  "ok",
		Domains: domains,
		Message: "Blocklists reloaded successfully",
	}

	s.writeJSON(w, http.StatusOK, response)
}
