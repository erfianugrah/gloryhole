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

	if allowed, limited, _, label := mgr.Allow("192.168.1.1"); !allowed || limited || label != "global" {
		t.Fatalf("first request should be allowed with global label (allowed=%v limited=%v label=%s)", allowed, limited, label)
	}

	if allowed, limited, _, label := mgr.Allow("192.168.1.1"); allowed || !limited || label != "global" {
		t.Fatalf("second request immediately should be limited with global label (allowed=%v limited=%v label=%s)", allowed, limited, label)
	}
}

func TestManagerOverrideByClient(t *testing.T) {
	low := 0.5
	cfg := &config.RateLimitConfig{
		Enabled:           true,
		RequestsPerSecond: 50,
		Burst:             50,
		Action:            config.RateLimitActionNXDOMAIN,
		CleanupInterval:   time.Minute,
		Overrides: []config.RateLimitOverride{
			{
				Name:              "slow-client",
				Clients:           []string{"10.0.0.5"},
				RequestsPerSecond: &low,
				Burst:             intPtr(1),
				Action:            actionPtr(config.RateLimitActionDrop),
			},
		},
	}

	mgr := NewManager(cfg, logging.NewDefault())
	if mgr == nil {
		t.Fatalf("expected manager instance")
	}
	defer mgr.Stop()

	// Default client should not be throttled
	if allowed, limited, action, label := mgr.Allow("192.168.1.1"); !allowed || limited || action != cfg.Action || label != "global" {
		t.Fatalf("default client should use global settings")
	}

	// Override client should get drop action
	if allowed, limited, action, label := mgr.Allow("10.0.0.5"); !allowed || limited || action != config.RateLimitActionDrop || label != "slow-client" {
		t.Fatalf("first request for override client should pass with drop action")
	}

	if allowed, limited, action, label := mgr.Allow("10.0.0.5"); allowed || !limited || action != config.RateLimitActionDrop || label != "slow-client" {
		t.Fatalf("second request should be limited with drop action")
	}
}

func TestManagerOverrideByCIDR(t *testing.T) {
	low := 1.0
	cfg := &config.RateLimitConfig{
		Enabled:           true,
		RequestsPerSecond: 100,
		Burst:             100,
		Action:            config.RateLimitActionNXDOMAIN,
		CleanupInterval:   time.Minute,
		Overrides: []config.RateLimitOverride{
			{
				Name:              "iot",
				CIDRs:             []string{"192.168.10.0/24"},
				RequestsPerSecond: &low,
				Burst:             intPtr(1),
			},
		},
	}

	mgr := NewManager(cfg, logging.NewDefault())
	if mgr == nil {
		t.Fatalf("expected manager instance")
	}
	defer mgr.Stop()

	if allowed, limited, _, label := mgr.Allow("192.168.10.1"); !allowed || limited || label != "iot" {
		t.Fatalf("first request should be allowed")
	}

	if allowed, limited, _, label := mgr.Allow("192.168.10.1"); allowed || !limited || label != "iot" {
		t.Fatalf("second request from CIDR should be limited")
	}
}

func intPtr(v int) *int {
	return &v
}

func actionPtr(v config.RateLimitAction) *config.RateLimitAction {
	return &v
}
