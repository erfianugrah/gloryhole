package config

import "time"

// ConditionalForwardingConfig holds conditional forwarding configuration
type ConditionalForwardingConfig struct {
	Enabled bool             `yaml:"enabled"`
	Rules   []ForwardingRule `yaml:"rules"`
}

// ForwardingRule defines a conditional forwarding rule
type ForwardingRule struct {
	Name        string        `yaml:"name"`
	Priority    int           `yaml:"priority"`     // Higher priority = evaluated first (default: 50)
	Domains     []string      `yaml:"domains"`      // Domain patterns to match (*.local, nas.local, etc.)
	ClientCIDRs []string      `yaml:"client_cidrs"` // Client IP ranges to match (10.0.0.0/8, etc.)
	QueryTypes  []string      `yaml:"query_types"`  // Query types to match (A, AAAA, PTR, etc.)
	Upstreams   []string      `yaml:"upstreams"`    // Upstream DNS servers to forward to
	Failover    bool          `yaml:"failover"`     // Try next upstream on failure
	Timeout     time.Duration `yaml:"timeout"`      // Per-rule timeout (default: use forwarder timeout)
	MaxRetries  int           `yaml:"max_retries"`  // Max retries per upstream (default: use forwarder retries)
	Enabled     bool          `yaml:"enabled"`      // Enable/disable this rule
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
	ErrInvalidName            = &ConfigError{Field: "name", Message: "rule name cannot be empty"}
	ErrNoUpstreams            = &ConfigError{Field: "upstreams", Message: "at least one upstream is required"}
	ErrInvalidUpstream        = &ConfigError{Field: "upstreams", Message: "upstream cannot be empty"}
	ErrInvalidPriority        = &ConfigError{Field: "priority", Message: "priority must be between 1 and 100"}
	ErrNoMatchingConditions   = &ConfigError{Field: "rules", Message: "at least one matching condition required (domains, client_cidrs, or query_types)"}
)

// ConfigError represents a configuration validation error
type ConfigError struct {
	Field   string
	Message string
}

func (e *ConfigError) Error() string {
	return "config validation error: " + e.Field + ": " + e.Message
}
