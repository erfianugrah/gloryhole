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

	"github.com/dustin/go-humanize"

	"glory-hole/pkg/storage"
)

//go:embed ui/templates/*
var templatesFS embed.FS

//go:embed ui/static/*
var staticFS embed.FS

// UI templates grouped per page/partial to prevent block collisions.
var (
	dashboardTemplate       *template.Template
	queriesTemplate         *template.Template
	policiesTemplate        *template.Template
	settingsTemplate        *template.Template
	clientsTemplate         *template.Template
	blocklistsTemplate      *template.Template
	statsPartialTemplate    *template.Template
	queriesPartialTemplate  *template.Template
	topDomainsTemplate      *template.Template
	policiesPartialTemplate *template.Template
	clientsPartialTemplate  *template.Template
)

const dnsRcodeNameError = 3

func formatVersionLabel(version string) string {
	v := strings.TrimSpace(version)
	if v == "" {
		return "vdev"
	}
	if strings.HasPrefix(strings.ToLower(v), "v") {
		return v
	}
	return "v" + v
}

func (s *Server) uiVersion() string {
	return formatVersionLabel(s.version)
}

// initTemplates initializes the HTML templates
func initTemplates() error {
	var err error

	tmplFS, err := fs.Sub(templatesFS, "ui/templates")
	if err != nil {
		return err
	}

	funcMap := template.FuncMap{
		"lower": strings.ToLower,
		"json": func(v interface{}) string {
			b, _ := json.Marshal(v)
			return string(b)
		},
		"add": func(a, b int) int {
			return a + b
		},
		"join":       strings.Join,
		"humanBytes": humanize.Bytes,
	}

	baseTemplate, err := template.New("base.html").Funcs(funcMap).ParseFS(tmplFS, "base.html")
	if err != nil {
		return err
	}

	parsePage := func(name string) (*template.Template, error) {
		clone, cloneErr := baseTemplate.Clone()
		if cloneErr != nil {
			return nil, cloneErr
		}
		if _, parseErr := clone.ParseFS(tmplFS, name); parseErr != nil {
			return nil, parseErr
		}
		return clone, nil
	}

	if dashboardTemplate, err = parsePage("dashboard.html"); err != nil {
		return err
	}
	if queriesTemplate, err = parsePage("queries.html"); err != nil {
		return err
	}
	if policiesTemplate, err = parsePage("policies.html"); err != nil {
		return err
	}
	if settingsTemplate, err = parsePage("settings.html"); err != nil {
		return err
	}
	if clientsTemplate, err = parsePage("clients.html"); err != nil {
		return err
	}
	if _, err = clientsTemplate.ParseFS(tmplFS, "clients_partial.html"); err != nil {
		return err
	}
	if blocklistsTemplate, err = parsePage("blocklists.html"); err != nil {
		return err
	}

	parseStandalone := func(name string) (*template.Template, error) {
		return template.New(name).Funcs(funcMap).ParseFS(tmplFS, name)
	}

	if statsPartialTemplate, err = parseStandalone("stats_partial.html"); err != nil {
		return err
	}
	if queriesPartialTemplate, err = parseStandalone("queries_partial.html"); err != nil {
		return err
	}
	if topDomainsTemplate, err = parseStandalone("top_domains_partial.html"); err != nil {
		return err
	}
	if policiesPartialTemplate, err = parseStandalone("policies_partial.html"); err != nil {
		return err
	}
	if clientsPartialTemplate, err = parseStandalone("clients_partial.html"); err != nil {
		return err
	}

	return nil
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
		Version: s.uiVersion(),
		Uptime:  s.getUptime(),
	}

	if err := dashboardTemplate.ExecuteTemplate(w, "dashboard.html", data); err != nil {
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
		Version: s.uiVersion(),
	}

	if err := queriesTemplate.ExecuteTemplate(w, "queries.html", data); err != nil {
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

	sysMetrics := collectSystemMetrics(r.Context())
	stats := StatsResponse{
		CPUUsagePercent:    sysMetrics.CPUPercent,
		MemoryUsageBytes:   sysMetrics.MemUsed,
		MemoryTotalBytes:   sysMetrics.MemTotal,
		MemoryUsagePercent: sysMetrics.MemPercent,
		Period:             since.String(),
		Timestamp:          time.Now().Format(time.RFC3339),
	}

	if sysMetrics.TemperatureAvailable() {
		stats.TemperatureCelsius = sysMetrics.TemperatureC
		stats.TemperatureAvailable = true
	}

	if s.storage != nil {
		dbStats, err := s.storage.GetStatistics(r.Context(), time.Now().Add(-since))
		if err == nil {
			stats.TotalQueries = dbStats.TotalQueries
			stats.BlockedQueries = dbStats.BlockedQueries
			stats.CachedQueries = dbStats.CachedQueries
			stats.BlockRate = dbStats.BlockRate
			stats.CacheHitRate = dbStats.CacheHitRate
			stats.AvgResponseMs = dbStats.AvgResponseTimeMs
		}
	}

	if err := statsPartialTemplate.ExecuteTemplate(w, "stats_partial.html", stats); err != nil {
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
		Version: s.uiVersion(),
	}

	if err := policiesTemplate.ExecuteTemplate(w, "policies.html", data); err != nil {
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

	if err := settingsTemplate.ExecuteTemplate(w, "settings.html", data); err != nil {
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
		Blocked bool
	}{
		Domains: domains,
		Blocked: blocked,
	}

	if err := topDomainsTemplate.ExecuteTemplate(w, "top_domains_partial.html", data); err != nil {
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
		ResponseCode   int
		Status         string
		StatusLabel    string
		BlockTrace     []storage.BlockTraceEntry
		ResponseTimeMs float64
		UpstreamTimeMs float64
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
				status, label := classifyQuery(q)
				queries = append(queries, QueryData{
					Timestamp:      q.Timestamp,
					ClientIP:       q.ClientIP,
					Domain:         q.Domain,
					QueryType:      q.QueryType,
					Blocked:        q.Blocked,
					Cached:         q.Cached,
					ResponseCode:   q.ResponseCode,
					Status:         status,
					StatusLabel:    label,
					BlockTrace:     q.BlockTrace,
					ResponseTimeMs: q.ResponseTimeMs,
					UpstreamTimeMs: q.UpstreamTimeMs,
				})
			}
		}
	}

	data := struct {
		Queries []QueryData
	}{
		Queries: queries,
	}

	if err := queriesPartialTemplate.ExecuteTemplate(w, "queries_partial.html", data); err != nil {
		s.logger.Error("Failed to render queries partial", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// handleClientsPartial renders the clients table as an HTML fragment for HTMX.
func (s *Server) handleClientsPartial(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	limit := parsePositiveInt(r.URL.Query().Get("limit"), defaultClientPageSize, maxClientPageSize)
	offset := parseNonNegativeInt(r.URL.Query().Get("offset"), 0)
	search := strings.TrimSpace(r.URL.Query().Get("search"))

	clients := []*storage.ClientSummary{}

	if s.storage != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		if search != "" {
			ctx = storage.WithClientSearch(ctx, search)
		}

		if records, err := s.storage.GetClientSummaries(ctx, limit, offset); err == nil {
			clients = records
		} else {
			s.logger.Error("Failed to list client summaries", "error", err)
		}
	}

	data := struct {
		Clients []*storage.ClientSummary
	}{
		Clients: clients,
	}

	meta := map[string]any{
		"limit":    limit,
		"offset":   offset,
		"count":    len(clients),
		"has_more": len(clients) >= limit,
	}
	if search != "" {
		meta["search"] = search
	}
	if payload, err := json.Marshal(map[string]any{"clients-page-meta": meta}); err == nil {
		w.Header().Set("HX-Trigger", string(payload))
	}

	if err := clientsPartialTemplate.ExecuteTemplate(w, "clients_partial.html", data); err != nil {
		s.logger.Error("Failed to render clients partial", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

func classifyQuery(q *storage.QueryLog) (string, string) {
	if q == nil {
		return "allowed", "Allowed"
	}
	switch {
	case q.Blocked:
		return "blocked", "Blocked"
	case q.ResponseCode == dnsRcodeNameError:
		return "nxdomain", "NXDOMAIN"
	case q.Cached:
		return "cached", "Cached"
	default:
		return "allowed", "Allowed"
	}
}
