package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds the application configuration
type Config struct {
	// Server settings
	Server ServerConfig `yaml:"server"`

	// Upstream DNS servers (global default)
	UpstreamDNSServers []string `yaml:"upstream_dns_servers"`

	// Update settings
	UpdateInterval      time.Duration `yaml:"update_interval"`
	AutoUpdateBlocklists bool          `yaml:"auto_update_blocklists"`

	// Blocklists and filtering
	Blocklists     []string          `yaml:"blocklists"`
	Whitelist      []string          `yaml:"whitelist"`
	Overrides      map[string]string `yaml:"overrides"`
	CNAMEOverrides map[string]string `yaml:"cname_overrides"`

	// Storage
	Storage StorageConfig `yaml:"storage"`

	// Cache settings
	Cache CacheConfig `yaml:"cache"`

	// Logging
	Logging LoggingConfig `yaml:"logging"`

	// Telemetry (OTEL)
	Telemetry TelemetryConfig `yaml:"telemetry"`
}

// ServerConfig holds server-specific settings
type ServerConfig struct {
	ListenAddress string `yaml:"listen_address"`
	TCPEnabled    bool   `yaml:"tcp_enabled"`
	UDPEnabled    bool   `yaml:"udp_enabled"`
	WebUIAddress  string `yaml:"web_ui_address"`
}

// StorageConfig holds storage settings
type StorageConfig struct {
	DatabasePath      string `yaml:"database_path"`
	LogQueries        bool   `yaml:"log_queries"`
	LogRetentionDays  int    `yaml:"log_retention_days"`
	BufferSize        int    `yaml:"buffer_size"`
}

// CacheConfig holds cache settings
type CacheConfig struct {
	Enabled     bool          `yaml:"enabled"`
	MaxEntries  int           `yaml:"max_entries"`
	MinTTL      time.Duration `yaml:"min_ttl"`
	MaxTTL      time.Duration `yaml:"max_ttl"`
	NegativeTTL time.Duration `yaml:"negative_ttl"`
}

// LoggingConfig holds logging settings
type LoggingConfig struct {
	Level      string `yaml:"level"`        // debug, info, warn, error
	Format     string `yaml:"format"`       // json, text
	Output     string `yaml:"output"`       // stdout, stderr, file
	FilePath   string `yaml:"file_path"`    // if output=file
	AddSource  bool   `yaml:"add_source"`   // include source file/line (adds ~1-2μs overhead per log)
	MaxSize    int    `yaml:"max_size"`     // MB
	MaxBackups int    `yaml:"max_backups"`  // number of old log files
	MaxAge     int    `yaml:"max_age"`      // days
}

// TelemetryConfig holds OpenTelemetry settings
type TelemetryConfig struct {
	Enabled           bool   `yaml:"enabled"`
	ServiceName       string `yaml:"service_name"`
	ServiceVersion    string `yaml:"service_version"`
	PrometheusEnabled bool   `yaml:"prometheus_enabled"`
	PrometheusPort    int    `yaml:"prometheus_port"`
	TracingEnabled    bool   `yaml:"tracing_enabled"`
	TracingEndpoint   string `yaml:"tracing_endpoint"`
}

// Load loads the configuration from a YAML file
func Load(path string) (*Config, error) {
	// Read the file
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
	return cfg
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

	// Storage defaults
	if c.Storage.DatabasePath == "" {
		c.Storage.DatabasePath = "./gloryhole.db"
	}
	if c.Storage.LogRetentionDays == 0 {
		c.Storage.LogRetentionDays = 30
	}
	if c.Storage.BufferSize == 0 {
		c.Storage.BufferSize = 1000
	}

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

	return nil
}
