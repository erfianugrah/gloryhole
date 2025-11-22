package storage

import (
	"context"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if !cfg.Enabled {
		t.Error("expected storage to be enabled by default")
	}

	if cfg.Backend != BackendSQLite {
		t.Errorf("expected backend to be sqlite, got %s", cfg.Backend)
	}

	if cfg.BufferSize < 1 {
		t.Error("expected buffer size to be positive")
	}

	if cfg.RetentionDays < 1 {
		t.Error("expected retention days to be positive")
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid sqlite config",
			config: Config{
				Enabled:       true,
				Backend:       BackendSQLite,
				BufferSize:    100,
				BatchSize:     50,
				RetentionDays: 7,
			},
			wantErr: false,
		},
		{
			name: "valid d1 config",
			config: Config{
				Enabled:       true,
				Backend:       BackendD1,
				BufferSize:    100,
				BatchSize:     50,
				RetentionDays: 7,
			},
			wantErr: false,
		},
		{
			name: "invalid backend",
			config: Config{
				Enabled: true,
				Backend: "invalid",
			},
			wantErr: true,
		},
		{
			name: "disabled storage",
			config: Config{
				Enabled:    false,
				Backend:    "invalid", // Should not error when disabled
				BufferSize: 10,        // Valid value since we skip validation when disabled
			},
			wantErr: false,
		},
		{
			name: "negative buffer size gets corrected",
			config: Config{
				Enabled:    true,
				Backend:    BackendSQLite,
				BufferSize: -10,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Check that negative values were corrected (only when enabled)
			if !tt.wantErr && tt.config.Enabled && tt.config.BufferSize < 1 {
				t.Error("expected buffer size to be corrected to positive value")
			}
		})
	}
}

func TestNewNoOpStorage(t *testing.T) {
	storage := NewNoOpStorage()
	if storage == nil {
		t.Fatal("expected non-nil storage")
	}

	ctx := context.Background()

	// Test all methods return without error
	if err := storage.LogQuery(ctx, &QueryLog{}); err != nil {
		t.Errorf("LogQuery() error = %v", err)
	}

	if _, err := storage.GetRecentQueries(ctx, 10, 0); err != nil {
		t.Errorf("GetRecentQueries() error = %v", err)
	}

	if _, err := storage.GetQueriesByDomain(ctx, "example.com", 10); err != nil {
		t.Errorf("GetQueriesByDomain() error = %v", err)
	}

	if _, err := storage.GetQueriesByClientIP(ctx, "192.168.1.1", 10); err != nil {
		t.Errorf("GetQueriesByClientIP() error = %v", err)
	}

	if _, err := storage.GetStatistics(ctx, time.Now()); err != nil {
		t.Errorf("GetStatistics() error = %v", err)
	}

	if _, err := storage.GetTopDomains(ctx, 10, false); err != nil {
		t.Errorf("GetTopDomains() error = %v", err)
	}

	if _, err := storage.GetBlockedCount(ctx, time.Now()); err != nil {
		t.Errorf("GetBlockedCount() error = %v", err)
	}

	if _, err := storage.GetQueryCount(ctx, time.Now()); err != nil {
		t.Errorf("GetQueryCount() error = %v", err)
	}

	if err := storage.Cleanup(ctx, time.Now()); err != nil {
		t.Errorf("Cleanup() error = %v", err)
	}

	if err := storage.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	if err := storage.Ping(ctx); err != nil {
		t.Errorf("Ping() error = %v", err)
	}
}

func TestNewWithDisabledConfig(t *testing.T) {
	cfg := &Config{
		Enabled: false,
	}

	storage, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if storage == nil {
		t.Fatal("expected non-nil storage")
	}

	// Should return no-op storage
	if _, ok := storage.(*NoOpStorage); !ok {
		t.Error("expected NoOpStorage when disabled")
	}
}

func TestNewWithInvalidBackend(t *testing.T) {
	cfg := &Config{
		Enabled: true,
		Backend: "invalid",
	}

	_, err := New(cfg)
	if err == nil {
		t.Error("expected error for invalid backend")
	}
}
