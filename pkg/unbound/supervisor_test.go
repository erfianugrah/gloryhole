package unbound

import (
	"testing"

	"glory-hole/pkg/config"
	"glory-hole/pkg/logging"
)

func testLogger(t *testing.T) *logging.Logger {
	t.Helper()
	cfg := &config.LoggingConfig{Level: "error", Format: "text", Output: "stdout"}
	l, err := logging.New(cfg)
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	return l
}

func TestNewSupervisor(t *testing.T) {
	cfg := &config.UnboundConfig{
		Enabled:       true,
		Managed:       true,
		ListenPort:    5353,
		ConfigPath:    "/etc/unbound/unbound.conf",
		ControlSocket: "/var/run/unbound/control.sock",
	}

	s := NewSupervisor(cfg, testLogger(t))

	state, err := s.Status()
	if state != StateStopped {
		t.Errorf("expected state %q, got %q", StateStopped, state)
	}
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if s.ListenAddr() != "127.0.0.1:5353" {
		t.Errorf("expected listen addr 127.0.0.1:5353, got %s", s.ListenAddr())
	}
}

func TestSupervisorStateTransitions(t *testing.T) {
	cfg := &config.UnboundConfig{
		Enabled:       true,
		Managed:       true,
		ListenPort:    5353,
		ConfigPath:    "/etc/unbound/unbound.conf",
		ControlSocket: "/var/run/unbound/control.sock",
	}

	s := NewSupervisor(cfg, testLogger(t))

	// Initial state
	state, _ := s.Status()
	if state != StateStopped {
		t.Fatalf("initial state: expected %q, got %q", StateStopped, state)
	}

	// Transition to starting
	s.setState(StateStarting, nil)
	state, _ = s.Status()
	if state != StateStarting {
		t.Fatalf("expected %q, got %q", StateStarting, state)
	}

	// Transition to running
	s.setState(StateRunning, nil)
	state, _ = s.Status()
	if state != StateRunning {
		t.Fatalf("expected %q, got %q", StateRunning, state)
	}

	// Transition to degraded with error
	s.setState(StateDegraded, errForTest("health check failed"))
	state, err := s.Status()
	if state != StateDegraded {
		t.Fatalf("expected %q, got %q", StateDegraded, state)
	}
	if err == nil || err.Error() != "health check failed" {
		t.Fatalf("expected error 'health check failed', got %v", err)
	}

	// Transition to failed
	s.setState(StateFailed, errForTest("too many restarts"))
	state, err = s.Status()
	if state != StateFailed {
		t.Fatalf("expected %q, got %q", StateFailed, state)
	}
	if err == nil {
		t.Fatal("expected non-nil error")
	}

	// Transition back to stopped
	s.setState(StateStopped, nil)
	state, err = s.Status()
	if state != StateStopped {
		t.Fatalf("expected %q, got %q", StateStopped, state)
	}
	if err != nil {
		t.Fatalf("expected nil error after stop, got %v", err)
	}
}

func TestServerConfigGetSet(t *testing.T) {
	cfg := &config.UnboundConfig{
		Enabled:       true,
		Managed:       true,
		ListenPort:    5353,
		ControlSocket: "/var/run/unbound/control.sock",
	}

	s := NewSupervisor(cfg, testLogger(t))

	// Initially nil
	if s.ServerConfig() != nil {
		t.Fatal("expected nil server config initially")
	}

	// Set and get
	scfg := DefaultServerConfig(5353, "/var/run/unbound/control.sock")
	s.SetServerConfig(scfg)

	got := s.ServerConfig()
	if got == nil {
		t.Fatal("expected non-nil server config after set")
	}
	if got.Server.Port != 5353 {
		t.Errorf("expected port 5353, got %d", got.Server.Port)
	}
	if got.Server.NumThreads != 2 {
		t.Errorf("expected 2 threads, got %d", got.Server.NumThreads)
	}
}

func TestFindBinaryNotFound(t *testing.T) {
	_, err := findBinary("nonexistent-binary-xyz-123")
	if err == nil {
		t.Error("expected error for nonexistent binary")
	}
}

func TestStopIdempotent(t *testing.T) {
	cfg := &config.UnboundConfig{
		Enabled:       true,
		Managed:       true,
		ListenPort:    5353,
		ConfigPath:    "/etc/unbound/unbound.conf",
		ControlSocket: "/var/run/unbound/control.sock",
	}

	s := NewSupervisor(cfg, testLogger(t))

	// Stop on a never-started supervisor should not panic
	if err := s.Stop(); err != nil {
		t.Fatalf("expected nil error on stop, got %v", err)
	}

	state, _ := s.Status()
	if state != StateStopped {
		t.Errorf("expected %q after stop, got %q", StateStopped, state)
	}
}

func TestParseStats(t *testing.T) {
	output := `total.num.queries=1234
total.num.cachehits=500
total.num.cachemiss=734
total.recursion.time.avg=0.025000
msg.cache.count=100
rrset.cache.count=200
mem.total.sbrk=4194304
time.up=3600.5
num.query.type.A=800
num.query.type.AAAA=300
num.query.type.MX=134
num.answer.rcode.NOERROR=1200
num.answer.rcode.NXDOMAIN=34
`
	s := parseStats(output)

	if s.TotalQueries != 1234 {
		t.Errorf("expected 1234 queries, got %d", s.TotalQueries)
	}
	if s.CacheHits != 500 {
		t.Errorf("expected 500 cache hits, got %d", s.CacheHits)
	}
	if s.CacheMiss != 734 {
		t.Errorf("expected 734 cache miss, got %d", s.CacheMiss)
	}
	expectedRate := float64(500) / float64(1234) * 100
	if abs(s.CacheHitRate-expectedRate) > 0.01 {
		t.Errorf("expected cache hit rate ~%.2f%%, got %.2f%%", expectedRate, s.CacheHitRate)
	}
	if abs(s.AvgRecursionMs-25.0) > 0.01 {
		t.Errorf("expected ~25ms avg recursion, got %.2fms", s.AvgRecursionMs)
	}
	if s.MsgCacheCount != 100 {
		t.Errorf("expected 100 msg cache count, got %d", s.MsgCacheCount)
	}
	if s.RRSetCacheCount != 200 {
		t.Errorf("expected 200 rrset cache count, got %d", s.RRSetCacheCount)
	}
	if s.MemTotalBytes != 4194304 {
		t.Errorf("expected 4194304 mem bytes, got %d", s.MemTotalBytes)
	}
	if s.UptimeSeconds != 3600 {
		t.Errorf("expected 3600 uptime, got %d", s.UptimeSeconds)
	}
	if s.QueryTypes["A"] != 800 {
		t.Errorf("expected 800 A queries, got %d", s.QueryTypes["A"])
	}
	if s.QueryTypes["AAAA"] != 300 {
		t.Errorf("expected 300 AAAA queries, got %d", s.QueryTypes["AAAA"])
	}
	if s.ResponseCodes["NOERROR"] != 1200 {
		t.Errorf("expected 1200 NOERROR, got %d", s.ResponseCodes["NOERROR"])
	}
}

func TestParseStatsEmpty(t *testing.T) {
	s := parseStats("")
	if s.TotalQueries != 0 {
		t.Errorf("expected 0 queries for empty input, got %d", s.TotalQueries)
	}
	if s.CacheHitRate != 0 {
		t.Errorf("expected 0%% cache hit rate, got %.2f%%", s.CacheHitRate)
	}
}

// helpers

type testError string

func errForTest(msg string) error {
	return testError(msg)
}

func (e testError) Error() string {
	return string(e)
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
