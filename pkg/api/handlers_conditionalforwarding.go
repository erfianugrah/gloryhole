package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"glory-hole/pkg/config"
)

// ForwardingRuleResponse represents a forwarding rule in API responses
type ForwardingRuleResponse struct {
	ID          string   `json:"id"`          // Unique identifier (name:index)
	Name        string   `json:"name"`
	Domains     []string `json:"domains,omitempty"`
	ClientCIDRs []string `json:"client_cidrs,omitempty"`
	QueryTypes  []string `json:"query_types,omitempty"`
	Upstreams   []string `json:"upstreams"`
	Priority    int      `json:"priority"`
	Timeout     string   `json:"timeout,omitempty"`
	MaxRetries  int      `json:"max_retries,omitempty"`
	Failover    bool     `json:"failover"`
	Enabled     bool     `json:"enabled"`
}

// ConditionalForwardingListResponse represents the list of forwarding rules
type ConditionalForwardingListResponse struct {
	Rules   []ForwardingRuleResponse `json:"rules"`
	Total   int                      `json:"total"`
	Enabled bool                     `json:"enabled"`
}

// ForwardingRuleAddRequest represents a request to add a forwarding rule
type ForwardingRuleAddRequest struct {
	Name        string   `json:"name"`
	Domains     []string `json:"domains,omitempty"`
	ClientCIDRs []string `json:"client_cidrs,omitempty"`
	QueryTypes  []string `json:"query_types,omitempty"`
	Upstreams   []string `json:"upstreams"`
	Priority    int      `json:"priority"`
	Timeout     string   `json:"timeout,omitempty"`
	MaxRetries  int      `json:"max_retries,omitempty"`
	Failover    bool     `json:"failover"`
}

// handleGetConditionalForwarding returns all conditional forwarding rules
func (s *Server) handleGetConditionalForwarding(w http.ResponseWriter, r *http.Request) {
	rules := make([]ForwardingRuleResponse, 0)

	// Get forwarding rules from config
	cfg := s.currentConfig()
	enabled := false
	if cfg != nil {
		enabled = cfg.ConditionalForwarding.Enabled
		for idx, rule := range cfg.ConditionalForwarding.Rules {
			// Generate unique ID
			id := fmt.Sprintf("%s:%d", rule.Name, idx)

			// Convert timeout to string
			timeoutStr := ""
			if rule.Timeout > 0 {
				timeoutStr = rule.Timeout.String()
			}

			rules = append(rules, ForwardingRuleResponse{
				ID:          id,
				Name:        rule.Name,
				Domains:     rule.Domains,
				ClientCIDRs: rule.ClientCIDRs,
				QueryTypes:  rule.QueryTypes,
				Upstreams:   rule.Upstreams,
				Priority:    rule.Priority,
				Timeout:     timeoutStr,
				MaxRetries:  rule.MaxRetries,
				Failover:    rule.Failover,
				Enabled:     rule.Enabled,
			})
		}
	}

	// Sort rules by priority (descending), then by name
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].Priority != rules[j].Priority {
			return rules[i].Priority > rules[j].Priority
		}
		return rules[i].Name < rules[j].Name
	})

	// Check if request wants HTML (from HTMX or browser)
	if r.Header.Get("HX-Request") == "true" || r.Header.Get("Accept") == "text/html" {
		// Return HTML partial
		data := struct {
			Rules   []ForwardingRuleResponse
			Enabled bool
		}{
			Rules:   rules,
			Enabled: enabled,
		}

		if err := conditionalForwardingPartialTemplate.Execute(w, data); err != nil {
			s.logger.Error("Failed to render conditional forwarding partial", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}

	// Return JSON
	s.writeJSON(w, http.StatusOK, ConditionalForwardingListResponse{
		Rules:   rules,
		Total:   len(rules),
		Enabled: enabled,
	})
}

// handleAddConditionalForwarding adds a new forwarding rule
func (s *Server) handleAddConditionalForwarding(w http.ResponseWriter, r *http.Request) {
	// Parse request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Failed to read request body")
		return
	}

	var req ForwardingRuleAddRequest
	if err := json.Unmarshal(body, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	// Validate required fields
	if req.Name == "" {
		s.writeError(w, http.StatusBadRequest, "Rule name is required")
		return
	}
	if len(req.Upstreams) == 0 {
		s.writeError(w, http.StatusBadRequest, "At least one upstream is required")
		return
	}
	if len(req.Domains) == 0 && len(req.ClientCIDRs) == 0 && len(req.QueryTypes) == 0 {
		s.writeError(w, http.StatusBadRequest, "At least one matching condition required (domains, client_cidrs, or query_types)")
		return
	}

	// Default priority to 50 if not specified
	if req.Priority == 0 {
		req.Priority = 50
	}

	// Validate priority range
	if req.Priority < 1 || req.Priority > 100 {
		s.writeError(w, http.StatusBadRequest, "Priority must be between 1 and 100")
		return
	}

	// Create config entry
	rule := config.ForwardingRule{
		Name:        req.Name,
		Domains:     req.Domains,
		ClientCIDRs: req.ClientCIDRs,
		QueryTypes:  req.QueryTypes,
		Upstreams:   req.Upstreams,
		Priority:    req.Priority,
		MaxRetries:  req.MaxRetries,
		Failover:    req.Failover,
		Enabled:     true, // New rules are enabled by default
	}

	// Parse timeout if provided
	if req.Timeout != "" {
		timeout, err := parseDurationField(req.Timeout, "timeout")
		if err != nil {
			s.writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid timeout format: %v", err))
			return
		}
		rule.Timeout = timeout
	}

	// Persist to config
	if err := s.persistConditionalForwardingConfig(func(cfg *config.Config) error {
		cfg.ConditionalForwarding.Enabled = true
		cfg.ConditionalForwarding.Rules = append(cfg.ConditionalForwarding.Rules, rule)
		return nil
	}); err != nil {
		s.logger.Error("Failed to persist forwarding rule to config", "error", err)
		s.writeError(w, http.StatusInternalServerError, "Failed to save rule")
		return
	}

	// Log the action
	s.logger.Info("Added conditional forwarding rule",
		"name", req.Name,
		"priority", req.Priority,
		"user", "admin") // TODO: Get actual user from session

	// Return updated list
	s.handleGetConditionalForwarding(w, r)
}

// handleRemoveConditionalForwarding removes a forwarding rule
func (s *Server) handleRemoveConditionalForwarding(w http.ResponseWriter, r *http.Request) {
	ruleID := r.PathValue("id")
	if ruleID == "" {
		s.writeError(w, http.StatusBadRequest, "Rule ID parameter required")
		return
	}

	// Parse rule ID (format: name:index)
	parts := strings.Split(ruleID, ":")
	if len(parts) != 2 {
		s.writeError(w, http.StatusBadRequest, "Invalid rule ID format")
		return
	}

	name := parts[0]
	removed := false

	// Persist to config - remove from config list
	if err := s.persistConditionalForwardingConfig(func(cfg *config.Config) error {
		newRules := make([]config.ForwardingRule, 0, len(cfg.ConditionalForwarding.Rules))
		for _, rule := range cfg.ConditionalForwarding.Rules {
			// Match by name
			if rule.Name == name {
				// Skip this rule (remove it)
				removed = true
				continue
			}
			newRules = append(newRules, rule)
		}

		if !removed {
			return fmt.Errorf("rule not found")
		}

		cfg.ConditionalForwarding.Rules = newRules
		return nil
	}); err != nil {
		s.logger.Error("Failed to persist forwarding rules to config", "error", err)
		if strings.Contains(err.Error(), "not found") {
			s.writeError(w, http.StatusNotFound, "Rule not found")
			return
		}
		s.writeError(w, http.StatusInternalServerError, "Failed to remove rule")
		return
	}

	// Log the action
	s.logger.Info("Removed conditional forwarding rule",
		"name", name,
		"user", "admin") // TODO: Get actual user from session

	// Return updated list
	s.handleGetConditionalForwarding(w, r)
}

// persistConditionalForwardingConfig persists conditional forwarding changes to config file
func (s *Server) persistConditionalForwardingConfig(mutator func(cfg *config.Config) error) error {
	if s.configPath == "" {
		// In tests or ephemeral runs without a config file, keep rules in-memory only.
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
