package api

import (
	"embed"
	"html/template"
	"io/fs"
	"net/http"
	"strconv"
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
	templates, err = template.ParseFS(tmplFS, "*.html")
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

	// Template-friendly query data
	type QueryData struct {
		Timestamp      time.Time
		ClientIP       string
		Domain         string
		QueryType      string
		Blocked        bool
		Cached         bool
		ResponseTimeMs int64
	}

	queries := []QueryData{}

	// Get queries from storage
	if s.storage != nil {
		dbQueries, err := s.storage.GetRecentQueries(r.Context(), limit)
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
