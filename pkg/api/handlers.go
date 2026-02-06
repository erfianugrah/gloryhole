package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"glory-hole/pkg/config"
	"glory-hole/pkg/storage"
)

const (
	statusOK            = "ok"
	statusNotConfigured = "not_configured"
	maxTimeSeriesPoints = 720
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

	// Parse 'since' query parameter (default: 24 hours)
	sinceParam := r.URL.Query().Get("since")
	since := parseDuration(sinceParam, 24*time.Hour)
	sinceTime := time.Now().Add(-since)

	sysMetrics := collectSystemMetrics(r.Context())

	var stats *storage.Statistics
	if s.storage != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		storageStats, err := s.storage.GetStatistics(ctx, sinceTime)
		if err != nil {
			s.logger.Error("Failed to get statistics", "error", err)
			s.writeError(w, http.StatusInternalServerError, "Failed to retrieve statistics")
			return
		}
		stats = storageStats
	}

	response := StatsResponse{
		Period:             since.String(),
		Timestamp:          time.Now().Format(time.RFC3339),
		CPUUsagePercent:    sysMetrics.CPUPercent,
		MemoryUsageBytes:   sysMetrics.MemUsed,
		MemoryTotalBytes:   sysMetrics.MemTotal,
		MemoryUsagePercent: sysMetrics.MemPercent,
	}

	if sysMetrics.TemperatureAvailable() {
		response.TemperatureCelsius = sysMetrics.TemperatureC
		response.TemperatureAvailable = true
	}

	if stats != nil {
		response.TotalQueries = stats.TotalQueries
		response.BlockedQueries = stats.BlockedQueries
		response.CachedQueries = stats.CachedQueries
		response.BlockRate = stats.BlockRate
		response.CacheHitRate = stats.CacheHitRate
		response.AvgResponseMs = stats.AvgResponseTimeMs
	}

	s.writeJSON(w, http.StatusOK, response)
}

// handleStatsTimeSeries handles GET /api/stats/timeseries
func (s *Server) handleStatsTimeSeries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	if s.storage == nil {
		s.writeError(w, http.StatusServiceUnavailable, "Storage not available")
		return
	}

	periodDuration, normalizedPeriod := parseTimeSeriesPeriod(r.URL.Query().Get("period"))
	points := parseTimeSeriesPoints(r.URL.Query().Get("points"))

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	series, err := s.storage.GetTimeSeriesStats(ctx, periodDuration, points)
	if err != nil {
		s.logger.Error("Failed to get time-series statistics", "error", err)
		s.writeError(w, http.StatusInternalServerError, "Failed to retrieve time-series statistics")
		return
	}

	response := TimeSeriesResponse{
		Period: normalizedPeriod,
		Points: points,
		Data:   convertTimeSeriesPoints(series),
	}

	s.writeJSON(w, http.StatusOK, response)
}

// handleQueryTypes handles GET /api/stats/query-types
func (s *Server) handleQueryTypes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	if s.storage == nil {
		s.writeError(w, http.StatusServiceUnavailable, "Storage not available")
		return
	}

	limit := 10
	if limitParam := r.URL.Query().Get("limit"); limitParam != "" {
		if parsed, err := strconv.Atoi(limitParam); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	var sinceTime time.Time
	if sinceParam := r.URL.Query().Get("since"); sinceParam != "" {
		sinceDuration := parseDuration(sinceParam, 0)
		if sinceDuration > 0 {
			sinceTime = time.Now().Add(-sinceDuration)
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	stats, err := s.storage.GetQueryTypeStats(ctx, limit, sinceTime)
	if err != nil {
		s.logger.Error("Failed to get query-type stats", "error", err)
		s.writeError(w, http.StatusInternalServerError, "Failed to retrieve query-type statistics")
		return
	}

	response := convertQueryTypeStats(stats, limit)
	s.writeJSON(w, http.StatusOK, response)
}

// handleGetConfig handles GET /api/config
func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	cfg := s.currentConfig()
	if cfg == nil {
		s.writeError(w, http.StatusServiceUnavailable, "Configuration not available")
		return
	}

	response := convertConfigResponse(cfg)
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

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	stage := r.URL.Query().Get("stage")
	action := r.URL.Query().Get("action")
	rule := r.URL.Query().Get("rule")
	source := r.URL.Query().Get("source")

	// Legacy trace filters (used by debugging UI)
	if stage != "" || action != "" || rule != "" || source != "" {
		traceFilter := storage.TraceFilter{
			Stage:  stage,
			Action: action,
			Rule:   rule,
			Source: source,
		}
		queries, err := s.storage.GetQueriesWithTraceFilter(ctx, traceFilter, limit, offset)
		if err != nil {
			s.logger.Error("Failed to get queries", "error", err)
			s.writeError(w, http.StatusInternalServerError, "Failed to retrieve queries")
			return
		}
		s.writeQueriesResponse(w, queries, limit, offset)
		return
	}

	filter := buildQueryFilterFromRequest(r)
	queries, err := s.storage.GetQueriesFiltered(ctx, filter, limit, offset)
	if err != nil {
		s.logger.Error("Failed to get queries", "error", err)
		s.writeError(w, http.StatusInternalServerError, "Failed to retrieve queries")
		return
	}

	s.writeQueriesResponse(w, queries, limit, offset)
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

	// Parse since parameter (optional)
	sinceParam := r.URL.Query().Get("since")
	var sinceTime time.Time
	if sinceParam != "" {
		d := parseDuration(sinceParam, 0)
		if d > 0 {
			sinceTime = time.Now().Add(-d)
		}
	}

	// Get top domains from storage
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	domains, err := s.storage.GetTopDomains(ctx, limit, blocked, sinceTime)
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

	// Clear cached blocklist decisions so new blocklist takes effect immediately
	if s.cache != nil {
		s.cache.ClearBlocklistDecisions()
		s.logger.Info("Cleared blocklist cache entries after reload")
	}

	domains := s.blocklistManager.Size()

	response := BlocklistReloadResponse{
		Status:  "ok",
		Domains: domains,
		Message: "Blocklists reloaded successfully",
	}

	s.writeJSON(w, http.StatusOK, response)
}

// handleCachePurge handles POST /api/cache/purge
func (s *Server) handleCachePurge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Check if cache is available
	if s.cache == nil {
		s.writeError(w, http.StatusServiceUnavailable, "Cache not available")
		return
	}

	// Get stats before clearing
	statsBefore := s.cache.Stats()
	entriesBefore := statsBefore.Entries

	// Clear the cache
	s.cache.Clear()

	s.logger.Info("DNS cache purged", "entries_cleared", entriesBefore)

	response := CachePurgeResponse{
		Status:         "ok",
		Message:        "DNS cache purged successfully",
		EntriesCleared: entriesBefore,
	}

	s.writeJSON(w, http.StatusOK, response)
}

// handleTraceStatistics handles GET /api/traces/stats
func (s *Server) handleTraceStatistics(w http.ResponseWriter, r *http.Request) {
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
	sinceParam := r.URL.Query().Get("since")
	since := time.Now().Add(-24 * time.Hour) // Default to last 24 hours
	if sinceParam != "" {
		if duration, err := time.ParseDuration(sinceParam); err == nil {
			since = time.Now().Add(-duration)
		} else if t, err := time.Parse(time.RFC3339, sinceParam); err == nil {
			since = t
		}
	}

	// Get trace statistics from storage
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	stats, err := s.storage.GetTraceStatistics(ctx, since)
	if err != nil {
		s.logger.Error("Failed to get trace statistics", "error", err)
		s.writeError(w, http.StatusInternalServerError, "Failed to retrieve trace statistics")
		return
	}

	response := convertTraceStatistics(stats)
	s.writeJSON(w, http.StatusOK, response)
}

func parseTimeSeriesPeriod(value string) (time.Duration, string) {
	switch strings.ToLower(value) {
	case "day", "daily":
		return 24 * time.Hour, "day"
	case "week", "weekly":
		return 7 * 24 * time.Hour, "week"
	default:
		return time.Hour, "hour"
	}
}

func parseTimeSeriesPoints(value string) int {
	points := 24
	if value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			points = parsed
		}
	}

	if points < 1 {
		points = 1
	}
	if points > maxTimeSeriesPoints {
		points = maxTimeSeriesPoints
	}

	return points
}

func (s *Server) currentConfig() *config.Config {
	if s.configWatcher != nil {
		return s.configWatcher.Config()
	}
	return s.configSnapshot
}

func (s *Server) writeQueriesResponse(w http.ResponseWriter, queries []*storage.QueryLog, limit, offset int) {
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

func buildQueryFilterFromRequest(r *http.Request) storage.QueryFilter {
	filter := storage.QueryFilter{}
	values := r.URL.Query()

	if domain := strings.TrimSpace(values.Get("domain")); domain != "" {
		filter.Domain = domain
	}

	if qtype := strings.TrimSpace(values.Get("type")); qtype != "" {
		filter.QueryType = qtype
	}

	if clientIP := strings.TrimSpace(values.Get("client")); clientIP != "" {
		filter.ClientIP = clientIP
	}

	if upstream := strings.TrimSpace(values.Get("upstream")); upstream != "" {
		filter.Upstream = upstream
	}

	if responseCode := strings.TrimSpace(values.Get("response_code")); responseCode != "" {
		if code, err := strconv.Atoi(responseCode); err == nil && code > 0 {
			filter.ResponseCode = code
		}
	}

	if status := strings.ToLower(values.Get("status")); status != "" {
		switch status {
		case "blocked":
			filter.Blocked = boolPtr(true)
		case "allowed":
			filter.Blocked = boolPtr(false)
		case "cached":
			filter.Cached = boolPtr(true)
		}
	}

	if start, ok := parseTimeParamValue(values.Get("start")); ok {
		filter.Start = start
	}

	if end, ok := parseTimeParamValue(values.Get("end")); ok {
		filter.End = end
	}

	return filter
}

func parseTimeParamValue(value string) (time.Time, bool) {
	if value == "" {
		return time.Time{}, false
	}

	if ts, err := time.Parse(time.RFC3339, value); err == nil {
		return ts, true
	}

	if dur, err := time.ParseDuration(value); err == nil {
		if dur > 0 {
			return time.Now().Add(-dur), true
		}
		return time.Now().Add(dur), true
	}

	return time.Time{}, false
}

func boolPtr(v bool) *bool {
	b := v
	return &b
}
