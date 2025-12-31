package dns

import (
	"context"
	"net"
	"testing"
	"time"

	"glory-hole/pkg/config"
	"glory-hole/pkg/forwarder"
	"glory-hole/pkg/logging"
	"glory-hole/pkg/policy"
	"glory-hole/pkg/telemetry"

	"github.com/miekg/dns"
)

// TestConditionalForwarding_DomainMatching tests domain-based conditional forwarding
func TestConditionalForwarding_DomainMatching(t *testing.T) {
	// Create mock upstream DNS server for *.local domains
	mockUpstream := createMockDNSServer(t, "192.168.1.100")
	defer func() { _ = mockUpstream.Shutdown() }()

	// Create test configuration with conditional forwarding
	cfg := &config.Config{
		Server: config.ServerConfig{
			ListenAddress: "127.0.0.1:15354",
			UDPEnabled:    true,
		},
		UpstreamDNSServers: []string{"1.1.1.1:53"},
		ConditionalForwarding: config.ConditionalForwardingConfig{
			Enabled: true,
			Rules: []config.ForwardingRule{
				{
					Name:      "Local domains",
					Priority:  90,
					Domains:   []string{"*.local"},
					Upstreams: []string{mockUpstream.PacketConn.LocalAddr().String()},
					Enabled:   true,
				},
			},
		},
	}

	// Setup server
	logger, _ := logging.New(&config.LoggingConfig{
		Level:  "error",
		Format: "text",
		Output: "stdout",
	})

	ctx := context.Background()
	telem, _ := telemetry.New(ctx, &config.TelemetryConfig{Enabled: false}, logger)
	metrics, _ := telem.InitMetrics()

	handler := NewHandler()
	fwd := forwarder.NewForwarder(cfg, logger)
	handler.SetForwarder(fwd)

	// Initialize conditional forwarding
	ruleEvaluator, err := forwarder.NewRuleEvaluator(&cfg.ConditionalForwarding)
	if err != nil {
		t.Fatalf("Failed to create rule evaluator: %v", err)
	}
	handler.RuleEvaluator = ruleEvaluator

	server := NewServer(cfg, handler, logger, metrics)

	// Start server
	serverCtx, serverCancel := context.WithCancel(ctx)
	defer serverCancel()

	go func() {
		_ = server.Start(serverCtx)
	}()

	time.Sleep(100 * time.Millisecond)

	// Test 1: Query for *.local domain should be forwarded to mock upstream
	client := &dns.Client{Timeout: 2 * time.Second}
	m := new(dns.Msg)
	m.SetQuestion("test.local.", dns.TypeA)

	r, _, err := client.Exchange(m, cfg.Server.ListenAddress)
	if err != nil {
		t.Fatalf("DNS query failed: %v", err)
	}

	if len(r.Answer) == 0 {
		t.Fatal("Expected answer, got none")
	}

	// Verify response is from mock upstream (192.168.1.100)
	if aRecord, ok := r.Answer[0].(*dns.A); ok {
		if aRecord.A.String() != "192.168.1.100" {
			t.Errorf("Expected IP 192.168.1.100, got %s", aRecord.A.String())
		}
	} else {
		t.Error("Expected A record in answer")
	}

	// Test 2: Query for non-local domain should use default upstream
	m2 := new(dns.Msg)
	m2.SetQuestion("example.com.", dns.TypeA)

	r2, _, err := client.Exchange(m2, cfg.Server.ListenAddress)
	if err != nil {
		t.Fatalf("DNS query failed: %v", err)
	}

	// Should get a real response from default upstream (not our mock)
	if len(r2.Answer) > 0 {
		if aRecord, ok := r2.Answer[0].(*dns.A); ok {
			// Should NOT be from mock upstream
			if aRecord.A.String() == "192.168.1.100" {
				t.Error("Query should not have been forwarded to mock upstream")
			}
		}
	}
}

// TestConditionalForwarding_PriorityOrdering tests that higher priority rules match first
func TestConditionalForwarding_PriorityOrdering(t *testing.T) {
	// Create two mock upstream servers
	mockUpstream1 := createMockDNSServer(t, "10.0.0.1")
	defer func() { _ = mockUpstream1.Shutdown() }()

	mockUpstream2 := createMockDNSServer(t, "10.0.0.2")
	defer func() { _ = mockUpstream2.Shutdown() }()

	cfg := &config.Config{
		Server: config.ServerConfig{
			ListenAddress: "127.0.0.1:15355",
			UDPEnabled:    true,
		},
		UpstreamDNSServers: []string{"1.1.1.1:53"},
		ConditionalForwarding: config.ConditionalForwardingConfig{
			Enabled: true,
			Rules: []config.ForwardingRule{
				{
					Name:      "Specific subdomain (high priority)",
					Priority:  90,
					Domains:   []string{"nas.local"},
					Upstreams: []string{mockUpstream1.PacketConn.LocalAddr().String()},
					Enabled:   true,
				},
				{
					Name:      "Wildcard (low priority)",
					Priority:  10,
					Domains:   []string{"*.local"},
					Upstreams: []string{mockUpstream2.PacketConn.LocalAddr().String()},
					Enabled:   true,
				},
			},
		},
	}

	logger, _ := logging.New(&config.LoggingConfig{
		Level:  "error",
		Format: "text",
		Output: "stdout",
	})

	ctx := context.Background()
	telem, _ := telemetry.New(ctx, &config.TelemetryConfig{Enabled: false}, logger)
	metrics, _ := telem.InitMetrics()

	handler := NewHandler()
	fwd := forwarder.NewForwarder(cfg, logger)
	handler.SetForwarder(fwd)

	ruleEvaluator, err := forwarder.NewRuleEvaluator(&cfg.ConditionalForwarding)
	if err != nil {
		t.Fatalf("Failed to create rule evaluator: %v", err)
	}
	handler.RuleEvaluator = ruleEvaluator

	server := NewServer(cfg, handler, logger, metrics)

	serverCtx, serverCancel := context.WithCancel(ctx)
	defer serverCancel()

	go func() {
		_ = server.Start(serverCtx)
	}()

	time.Sleep(100 * time.Millisecond)

	client := &dns.Client{Timeout: 2 * time.Second}

	// Test 1: nas.local matches both rules, but higher priority should win
	m1 := new(dns.Msg)
	m1.SetQuestion("nas.local.", dns.TypeA)

	r1, _, err := client.Exchange(m1, cfg.Server.ListenAddress)
	if err != nil {
		t.Fatalf("DNS query failed: %v", err)
	}

	if len(r1.Answer) == 0 {
		t.Fatal("Expected answer for nas.local")
	}

	// Should be from mockUpstream1 (10.0.0.1)
	if aRecord, ok := r1.Answer[0].(*dns.A); ok {
		if aRecord.A.String() != "10.0.0.1" {
			t.Errorf("Expected IP 10.0.0.1 (high priority), got %s", aRecord.A.String())
		}
	}

	// Test 2: router.local only matches wildcard rule
	m2 := new(dns.Msg)
	m2.SetQuestion("router.local.", dns.TypeA)

	r2, _, err := client.Exchange(m2, cfg.Server.ListenAddress)
	if err != nil {
		t.Fatalf("DNS query failed: %v", err)
	}

	if len(r2.Answer) == 0 {
		t.Fatal("Expected answer for router.local")
	}

	// Should be from mockUpstream2 (10.0.0.2)
	if aRecord, ok := r2.Answer[0].(*dns.A); ok {
		if aRecord.A.String() != "10.0.0.2" {
			t.Errorf("Expected IP 10.0.0.2 (wildcard), got %s", aRecord.A.String())
		}
	}
}

// TestConditionalForwarding_PolicyForward tests policy engine FORWARD action
func TestConditionalForwarding_PolicyForward(t *testing.T) {
	mockUpstream := createMockDNSServer(t, "192.168.1.50")
	defer func() { _ = mockUpstream.Shutdown() }()

	cfg := &config.Config{
		Server: config.ServerConfig{
			ListenAddress: "127.0.0.1:15356",
			UDPEnabled:    true,
		},
		UpstreamDNSServers: []string{"1.1.1.1:53"},
	}

	logger, _ := logging.New(&config.LoggingConfig{
		Level:  "error",
		Format: "text",
		Output: "stdout",
	})

	ctx := context.Background()
	telem, _ := telemetry.New(ctx, &config.TelemetryConfig{Enabled: false}, logger)
	metrics, _ := telem.InitMetrics()

	handler := NewHandler()
	fwd := forwarder.NewForwarder(cfg, logger)
	handler.SetForwarder(fwd)

	// Setup policy engine with FORWARD action
	policyEngine := policy.NewEngine(nil)
	rule := &policy.Rule{
		Name:       "Forward local domains",
		Logic:      `DomainMatches(Domain, ".local")`,
		Action:     policy.ActionForward,
		ActionData: mockUpstream.PacketConn.LocalAddr().String(),
		Enabled:    true,
	}
	if err := policyEngine.AddRule(rule); err != nil {
		t.Fatalf("Failed to add policy rule: %v", err)
	}
	handler.SetPolicyEngine(policyEngine)

	server := NewServer(cfg, handler, logger, metrics)

	serverCtx, serverCancel := context.WithCancel(ctx)
	defer serverCancel()

	go func() {
		_ = server.Start(serverCtx)
	}()

	time.Sleep(100 * time.Millisecond)

	// Query for .local domain should be forwarded via policy engine
	client := &dns.Client{Timeout: 2 * time.Second}
	m := new(dns.Msg)
	m.SetQuestion("test.local.", dns.TypeA)

	r, _, err := client.Exchange(m, cfg.Server.ListenAddress)
	if err != nil {
		t.Fatalf("DNS query failed: %v", err)
	}

	if len(r.Answer) == 0 {
		t.Fatal("Expected answer from policy FORWARD")
	}

	// Verify response is from mock upstream
	if aRecord, ok := r.Answer[0].(*dns.A); ok {
		if aRecord.A.String() != "192.168.1.50" {
			t.Errorf("Expected IP 192.168.1.50, got %s", aRecord.A.String())
		}
	}
}

// createMockDNSServer creates a simple mock DNS server that responds with a fixed IP
func createMockDNSServer(t *testing.T, responseIP string) *dns.Server {
	t.Helper()

	// Create handler that responds with fixed IP
	handler := dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)

		if len(r.Question) > 0 {
			q := r.Question[0]
			if q.Qtype == dns.TypeA {
				rr := &dns.A{
					Hdr: dns.RR_Header{
						Name:   q.Name,
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    300,
					},
					A: net.ParseIP(responseIP).To4(),
				}
				m.Answer = append(m.Answer, rr)
			}
		}

		_ = w.WriteMsg(m)
	})

	// Create server on random port
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create mock DNS server: %v", err)
	}

	server := &dns.Server{
		PacketConn: pc,
		Handler:    handler,
	}

	go func() {
		_ = server.ActivateAndServe()
	}()

	// Wait for server to start
	time.Sleep(50 * time.Millisecond)

	return server
}
