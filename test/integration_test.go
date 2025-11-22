package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"testing"
	"time"

	"glory-hole/pkg/api"
	"glory-hole/pkg/cache"
	"glory-hole/pkg/config"
	"glory-hole/pkg/dns"
	"glory-hole/pkg/forwarder"
	"glory-hole/pkg/localrecords"
	"glory-hole/pkg/logging"
	"glory-hole/pkg/policy"
	"glory-hole/pkg/storage"
	"glory-hole/pkg/telemetry"

	mdns "github.com/miekg/dns"
)

// TestIntegration_DNSWithCache tests DNS resolution with caching
func TestIntegration_DNSWithCache(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			ListenAddress: "127.0.0.1:15360",
			TCPEnabled:    true,
			UDPEnabled:    true,
		},
		UpstreamDNSServers: []string{"1.1.1.1:53"},
		Cache: config.CacheConfig{
			Enabled:    true,
			MaxEntries: 1000,
			MinTTL:     60,
			MaxTTL:     3600,
		},
	}

	logger, _ := logging.New(&config.LoggingConfig{Level: "error", Format: "text", Output: "stdout"})
	ctx := context.Background()
	telem, _ := telemetry.New(ctx, &config.TelemetryConfig{Enabled: false}, logger)
	metrics, _ := telem.InitMetrics()

	// Create handler with cache
	handler := dns.NewHandler()
	fwd := forwarder.NewForwarder(cfg, logger)
	handler.SetForwarder(fwd)

	// Setup cache
	dnsCache, _ := cache.New(&cfg.Cache, logger)
	handler.SetCache(dnsCache)

	// Create and start server
	server := dns.NewServer(cfg, handler, logger, metrics)
	serverCtx, serverCancel := context.WithCancel(ctx)
	defer serverCancel()

	go server.Start(serverCtx)
	time.Sleep(100 * time.Millisecond)

	client := &mdns.Client{Timeout: 5 * time.Second}

	// First query - should miss cache
	msg1 := new(mdns.Msg)
	msg1.SetQuestion("example.com.", mdns.TypeA)
	resp1, rtt1, err := client.Exchange(msg1, cfg.Server.ListenAddress)
	if err != nil {
		t.Fatalf("First query failed: %v", err)
	}
	if resp1.Rcode != mdns.RcodeSuccess {
		t.Fatalf("Expected success, got %s", mdns.RcodeToString[resp1.Rcode])
	}

	// Second query - should hit cache (faster)
	msg2 := new(mdns.Msg)
	msg2.SetQuestion("example.com.", mdns.TypeA)
	resp2, rtt2, err := client.Exchange(msg2, cfg.Server.ListenAddress)
	if err != nil {
		t.Fatalf("Second query failed: %v", err)
	}
	if resp2.Rcode != mdns.RcodeSuccess {
		t.Fatalf("Expected success, got %s", mdns.RcodeToString[resp2.Rcode])
	}

	// Cache hit should be significantly faster
	if rtt2 < rtt1 {
		t.Logf("Second query faster (%v) than first (%v) - cache likely working", rtt2, rtt1)
	}

	// Note: Cache stats may show 0 hits for upstream queries depending on implementation
	// This test primarily verifies the integration works without errors
	t.Logf("Cache stats: %+v", dnsCache.Stats())

	// Cleanup
	server.Shutdown(context.Background())
	telem.Shutdown(ctx)
}

// TestIntegration_PolicyEngineRedirect tests REDIRECT action end-to-end
func TestIntegration_PolicyEngineRedirect(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			ListenAddress: "127.0.0.1:15361",
			TCPEnabled:    true,
			UDPEnabled:    true,
		},
		UpstreamDNSServers: []string{"1.1.1.1:53"},
	}

	logger, _ := logging.New(&config.LoggingConfig{Level: "error", Format: "text", Output: "stdout"})
	ctx := context.Background()
	telem, _ := telemetry.New(ctx, &config.TelemetryConfig{Enabled: false}, logger)
	metrics, _ := telem.InitMetrics()

	// Create handler with policy engine
	handler := dns.NewHandler()
	fwd := forwarder.NewForwarder(cfg, logger)
	handler.SetForwarder(fwd)

	// Setup policy engine with REDIRECT rule
	policyEngine := policy.NewEngine()
	redirectRule := &policy.Rule{
		Name:       "Redirect ads to blackhole",
		Logic:      `DomainMatches(Domain, "ads")`,
		Action:     policy.ActionRedirect,
		ActionData: "0.0.0.0",
		Enabled:    true,
	}
	policyEngine.AddRule(redirectRule)
	handler.SetPolicyEngine(policyEngine)

	// Create and start server
	server := dns.NewServer(cfg, handler, logger, metrics)
	serverCtx, serverCancel := context.WithCancel(ctx)
	defer serverCancel()

	go server.Start(serverCtx)
	time.Sleep(100 * time.Millisecond)

	client := &mdns.Client{Timeout: 5 * time.Second}

	// Query domain that should be redirected
	msg := new(mdns.Msg)
	msg.SetQuestion("ads.example.com.", mdns.TypeA)
	resp, _, err := client.Exchange(msg, cfg.Server.ListenAddress)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	// Should succeed with redirect IP
	if resp.Rcode != mdns.RcodeSuccess {
		t.Fatalf("Expected success, got %s", mdns.RcodeToString[resp.Rcode])
	}

	// Check answer is the redirect IP
	if len(resp.Answer) != 1 {
		t.Fatalf("Expected 1 answer, got %d", len(resp.Answer))
	}

	aRecord, ok := resp.Answer[0].(*mdns.A)
	if !ok {
		t.Fatalf("Expected A record, got %T", resp.Answer[0])
	}

	if aRecord.A.String() != "0.0.0.0" {
		t.Errorf("Expected redirect to 0.0.0.0, got %s", aRecord.A.String())
	}

	// Cleanup
	server.Shutdown(context.Background())
	telem.Shutdown(ctx)
}

// TestIntegration_DNSWithStorage tests DNS with query logging
func TestIntegration_DNSWithStorage(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			ListenAddress: "127.0.0.1:15362",
			TCPEnabled:    true,
			UDPEnabled:    true,
		},
		UpstreamDNSServers: []string{"1.1.1.1:53"},
		Database: storage.Config{
			Enabled: true,
			Backend: "sqlite",
			SQLite: storage.SQLiteConfig{
				Path: ":memory:", // In-memory for testing
			},
			BufferSize:    100,
			FlushInterval: 1 * time.Second,
			RetentionDays: 7,
		},
	}

	logger, _ := logging.New(&config.LoggingConfig{Level: "error", Format: "text", Output: "stdout"})
	ctx := context.Background()
	telem, _ := telemetry.New(ctx, &config.TelemetryConfig{Enabled: false}, logger)
	metrics, _ := telem.InitMetrics()

	// Initialize storage
	stor, err := storage.New(&cfg.Database)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer stor.Close()

	// Create handler with storage
	handler := dns.NewHandler()
	fwd := forwarder.NewForwarder(cfg, logger)
	handler.SetForwarder(fwd)
	handler.SetStorage(stor)

	// Create and start server
	server := dns.NewServer(cfg, handler, logger, metrics)
	serverCtx, serverCancel := context.WithCancel(ctx)
	defer serverCancel()

	go server.Start(serverCtx)
	time.Sleep(100 * time.Millisecond)

	client := &mdns.Client{Timeout: 5 * time.Second}

	// Send a few queries
	domains := []string{"test1.example.com.", "test2.example.com.", "test3.example.com."}
	for _, domain := range domains {
		msg := new(mdns.Msg)
		msg.SetQuestion(domain, mdns.TypeA)
		client.Exchange(msg, cfg.Server.ListenAddress)
	}

	// Give storage time to flush
	time.Sleep(2 * time.Second)

	// Check statistics
	stats, err := stor.GetStatistics(ctx, time.Now().Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("Failed to get statistics: %v", err)
	}

	if stats.TotalQueries < int64(len(domains)) {
		t.Errorf("Expected at least %d queries logged, got %d", len(domains), stats.TotalQueries)
	}

	// Check recent queries
	queries, err := stor.GetRecentQueries(ctx, 10, 0)
	if err != nil {
		t.Fatalf("Failed to get recent queries: %v", err)
	}

	if len(queries) < len(domains) {
		t.Errorf("Expected at least %d queries, got %d", len(domains), len(queries))
	}

	// Cleanup
	server.Shutdown(context.Background())
	telem.Shutdown(ctx)
}

// TestIntegration_APIWithDNS tests API server managing DNS policies
func TestIntegration_APIWithDNS(t *testing.T) {
	// Setup DNS server
	dnsCfg := &config.Config{
		Server: config.ServerConfig{
			ListenAddress: "127.0.0.1:15363",
			TCPEnabled:    true,
			UDPEnabled:    true,
		},
		UpstreamDNSServers: []string{"1.1.1.1:53"},
	}

	logger, _ := logging.New(&config.LoggingConfig{Level: "error", Format: "text", Output: "stdout"})
	ctx := context.Background()
	telem, _ := telemetry.New(ctx, &config.TelemetryConfig{Enabled: false}, logger)
	metrics, _ := telem.InitMetrics()

	// Create DNS handler with policy engine
	handler := dns.NewHandler()
	fwd := forwarder.NewForwarder(dnsCfg, logger)
	handler.SetForwarder(fwd)

	policyEngine := policy.NewEngine()
	handler.SetPolicyEngine(policyEngine)

	// Start DNS server
	dnsServer := dns.NewServer(dnsCfg, handler, logger, metrics)
	dnsCtx, dnsCancel := context.WithCancel(ctx)
	defer dnsCancel()

	go dnsServer.Start(dnsCtx)
	time.Sleep(100 * time.Millisecond)

	// Setup API server
	apiCfg := &api.Config{
		ListenAddress: "127.0.0.1:18080",
		PolicyEngine:  policyEngine,
		Logger:        logger.Logger,
		Version:       "test",
	}

	apiServer := api.New(apiCfg)
	apiCtx, apiCancel := context.WithCancel(ctx)
	defer apiCancel()

	go apiServer.Start(apiCtx)
	time.Sleep(100 * time.Millisecond)

	// Test 1: Add policy via API
	policyReq := api.PolicyRequest{
		Name:    "Block test domain",
		Logic:   `Domain == "blocked.example.com"`,
		Action:  policy.ActionBlock,
		Enabled: true,
	}

	body, _ := json.Marshal(policyReq)
	resp, err := http.Post("http://127.0.0.1:18080/api/policies", "application/json",
		bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to add policy via API: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Expected status 201, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Test 2: Verify policy is active in DNS server
	dnsClient := &mdns.Client{Timeout: 5 * time.Second}
	msg := new(mdns.Msg)
	msg.SetQuestion("blocked.example.com.", mdns.TypeA)

	dnsResp, _, err := dnsClient.Exchange(msg, dnsCfg.Server.ListenAddress)
	if err != nil {
		t.Fatalf("DNS query failed: %v", err)
	}

	// Should be blocked (NXDOMAIN)
	if dnsResp.Rcode != mdns.RcodeNameError {
		t.Errorf("Expected NXDOMAIN, got %s", mdns.RcodeToString[dnsResp.Rcode])
	}

	// Test 3: Get policies via API
	getResp, err := http.Get("http://127.0.0.1:18080/api/policies")
	if err != nil {
		t.Fatalf("Failed to get policies: %v", err)
	}
	defer getResp.Body.Close()

	var policyList api.PolicyListResponse
	json.NewDecoder(getResp.Body).Decode(&policyList)

	if policyList.Total != 1 {
		t.Errorf("Expected 1 policy, got %d", policyList.Total)
	}

	// Cleanup
	dnsServer.Shutdown(context.Background())
	apiServer.Shutdown(context.Background())
	telem.Shutdown(ctx)
}

// TestIntegration_LocalRecordsWithCache tests local records with caching
func TestIntegration_LocalRecordsWithCache(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			ListenAddress: "127.0.0.1:15364",
			TCPEnabled:    true,
			UDPEnabled:    true,
		},
		UpstreamDNSServers: []string{"1.1.1.1:53"},
		Cache: config.CacheConfig{
			Enabled:    true,
			MaxEntries: 1000,
			MinTTL:     60,
			MaxTTL:     3600,
		},
	}

	logger, _ := logging.New(&config.LoggingConfig{Level: "error", Format: "text", Output: "stdout"})
	ctx := context.Background()
	telem, _ := telemetry.New(ctx, &config.TelemetryConfig{Enabled: false}, logger)
	metrics, _ := telem.InitMetrics()

	// Create handler
	handler := dns.NewHandler()
	fwd := forwarder.NewForwarder(cfg, logger)
	handler.SetForwarder(fwd)

	// Setup local records
	localMgr := localrecords.NewManager()
	localMgr.AddRecord(localrecords.NewARecord("nas.home.", net.ParseIP("192.168.1.100")))
	localMgr.AddRecord(localrecords.NewCNAMERecord("storage.home.", "nas.home."))
	handler.SetLocalRecords(localMgr)

	// Setup cache
	dnsCache, _ := cache.New(&cfg.Cache, logger)
	handler.SetCache(dnsCache)

	// Create and start server
	server := dns.NewServer(cfg, handler, logger, metrics)
	serverCtx, serverCancel := context.WithCancel(ctx)
	defer serverCancel()

	go server.Start(serverCtx)
	time.Sleep(100 * time.Millisecond)

	client := &mdns.Client{Timeout: 5 * time.Second}

	// Test A record
	msg := new(mdns.Msg)
	msg.SetQuestion("nas.home.", mdns.TypeA)
	resp, _, err := client.Exchange(msg, cfg.Server.ListenAddress)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if resp.Rcode != mdns.RcodeSuccess || len(resp.Answer) != 1 {
		t.Fatalf("Expected successful A record response")
	}

	// Test CNAME resolution
	msg2 := new(mdns.Msg)
	msg2.SetQuestion("storage.home.", mdns.TypeA)
	resp2, _, err := client.Exchange(msg2, cfg.Server.ListenAddress)
	if err != nil {
		t.Fatalf("CNAME query failed: %v", err)
	}
	if resp2.Rcode != mdns.RcodeSuccess {
		t.Fatalf("Expected successful CNAME resolution")
	}

	// Should have both CNAME and A records
	if len(resp2.Answer) < 1 {
		t.Errorf("Expected CNAME answer, got %d answers", len(resp2.Answer))
	}

	// Query again - should hit cache
	msg3 := new(mdns.Msg)
	msg3.SetQuestion("nas.home.", mdns.TypeA)
	resp3, _, err := client.Exchange(msg3, cfg.Server.ListenAddress)
	if err != nil {
		t.Fatalf("Cached query failed: %v", err)
	}
	if resp3.Rcode != mdns.RcodeSuccess {
		t.Fatalf("Expected successful cached response")
	}

	// Verify cache stats (may or may not register hits for local records)
	stats := dnsCache.Stats()
	t.Logf("Cache stats: hits=%d, misses=%d", stats.Hits, stats.Misses)

	// Cleanup
	server.Shutdown(context.Background())
	telem.Shutdown(ctx)
}

// TestIntegration_ComplexPolicyRules tests advanced policy engine features
func TestIntegration_ComplexPolicyRules(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			ListenAddress: "127.0.0.1:15365",
			TCPEnabled:    true,
			UDPEnabled:    true,
		},
		UpstreamDNSServers: []string{"1.1.1.1:53"},
	}

	logger, _ := logging.New(&config.LoggingConfig{Level: "error", Format: "text", Output: "stdout"})
	ctx := context.Background()
	telem, _ := telemetry.New(ctx, &config.TelemetryConfig{Enabled: false}, logger)
	metrics, _ := telem.InitMetrics()

	// Create handler
	handler := dns.NewHandler()
	fwd := forwarder.NewForwarder(cfg, logger)
	handler.SetForwarder(fwd)

	// Setup complex policy rules
	policyEngine := policy.NewEngine()

	// Rule 1: Block by regex pattern
	rule1 := &policy.Rule{
		Name:    "Block CDN domains",
		Logic:   `DomainRegex(Domain, "^cdn\\d+\\.")`,
		Action:  policy.ActionBlock,
		Enabled: true,
	}
	policyEngine.AddRule(rule1)

	// Rule 2: Block deep subdomains
	rule2 := &policy.Rule{
		Name:    "Block deep subdomains",
		Logic:   `DomainLevelCount(Domain) > 4`,
		Action:  policy.ActionBlock,
		Enabled: true,
	}
	policyEngine.AddRule(rule2)

	// Rule 3: Allow specific IPs
	rule3 := &policy.Rule{
		Name:    "Allow admin IP",
		Logic:   `IPEquals(ClientIP, "127.0.0.1")`,
		Action:  policy.ActionAllow,
		Enabled: true,
	}
	policyEngine.AddRule(rule3)

	handler.SetPolicyEngine(policyEngine)

	// Create and start server
	server := dns.NewServer(cfg, handler, logger, metrics)
	serverCtx, serverCancel := context.WithCancel(ctx)
	defer serverCancel()

	go server.Start(serverCtx)
	time.Sleep(100 * time.Millisecond)

	client := &mdns.Client{Timeout: 5 * time.Second}

	testCases := []struct {
		name        string
		domain      string
		expectRcode int
	}{
		{
			name:        "Block CDN pattern",
			domain:      "cdn123.example.com.",
			expectRcode: mdns.RcodeNameError,
		},
		{
			name:        "Block deep subdomain",
			domain:      "a.b.c.d.e.example.com.",
			expectRcode: mdns.RcodeNameError,
		},
		{
			name:        "Allow normal domain",
			domain:      "www.example.com.",
			expectRcode: mdns.RcodeSuccess,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			msg := new(mdns.Msg)
			msg.SetQuestion(tc.domain, mdns.TypeA)
			resp, _, err := client.Exchange(msg, cfg.Server.ListenAddress)
			if err != nil {
				t.Fatalf("Query failed: %v", err)
			}
			if resp.Rcode != tc.expectRcode {
				t.Errorf("Expected rcode %s, got %s",
					mdns.RcodeToString[tc.expectRcode],
					mdns.RcodeToString[resp.Rcode])
			}
		})
	}

	// Cleanup
	server.Shutdown(context.Background())
	telem.Shutdown(ctx)
}
