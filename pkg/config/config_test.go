package config

import (
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	cfg, err := Load("testdata/config.yml")
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if cfg == nil {
		t.Fatal("Load() returned nil config")
	}

	// Test that values from file are loaded
	if cfg.Server.ListenAddress != ":5353" {
		t.Errorf("Expected listen address :5353, got %s", cfg.Server.ListenAddress)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("Expected log level debug, got %s", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "json" {
		t.Errorf("Expected log format json, got %s", cfg.Logging.Format)
	}

	// Test that defaults are applied
	if cfg.Server.WebUIAddress != ":8080" {
		t.Errorf("Expected default web UI address :8080, got %s", cfg.Server.WebUIAddress)
	}
	if cfg.UpdateInterval != 24*time.Hour {
		t.Errorf("Expected default update interval 24h, got %s", cfg.UpdateInterval)
	}
}

func TestLoadWithDefaults(t *testing.T) {
	cfg := LoadWithDefaults()
	if cfg == nil {
		t.Fatal("LoadWithDefaults() returned nil")
	}

	// Check that defaults are set
	if cfg.Server.ListenAddress != ":53" {
		t.Errorf("Expected default listen address :53, got %s", cfg.Server.ListenAddress)
	}
	if len(cfg.UpstreamDNSServers) != 2 {
		t.Errorf("Expected 2 default upstream servers, got %d", len(cfg.UpstreamDNSServers))
	}
	if cfg.Logging.Level != "info" {
		t.Errorf("Expected default log level info, got %s", cfg.Logging.Level)
	}
	if cfg.Cache.MaxEntries != 10000 {
		t.Errorf("Expected default cache max entries 10000, got %d", cfg.Cache.MaxEntries)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		cfg     *Config
		name    string
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: &Config{
				Server: ServerConfig{
					ListenAddress: ":53",
					UDPEnabled:    true,
				},
				UpstreamDNSServers: []string{"1.1.1.1:53"},
				Logging: LoggingConfig{
					Level:  "info",
					Format: "text",
					Output: "stdout",
				},
			},
			wantErr: false,
		},
		{
			name: "empty listen address",
			cfg: &Config{
				Server: ServerConfig{
					ListenAddress: "",
					UDPEnabled:    true,
				},
				UpstreamDNSServers: []string{"1.1.1.1:53"},
				Logging: LoggingConfig{
					Level:  "info",
					Format: "text",
					Output: "stdout",
				},
			},
			wantErr: true,
		},
		{
			name: "no upstream servers",
			cfg: &Config{
				Server: ServerConfig{
					ListenAddress: ":53",
					UDPEnabled:    true,
				},
				UpstreamDNSServers: []string{},
				Logging: LoggingConfig{
					Level:  "info",
					Format: "text",
					Output: "stdout",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid log level",
			cfg: &Config{
				Server: ServerConfig{
					ListenAddress: ":53",
					UDPEnabled:    true,
				},
				UpstreamDNSServers: []string{"1.1.1.1:53"},
				Logging: LoggingConfig{
					Level:  "invalid",
					Format: "text",
					Output: "stdout",
				},
			},
			wantErr: true,
		},
		{
			name: "file output without path",
			cfg: &Config{
				Server: ServerConfig{
					ListenAddress: ":53",
					UDPEnabled:    true,
				},
				UpstreamDNSServers: []string{"1.1.1.1:53"},
				Logging: LoggingConfig{
					Level:  "info",
					Format: "text",
					Output: "file",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadNonExistentFile(t *testing.T) {
	_, err := Load("nonexistent.yml")
	if err == nil {
		t.Error("Expected error when loading non-existent file")
	}
}
