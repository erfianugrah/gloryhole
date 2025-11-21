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
	localMgr.AddRecord(localrecords.NewARecord("test.local.", net.ParseIP("192.168.1.100")))
	handler.SetLocalRecords(localMgr)

	// Setup policy engine
	policyEngine := policy.NewEngine()
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
		name          string
		domain        string
		qtype         uint16
		expectRcode   int
		expectAnswers int
		expectBlocked bool
		checkAnswer   func(*testing.T, *dns.Msg)
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
		server.Start(serverCtx)
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
	server.Shutdown(shutdownCtx)
	telem.Shutdown(ctx)
}
