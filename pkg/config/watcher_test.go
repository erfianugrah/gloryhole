package config

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"
)

func TestNewWatcher(t *testing.T) {
	logger := slog.Default()

	// Test with existing config file
	watcher, err := NewWatcher("testdata/config.yml", logger)
	if err != nil {
		t.Fatalf("NewWatcher() failed: %v", err)
	}
	defer func() { _ = watcher.Close() }()

	if watcher == nil {
		t.Fatal("NewWatcher() returned nil")
	}

	cfg := watcher.Config()
	if cfg == nil {
		t.Error("Config() returned nil")
	}
}

func TestNewWatcherNonExistent(t *testing.T) {
	logger := slog.Default()

	_, err := NewWatcher("nonexistent.yml", logger)
	if err == nil {
		t.Error("Expected error for non-existent file")
	}
}

func TestWatcherReload(t *testing.T) {
	logger := slog.Default()

	// Create a temporary config file
	tmpfile, err := os.CreateTemp("", "test-config-*.yml")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(tmpfile.Name()) }()

	// Write initial config
	initialConfig := `
server:
  listen_address: ":5353"
upstream_dns_servers:
  - "1.1.1.1:53"
logging:
  level: "info"
`
	if _, err := tmpfile.Write([]byte(initialConfig)); err != nil {
		t.Fatal(err)
	}
	_ = tmpfile.Close()

	// Create watcher
	watcher, err := NewWatcher(tmpfile.Name(), logger)
	if err != nil {
		t.Fatalf("NewWatcher() failed: %v", err)
	}
	defer func() { _ = watcher.Close() }()

	// Verify initial config
	cfg := watcher.Config()
	if cfg.Server.ListenAddress != ":5353" {
		t.Errorf("Initial listen address = %s, want :5353", cfg.Server.ListenAddress)
	}

	// Set up change notification
	changeDetected := make(chan bool, 1)
	watcher.OnChange(func(newCfg *Config) {
		changeDetected <- true
	})

	// Start watcher in background
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() {
		_ = watcher.Start(ctx)
	}()

	// Give watcher time to start
	time.Sleep(100 * time.Millisecond)

	// Update config file
	updatedConfig := `
server:
  listen_address: ":5454"
upstream_dns_servers:
  - "8.8.8.8:53"
logging:
  level: "debug"
`
	if err := os.WriteFile(tmpfile.Name(), []byte(updatedConfig), 0644); err != nil {
		t.Fatal(err)
	}

	// Wait for change notification (with timeout)
	select {
	case <-changeDetected:
		// Success - config changed
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for config change notification")
	}

	// Verify config was reloaded
	cfg = watcher.Config()
	if cfg.Server.ListenAddress != ":5454" {
		t.Errorf("Updated listen address = %s, want :5454", cfg.Server.ListenAddress)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("Updated log level = %s, want debug", cfg.Logging.Level)
	}
}

func TestWatcherConcurrentAccess(t *testing.T) {
	logger := slog.Default()

	watcher, err := NewWatcher("testdata/config.yml", logger)
	if err != nil {
		t.Fatalf("NewWatcher() failed: %v", err)
	}
	defer func() { _ = watcher.Close() }()

	// Test concurrent reads
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				cfg := watcher.Config()
				if cfg == nil {
					t.Error("Config() returned nil during concurrent access")
				}
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestWatcherClose(t *testing.T) {
	logger := slog.Default()

	watcher, err := NewWatcher("testdata/config.yml", logger)
	if err != nil {
		t.Fatalf("NewWatcher() failed: %v", err)
	}

	if err := watcher.Close(); err != nil {
		t.Errorf("Close() failed: %v", err)
	}

	// Multiple closes should not panic
	if err := watcher.Close(); err != nil {
		// This is OK - second close might return error
	}
}
