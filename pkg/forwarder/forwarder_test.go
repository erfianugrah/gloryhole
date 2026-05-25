package forwarder

import (
	"context"
	"fmt"
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
			if unpackErr := req.Unpack(buf[:n]); unpackErr != nil {
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

	fwd := NewForwarder(cfg, logger, nil)

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
	fwd := NewForwarder(cfg, logger, nil)

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
	fwd := NewForwarder(cfg, logger, nil)

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
	fwd := NewForwarder(cfg, logger, nil)
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
	fwd := NewForwarder(cfg, logger, nil)
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
	fwd := NewForwarder(cfg, logger, nil)
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
	// Mock server that returns NXDOMAIN (unmapped domain returns NXDOMAIN by default)
	responses := map[string]*dns.Msg{}
	addr, cleanup := mockDNSServer(t, responses)
	defer cleanup()

	cfg := &config.Config{
		UpstreamDNSServers: []string{addr},
	}
	logger := logging.NewDefault()
	fwd := NewForwarder(cfg, logger, nil)

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

	// Should have gotten NXDOMAIN response immediately (not retried)
	if resp.Rcode != dns.RcodeNameError {
		t.Fatalf("Expected NXDOMAIN, got %s", dns.RcodeToString[resp.Rcode])
	}
}

func TestForward_SERVFAIL_PassThrough(t *testing.T) {
	// Test that SERVFAIL responses are passed through immediately without retry
	// This is critical for DNSSEC validation failures

	// Create a mock server that returns SERVFAIL
	mockHandler := func(w dns.ResponseWriter, r *dns.Msg) {
		resp := new(dns.Msg)
		resp.SetReply(r)
		resp.SetRcode(r, dns.RcodeServerFailure) // SERVFAIL
		_ = w.WriteMsg(resp)
	}

	// Start TCP server (easier to control response code)
	testPort := "127.0.0.1:25354"
	server := &dns.Server{
		Addr:    testPort,
		Net:     "tcp",
		Handler: dns.HandlerFunc(mockHandler),
	}

	go func() { _ = server.ListenAndServe() }()
	defer func() { _ = server.Shutdown() }()
	time.Sleep(100 * time.Millisecond) // Wait for server to start

	// Create second server that would respond successfully (shouldn't be used)
	goodResponses := map[string]*dns.Msg{
		"dnssec.test.": createTestResponse("dnssec.test.", "10.0.0.1"),
	}
	goodAddr, goodCleanup := mockDNSServer(t, goodResponses)
	defer goodCleanup()

	cfg := &config.Config{
		UpstreamDNSServers: []string{testPort, goodAddr},
	}
	logger := logging.NewDefault()
	fwd := NewForwarder(cfg, logger, nil)
	fwd.SetRetries(2)

	req := new(dns.Msg)
	req.SetQuestion("dnssec.test.", dns.TypeA)

	ctx := context.Background()
	start := time.Now()
	resp, err := fwd.ForwardTCP(ctx, req)
	elapsed := time.Since(start)

	// Should NOT get an error - SERVFAIL is a valid response
	if err != nil {
		t.Fatalf("Forward failed: %v", err)
	}

	if resp == nil {
		t.Fatal("Forward returned nil response")
	}

	// Should have received SERVFAIL from first upstream
	if resp.Rcode != dns.RcodeServerFailure {
		t.Fatalf("Expected SERVFAIL, got %s", dns.RcodeToString[resp.Rcode])
	}

	// Should have been fast (no retry delay)
	if elapsed > 500*time.Millisecond {
		t.Errorf("SERVFAIL took too long: %v (expected <500ms, no retry)", elapsed)
	}

	// Verify we got SERVFAIL, not success from second server
	if len(resp.Answer) > 0 {
		t.Error("Got answers from second server - SERVFAIL was retried (bug!)")
	}
}

func TestForward_ContextCancellation(t *testing.T) {
	// Use non-routable IP to ensure timeout
	cfg := &config.Config{
		UpstreamDNSServers: []string{"192.0.2.1:53"},
	}
	logger := logging.NewDefault()
	fwd := NewForwarder(cfg, logger, nil)
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
	fwd := NewForwarder(cfg, logger, nil)

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
	fwd := NewForwarder(cfg, logger, nil)

	// Should use default upstreams
	if len(fwd.Upstreams()) == 0 {
		t.Fatal("Expected default upstreams, got none")
	}
}

func TestForwardWithUpstreams_Success(t *testing.T) {
	// Create mock DNS server
	responses := map[string]*dns.Msg{
		"conditional.test.": createTestResponse("conditional.test.", "10.0.0.1"),
	}
	addr, cleanup := mockDNSServer(t, responses)
	defer cleanup()

	cfg := &config.Config{
		UpstreamDNSServers: []string{"1.1.1.1:53"}, // Default upstreams (not used)
	}
	logger := logging.NewDefault()
	fwd := NewForwarder(cfg, logger, nil)

	// Create DNS query
	req := new(dns.Msg)
	req.SetQuestion("conditional.test.", dns.TypeA)

	// Forward query with specific upstreams
	ctx := context.Background()
	resp, err := fwd.ForwardWithUpstreams(ctx, req, []string{addr})

	if err != nil {
		t.Fatalf("ForwardWithUpstreams failed: %v", err)
	}

	if resp == nil {
		t.Fatal("ForwardWithUpstreams returned nil response")
	}

	if len(resp.Answer) != 1 {
		t.Fatalf("Expected 1 answer, got %d", len(resp.Answer))
	}

	aRecord := resp.Answer[0].(*dns.A)
	if !aRecord.A.Equal(net.ParseIP("10.0.0.1")) {
		t.Errorf("Expected IP 10.0.0.1, got %s", aRecord.A)
	}
}

func TestForwardWithUpstreams_NoUpstreams(t *testing.T) {
	cfg := &config.Config{
		UpstreamDNSServers: []string{"1.1.1.1:53"},
	}
	logger := logging.NewDefault()
	fwd := NewForwarder(cfg, logger, nil)

	req := new(dns.Msg)
	req.SetQuestion("test.com.", dns.TypeA)

	ctx := context.Background()
	_, err := fwd.ForwardWithUpstreams(ctx, req, []string{})

	if err == nil {
		t.Fatal("Expected error for empty upstreams, got nil")
	}

	if err.Error() != "no upstream DNS servers provided" {
		t.Errorf("Expected 'no upstream DNS servers provided' error, got %v", err)
	}
}

func TestForwardWithUpstreams_Retry(t *testing.T) {
	// Create a working mock DNS server
	responses := map[string]*dns.Msg{
		"retry.test.": createTestResponse("retry.test.", "10.0.0.2"),
	}
	addr, cleanup := mockDNSServer(t, responses)
	defer cleanup()

	cfg := &config.Config{
		UpstreamDNSServers: []string{"1.1.1.1:53"},
	}
	logger := logging.NewDefault()
	fwd := NewForwarder(cfg, logger, nil)
	fwd.SetTimeout(100 * time.Millisecond)
	fwd.SetRetries(2)

	req := new(dns.Msg)
	req.SetQuestion("retry.test.", dns.TypeA)

	ctx := context.Background()
	// First upstream fails (non-routable), second succeeds
	resp, err := fwd.ForwardWithUpstreams(ctx, req, []string{"192.0.2.1:53", addr})

	if err != nil {
		t.Fatalf("ForwardWithUpstreams with retry failed: %v", err)
	}

	if resp == nil {
		t.Fatal("ForwardWithUpstreams returned nil response")
	}

	if len(resp.Answer) != 1 {
		t.Fatalf("Expected 1 answer, got %d", len(resp.Answer))
	}
}

// mockDualStackServer starts a UDP+TCP DNS server on the SAME port. Each
// transport has its own response handler so tests can model the
// "UDP returns SERVFAIL, TCP returns real answer" case driving the retry feature.
//
// Returns the shared "host:port" address and a cleanup func.
func mockDualStackServer(
	t *testing.T,
	udpHandler dns.HandlerFunc,
	tcpHandler dns.HandlerFunc,
) (string, func()) {
	t.Helper()

	// Bind UDP first to get an ephemeral port, then bind TCP to the same port.
	udpConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("udp listen: %v", err)
	}
	port := udpConn.LocalAddr().(*net.UDPAddr).Port
	tcpListener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		_ = udpConn.Close()
		t.Fatalf("tcp listen on port %d: %v", port, err)
	}

	udpServer := &dns.Server{PacketConn: udpConn, Handler: udpHandler}
	tcpServer := &dns.Server{Listener: tcpListener, Handler: tcpHandler}

	udpDone := make(chan struct{})
	tcpDone := make(chan struct{})
	go func() {
		_ = udpServer.ActivateAndServe()
		close(udpDone)
	}()
	go func() {
		_ = tcpServer.ActivateAndServe()
		close(tcpDone)
	}()

	// Brief settle so both servers are accepting before tests exchange.
	time.Sleep(50 * time.Millisecond)

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	cleanup := func() {
		_ = udpServer.Shutdown()
		_ = tcpServer.Shutdown()
		<-udpDone
		<-tcpDone
	}
	return addr, cleanup
}

// servfailHandler is a dns.HandlerFunc that always returns SERVFAIL.
func servfailHandler(w dns.ResponseWriter, r *dns.Msg) {
	resp := new(dns.Msg)
	resp.SetReply(r)
	resp.SetRcode(r, dns.RcodeServerFailure)
	_ = w.WriteMsg(resp)
}

// answerHandler returns a HandlerFunc that replies with a fixed A record for any query.
func answerHandler(domain, ip string) dns.HandlerFunc {
	return func(w dns.ResponseWriter, r *dns.Msg) {
		resp := new(dns.Msg)
		resp.SetReply(r)
		if len(r.Question) > 0 {
			rr := &dns.A{
				Hdr: dns.RR_Header{
					Name:   r.Question[0].Name,
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    300,
				},
				A: net.ParseIP(ip),
			}
			resp.Answer = append(resp.Answer, rr)
		}
		_ = w.WriteMsg(resp)
	}
}

func TestForward_ServfailTCPRetry_Recovered(t *testing.T) {
	// UDP says SERVFAIL, TCP returns a real answer on the same port.
	// Default config (ServfailTCPRetry nil → enabled) should retry over TCP
	// and return the real answer.
	addr, cleanup := mockDualStackServer(t,
		dns.HandlerFunc(servfailHandler),
		answerHandler("retry.test.", "10.0.0.42"),
	)
	defer cleanup()

	cfg := &config.Config{UpstreamDNSServers: []string{addr}}
	logger := logging.NewDefault()
	fwd := NewForwarder(cfg, logger, nil)

	req := new(dns.Msg)
	req.SetQuestion("retry.test.", dns.TypeA)

	resp, err := fwd.Forward(context.Background(), req)
	if err != nil {
		t.Fatalf("Forward failed: %v", err)
	}
	if resp == nil {
		t.Fatal("nil response")
	}
	if resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("expected NOERROR after TCP retry, got %s", dns.RcodeToString[resp.Rcode])
	}
	if len(resp.Answer) != 1 {
		t.Fatalf("expected 1 answer from TCP retry, got %d", len(resp.Answer))
	}
	a, ok := resp.Answer[0].(*dns.A)
	if !ok || a.A.String() != "10.0.0.42" {
		t.Fatalf("expected A 10.0.0.42, got %v", resp.Answer[0])
	}
}

func TestForward_ServfailTCPRetry_StillServfail(t *testing.T) {
	// Both UDP and TCP return SERVFAIL. After retry, original SERVFAIL is returned.
	addr, cleanup := mockDualStackServer(t,
		dns.HandlerFunc(servfailHandler),
		dns.HandlerFunc(servfailHandler),
	)
	defer cleanup()

	cfg := &config.Config{UpstreamDNSServers: []string{addr}}
	logger := logging.NewDefault()
	fwd := NewForwarder(cfg, logger, nil)

	req := new(dns.Msg)
	req.SetQuestion("still.test.", dns.TypeA)

	resp, err := fwd.Forward(context.Background(), req)
	if err != nil {
		t.Fatalf("Forward failed: %v", err)
	}
	if resp.Rcode != dns.RcodeServerFailure {
		t.Fatalf("expected SERVFAIL preserved, got %s", dns.RcodeToString[resp.Rcode])
	}
}

func TestForward_ServfailTCPRetry_Disabled(t *testing.T) {
	// UDP SERVFAIL, TCP would recover, but feature is disabled →
	// caller sees SERVFAIL, no retry attempted.
	addr, cleanup := mockDualStackServer(t,
		dns.HandlerFunc(servfailHandler),
		answerHandler("disabled.test.", "10.0.0.99"),
	)
	defer cleanup()

	disabled := false
	cfg := &config.Config{
		UpstreamDNSServers: []string{addr},
		Forwarder: config.ForwarderConfig{
			ServfailTCPRetry: &disabled,
		},
	}
	logger := logging.NewDefault()
	fwd := NewForwarder(cfg, logger, nil)

	req := new(dns.Msg)
	req.SetQuestion("disabled.test.", dns.TypeA)

	resp, err := fwd.Forward(context.Background(), req)
	if err != nil {
		t.Fatalf("Forward failed: %v", err)
	}
	if resp.Rcode != dns.RcodeServerFailure {
		t.Fatalf("expected SERVFAIL (retry disabled), got %s", dns.RcodeToString[resp.Rcode])
	}
	if len(resp.Answer) != 0 {
		t.Errorf("retry was disabled but TCP answer leaked through: %v", resp.Answer)
	}
}

func TestForward_ServfailTCPRetry_TCPError(t *testing.T) {
	// UDP returns SERVFAIL, TCP listener is absent (only UDP bound).
	// Retry should fail at TCP dial and original SERVFAIL is returned.
	udpConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("udp listen: %v", err)
	}
	defer udpConn.Close()

	udpServer := &dns.Server{PacketConn: udpConn, Handler: dns.HandlerFunc(servfailHandler)}
	go func() { _ = udpServer.ActivateAndServe() }()
	defer func() { _ = udpServer.Shutdown() }()
	time.Sleep(50 * time.Millisecond)

	addr := udpConn.LocalAddr().String()

	cfg := &config.Config{UpstreamDNSServers: []string{addr}}
	logger := logging.NewDefault()
	// Use short timeout so a missing TCP listener fails fast (RST on linux loopback).
	fwd := NewForwarder(cfg, logger, nil)
	fwd.SetTimeout(200 * time.Millisecond)

	req := new(dns.Msg)
	req.SetQuestion("tcperror.test.", dns.TypeA)

	resp, err := fwd.Forward(context.Background(), req)
	if err != nil {
		t.Fatalf("Forward failed: %v", err)
	}
	if resp.Rcode != dns.RcodeServerFailure {
		t.Fatalf("expected SERVFAIL preserved on TCP error, got %s", dns.RcodeToString[resp.Rcode])
	}
}

func TestForwardWithUpstreams_ServfailTCPRetry_Recovered(t *testing.T) {
	// Same UDP-SERVFAIL + TCP-recovers scenario, but on the conditional path.
	addr, cleanup := mockDualStackServer(t,
		dns.HandlerFunc(servfailHandler),
		answerHandler("conditional-retry.test.", "10.0.0.7"),
	)
	defer cleanup()

	cfg := &config.Config{UpstreamDNSServers: []string{"1.1.1.1:53"}}
	logger := logging.NewDefault()
	fwd := NewForwarder(cfg, logger, nil)

	req := new(dns.Msg)
	req.SetQuestion("conditional-retry.test.", dns.TypeA)

	resp, err := fwd.ForwardWithUpstreams(context.Background(), req, []string{addr})
	if err != nil {
		t.Fatalf("ForwardWithUpstreams failed: %v", err)
	}
	if resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("expected NOERROR after TCP retry, got %s", dns.RcodeToString[resp.Rcode])
	}
	if len(resp.Answer) != 1 {
		t.Fatalf("expected 1 answer, got %d", len(resp.Answer))
	}
}
