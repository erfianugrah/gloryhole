// Package config defines the runtime configuration structs, parsing helpers,
// and hot-reload wiring shared across services.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"glory-hole/pkg/storage"

	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
)

// Config holds the application configuration
//
//nolint:fieldalignment // Struct is organized for readability; padding cost is acceptable.
type Config struct {
	Telemetry             TelemetryConfig             `yaml:"telemetry"`
	Server                ServerConfig                `yaml:"server"`
	Policy                PolicyConfig                `yaml:"policy"`
	Auth                  AuthConfig                  `yaml:"auth"`
	LocalRecords          LocalRecordsConfig          `yaml:"local_records"`
	ConditionalForwarding ConditionalForwardingConfig `yaml:"conditional_forwarding"`
	Forwarder             ForwarderConfig             `yaml:"forwarder"`        // Upstream DNS forwarder config
	UpstreamDNSServers    []string                    `yaml:"upstream_dns_servers"`
	Blocklists            []string                    `yaml:"blocklists"`
	Whitelist             []string                    `yaml:"whitelist"`
	Logging               LoggingConfig               `yaml:"logging"`
	Database              storage.Config              `yaml:"database"`
	Cache                 CacheConfig                 `yaml:"cache"`
	UpdateInterval        time.Duration               `yaml:"update_interval"`
	AutoUpdateBlocklists  bool                        `yaml:"auto_update_blocklists"`
}

// ForwarderConfig holds DNS forwarder configuration
type ForwarderConfig struct {
	CircuitBreaker CircuitBreakerConfig `yaml:"circuit_breaker"` // Circuit breaker for upstream health
}

// CircuitBreakerConfig holds circuit breaker settings
type CircuitBreakerConfig struct {
	Enabled          bool `yaml:"enabled"`           // Enable circuit breaker (default: true)
	FailureThreshold int  `yaml:"failure_threshold"` // Failures before opening (default: 5)
	SuccessThreshold int  `yaml:"success_threshold"` // Successes to close from half-open (default: 2)
	TimeoutSeconds   int  `yaml:"timeout_seconds"`   // Seconds before half-open (default: 30)
}

// ServerConfig holds server-specific settings
type ServerConfig struct {
	ListenAddress      string            `yaml:"listen_address"`
	WebUIAddress       string            `yaml:"web_ui_address"`
	TCPEnabled         bool              `yaml:"tcp_enabled"`
	UDPEnabled         bool              `yaml:"udp_enabled"`
	EnableBlocklist    bool              `yaml:"enable_blocklist"`     // Kill-switch for ad-blocking
	EnablePolicies     bool              `yaml:"enable_policies"`      // Kill-switch for policy engine
	DecisionTrace      bool              `yaml:"decision_trace"`       // Capture block decision traces
	CORSAllowedOrigins []string          `yaml:"cors_allowed_origins"` // Allowed CORS origins (empty = none, "*" = all)
	DotEnabled         bool              `yaml:"dot_enabled"`
	DotAddress         string            `yaml:"dot_address"`
	TLS                TLSConfig         `yaml:"tls"`
	QueryLogger        QueryLoggerConfig `yaml:"query_logger"` // Worker pool config for async query logging
}

// QueryLoggerConfig holds query logger worker pool settings
type QueryLoggerConfig struct {
	Enabled    bool `yaml:"enabled"`     // Enable worker pool (default: true)
	BufferSize int  `yaml:"buffer_size"` // Query log buffer size (default: 50000)
	Workers    int  `yaml:"workers"`     // Number of worker goroutines (default: 8)
}

// TLSConfig holds TLS settings for DoT (and optional future listeners).
type TLSConfig struct {
	CertFile string         `yaml:"cert_file"`
	KeyFile  string         `yaml:"key_file"`
	Autocert AutocertConfig `yaml:"autocert"`
	ACME     ACMEConfig     `yaml:"acme"`
}

// AutocertConfig controls automatic certificate provisioning via ACME.
type AutocertConfig struct {
	Enabled       bool     `yaml:"enabled"`
	Hosts         []string `yaml:"hosts"`
	CacheDir      string   `yaml:"cache_dir"`
	Email         string   `yaml:"email"`
	HTTP01Address string   `yaml:"http01_address"`
}

// ACMEConfig enables native DNS-01 issuance (Cloudflare-only for now).
type ACMEConfig struct {
	Enabled     bool          `yaml:"enabled"`
	DNSProvider string        `yaml:"dns_provider"` // "cloudflare"
	Hosts       []string      `yaml:"hosts"`
	Upstreams   []string      `yaml:"upstream_dns_servers"` // optional: override ACME/CF resolver
	CacheDir    string        `yaml:"cache_dir"`
	Email       string        `yaml:"email"`
	RenewBefore time.Duration `yaml:"renew_before"` // duration before expiry to renew
	Cloudflare  CFConfig      `yaml:"cloudflare"`
}

// CFConfig holds Cloudflare credentials for DNS-01 (prefer env CF_DNS_API_TOKEN).
type CFConfig struct {
	APIToken           string        `yaml:"api_token"`
	ZoneID             string        `yaml:"zone_id"`                  // optional: skip zone discovery
	TTL                int           `yaml:"ttl"`                      // TXT record TTL (min 120)
	PropagationTimeout time.Duration `yaml:"propagation_timeout"`      // how long to wait for TXT to show up
	PollingInterval    time.Duration `yaml:"polling_interval"`         // how often to poll during propagation
	SkipAuthNSCheck    bool          `yaml:"skip_authoritative_check"` // if true, rely on recursive NS only
}

// AuthConfig controls static authentication for the API/UI layer.
type AuthConfig struct {
	Enabled      bool   `yaml:"enabled"`
	APIKey       string `yaml:"api_key"`
	Header       string `yaml:"header"`
	Username     string `yaml:"username"`
	Password     string `yaml:"password"`      // DEPRECATED: Plaintext password (use password_hash instead)
	PasswordHash string `yaml:"password_hash"` // Bcrypt hash of password (recommended)
}

func (a *AuthConfig) normalize() {
	if a == nil {
		return
	}
	if strings.TrimSpace(a.Header) == "" {
		a.Header = "Authorization"
	}
	// Migrate plaintext password to bcrypt hash
	if a.Password != "" && a.PasswordHash == "" {
		a.migratePasswordToHash()
	}
}

// migratePasswordToHash automatically converts plaintext password to bcrypt hash
func (a *AuthConfig) migratePasswordToHash() {
	// Import is already at package level
	hash, err := bcrypt.GenerateFromPassword([]byte(a.Password), 12)
	if err != nil {
		// Log error but don't fail - keep plaintext for backward compat
		return
	}
	a.PasswordHash = string(hash)
	a.Password = "" // Clear plaintext
}

// CacheConfig holds cache settings
type CacheConfig struct {
	Enabled     bool          `yaml:"enabled"`
	MaxEntries  int           `yaml:"max_entries"`
	MinTTL      time.Duration `yaml:"min_ttl"`
	MaxTTL      time.Duration `yaml:"max_ttl"`
	NegativeTTL time.Duration `yaml:"negative_ttl"` // TTL for upstream NXDOMAIN responses
	BlockedTTL  time.Duration `yaml:"blocked_ttl"`  // TTL for blocked domain responses
	ShardCount  int           `yaml:"shard_count"`  // Number of shards for concurrent access (0 = use non-sharded cache)
}

// LocalRecordsConfig holds local DNS records configuration
type LocalRecordsConfig struct {
	Records []LocalRecordEntry `yaml:"records"`
	Enabled bool               `yaml:"enabled"`
}

// LocalRecordEntry represents a single local DNS record in the config
type LocalRecordEntry struct {
	CaaFlag    *uint8   `yaml:"caa_flag,omitempty"` // CAA: Flags (usually 0 or 128)
	Priority   *uint16  `yaml:"priority,omitempty"`
	Weight     *uint16  `yaml:"weight,omitempty"`
	Port       *uint16  `yaml:"port,omitempty"`
	Expire     *uint32  `yaml:"expire,omitempty"`
	Minttl     *uint32  `yaml:"minttl,omitempty"`
	Refresh    *uint32  `yaml:"refresh,omitempty"`
	Retry      *uint32  `yaml:"retry,omitempty"`
	Serial     *uint32  `yaml:"serial,omitempty"`
	CaaTag     string   `yaml:"caa_tag,omitempty"`   // CAA: Tag (issue/issuewild/iodef)
	CaaValue   string   `yaml:"caa_value,omitempty"` // CAA: Value (CA domain or URL)
	Mbox       string   `yaml:"mbox,omitempty"`
	Ns         string   `yaml:"ns,omitempty"`
	Target     string   `yaml:"target"`
	Type       string   `yaml:"type"`
	Domain     string   `yaml:"domain"`
	TxtRecords []string `yaml:"txt,omitempty"`
	IPs        []string `yaml:"ips"`
	TTL        uint32   `yaml:"ttl"`
	Wildcard   bool     `yaml:"wildcard"`
}

// PolicyConfig holds policy engine configuration
type PolicyConfig struct {
	Rules   []PolicyRuleEntry `yaml:"rules"`
	Enabled bool              `yaml:"enabled"`
}

// PolicyRuleEntry represents a single policy rule in the config
type PolicyRuleEntry struct {
	Name       string `yaml:"name"`        // Human-readable name
	Logic      string `yaml:"logic"`       // Expression to evaluate
	Action     string `yaml:"action"`      // Action: BLOCK, ALLOW, REDIRECT
	ActionData string `yaml:"action_data"` // Optional action data (e.g., redirect target)
	Enabled    bool   `yaml:"enabled"`     // Whether the rule is active
}

// LoggingConfig holds logging settings
type LoggingConfig struct {
	Level      string `yaml:"level"`       // debug, info, warn, error
	Format     string `yaml:"format"`      // json, text
	Output     string `yaml:"output"`      // stdout, stderr, file
	FilePath   string `yaml:"file_path"`   // if output=file
	AddSource  bool   `yaml:"add_source"`  // include source file/line (adds ~1-2μs overhead per log)
	MaxSize    int    `yaml:"max_size"`    // MB
	MaxBackups int    `yaml:"max_backups"` // number of old log files
	MaxAge     int    `yaml:"max_age"`     // days
}

// TelemetryConfig holds OpenTelemetry settings
type TelemetryConfig struct {
	ServiceName       string `yaml:"service_name"`
	ServiceVersion    string `yaml:"service_version"`
	TracingEndpoint   string `yaml:"tracing_endpoint"`
	PrometheusPort    int    `yaml:"prometheus_port"`
	Enabled           bool   `yaml:"enabled"`
	PrometheusEnabled bool   `yaml:"prometheus_enabled"`
	TracingEnabled    bool   `yaml:"tracing_enabled"`
}

// Load loads the configuration from a YAML file
func Load(path string) (*Config, error) {
	// Read the file
	// #nosec G304 - Config file path is provided by user via CLI flag, this is intentional
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse YAML
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config YAML: %w", err)
	}

	// Apply defaults
	cfg.applyDefaults()
	cfg.applyEnvOverrides()

	// Validate
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
}

// LoadWithDefaults creates a configuration with sensible defaults
func LoadWithDefaults() *Config {
	cfg := &Config{}
	cfg.applyDefaults()
	cfg.applyEnvOverrides()
	return cfg
}

// Clone creates a deep copy of the configuration
// This is used to safely mutate config before persisting
func (c *Config) Clone() (*Config, error) {
	// Use YAML marshal/unmarshal for deep copy
	// This ensures all nested structs are properly copied
	data, err := yaml.Marshal(c)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config for cloning: %w", err)
	}

	var clone Config
	if err := yaml.Unmarshal(data, &clone); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config clone: %w", err)
	}

	// Reapply normalization and defaults (they might not survive YAML round-trip)
	clone.applyDefaults()
	clone.Auth.normalize()

	return &clone, nil
}

// Save writes the configuration back to a YAML file
// This is used by the kill-switch feature to persist runtime changes
func Save(path string, cfg *Config) error {
	// Marshal config to YAML
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write atomically: write to temp file, then rename
	// This prevents corruption if write is interrupted
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write temp config: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath) // Clean up temp file on failure
		return fmt.Errorf("failed to rename config: %w", err)
	}

	return nil
}

// applyDefaults sets default values for unset configuration fields
func (c *Config) applyDefaults() {
	// Server defaults
	if c.Server.ListenAddress == "" {
		c.Server.ListenAddress = ":53"
	}
	if !c.Server.TCPEnabled && !c.Server.UDPEnabled {
		c.Server.TCPEnabled = true
		c.Server.UDPEnabled = true
	}
	if c.Server.WebUIAddress == "" {
		c.Server.WebUIAddress = ":8080"
	}
	if c.Server.DotAddress == "" {
		c.Server.DotAddress = ":853"
	}
	if c.Server.TLS.Autocert.HTTP01Address == "" {
		c.Server.TLS.Autocert.HTTP01Address = ":80"
	}
	if c.Server.TLS.ACME.CacheDir == "" {
		c.Server.TLS.ACME.CacheDir = "./.cache/acme"
	}
	if c.Server.TLS.ACME.RenewBefore == 0 {
		c.Server.TLS.ACME.RenewBefore = 30 * 24 * time.Hour // 30 days
	}
	// ACME upstream default: inherit global upstreams if none specified
	if len(c.Server.TLS.ACME.Upstreams) == 0 {
		c.Server.TLS.ACME.Upstreams = append([]string{}, c.UpstreamDNSServers...)
	}
	if c.Server.TLS.ACME.Cloudflare.TTL == 0 {
		c.Server.TLS.ACME.Cloudflare.TTL = 120
	}
	if c.Server.TLS.ACME.Cloudflare.PropagationTimeout == 0 {
		c.Server.TLS.ACME.Cloudflare.PropagationTimeout = 2 * time.Minute
	}
	if c.Server.TLS.ACME.Cloudflare.PollingInterval == 0 {
		c.Server.TLS.ACME.Cloudflare.PollingInterval = 2 * time.Second
	}

	// Kill-switch defaults: Enable both if neither is explicitly configured
	// This provides backward compatibility (both enabled by default)
	// To disable, explicitly set to false in config.yml
	if !c.Server.EnableBlocklist && !c.Server.EnablePolicies {
		c.Server.EnableBlocklist = true
		c.Server.EnablePolicies = true
	}

	// Upstream DNS defaults
	if len(c.UpstreamDNSServers) == 0 {
		c.UpstreamDNSServers = []string{
			"1.1.1.1:53",
			"8.8.8.8:53",
		}
	}

	// Update interval default
	if c.UpdateInterval == 0 {
		c.UpdateInterval = 24 * time.Hour
	}

	// Database defaults
	if c.Database.Backend == "" {
		c.Database.Backend = storage.BackendSQLite
	}
	if c.Database.SQLite.Path == "" {
		c.Database.SQLite.Path = "./glory-hole.db"
	}
	if c.Database.SQLite.BusyTimeout == 0 {
		c.Database.SQLite.BusyTimeout = 5000
	}
	if c.Database.SQLite.CacheSize == 0 {
		c.Database.SQLite.CacheSize = 4096
	}
	if c.Database.SQLite.MMapSize == 0 {
		c.Database.SQLite.MMapSize = 268435456
	}
	if c.Database.BufferSize == 0 {
		c.Database.BufferSize = 500
	}
	if c.Database.FlushInterval == 0 {
		c.Database.FlushInterval = 5 * time.Second
	}
	if c.Database.BatchSize == 0 {
		c.Database.BatchSize = 100
	}
	if c.Database.RetentionDays == 0 {
		c.Database.RetentionDays = 7
	}
	if c.Database.Statistics.AggregationInterval == 0 {
		c.Database.Statistics.AggregationInterval = 1 * time.Hour
	}
	// Enable WAL mode by default for better concurrency
	c.Database.SQLite.WALMode = true

	// Cache defaults
	if c.Cache.MaxEntries == 0 {
		c.Cache.MaxEntries = 10000
	}
	if c.Cache.MinTTL == 0 {
		c.Cache.MinTTL = 60 * time.Second
	}
	if c.Cache.MaxTTL == 0 {
		c.Cache.MaxTTL = 24 * time.Hour
	}
	if c.Cache.NegativeTTL == 0 {
		c.Cache.NegativeTTL = 5 * time.Minute
	}
	if c.Cache.BlockedTTL == 0 {
		c.Cache.BlockedTTL = 1 * time.Second
	}

	// Logging defaults
	if c.Logging.Level == "" {
		c.Logging.Level = "info"
	}
	if c.Logging.Format == "" {
		c.Logging.Format = "text"
	}
	if c.Logging.Output == "" {
		c.Logging.Output = "stdout"
	}
	if c.Logging.MaxSize == 0 {
		c.Logging.MaxSize = 100 // 100MB
	}
	if c.Logging.MaxBackups == 0 {
		c.Logging.MaxBackups = 3
	}
	if c.Logging.MaxAge == 0 {
		c.Logging.MaxAge = 7 // 7 days
	}

	// Telemetry defaults
	if c.Telemetry.ServiceName == "" {
		c.Telemetry.ServiceName = "glory-hole"
	}
	if c.Telemetry.ServiceVersion == "" {
		c.Telemetry.ServiceVersion = "dev"
	}
	if c.Telemetry.PrometheusPort == 0 {
		c.Telemetry.PrometheusPort = 9090
	}

	c.Auth.normalize()
}

const (
	envAPIKey   = "GLORYHOLE_API_KEY"
	envAuthUser = "GLORYHOLE_BASIC_USER"
	envAuthPass = "GLORYHOLE_BASIC_PASS"
)

func (c *Config) applyEnvOverrides() {
	key := strings.TrimSpace(os.Getenv(envAPIKey))
	if key != "" {
		c.Auth.APIKey = key
		c.Auth.Enabled = true
	}

	user := strings.TrimSpace(os.Getenv(envAuthUser))
	if user != "" {
		c.Auth.Username = user
		c.Auth.Enabled = true
	}

	if pass, ok := os.LookupEnv(envAuthPass); ok {
		c.Auth.Password = pass
		c.Auth.Enabled = true
	}

	c.Auth.normalize()
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	// Validate server config
	if c.Server.ListenAddress == "" {
		return fmt.Errorf("server.listen_address cannot be empty")
	}
	if !c.Server.TCPEnabled && !c.Server.UDPEnabled {
		return fmt.Errorf("at least one of TCP or UDP must be enabled")
	}

	if c.Server.DotEnabled {
		if strings.TrimSpace(c.Server.DotAddress) == "" {
			return fmt.Errorf("server.dot_address cannot be empty when DoT is enabled")
		}

		certSet := strings.TrimSpace(c.Server.TLS.CertFile) != "" || strings.TrimSpace(c.Server.TLS.KeyFile) != ""
		if certSet {
			if strings.TrimSpace(c.Server.TLS.CertFile) == "" || strings.TrimSpace(c.Server.TLS.KeyFile) == "" {
				return fmt.Errorf("tls.cert_file and tls.key_file must both be set when providing manual certificates")
			}
		}

		if c.Server.TLS.Autocert.Enabled {
			if len(c.Server.TLS.Autocert.Hosts) == 0 {
				return fmt.Errorf("tls.autocert.hosts must be set when autocert is enabled")
			}
			if strings.TrimSpace(c.Server.TLS.Autocert.HTTP01Address) == "" {
				return fmt.Errorf("tls.autocert.http01_address must be set when autocert is enabled")
			}
		}

		if !certSet && !c.Server.TLS.Autocert.Enabled && !c.Server.TLS.ACME.Enabled {
			return fmt.Errorf("DoT requires TLS: provide cert/key, autocert, or acme.dns_provider")
		}

		if c.Server.TLS.ACME.Enabled {
			if len(c.Server.TLS.ACME.Hosts) == 0 {
				return fmt.Errorf("tls.acme.hosts must be set when ACME is enabled")
			}
			if c.Server.TLS.ACME.DNSProvider != "cloudflare" {
				return fmt.Errorf("tls.acme.dns_provider must be 'cloudflare' (only provider supported)")
			}
			if c.Server.TLS.ACME.Cloudflare.TTL > 0 && c.Server.TLS.ACME.Cloudflare.TTL < 120 {
				return fmt.Errorf("tls.acme.cloudflare.ttl must be >= 120 seconds")
			}
			if c.Server.TLS.ACME.Cloudflare.PropagationTimeout < 0 {
				return fmt.Errorf("tls.acme.cloudflare.propagation_timeout must be >= 0")
			}
			if c.Server.TLS.ACME.Cloudflare.PollingInterval < 0 {
				return fmt.Errorf("tls.acme.cloudflare.polling_interval must be >= 0")
			}
		}
	}

	// Validate upstream servers
	if len(c.UpstreamDNSServers) == 0 {
		return fmt.Errorf("at least one upstream DNS server must be configured")
	}

	// Validate logging level
	validLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}
	if !validLevels[c.Logging.Level] {
		return fmt.Errorf("invalid logging level: %s (must be debug, info, warn, or error)", c.Logging.Level)
	}

	// Validate logging format
	if c.Logging.Format != "json" && c.Logging.Format != "text" {
		return fmt.Errorf("invalid logging format: %s (must be json or text)", c.Logging.Format)
	}

	// Validate logging output
	validOutputs := map[string]bool{
		"stdout": true,
		"stderr": true,
		"file":   true,
	}
	if !validOutputs[c.Logging.Output] {
		return fmt.Errorf("invalid logging output: %s (must be stdout, stderr, or file)", c.Logging.Output)
	}
	if c.Logging.Output == "file" && c.Logging.FilePath == "" {
		return fmt.Errorf("logging.file_path must be set when output is 'file'")
	}

	if c.Auth.Enabled {
		c.Auth.normalize()
		if strings.TrimSpace(c.Auth.APIKey) == "" && (c.Auth.Username == "" || c.Auth.Password == "") {
			return fmt.Errorf("auth requires api_key or username/password when enabled")
		}
	}

	// Validate conditional forwarding
	if err := c.ConditionalForwarding.Validate(); err != nil {
		return fmt.Errorf("conditional_forwarding validation failed: %w", err)
	}

	return nil
}
