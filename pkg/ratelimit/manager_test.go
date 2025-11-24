package ratelimit

import (
	"testing"
	"time"

	"glory-hole/pkg/config"
	"glory-hole/pkg/logging"
)

func TestManagerAllow(t *testing.T) {
	cfg := &config.RateLimitConfig{
		Enabled:           true,
		RequestsPerSecond: 1,
		Burst:             1,
		CleanupInterval:   5 * time.Second,
		MaxTrackedClients: 10,
	}
	mgr := NewManager(cfg, logging.NewDefault())
	if mgr == nil {
		t.Fatalf("expected manager instance")
	}
	defer mgr.Stop()

	if allowed, limited := mgr.Allow("192.168.1.1"); !allowed || limited {
		t.Fatalf("first request should be allowed, got allowed=%v limited=%v", allowed, limited)
	}

	if allowed, limited := mgr.Allow("192.168.1.1"); allowed || !limited {
		t.Fatalf("second request immediately should be limited, got allowed=%v limited=%v", allowed, limited)
	}
}
