package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/miekg/dns"
)

type blocklistSummaryResponse struct {
	Enabled        bool           `json:"enabled"`
	AutoUpdate     bool           `json:"auto_update"`
	UpdateInterval string         `json:"update_interval"`
	TotalDomains   int            `json:"total_domains"`
	ExactDomains   int            `json:"exact_domains"`
	PatternStats   map[string]int `json:"pattern_stats"`
	LastUpdated    string         `json:"last_updated,omitempty"`
	Sources        []string       `json:"sources"`
}

type blocklistsPageData struct {
	Version string
	Summary blocklistSummaryResponse
}

func (s *Server) handleBlocklistsPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	summary := s.buildBlocklistSummary(r.Context())
	data := blocklistsPageData{
		Version: s.uiVersion(),
		Summary: summary,
	}

	if err := blocklistsTemplate.ExecuteTemplate(w, "blocklists.html", data); err != nil {
		s.logger.Error("Failed to render blocklists template", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (s *Server) handleGetBlocklists(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	summary := s.buildBlocklistSummary(r.Context())
	s.writeJSON(w, http.StatusOK, summary)
}

func (s *Server) handleCheckBlocklist(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	domain := strings.TrimSpace(r.URL.Query().Get("domain"))
	if domain == "" {
		s.writeError(w, http.StatusBadRequest, "Domain is required")
		return
	}

	if s.blocklistManager == nil {
		s.writeJSON(w, http.StatusOK, map[string]any{
			"domain":  domain,
			"blocked": false,
			"enabled": false,
		})
		return
	}

	normalized := normalizeDomain(domain)
	fqdn := dns.Fqdn(normalized)
	blocked := s.blocklistManager.IsBlocked(normalized) || s.blocklistManager.IsBlocked(fqdn)
	s.writeJSON(w, http.StatusOK, map[string]any{
		"domain":  normalized,
		"blocked": blocked,
		"enabled": true,
	})
}

func (s *Server) buildBlocklistSummary(ctx context.Context) blocklistSummaryResponse {
	cfg := s.currentConfig()
	summary := blocklistSummaryResponse{
		PatternStats: make(map[string]int),
		Sources:      []string{},
	}

	if cfg != nil {
		summary.Enabled = cfg.Server.EnableBlocklist
		summary.AutoUpdate = cfg.AutoUpdateBlocklists
		if cfg.UpdateInterval > 0 {
			summary.UpdateInterval = cfg.UpdateInterval.String()
		}
		summary.Sources = append(summary.Sources, cfg.Blocklists...)
	}

	if s.blocklistManager != nil {
		stats := s.blocklistManager.Stats()
		summary.ExactDomains = stats["exact"]
		summary.TotalDomains = stats["total"]
		summary.PatternStats["exact"] = stats["pattern_exact"]
		summary.PatternStats["wildcard"] = stats["pattern_wildcard"]
		summary.PatternStats["regex"] = stats["pattern_regex"]

		if ts := s.blocklistManager.LastUpdated(); !ts.IsZero() {
			summary.LastUpdated = ts.UTC().Format(time.RFC3339)
		}
	}

	return summary
}

func normalizeDomain(value string) string {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if trimmed == "" {
		return ""
	}
	return strings.TrimSuffix(trimmed, ".")
}
