package dns

import (
	"context"
	"net"
	"testing"
	"time"

	"glory-hole/pkg/config"
	"glory-hole/pkg/forwarder"
	"glory-hole/pkg/localrecords"
	"glory-hole/pkg/logging"
	"glory-hole/pkg/policy"
	"glory-hole/pkg/telemetry"

	"github.com/miekg/dns"
)

// TestE2E_FullDNSServer tests the complete DNS server stack end-to-end
func TestE2E_FullDNSServer(t *testing.T) {
	// Create test configuration
	cfg := &config.Config{
		Server: config.ServerConfig{
			ListenAddress: "127.0.0.1:15353", // Use non-standard port for testing
			TCPEnabled:    true,
			UDPEnabled:    true,
		},
		UpstreamDNSServers: []string{"1.1.1.1:53"},
	}

	// Initialize logger
	logger, err := logging.New(&config.LoggingConfig{
		Level:  "error", // Quiet for tests
		Format: "text",
		Output: "stdout",
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Initialize telemetry
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

	// Create handler
	handler := NewHandler()

	// Setup forwarder
	fwd := forwarder.NewForwarder(cfg, logger)
	handler.SetForwarder(fwd)

	// Setup local records
	localMgr := localrecords.NewManager()
	_ = localMgr.AddRecord(localrecords.NewARecord("test.local.", net.ParseIP("192.168.1.100")))
	handler.SetLocalRecords(localMgr)

	// Setup policy engine
	policyEngine := policy.NewEngine(nil)
	blockRule := &policy.Rule{
		Name:    "Block Test Domain",
		Logic:   `Domain == "blocked.test."`,
		Action:  policy.ActionBlock,
		Enabled: true,
	}
	if err := policyEngine.AddRule(blockRule); err != nil {
		t.Fatalf("Failed to add policy rule: %v", err)
	}
	handler.SetPolicyEngine(policyEngine)

	// Setup blocklist (manual add for testing)
	// Note: BlocklistManager is tested separately in pkg/blocklist tests
	handler.Blocklist["blocklist.test."] = struct{}{}

	// Create server
	server := NewServer(cfg, handler, logger, metrics)

	// Start server in background
	serverCtx, serverCancel := context.WithCancel(ctx)
	defer serverCancel()

	errChan := make(chan error, 1)
	go func() {
		if err := server.Start(serverCtx); err != nil {
			errChan <- err
		}
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Verify server is running
	if !server.IsRunning() {
		t.Fatal("Server failed to start")
	}

	// Run test cases
	testCases := []struct {
		checkAnswer   func(*testing.T, *dns.Msg)
		name          string
		domain        string
		expectRcode   int
		expectAnswers int
		qtype         uint16
		expectBlocked bool
	}{
		{
			name:          "Local Record Resolution",
			domain:        "test.local.",
			qtype:         dns.TypeA,
			expectRcode:   dns.RcodeSuccess,
			expectAnswers: 1,
			expectBlocked: false,
			checkAnswer: func(t *testing.T, msg *dns.Msg) {
				if len(msg.Answer) != 1 {
					t.Errorf("Expected 1 answer, got %d", len(msg.Answer))
					return
				}
				a, ok := msg.Answer[0].(*dns.A)
				if !ok {
					t.Errorf("Expected A record, got %T", msg.Answer[0])
					return
				}
				if a.A.String() != "192.168.1.100" {
					t.Errorf("Expected IP 192.168.1.100, got %s", a.A.String())
				}
			},
		},
		{
			name:          "Policy Engine Block",
			domain:        "blocked.test.",
			qtype:         dns.TypeA,
			expectRcode:   dns.RcodeNameError,
			expectAnswers: 0,
			expectBlocked: true,
		},
		{
			name:          "Blocklist Block",
			domain:        "blocklist.test.",
			qtype:         dns.TypeA,
			expectRcode:   dns.RcodeNameError,
			expectAnswers: 0,
			expectBlocked: true,
		},
		{
			name:          "Forward to Upstream",
			domain:        "google.com.",
			qtype:         dns.TypeA,
			expectRcode:   dns.RcodeSuccess,
			expectAnswers: -1, // Variable number of answers
			expectBlocked: false,
		},
	}

	client := &dns.Client{
		Timeout: 5 * time.Second,
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create query
			msg := new(dns.Msg)
			msg.SetQuestion(tc.domain, tc.qtype)
			msg.RecursionDesired = true

			// Send query
			resp, _, err := client.Exchange(msg, cfg.Server.ListenAddress)
			if err != nil {
				t.Fatalf("Failed to query DNS server: %v", err)
			}

			// Check response code
			if resp.Rcode != tc.expectRcode {
				t.Errorf("Expected rcode %d (%s), got %d (%s)",
					tc.expectRcode, dns.RcodeToString[tc.expectRcode],
					resp.Rcode, dns.RcodeToString[resp.Rcode])
			}

			// Check number of answers (if specified)
			if tc.expectAnswers >= 0 && len(resp.Answer) != tc.expectAnswers {
				t.Errorf("Expected %d answers, got %d", tc.expectAnswers, len(resp.Answer))
			}

			// Run custom answer check if provided
			if tc.checkAnswer != nil {
				tc.checkAnswer(t, resp)
			}
		})
	}

	// Shutdown server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		t.Errorf("Failed to shutdown server: %v", err)
	}

	// Verify server stopped
	if server.IsRunning() {
		t.Error("Server still running after shutdown")
	}

	// Cleanup
	if err := telem.Shutdown(ctx); err != nil {
		t.Errorf("Failed to shutdown telemetry: %v", err)
	}
}

// TestE2E_ConcurrentQueries tests the server under concurrent load
func TestE2E_ConcurrentQueries(t *testing.T) {
	// Create minimal test configuration
	cfg := &config.Config{
		Server: config.ServerConfig{
			ListenAddress: "127.0.0.1:15354", // Different port
			TCPEnabled:    true,
			UDPEnabled:    true,
		},
		UpstreamDNSServers: []string{"1.1.1.1:53"},
	}

	// Initialize logger
	logger, err := logging.New(&config.LoggingConfig{
		Level:  "error",
		Format: "text",
		Output: "stdout",
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Initialize telemetry
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

	// Create handler with forwarder
	handler := NewHandler()
	fwd := forwarder.NewForwarder(cfg, logger)
	handler.SetForwarder(fwd)

	// Create and start server
	server := NewServer(cfg, handler, logger, metrics)
	serverCtx, serverCancel := context.WithCancel(ctx)
	defer serverCancel()

	go func() {
		_ = server.Start(serverCtx)
	}()

	time.Sleep(100 * time.Millisecond)

	// Send concurrent queries
	numClients := 50
	queriesPerClient := 20
	done := make(chan bool, numClients)
	errors := make(chan error, numClients*queriesPerClient)

	client := &dns.Client{
		Timeout: 5 * time.Second,
	}

	for i := 0; i < numClients; i++ {
		go func(clientID int) {
			for j := 0; j < queriesPerClient; j++ {
				msg := new(dns.Msg)
				msg.SetQuestion("google.com.", dns.TypeA)

				resp, _, err := client.Exchange(msg, cfg.Server.ListenAddress)
				if err != nil {
					errors <- err
					continue
				}

				if resp.Rcode != dns.RcodeSuccess {
					errors <- err
				}
			}
			done <- true
		}(i)
	}

	// Wait for all clients to finish
	for i := 0; i < numClients; i++ {
		<-done
	}

	close(errors)

	// Check for errors
	errorCount := 0
	for err := range errors {
		if err != nil {
			errorCount++
		}
	}

	if errorCount > 0 {
		t.Logf("Warning: %d/%d queries failed", errorCount, numClients*queriesPerClient)
	}

	// Success rate should be high (allow 5% failures due to network)
	successRate := float64(numClients*queriesPerClient-errorCount) / float64(numClients*queriesPerClient)
	if successRate < 0.95 {
		t.Errorf("Success rate too low: %.2f%% (expected >= 95%%)", successRate*100)
	}

	// Shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)
	_ = telem.Shutdown(ctx)
}

// TestE2E_AllRecordTypes tests all DNS record types together
func TestE2E_AllRecordTypes(t *testing.T) {
	// Create test configuration
	cfg := &config.Config{
		Server: config.ServerConfig{
			ListenAddress: "127.0.0.1:15355",
			TCPEnabled:    true,
			UDPEnabled:    true,
		},
		UpstreamDNSServers: []string{"1.1.1.1:53"},
	}

	// Initialize logger
	logger, err := logging.New(&config.LoggingConfig{
		Level:  "error",
		Format: "text",
		Output: "stdout",
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Initialize telemetry
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

	// Create handler
	handler := NewHandler()

	// Setup forwarder
	fwd := forwarder.NewForwarder(cfg, logger)
	handler.SetForwarder(fwd)

	// Setup local records with all types
	localMgr := localrecords.NewManager()

	// A record
	_ = localMgr.AddRecord(localrecords.NewARecord("example.local.", net.ParseIP("192.168.1.100")))

	// AAAA record
	_ = localMgr.AddRecord(localrecords.NewAAAARecord("example.local.", net.ParseIP("fe80::1")))

	// CNAME record
	_ = localMgr.AddRecord(localrecords.NewCNAMERecord("alias.local.", "example.local."))

	// TXT record
	_ = localMgr.AddRecord(localrecords.NewTXTRecord("example.local.", []string{
		"v=spf1 include:_spf.example.com ~all",
		"google-site-verification=abc123",
	}))

	// MX records (multiple, different priorities)
	_ = localMgr.AddRecord(localrecords.NewMXRecord("example.local.", "mail1.example.local.", 10))
	_ = localMgr.AddRecord(localrecords.NewMXRecord("example.local.", "mail2.example.local.", 20))

	// PTR record
	_ = localMgr.AddRecord(localrecords.NewPTRRecord("100.1.168.192.in-addr.arpa.", "example.local."))

	// SRV record
	_ = localMgr.AddRecord(localrecords.NewSRVRecord("_ldap._tcp.example.local.", "ldap.example.local.", 0, 5, 389))

	// NS record
	_ = localMgr.AddRecord(localrecords.NewNSRecord("subdomain.example.local.", "ns1.example.local."))

	// SOA record
	_ = localMgr.AddRecord(localrecords.NewSOARecord("example.local.", "ns1.example.local.", "admin.example.local.", 1, 3600, 600, 86400, 300))

	// CAA record
	_ = localMgr.AddRecord(localrecords.NewCAARecord("example.local.", "issue", "letsencrypt.org", 0))

	// Wildcard A record
	wildcardA := localrecords.NewARecord("*.wild.local.", net.ParseIP("192.168.1.200"))
	wildcardA.Wildcard = true
	_ = localMgr.AddRecord(wildcardA)

	handler.SetLocalRecords(localMgr)

	// Create and start server
	server := NewServer(cfg, handler, logger, metrics)
	serverCtx, serverCancel := context.WithCancel(ctx)
	defer serverCancel()

	go func() {
		_ = server.Start(serverCtx)
	}()

	time.Sleep(100 * time.Millisecond)

	client := &dns.Client{
		Timeout: 5 * time.Second,
	}

	// Test cases for all record types
	testCases := []struct {
		checkAnswer   func(*testing.T, *dns.Msg)
		name          string
		domain        string
		qtype         uint16
		expectRcode   int
		expectAnswers int
	}{
		{
			name:          "A Record",
			domain:        "example.local.",
			qtype:         dns.TypeA,
			expectRcode:   dns.RcodeSuccess,
			expectAnswers: 1,
			checkAnswer: func(t *testing.T, msg *dns.Msg) {
				a, ok := msg.Answer[0].(*dns.A)
				if !ok {
					t.Errorf("Expected A record, got %T", msg.Answer[0])
					return
				}
				if a.A.String() != "192.168.1.100" {
					t.Errorf("Expected IP 192.168.1.100, got %s", a.A.String())
				}
			},
		},
		{
			name:          "AAAA Record",
			domain:        "example.local.",
			qtype:         dns.TypeAAAA,
			expectRcode:   dns.RcodeSuccess,
			expectAnswers: 1,
			checkAnswer: func(t *testing.T, msg *dns.Msg) {
				aaaa, ok := msg.Answer[0].(*dns.AAAA)
				if !ok {
					t.Errorf("Expected AAAA record, got %T", msg.Answer[0])
					return
				}
				if aaaa.AAAA.String() != "fe80::1" {
					t.Errorf("Expected IP fe80::1, got %s", aaaa.AAAA.String())
				}
			},
		},
		{
			name:          "CNAME Record",
			domain:        "alias.local.",
			qtype:         dns.TypeCNAME,
			expectRcode:   dns.RcodeSuccess,
			expectAnswers: 1,
			checkAnswer: func(t *testing.T, msg *dns.Msg) {
				cname, ok := msg.Answer[0].(*dns.CNAME)
				if !ok {
					t.Errorf("Expected CNAME record, got %T", msg.Answer[0])
					return
				}
				if cname.Target != "example.local." {
					t.Errorf("Expected target example.local., got %s", cname.Target)
				}
			},
		},
		{
			name:          "TXT Record",
			domain:        "example.local.",
			qtype:         dns.TypeTXT,
			expectRcode:   dns.RcodeSuccess,
			expectAnswers: 1,
			checkAnswer: func(t *testing.T, msg *dns.Msg) {
				txt, ok := msg.Answer[0].(*dns.TXT)
				if !ok {
					t.Errorf("Expected TXT record, got %T", msg.Answer[0])
					return
				}
				if len(txt.Txt) != 2 {
					t.Errorf("Expected 2 TXT strings, got %d", len(txt.Txt))
					return
				}
				if txt.Txt[0] != "v=spf1 include:_spf.example.com ~all" {
					t.Errorf("Unexpected TXT value: %s", txt.Txt[0])
				}
			},
		},
		{
			name:          "MX Records (Multiple)",
			domain:        "example.local.",
			qtype:         dns.TypeMX,
			expectRcode:   dns.RcodeSuccess,
			expectAnswers: 2,
			checkAnswer: func(t *testing.T, msg *dns.Msg) {
				if len(msg.Answer) != 2 {
					t.Errorf("Expected 2 MX records, got %d", len(msg.Answer))
					return
				}
				mx1, ok := msg.Answer[0].(*dns.MX)
				if !ok {
					t.Errorf("Expected MX record, got %T", msg.Answer[0])
					return
				}
				// Should be sorted by priority
				if mx1.Preference != 10 {
					t.Errorf("Expected priority 10, got %d", mx1.Preference)
				}
				if mx1.Mx != "mail1.example.local." {
					t.Errorf("Expected mail1.example.local., got %s", mx1.Mx)
				}
			},
		},
		{
			name:          "PTR Record",
			domain:        "100.1.168.192.in-addr.arpa.",
			qtype:         dns.TypePTR,
			expectRcode:   dns.RcodeSuccess,
			expectAnswers: 1,
			checkAnswer: func(t *testing.T, msg *dns.Msg) {
				ptr, ok := msg.Answer[0].(*dns.PTR)
				if !ok {
					t.Errorf("Expected PTR record, got %T", msg.Answer[0])
					return
				}
				if ptr.Ptr != "example.local." {
					t.Errorf("Expected example.local., got %s", ptr.Ptr)
				}
			},
		},
		{
			name:          "SRV Record",
			domain:        "_ldap._tcp.example.local.",
			qtype:         dns.TypeSRV,
			expectRcode:   dns.RcodeSuccess,
			expectAnswers: 1,
			checkAnswer: func(t *testing.T, msg *dns.Msg) {
				srv, ok := msg.Answer[0].(*dns.SRV)
				if !ok {
					t.Errorf("Expected SRV record, got %T", msg.Answer[0])
					return
				}
				if srv.Target != "ldap.example.local." {
					t.Errorf("Expected ldap.example.local., got %s", srv.Target)
				}
				if srv.Port != 389 {
					t.Errorf("Expected port 389, got %d", srv.Port)
				}
			},
		},
		{
			name:          "NS Record",
			domain:        "subdomain.example.local.",
			qtype:         dns.TypeNS,
			expectRcode:   dns.RcodeSuccess,
			expectAnswers: 1,
			checkAnswer: func(t *testing.T, msg *dns.Msg) {
				ns, ok := msg.Answer[0].(*dns.NS)
				if !ok {
					t.Errorf("Expected NS record, got %T", msg.Answer[0])
					return
				}
				if ns.Ns != "ns1.example.local." {
					t.Errorf("Expected ns1.example.local., got %s", ns.Ns)
				}
			},
		},
		{
			name:          "SOA Record",
			domain:        "example.local.",
			qtype:         dns.TypeSOA,
			expectRcode:   dns.RcodeSuccess,
			expectAnswers: 1,
			checkAnswer: func(t *testing.T, msg *dns.Msg) {
				soa, ok := msg.Answer[0].(*dns.SOA)
				if !ok {
					t.Errorf("Expected SOA record, got %T", msg.Answer[0])
					return
				}
				if soa.Ns != "ns1.example.local." {
					t.Errorf("Expected ns1.example.local., got %s", soa.Ns)
				}
				if soa.Mbox != "admin.example.local." {
					t.Errorf("Expected admin.example.local., got %s", soa.Mbox)
				}
			},
		},
		{
			name:          "CAA Record",
			domain:        "example.local.",
			qtype:         dns.TypeCAA,
			expectRcode:   dns.RcodeSuccess,
			expectAnswers: 1,
			checkAnswer: func(t *testing.T, msg *dns.Msg) {
				caa, ok := msg.Answer[0].(*dns.CAA)
				if !ok {
					t.Errorf("Expected CAA record, got %T", msg.Answer[0])
					return
				}
				if caa.Tag != "issue" {
					t.Errorf("Expected tag 'issue', got %s", caa.Tag)
				}
				if caa.Value != "letsencrypt.org" {
					t.Errorf("Expected value 'letsencrypt.org', got %s", caa.Value)
				}
			},
		},
		{
			name:          "Wildcard A Record",
			domain:        "test.wild.local.",
			qtype:         dns.TypeA,
			expectRcode:   dns.RcodeSuccess,
			expectAnswers: 1,
			checkAnswer: func(t *testing.T, msg *dns.Msg) {
				a, ok := msg.Answer[0].(*dns.A)
				if !ok {
					t.Errorf("Expected A record, got %T", msg.Answer[0])
					return
				}
				if a.A.String() != "192.168.1.200" {
					t.Errorf("Expected IP 192.168.1.200, got %s", a.A.String())
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			msg := new(dns.Msg)
			msg.SetQuestion(tc.domain, tc.qtype)
			msg.RecursionDesired = true

			resp, _, err := client.Exchange(msg, cfg.Server.ListenAddress)
			if err != nil {
				t.Fatalf("Failed to query DNS server: %v", err)
			}

			if resp.Rcode != tc.expectRcode {
				t.Errorf("Expected rcode %d (%s), got %d (%s)",
					tc.expectRcode, dns.RcodeToString[tc.expectRcode],
					resp.Rcode, dns.RcodeToString[resp.Rcode])
			}

			if len(resp.Answer) != tc.expectAnswers {
				t.Errorf("Expected %d answers, got %d", tc.expectAnswers, len(resp.Answer))
			}

			if tc.checkAnswer != nil {
				tc.checkAnswer(t, resp)
			}
		})
	}

	// Shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)
	_ = telem.Shutdown(ctx)
}

// TestE2E_MultipleRecordsSameDomain tests multiple records of different types for the same domain
func TestE2E_MultipleRecordsSameDomain(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			ListenAddress: "127.0.0.1:15356",
			TCPEnabled:    true,
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
	fwd := forwarder.NewForwarder(cfg, logger)
	handler.SetForwarder(fwd)

	localMgr := localrecords.NewManager()

	// Add multiple A records for load balancing
	_ = localMgr.AddRecord(localrecords.NewARecord("multi.local.", net.ParseIP("192.168.1.10")))
	_ = localMgr.AddRecord(localrecords.NewARecord("multi.local.", net.ParseIP("192.168.1.11")))
	_ = localMgr.AddRecord(localrecords.NewARecord("multi.local.", net.ParseIP("192.168.1.12")))

	// Add multiple TXT records
	_ = localMgr.AddRecord(localrecords.NewTXTRecord("multi.local.", []string{"txt1"}))
	_ = localMgr.AddRecord(localrecords.NewTXTRecord("multi.local.", []string{"txt2"}))

	// Add multiple MX records with different priorities
	_ = localMgr.AddRecord(localrecords.NewMXRecord("multi.local.", "mail1.multi.local.", 10))
	_ = localMgr.AddRecord(localrecords.NewMXRecord("multi.local.", "mail2.multi.local.", 20))
	_ = localMgr.AddRecord(localrecords.NewMXRecord("multi.local.", "mail3.multi.local.", 30))

	handler.SetLocalRecords(localMgr)

	server := NewServer(cfg, handler, logger, metrics)
	serverCtx, serverCancel := context.WithCancel(ctx)
	defer serverCancel()

	go func() {
		_ = server.Start(serverCtx)
	}()

	time.Sleep(100 * time.Millisecond)

	client := &dns.Client{
		Timeout: 5 * time.Second,
	}

	// Test multiple A records
	t.Run("Multiple A Records", func(t *testing.T) {
		msg := new(dns.Msg)
		msg.SetQuestion("multi.local.", dns.TypeA)
		msg.RecursionDesired = true

		resp, _, err := client.Exchange(msg, cfg.Server.ListenAddress)
		if err != nil {
			t.Fatalf("Failed to query: %v", err)
		}

		if len(resp.Answer) != 3 {
			t.Errorf("Expected 3 A records, got %d", len(resp.Answer))
		}

		// Verify all IPs are present
		ips := make(map[string]bool)
		for _, rr := range resp.Answer {
			if a, ok := rr.(*dns.A); ok {
				ips[a.A.String()] = true
			}
		}

		expectedIPs := []string{"192.168.1.10", "192.168.1.11", "192.168.1.12"}
		for _, ip := range expectedIPs {
			if !ips[ip] {
				t.Errorf("Expected IP %s not found in response", ip)
			}
		}
	})

	// Test multiple TXT records
	t.Run("Multiple TXT Records", func(t *testing.T) {
		msg := new(dns.Msg)
		msg.SetQuestion("multi.local.", dns.TypeTXT)
		msg.RecursionDesired = true

		resp, _, err := client.Exchange(msg, cfg.Server.ListenAddress)
		if err != nil {
			t.Fatalf("Failed to query: %v", err)
		}

		if len(resp.Answer) != 2 {
			t.Errorf("Expected 2 TXT records, got %d", len(resp.Answer))
		}
	})

	// Test multiple MX records (should be sorted by priority)
	t.Run("Multiple MX Records Sorted", func(t *testing.T) {
		msg := new(dns.Msg)
		msg.SetQuestion("multi.local.", dns.TypeMX)
		msg.RecursionDesired = true

		resp, _, err := client.Exchange(msg, cfg.Server.ListenAddress)
		if err != nil {
			t.Fatalf("Failed to query: %v", err)
		}

		if len(resp.Answer) != 3 {
			t.Errorf("Expected 3 MX records, got %d", len(resp.Answer))
		}

		// Verify sorting by priority
		for i := 0; i < len(resp.Answer)-1; i++ {
			mx1 := resp.Answer[i].(*dns.MX)
			mx2 := resp.Answer[i+1].(*dns.MX)
			if mx1.Preference > mx2.Preference {
				t.Errorf("MX records not sorted: %d before %d", mx1.Preference, mx2.Preference)
			}
		}
	})

	// Shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)
	_ = telem.Shutdown(ctx)
}
