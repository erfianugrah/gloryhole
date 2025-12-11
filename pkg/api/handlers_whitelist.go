package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"glory-hole/pkg/config"
	"glory-hole/pkg/pattern"
)

// WhitelistEntry represents a single whitelist entry
type WhitelistEntry struct {
	Domain  string `json:"domain"`
	Pattern bool   `json:"pattern"` // true if wildcard/regex pattern, false if exact match
}

// WhitelistResponse represents the list of whitelisted domains
type WhitelistResponse struct {
	Entries []WhitelistEntry `json:"entries"`
	Total   int              `json:"total"`
}

// WhitelistAddRequest represents a request to add domain(s) to whitelist
type WhitelistAddRequest struct {
	Domains []string `json:"domains"`
}

// handleGetWhitelist returns all whitelisted domains
func (s *Server) handleGetWhitelist(w http.ResponseWriter, r *http.Request) {
	entries := make([]WhitelistEntry, 0)

	// Get whitelist from config (source of truth)
	cfg := s.currentConfig()
	if cfg != nil {
		for _, domain := range cfg.Whitelist {
			entries = append(entries, WhitelistEntry{
				Domain:  domain,
				Pattern: isPattern(domain),
			})
		}
	}

	// Sort entries alphabetically
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Domain < entries[j].Domain
	})

	// Check if request wants HTML (from HTMX or browser)
	if r.Header.Get("HX-Request") == "true" || r.Header.Get("Accept") == "text/html" {
		// Return HTML partial
		data := struct {
			Entries []WhitelistEntry
		}{
			Entries: entries,
		}

		if err := whitelistPartialTemplate.ExecuteTemplate(w, "whitelist_partial.html", data); err != nil {
			s.logger.Error("Failed to render whitelist partial", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}

	// Return JSON
	s.writeJSON(w, http.StatusOK, WhitelistResponse{
		Entries: entries,
		Total:   len(entries),
	})
}

// handleAddWhitelist adds domain(s) to the whitelist
func (s *Server) handleAddWhitelist(w http.ResponseWriter, r *http.Request) {
	if s.dnsHandler == nil {
		s.writeError(w, http.StatusInternalServerError, "DNS handler not configured")
		return
	}

	// Parse request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Failed to read request body")
		return
	}

	var req WhitelistAddRequest
	if err := json.Unmarshal(body, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	if len(req.Domains) == 0 {
		s.writeError(w, http.StatusBadRequest, "No domains provided")
		return
	}

	// Separate exact matches from patterns
	exactDomains := make([]string, 0)
	patternDomains := make([]string, 0)

	for _, domain := range req.Domains {
		domain = strings.TrimSpace(domain)
		if domain == "" {
			continue
		}

		// Check if it's a pattern (contains * or regex characters)
		if isPattern(domain) {
			patternDomains = append(patternDomains, domain)
		} else {
			// Ensure FQDN format (trailing dot)
			if !strings.HasSuffix(domain, ".") {
				domain = domain + "."
			}
			exactDomains = append(exactDomains, domain)
		}
	}

	// Add exact matches
	if len(exactDomains) > 0 {
		wl := s.dnsHandler.Whitelist.Load()
		newWhitelist := make(map[string]struct{})

		// Copy existing entries
		if wl != nil {
			for k, v := range *wl {
				newWhitelist[k] = v
			}
		}

		// Add new entries
		for _, domain := range exactDomains {
			newWhitelist[domain] = struct{}{}
		}

		s.dnsHandler.Whitelist.Store(&newWhitelist)
	}

	// Add patterns - we'll rebuild the matcher from config after persistence
	// For now just validate patterns
	if len(patternDomains) > 0 {
		// Validate patterns by trying to create matcher
		_, err := pattern.NewMatcher(patternDomains)
		if err != nil {
			s.logger.Error("Failed to validate pattern", "error", err)
			s.writeError(w, http.StatusBadRequest, "Invalid pattern: "+err.Error())
			return
		}
	}

	// Persist to config - this will collect from runtime and save
	if err := s.persistWhitelistConfig(func(cfg *config.Config) error {
		// Merge with existing config entries
		existingSet := make(map[string]struct{})
		for _, d := range cfg.Whitelist {
			existingSet[d] = struct{}{}
		}

		// Add exact domains (without trailing dot for config)
		for _, d := range exactDomains {
			existingSet[strings.TrimSuffix(d, ".")] = struct{}{}
		}

		// Add patterns
		for _, p := range patternDomains {
			existingSet[p] = struct{}{}
		}

		// Convert back to slice
		newWhitelist := make([]string, 0, len(existingSet))
		for d := range existingSet {
			newWhitelist = append(newWhitelist, d)
		}
		sort.Strings(newWhitelist)

		cfg.Whitelist = newWhitelist
		return nil
	}); err != nil {
		s.logger.Error("Failed to persist whitelist to config", "error", err)
		// Don't fail the request - the runtime update succeeded
	} else {
		// After successful persistence, rebuild pattern matcher from config
		cfg := s.currentConfig()
		if cfg != nil && len(patternDomains) > 0 {
			// Rebuild patterns from config
			patterns := make([]string, 0)
			for _, entry := range cfg.Whitelist {
				if isPattern(entry) {
					patterns = append(patterns, entry)
				}
			}
			if len(patterns) > 0 {
				matcher, err := pattern.NewMatcher(patterns)
				if err != nil {
					s.logger.Error("Failed to rebuild pattern matcher", "error", err)
				} else {
					s.dnsHandler.WhitelistPatterns.Store(matcher)
				}
			}
		}
	}

	// Log the action
	s.logger.Info("Added domains to whitelist",
		"exact_count", len(exactDomains),
		"pattern_count", len(patternDomains),
		"user", "admin") // TODO: Get actual user from session

	// Return updated list
	s.handleGetWhitelist(w, r)
}

// handleRemoveWhitelist removes a domain from the whitelist
func (s *Server) handleRemoveWhitelist(w http.ResponseWriter, r *http.Request) {
	if s.dnsHandler == nil {
		s.writeError(w, http.StatusInternalServerError, "DNS handler not configured")
		return
	}

	domain := r.PathValue("domain")
	if domain == "" {
		s.writeError(w, http.StatusBadRequest, "Domain parameter required")
		return
	}

	removed := false

	// Check if it's a pattern
	if isPattern(domain) {
		// For patterns, we'll remove from config and rebuild the matcher
		// Mark as tentatively removed (will verify in config)
		removed = true
	} else {
		// Remove from exact matches
		// Ensure FQDN format
		if !strings.HasSuffix(domain, ".") {
			domain = domain + "."
		}

		wl := s.dnsHandler.Whitelist.Load()
		if wl != nil {
			if _, exists := (*wl)[domain]; exists {
				newWhitelist := make(map[string]struct{})
				for k, v := range *wl {
					if k != domain {
						newWhitelist[k] = v
					}
				}
				s.dnsHandler.Whitelist.Store(&newWhitelist)
				removed = true
			}
		}
	}

	if !removed {
		s.writeError(w, http.StatusNotFound, "Domain not found in whitelist")
		return
	}

	// Persist to config - remove from config list
	if err := s.persistWhitelistConfig(func(cfg *config.Config) error {
		// Normalize domain for comparison
		domainToRemove := strings.TrimSuffix(domain, ".")

		// Filter out the removed domain
		newWhitelist := make([]string, 0, len(cfg.Whitelist))
		actuallyRemoved := false
		for _, d := range cfg.Whitelist {
			if d != domainToRemove {
				newWhitelist = append(newWhitelist, d)
			} else {
				actuallyRemoved = true
			}
		}

		if !actuallyRemoved {
			return fmt.Errorf("domain not found in config")
		}

		cfg.Whitelist = newWhitelist
		return nil
	}); err != nil {
		s.logger.Error("Failed to persist whitelist to config", "error", err)
		if strings.Contains(err.Error(), "not found") {
			removed = false
		}
	} else {
		// After successful persistence, rebuild pattern matcher if it was a pattern
		if isPattern(domain) {
			cfg := s.currentConfig()
			if cfg != nil {
				// Rebuild patterns from config
				patterns := make([]string, 0)
				for _, entry := range cfg.Whitelist {
					if isPattern(entry) {
						patterns = append(patterns, entry)
					}
				}
				if len(patterns) > 0 {
					matcher, err := pattern.NewMatcher(patterns)
					if err != nil {
						s.logger.Error("Failed to rebuild pattern matcher", "error", err)
					} else {
						s.dnsHandler.WhitelistPatterns.Store(matcher)
					}
				} else {
					s.dnsHandler.WhitelistPatterns.Store(nil)
				}
			}
		}
	}

	// Log the action
	s.logger.Info("Removed domain from whitelist",
		"domain", domain,
		"user", "admin") // TODO: Get actual user from session

	// Return updated list
	s.handleGetWhitelist(w, r)
}

// handleBulkImportWhitelist imports multiple domains at once
func (s *Server) handleBulkImportWhitelist(w http.ResponseWriter, r *http.Request) {
	if s.dnsHandler == nil {
		s.writeError(w, http.StatusInternalServerError, "DNS handler not configured")
		return
	}

	// Read text from request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Failed to read request body")
		return
	}

	// Parse domains (one per line, skip empty lines and comments)
	text := string(body)
	lines := strings.Split(text, "\n")
	domains := make([]string, 0)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		domains = append(domains, line)
	}

	if len(domains) == 0 {
		s.writeError(w, http.StatusBadRequest, "No valid domains found")
		return
	}

	// Use the regular add handler logic
	req := WhitelistAddRequest{Domains: domains}
	reqJSON, _ := json.Marshal(req)
	r.Body = io.NopCloser(strings.NewReader(string(reqJSON)))

	s.handleAddWhitelist(w, r)
}

// persistWhitelistConfig persists whitelist changes to config file
func (s *Server) persistWhitelistConfig(mutator func(cfg *config.Config) error) error {
	if s.configPath == "" {
		// In tests or ephemeral runs without a config file, keep whitelist in-memory only.
		return nil
	}
	cfg := s.currentConfig()
	if cfg == nil {
		return fmt.Errorf("no config available")
	}

	// Clone config
	cloned, err := cfg.Clone()
	if err != nil {
		return fmt.Errorf("failed to clone config: %w", err)
	}

	// Apply mutation
	if err := mutator(cloned); err != nil {
		return err
	}

	// Validate
	if err := cloned.Validate(); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	// Write to disk
	return config.Save(s.configPath, cloned)
}

// isPattern checks if a domain string contains pattern characters
func isPattern(domain string) bool {
	return strings.Contains(domain, "*") ||
		strings.Contains(domain, "^") ||
		strings.Contains(domain, "$") ||
		strings.Contains(domain, "(") ||
		strings.Contains(domain, ")") ||
		strings.Contains(domain, "[") ||
		strings.Contains(domain, "]") ||
		strings.Contains(domain, "\\")
}
