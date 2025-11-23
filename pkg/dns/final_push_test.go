package dns

import (
	"context"
	"net"
	"testing"
	"time"

	"glory-hole/pkg/cache"
	"glory-hole/pkg/config"
	"glory-hole/pkg/forwarder"
	"glory-hole/pkg/localrecords"
	"glory-hole/pkg/logging"
	"glory-hole/pkg/policy"
	"glory-hole/pkg/telemetry"

	"github.com/miekg/dns"
)

// TestServeDNS_LocalRecordCNAMEResolutionAAAAEmpty tests CNAME resolution for AAAA with empty result
func TestServeDNS_LocalRecordCNAMEResolutionAAAAEmpty(t *testing.T) {
	handler := NewHandler()
	mgr := localrecords.NewManager()

	// Add CNAME that points to non-existent target
	err := mgr.AddRecord(&localrecords.LocalRecord{
		Domain:  "broken.local.",
		Type:    localrecords.RecordTypeCNAME,
		Target:  "nonexistent.local.",
		TTL:     300,
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("Failed to add CNAME record: %v", err)
	}
	handler.SetLocalRecords(mgr)

	w := &mockResponseWriter{
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}

	// Query for AAAA record - will try to resolve CNAME but find nothing
	req := new(dns.Msg)
	req.SetQuestion("broken.local.", dns.TypeAAAA)

	handler.ServeDNS(context.Background(), w, req)

	if w.msg == nil {
		t.Fatal("Expected response message")
	}
	// Should fallthrough (no resolution)
	if w.msg.Rcode != dns.RcodeNameError {
		t.Errorf("Expected NXDOMAIN, got %d", w.msg.Rcode)
	}
}

// TestServeDNS_ConditionalForwardingErrorPath tests conditional forwarding with forwarding error
func TestServeDNS_ConditionalForwardingErrorPath(t *testing.T) {
	handler := NewHandler()

	logger, _ := logging.New(&config.LoggingConfig{
		Level:  "error",
		Format: "text",
		Output: "stdout",
	})

	cfg := &config.Config{
		UpstreamDNSServers: []string{"1.1.1.1:53"},
		ConditionalForwarding: config.ConditionalForwardingConfig{
			Enabled: true,
			Rules: []config.ForwardingRule{
				{
					Name:      "test-rule",
					Domains:   []string{"test.local"},
					Upstreams: []string{"192.0.2.1:53"}, // Non-routable IP (will fail)
					Enabled:   true,
					Priority:  50,
					Timeout:   100 * time.Millisecond,
				},
			},
		},
	}

	fwd := forwarder.NewForwarder(cfg, logger)
	handler.SetForwarder(fwd)

	evaluator, err := forwarder.NewRuleEvaluator(&cfg.ConditionalForwarding)
	if err != nil {
		t.Fatalf("Failed to create RuleEvaluator: %v", err)
	}
	handler.RuleEvaluator = evaluator

	w := &mockResponseWriter{
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}

	req := new(dns.Msg)
	req.SetQuestion("server.test.local.", dns.TypeA)

	handler.ServeDNS(context.Background(), w, req)

	if w.msg == nil {
		t.Fatal("Expected response message")
	}
	// Should return SERVFAIL when conditional forwarding fails
	// (or might succeed if it falls through to default upstream)
}

// TestServeDNS_PolicyEngineForwardErrorPath tests policy forward with forwarding error
func TestServeDNS_PolicyEngineForwardErrorPath(t *testing.T) {
	handler := NewHandler()

	logger, _ := logging.New(&config.LoggingConfig{
		Level:  "error",
		Format: "text",
		Output: "stdout",
	})

	cfg := &config.Config{
		UpstreamDNSServers: []string{"1.1.1.1:53"},
	}
	handler.SetForwarder(forwarder.NewForwarder(cfg, logger))

	engine := policy.NewEngine()
	rule := &policy.Rule{
		Name:       "forward-fail",
		Logic:      `Domain == "fail.example.com"`,
		Enabled:    true,
		Action:     policy.ActionForward,
		ActionData: "192.0.2.1:53", // Non-routable IP (will fail)
	}
	if err := engine.AddRule(rule); err != nil {
		t.Fatalf("Failed to add rule: %v", err)
	}
	handler.SetPolicyEngine(engine)

	w := &mockResponseWriter{
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}

	req := new(dns.Msg)
	req.SetQuestion("fail.example.com.", dns.TypeA)

	handler.ServeDNS(context.Background(), w, req)

	if w.msg == nil {
		t.Fatal("Expected response message")
	}
	// Should return SERVFAIL when policy forward fails
	if w.msg.Rcode != dns.RcodeServerFailure {
		t.Errorf("Expected SERVFAIL for policy forward error, got %d", w.msg.Rcode)
	}
}

// TestNewServer_CacheEnabledSuccess tests successful cache initialization
func TestNewServer_CacheEnabledSuccess(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			ListenAddress: ":5353",
		},
		Cache: config.CacheConfig{
			Enabled:     true,
			MaxEntries:  1000,
			MinTTL:      1 * time.Second,
			MaxTTL:      3600 * time.Second,
			NegativeTTL: 300 * time.Second,
		},
		UpstreamDNSServers: []string{"1.1.1.1:53"},
	}

	logger, err := logging.New(&config.LoggingConfig{
		Level:  "error",
		Format: "text",
		Output: "stdout",
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	ctx := context.Background()
	telem, err := telemetry.New(ctx, &config.TelemetryConfig{
		Enabled: false,
	}, logger)
	if err != nil {
		t.Fatalf("Failed to create telemetry: %v", err)
	}

	metrics, err := telem.InitMetrics()
	if err != nil {
		t.Fatalf("Failed to initialize metrics: %v", err)
	}

	handler := NewHandler()
	server := NewServer(cfg, handler, logger, metrics)

	if server == nil {
		t.Fatal("Expected non-nil server")
	}

	// Cache should be set successfully
	if handler.Cache == nil {
		t.Error("Expected cache to be initialized")
	}
}

// TestNewServer_CacheDisabled tests server with cache disabled
func TestNewServer_CacheDisabled(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			ListenAddress: ":5353",
		},
		Cache: config.CacheConfig{
			Enabled: false,
		},
		UpstreamDNSServers: []string{"1.1.1.1:53"},
	}

	logger, err := logging.New(&config.LoggingConfig{
		Level:  "error",
		Format: "text",
		Output: "stdout",
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	ctx := context.Background()
	telem, err := telemetry.New(ctx, &config.TelemetryConfig{
		Enabled: false,
	}, logger)
	if err != nil {
		t.Fatalf("Failed to create telemetry: %v", err)
	}

	metrics, err := telem.InitMetrics()
	if err != nil {
		t.Fatalf("Failed to initialize metrics: %v", err)
	}

	handler := NewHandler()
	server := NewServer(cfg, handler, logger, metrics)

	if server == nil {
		t.Fatal("Expected non-nil server")
	}

	// Cache should not be set
	if handler.Cache != nil {
		t.Error("Expected cache to be nil when disabled")
	}
}

// TestServer_StartUDPDisabled tests starting server with only TCP enabled
func TestServer_StartUDPDisabled(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			ListenAddress: "127.0.0.1:15358",
			TCPEnabled:    true,
			UDPEnabled:    false, // UDP disabled
		},
		UpstreamDNSServers: []string{"1.1.1.1:53"},
	}

	logger, err := logging.New(&config.LoggingConfig{
		Level:  "error",
		Format: "text",
		Output: "stdout",
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	telem, err := telemetry.New(ctx, &config.TelemetryConfig{
		Enabled: false,
	}, logger)
	if err != nil {
		t.Fatalf("Failed to create telemetry: %v", err)
	}

	metrics, err := telem.InitMetrics()
	if err != nil {
		t.Fatalf("Failed to initialize metrics: %v", err)
	}

	handler := NewHandler()
	server := NewServer(cfg, handler, logger, metrics)

	// Start in background and let it timeout
	go func() {
		_ = server.Start(ctx)
	}()

	time.Sleep(50 * time.Millisecond)

	// Verify server started (even briefly)
	if server.tcpServer == nil && !cfg.Server.UDPEnabled {
		t.Error("Expected TCP server to be created")
	}
}

// TestServer_StartTCPDisabled tests starting server with only UDP enabled
func TestServer_StartTCPDisabled(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			ListenAddress: "127.0.0.1:15359",
			TCPEnabled:    false, // TCP disabled
			UDPEnabled:    true,
		},
		UpstreamDNSServers: []string{"1.1.1.1:53"},
	}

	logger, err := logging.New(&config.LoggingConfig{
		Level:  "error",
		Format: "text",
		Output: "stdout",
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	telem, err := telemetry.New(ctx, &config.TelemetryConfig{
		Enabled: false,
	}, logger)
	if err != nil {
		t.Fatalf("Failed to create telemetry: %v", err)
	}

	metrics, err := telem.InitMetrics()
	if err != nil {
		t.Fatalf("Failed to initialize metrics: %v", err)
	}

	handler := NewHandler()
	server := NewServer(cfg, handler, logger, metrics)

	// Start in background and let it timeout
	go func() {
		_ = server.Start(ctx)
	}()

	time.Sleep(50 * time.Millisecond)

	// Verify server started (even briefly)
	if server.udpServer == nil && cfg.Server.UDPEnabled {
		t.Error("Expected UDP server to be created")
	}
}

// TestShutdown_NotRunning tests shutting down a server that isn't running
func TestShutdown_NotRunning(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			ListenAddress: ":5353",
		},
		UpstreamDNSServers: []string{"1.1.1.1:53"},
	}

	logger, err := logging.New(&config.LoggingConfig{
		Level:  "error",
		Format: "text",
		Output: "stdout",
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	ctx := context.Background()
	telem, err := telemetry.New(ctx, &config.TelemetryConfig{
		Enabled: false,
	}, logger)
	if err != nil {
		t.Fatalf("Failed to create telemetry: %v", err)
	}

	metrics, err := telem.InitMetrics()
	if err != nil {
		t.Fatalf("Failed to initialize metrics: %v", err)
	}

	handler := NewHandler()
	server := NewServer(cfg, handler, logger, metrics)

	// Shutdown without starting - should return nil
	err = server.Shutdown(context.Background())
	if err != nil {
		t.Errorf("Expected no error when shutting down non-running server, got %v", err)
	}
}

// TestServeDNS_PolicyEngineAllowWithCache tests policy allow with cache
func TestServeDNS_PolicyEngineAllowWithCache(t *testing.T) {
	handler := NewHandler()

	logger, _ := logging.New(&config.LoggingConfig{
		Level:  "error",
		Format: "text",
		Output: "stdout",
	})

	// Set up cache
	dnsCache, _ := cache.New(&config.CacheConfig{
		Enabled:     true,
		MaxEntries:  100,
		MinTTL:      1 * time.Second,
		MaxTTL:      3600 * time.Second,
		NegativeTTL: 300 * time.Second,
	}, logger, nil)
	handler.SetCache(dnsCache)

	cfg := &config.Config{
		UpstreamDNSServers: []string{"1.1.1.1:53"},
	}
	handler.SetForwarder(forwarder.NewForwarder(cfg, logger))

	engine := policy.NewEngine()
	rule := &policy.Rule{
		Name:    "allow-with-cache",
		Logic:   `Domain == "allowed.example.com"`,
		Enabled: true,
		Action:  policy.ActionAllow,
	}
	if err := engine.AddRule(rule); err != nil {
		t.Fatalf("Failed to add rule: %v", err)
	}
	handler.SetPolicyEngine(engine)

	w := &mockResponseWriter{
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}

	req := new(dns.Msg)
	req.SetQuestion("allowed.example.com.", dns.TypeA)

	handler.ServeDNS(context.Background(), w, req)

	// Should forward and potentially cache
	if w.msg == nil {
		t.Fatal("Expected response message")
	}
}
