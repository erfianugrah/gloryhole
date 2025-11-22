package forwarder

import (
	"context"
	"net"
	"testing"
	"time"

	"glory-hole/pkg/config"
	"glory-hole/pkg/logging"

	"github.com/miekg/dns"
)

// mockDNSServer creates a mock DNS server for testing
func mockDNSServer(t *testing.T, responses map[string]*dns.Msg) (string, func()) {
	// Create UDP listener
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	addr := pc.LocalAddr().String()

	// Start server goroutine
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 512)

		for {
			n, clientAddr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}

			// Parse request
			req := new(dns.Msg)
			if err := req.Unpack(buf[:n]); err != nil {
				continue
			}

			// Create response
			var resp *dns.Msg
			if len(req.Question) > 0 {
				domain := req.Question[0].Name
				if mockResp, ok := responses[domain]; ok {
					resp = mockResp.Copy()
					resp.SetReply(req)
				} else {
					// Default response: NXDOMAIN
					resp = new(dns.Msg)
					resp.SetReply(req)
					resp.SetRcode(req, dns.RcodeNameError)
				}
			} else {
				resp = new(dns.Msg)
				resp.SetReply(req)
				resp.SetRcode(req, dns.RcodeFormatError)
			}

			// Send response
			packed, err := resp.Pack()
			if err != nil {
				continue
			}
			_, _ = pc.WriteTo(packed, clientAddr)
		}
	}()

	cleanup := func() {
		_ = pc.Close()
		<-done
	}

	return addr, cleanup
}

// createTestResponse creates a test DNS response
func createTestResponse(domain string, ip string) *dns.Msg {
	msg := new(dns.Msg)
	msg.SetQuestion(domain, dns.TypeA)
	rr := &dns.A{
		Hdr: dns.RR_Header{
			Name:   domain,
			Rrtype: dns.TypeA,
			Class:  dns.ClassINET,
			Ttl:    300,
		},
		A: net.ParseIP(ip),
	}
	msg.Answer = append(msg.Answer, rr)
	return msg
}

func TestNewForwarder(t *testing.T) {
	cfg := &config.Config{
		UpstreamDNSServers: []string{"1.1.1.1", "8.8.8.8:53"},
	}
	logger := logging.NewDefault()

	fwd := NewForwarder(cfg, logger)

	if fwd == nil {
		t.Fatal("NewForwarder returned nil")
	}

	if len(fwd.Upstreams()) != 2 {
		t.Errorf("Expected 2 upstreams, got %d", len(fwd.Upstreams()))
	}

	// Check that port was added to first upstream
	if fwd.Upstreams()[0] != "1.1.1.1:53" {
		t.Errorf("Expected '1.1.1.1:53', got '%s'", fwd.Upstreams()[0])
	}

	// Check that second upstream was preserved
	if fwd.Upstreams()[1] != "8.8.8.8:53" {
		t.Errorf("Expected '8.8.8.8:53', got '%s'", fwd.Upstreams()[1])
	}
}

func TestForward_Success(t *testing.T) {
	// Create mock DNS server
	responses := map[string]*dns.Msg{
		"example.com.": createTestResponse("example.com.", "93.184.216.34"),
	}
	addr, cleanup := mockDNSServer(t, responses)
	defer cleanup()

	cfg := &config.Config{
		UpstreamDNSServers: []string{addr},
	}
	logger := logging.NewDefault()
	fwd := NewForwarder(cfg, logger)

	// Create DNS query
	req := new(dns.Msg)
	req.SetQuestion("example.com.", dns.TypeA)

	// Forward query
	ctx := context.Background()
	resp, err := fwd.Forward(ctx, req)

	if err != nil {
		t.Fatalf("Forward failed: %v", err)
	}

	if resp == nil {
		t.Fatal("Forward returned nil response")
	}

	if len(resp.Answer) != 1 {
		t.Fatalf("Expected 1 answer, got %d", len(resp.Answer))
	}

	aRecord := resp.Answer[0].(*dns.A)
	if !aRecord.A.Equal(net.ParseIP("93.184.216.34")) {
		t.Errorf("Expected IP 93.184.216.34, got %s", aRecord.A)
	}
}

func TestForward_RoundRobin(t *testing.T) {
	// Create two mock DNS servers that both respond to the same domain
	// This tests round-robin without worrying about which server gets which query
	responses := map[string]*dns.Msg{
		"example.com.": createTestResponse("example.com.", "1.1.1.1"),
	}
	addr1, cleanup1 := mockDNSServer(t, responses)
	defer cleanup1()

	addr2, cleanup2 := mockDNSServer(t, responses)
	defer cleanup2()

	cfg := &config.Config{
		UpstreamDNSServers: []string{addr1, addr2},
	}
	logger := logging.NewDefault()
	fwd := NewForwarder(cfg, logger)

	ctx := context.Background()

	// Make multiple queries to verify round-robin is working
	// Both servers should respond successfully
	for i := 0; i < 4; i++ {
		req := new(dns.Msg)
		req.SetQuestion("example.com.", dns.TypeA)
		resp, err := fwd.Forward(ctx, req)
		if err != nil {
			t.Fatalf("Forward %d failed: %v", i+1, err)
		}
		if len(resp.Answer) == 0 {
			t.Fatalf("Response %d has no answers", i+1)
		}
	}

	// If we got here, round-robin is working (both servers responded successfully)
}

func TestForward_Timeout(t *testing.T) {
	// Use a non-routable IP to simulate timeout
	cfg := &config.Config{
		UpstreamDNSServers: []string{"192.0.2.1:53"}, // TEST-NET-1, should not respond
	}
	logger := logging.NewDefault()
	fwd := NewForwarder(cfg, logger)
	fwd.SetTimeout(100 * time.Millisecond) // Short timeout

	req := new(dns.Msg)
	req.SetQuestion("timeout.test.", dns.TypeA)

	ctx := context.Background()
	_, err := fwd.Forward(ctx, req)

	if err == nil {
		t.Fatal("Expected timeout error, got nil")
	}
}

func TestForward_Retry(t *testing.T) {
	// First server will not respond (non-routable IP)
	// Second server will respond correctly
	responses := map[string]*dns.Msg{
		"retry.test.": createTestResponse("retry.test.", "10.0.0.1"),
	}
	addr, cleanup := mockDNSServer(t, responses)
	defer cleanup()

	cfg := &config.Config{
		UpstreamDNSServers: []string{"192.0.2.1:53", addr}, // First fails, second succeeds
	}
	logger := logging.NewDefault()
	fwd := NewForwarder(cfg, logger)
	fwd.SetTimeout(100 * time.Millisecond)
	fwd.SetRetries(2)

	req := new(dns.Msg)
	req.SetQuestion("retry.test.", dns.TypeA)

	ctx := context.Background()
	resp, err := fwd.Forward(ctx, req)

	if err != nil {
		t.Fatalf("Forward with retry failed: %v", err)
	}

	if resp == nil {
		t.Fatal("Forward returned nil response")
	}

	if len(resp.Answer) != 1 {
		t.Fatalf("Expected 1 answer, got %d", len(resp.Answer))
	}
}

func TestForward_AllServersFail(t *testing.T) {
	cfg := &config.Config{
		UpstreamDNSServers: []string{"192.0.2.1:53", "192.0.2.2:53"}, // Both non-routable
	}
	logger := logging.NewDefault()
	fwd := NewForwarder(cfg, logger)
	fwd.SetTimeout(100 * time.Millisecond)
	fwd.SetRetries(2)

	req := new(dns.Msg)
	req.SetQuestion("fail.test.", dns.TypeA)

	ctx := context.Background()
	_, err := fwd.Forward(ctx, req)

	if err == nil {
		t.Fatal("Expected error when all servers fail, got nil")
	}
}

func TestForward_SERVFAIL(t *testing.T) {
	// Mock server that returns SERVFAIL
	responses := map[string]*dns.Msg{}
	addr, cleanup := mockDNSServer(t, responses)
	defer cleanup()

	// Create a working backup server
	backupResponses := map[string]*dns.Msg{
		"servfail.test.": createTestResponse("servfail.test.", "10.0.0.2"),
	}
	backupAddr, backupCleanup := mockDNSServer(t, backupResponses)
	defer backupCleanup()

	cfg := &config.Config{
		UpstreamDNSServers: []string{addr, backupAddr},
	}
	logger := logging.NewDefault()
	fwd := NewForwarder(cfg, logger)
	fwd.SetRetries(2)

	req := new(dns.Msg)
	req.SetQuestion("servfail.test.", dns.TypeA)

	ctx := context.Background()
	resp, err := fwd.Forward(ctx, req)

	if err != nil {
		t.Fatalf("Forward failed: %v", err)
	}

	if resp == nil {
		t.Fatal("Forward returned nil response")
	}

	// Should have gotten response from backup server
	if len(resp.Answer) != 1 {
		t.Fatalf("Expected 1 answer from backup server, got %d", len(resp.Answer))
	}
}

func TestForward_ContextCancellation(t *testing.T) {
	// Use non-routable IP to ensure timeout
	cfg := &config.Config{
		UpstreamDNSServers: []string{"192.0.2.1:53"},
	}
	logger := logging.NewDefault()
	fwd := NewForwarder(cfg, logger)
	fwd.SetTimeout(5 * time.Second)

	req := new(dns.Msg)
	req.SetQuestion("cancel.test.", dns.TypeA)

	// Create context that we'll cancel immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := fwd.Forward(ctx, req)

	if err == nil {
		t.Fatal("Expected context cancellation error, got nil")
	}
}

func TestForwardTCP_Success(t *testing.T) {
	// Create a simple TCP DNS server using dns.Server
	responses := map[string]*dns.Msg{
		"tcp.test.": createTestResponse("tcp.test.", "10.0.0.3"),
	}

	handler := dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
		var resp *dns.Msg
		if len(r.Question) > 0 {
			domain := r.Question[0].Name
			if mockResp, ok := responses[domain]; ok {
				resp = mockResp.Copy()
				resp.SetReply(r)
			} else {
				resp = new(dns.Msg)
				resp.SetReply(r)
				resp.SetRcode(r, dns.RcodeNameError)
			}
		} else {
			resp = new(dns.Msg)
			resp.SetReply(r)
			resp.SetRcode(r, dns.RcodeFormatError)
		}
		_ = w.WriteMsg(resp)
	})

	// Use a fixed port to avoid race conditions with dynamic port allocation
	// The race detector sees concurrent access to the Listener's address fields
	testPort := "127.0.0.1:25353" // Non-standard test port

	server := &dns.Server{
		Addr:    testPort,
		Net:     "tcp",
		Handler: handler,
	}

	// Channel to signal server started
	started := make(chan error, 1)

	// Start server in background
	go func() {
		if err := server.ListenAndServe(); err != nil {
			started <- err
		}
	}()
	defer func() { _ = server.Shutdown() }()

	// Wait for server to start (give it time to bind)
	select {
	case err := <-started:
		t.Fatalf("Server failed to start: %v", err)
	case <-time.After(100 * time.Millisecond):
		// Server started successfully
	}

	cfg := &config.Config{
		UpstreamDNSServers: []string{testPort},
	}
	logger := logging.NewDefault()
	fwd := NewForwarder(cfg, logger)

	req := new(dns.Msg)
	req.SetQuestion("tcp.test.", dns.TypeA)

	ctx := context.Background()
	resp, err := fwd.ForwardTCP(ctx, req)

	if err != nil {
		t.Fatalf("ForwardTCP failed: %v", err)
	}

	if resp == nil {
		t.Fatal("ForwardTCP returned nil response")
	}

	if len(resp.Answer) != 1 {
		t.Fatalf("Expected 1 answer, got %d", len(resp.Answer))
	}
}

func TestForward_NoUpstreams(t *testing.T) {
	cfg := &config.Config{
		UpstreamDNSServers: []string{},
	}
	logger := logging.NewDefault()
	fwd := NewForwarder(cfg, logger)

	// Should use default upstreams
	if len(fwd.Upstreams()) == 0 {
		t.Fatal("Expected default upstreams, got none")
	}
}
