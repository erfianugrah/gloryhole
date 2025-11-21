package api

import (
	"context"
	"net/http"
	"strconv"
	"time"
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

	queries, err := s.storage.GetRecentQueries(ctx, limit)
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
	blocked := false
	if blockedParam == "true" {
		blocked = true
	}

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
