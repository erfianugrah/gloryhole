package config

// DEPRECATED in v0.26 — see docs/plans/2026-05-25-v026-policy-consolidation.md
//
// ConditionalForwardingConfig is functionally subsumed by Policy FORWARD
// rules. The first-boot migrator (cmd/glory-hole/main.go::migrateConditionalForwardingToPolicies)
// converts existing rules into policy_rules entries. This struct is retained
// in v0.26 for migration-source compatibility and removed in v0.27 along
// with the API endpoints, UI page, and forwarder.RuleEvaluator.

// ConditionalForwardingConfig holds conditional forwarding configuration.
//
// Deprecated: use Policy FORWARD rules instead.
type ConditionalForwardingConfig struct {
	Rules   []ForwardingRule `yaml:"rules"`
	Enabled bool             `yaml:"enabled"`
}

// ForwardingRule defines a conditional forwarding rule.
//
// Deprecated: use Policy FORWARD rules instead.
//
// The Timeout / MaxRetries / Failover fields were removed in v0.26 — they
// were declared in YAML and round-tripped through the API but never read at
// runtime (compileRule didn't copy them; ForwardWithUpstreams uses global
// forwarder defaults). YAML files containing these keys will continue to
// load (yaml.v3 ignores unknown fields by default).
type ForwardingRule struct {
	Name        string   `yaml:"name"`
	Domains     []string `yaml:"domains"`
	ClientCIDRs []string `yaml:"client_cidrs"`
	QueryTypes  []string `yaml:"query_types"`
	Upstreams   []string `yaml:"upstreams"`
	Priority    int      `yaml:"priority"`
	Enabled     bool     `yaml:"enabled"`
}

// DefaultConditionalForwardingConfig returns default configuration
func DefaultConditionalForwardingConfig() ConditionalForwardingConfig {
	return ConditionalForwardingConfig{
		Enabled: false,
		Rules:   []ForwardingRule{},
	}
}

// Validate validates the conditional forwarding configuration
func (c *ConditionalForwardingConfig) Validate() error {
	if !c.Enabled {
		return nil
	}

	for i := range c.Rules {
		if err := c.Rules[i].Validate(); err != nil {
			return err
		}
	}

	return nil
}

// Validate validates a forwarding rule
func (r *ForwardingRule) Validate() error {
	// Name is required
	if r.Name == "" {
		return ErrInvalidName
	}

	// At least one upstream is required
	if len(r.Upstreams) == 0 {
		return ErrNoUpstreams
	}

	// Validate upstreams format (will be validated more thoroughly at runtime)
	for _, upstream := range r.Upstreams {
		if upstream == "" {
			return ErrInvalidUpstream
		}
	}

	// Priority defaults to 50 if not specified
	if r.Priority == 0 {
		r.Priority = 50
	}

	// Priority must be between 1 and 100
	if r.Priority < 1 || r.Priority > 100 {
		return ErrInvalidPriority
	}

	// At least one matching condition is required
	if len(r.Domains) == 0 && len(r.ClientCIDRs) == 0 && len(r.QueryTypes) == 0 {
		return ErrNoMatchingConditions
	}

	return nil
}

// Errors for validation
var (
	ErrInvalidName          = &ConfigError{Field: "name", Message: "rule name cannot be empty"}
	ErrNoUpstreams          = &ConfigError{Field: "upstreams", Message: "at least one upstream is required"}
	ErrInvalidUpstream      = &ConfigError{Field: "upstreams", Message: "upstream cannot be empty"}
	ErrInvalidPriority      = &ConfigError{Field: "priority", Message: "priority must be between 1 and 100"}
	ErrNoMatchingConditions = &ConfigError{Field: "rules", Message: "at least one matching condition required (domains, client_cidrs, or query_types)"}
)

// ConfigError represents a configuration validation error
type ConfigError struct {
	Field   string
	Message string
}

func (e *ConfigError) Error() string {
	return "config validation error: " + e.Field + ": " + e.Message
}
