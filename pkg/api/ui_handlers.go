package api

import (
	"context"
	"embed"
	"encoding/json"
	"html/template"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"time"
)

//go:embed ui/templates/*
var templatesFS embed.FS

//go:embed ui/static/*
var staticFS embed.FS

// UI templates
var templates *template.Template

// initTemplates initializes the HTML templates
func initTemplates() error {
	var err error
	// Parse templates from embedded filesystem
	tmplFS, err := fs.Sub(templatesFS, "ui/templates")
	if err != nil {
		return err
	}

	// Add template functions
	funcMap := template.FuncMap{
		"lower": func(s string) string {
			return strings.ToLower(s)
		},
		"json": func(v interface{}) string {
			b, _ := json.Marshal(v)
			return string(b)
		},
		"add": func(a, b int) int {
			return a + b
		},
		"join": strings.Join,
		"versionLabel": func(version string) string {
			v := strings.TrimSpace(version)
			if v == "" {
				return "vdev"
			}
			if strings.HasPrefix(strings.ToLower(v), "v") {
				return v
			}
			return "v" + v
		},
	}

	templates, err = template.New("").Funcs(funcMap).ParseFS(tmplFS, "*.html")
	return err
}

// getStaticFS returns the static files filesystem
func getStaticFS() (http.FileSystem, error) {
	staticFiles, err := fs.Sub(staticFS, "ui/static")
	if err != nil {
		return nil, err
	}
	return http.FS(staticFiles), nil
}

// handleDashboard serves the main dashboard page
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	data := struct {
		Version string
		Uptime  string
	}{
		Version: s.version,
		Uptime:  s.getUptime(),
	}

	if err := templates.ExecuteTemplate(w, "dashboard.html", data); err != nil {
		s.logger.Error("Failed to render dashboard template", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// handleQueriesPage serves the queries log page
func (s *Server) handleQueriesPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	data := struct {
		Version string
	}{
		Version: s.version,
	}

	if err := templates.ExecuteTemplate(w, "queries.html", data); err != nil {
		s.logger.Error("Failed to render queries template", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// handleStatsPartial returns stats as HTML fragment for HTMX
func (s *Server) handleStatsPartial(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get statistics
	since := parseDuration(r.URL.Query().Get("since"), 24*time.Hour)

	var stats StatsResponse
	if s.storage != nil {
		dbStats, err := s.storage.GetStatistics(r.Context(), time.Now().Add(-since))
		if err == nil {
			stats = StatsResponse{
				TotalQueries:   dbStats.TotalQueries,
				BlockedQueries: dbStats.BlockedQueries,
				CachedQueries:  dbStats.CachedQueries,
				BlockRate:      dbStats.BlockRate,
				CacheHitRate:   dbStats.CacheHitRate,
				AvgResponseMs:  dbStats.AvgResponseTimeMs,
				Period:         since.String(),
				Timestamp:      time.Now().Format(time.RFC3339),
			}
		}
	}

	if err := templates.ExecuteTemplate(w, "stats_partial.html", stats); err != nil {
		s.logger.Error("Failed to render stats partial", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// handlePoliciesPage serves the policies management page
func (s *Server) handlePoliciesPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	data := struct {
		Version string
	}{
		Version: s.version,
	}

	if err := templates.ExecuteTemplate(w, "policies.html", data); err != nil {
		s.logger.Error("Failed to render policies template", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// handleSettingsPage serves the settings/configuration page
func (s *Server) handleSettingsPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cfg := s.currentConfig()
	if cfg == nil {
		s.logger.Error("Configuration not available for settings page")
		http.Error(w, "Configuration not available", http.StatusServiceUnavailable)
		return
	}

	data := s.newSettingsPageData(cfg)

	if err := templates.ExecuteTemplate(w, "settings.html", data); err != nil {
		s.logger.Error("Failed to render settings template", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// handleTopDomainsPartial returns top domains as HTML fragment for HTMX
func (s *Server) handleTopDomainsPartial(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse parameters
	limitParam := r.URL.Query().Get("limit")
	limit := 10 // Default
	if limitParam != "" {
		if l, err := strconv.Atoi(limitParam); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	blockedParam := r.URL.Query().Get("blocked")
	blocked := blockedParam == "true"

	// Template-friendly domain data
	type DomainData struct {
		Domain     string
		Queries    int64
		Percentage float64
	}

	domains := []DomainData{}

	// Get domains from storage
	if s.storage != nil {
		dbDomains, err := s.storage.GetTopDomains(r.Context(), limit, blocked)
		if err == nil && len(dbDomains) > 0 {
			maxQueries := dbDomains[0].QueryCount
			for _, d := range dbDomains {
				percentage := float64(d.QueryCount) / float64(maxQueries) * 100
				domains = append(domains, DomainData{
					Domain:     d.Domain,
					Queries:    d.QueryCount,
					Percentage: percentage,
				})
			}
		}
	}

	data := struct {
		Domains []DomainData
	}{
		Domains: domains,
	}

	if err := templates.ExecuteTemplate(w, "top_domains_partial.html", data); err != nil {
		s.logger.Error("Failed to render top domains partial", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// handleQueriesPartial returns queries as HTML fragment for HTMX
func (s *Server) handleQueriesPartial(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse limit parameter
	limitParam := r.URL.Query().Get("limit")
	limit := 20 // Default for UI
	if limitParam != "" {
		if l, err := strconv.Atoi(limitParam); err == nil && l > 0 && l <= 1000 {
			limit = l
		}
	}

	// Parse offset parameter
	offsetParam := r.URL.Query().Get("offset")
	offset := 0 // Default offset
	if offsetParam != "" {
		if o, err := strconv.Atoi(offsetParam); err == nil && o >= 0 {
			offset = o
		}
	}

	// Template-friendly query data
	type QueryData struct {
		Timestamp      time.Time
		ClientIP       string
		Domain         string
		QueryType      string
		Blocked        bool
		Cached         bool
		ResponseTimeMs float64
	}

	queries := []QueryData{}

	// Get queries from storage
	if s.storage != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		filter := buildQueryFilterFromRequest(r)
		dbQueries, err := s.storage.GetQueriesFiltered(ctx, filter, limit, offset)
		if err == nil {
			for _, q := range dbQueries {
				queries = append(queries, QueryData{
					Timestamp:      q.Timestamp,
					ClientIP:       q.ClientIP,
					Domain:         q.Domain,
					QueryType:      q.QueryType,
					Blocked:        q.Blocked,
					Cached:         q.Cached,
					ResponseTimeMs: q.ResponseTimeMs,
				})
			}
		}
	}

	data := struct {
		Queries []QueryData
	}{
		Queries: queries,
	}

	if err := templates.ExecuteTemplate(w, "queries_partial.html", data); err != nil {
		s.logger.Error("Failed to render queries partial", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}
