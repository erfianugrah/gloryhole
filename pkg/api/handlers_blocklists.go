package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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

func (s *Server) handleBlocklistsPage(w http.ResponseWriter, r *http.Request) {
	s.serveAstroPage(w, r, "blocklists/index.html")
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

// handleUpdateBlocklistSources handles PUT /api/config/blocklists
func (s *Server) handleUpdateBlocklistSources(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	cfg, err := s.mutableConfig()
	if err != nil {
		s.writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxConfigPayloadSize)

	var req struct {
		Sources []string `json:"sources"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid JSON: %v", err))
		return
	}

	// Validate and deduplicate URLs
	seen := make(map[string]struct{})
	sources := make([]string, 0, len(req.Sources))
	for _, raw := range req.Sources {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		// URL validation — require http/https scheme to prevent SSRF via file:// etc.
		parsed, err := url.Parse(trimmed)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			s.writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid URL %q: must use http or https scheme", trimmed))
			return
		}
		lower := strings.ToLower(trimmed)
		if _, ok := seen[lower]; ok {
			continue
		}
		seen[lower] = struct{}{}
		sources = append(sources, trimmed)
	}

	updated := *cfg
	updated.Blocklists = sources

	if !s.persistConfigSection(w, r, &updated, "", "", cfg) {
		return
	}

	s.logger.Info("Blocklist sources updated via API", "count", len(sources))

	s.writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"message": "Blocklist sources updated",
		"sources": sources,
	})
}

func normalizeDomain(value string) string {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if trimmed == "" {
		return ""
	}
	return strings.TrimSuffix(trimmed, ".")
}
