package config

import "time"

// Config holds the application configuration
type Config struct {
	UpstreamDNSServers []string      `yaml:"upstream_dns_servers"`
	Blocklists         []string      `yaml:"blocklists"`
	Whitelist          []string      `yaml.inline"`
	Overrides          []string      `yaml.inline"`
	CNAMEOverrides     []string      `yaml.inline"`
	UpdateInterval     time.Duration `yaml:"update_interval"`
}

// Load loads the configuration from a file
func Load(path string) (*Config, error) {
	// Logic to load config from YAML file will go here.
	return &Config{}, nil
}
