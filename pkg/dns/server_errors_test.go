package dns

import (
	"context"
	"errors"
	"net"
	"testing"

	"glory-hole/pkg/config"
	"glory-hole/pkg/logging"
	"glory-hole/pkg/telemetry"

	"github.com/miekg/dns"
)

// TestWriteMsg_ErrorHandling tests error handling in writeMsg
func TestWriteMsg_ErrorHandling(t *testing.T) {
	handler := NewHandler()

	// Create a mock writer that always fails
	mockWriter := &errorResponseWriter{
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}

	msg := new(dns.Msg)
	msg.SetQuestion("example.com.", dns.TypeA)

	// Should not panic when WriteMsg fails
	handler.writeMsg(mockWriter, msg)

	// Verify the error was ignored gracefully
	if !mockWriter.writeAttempted {
		t.Error("Expected WriteMsg to be called")
	}
}

// TestWriteMsg_Success tests successful message writing
func TestWriteMsg_Success(t *testing.T) {
	handler := NewHandler()

	mockWriter := &mockResponseWriter{
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}

	msg := new(dns.Msg)
	msg.SetQuestion("example.com.", dns.TypeA)
	msg.SetReply(msg)

	// Should successfully write message
	handler.writeMsg(mockWriter, msg)

	// Verify message was written
	if mockWriter.msg == nil {
		t.Error("Expected message to be written")
	}
}

// TestGetClientIP_EdgeCases tests getClientIP with various edge cases
func TestGetClientIP_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		writer   dns.ResponseWriter
		expected string
	}{
		{
			name:     "Normal UDP address",
			writer:   &mockResponseWriter{remoteAddr: &net.UDPAddr{IP: net.ParseIP("192.168.1.100"), Port: 53}},
			expected: "192.168.1.100",
		},
		{
			name:     "Normal TCP address",
			writer:   &mockResponseWriter{remoteAddr: &net.TCPAddr{IP: net.ParseIP("10.0.0.5"), Port: 5353}},
			expected: "10.0.0.5",
		},
		{
			name:     "IPv6 address",
			writer:   &mockResponseWriter{remoteAddr: &net.TCPAddr{IP: net.ParseIP("fe80::1"), Port: 53}},
			expected: "fe80::1",
		},
		{
			name:     "Address without port (fallback)",
			writer:   &mockResponseWriter{remoteAddr: &badAddr{addr: "192.168.1.1"}},
			expected: "192.168.1.1",
		},
		{
			name:     "Nil RemoteAddr",
			writer:   &mockResponseWriter{remoteAddr: nil},
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getClientIP(tt.writer)
			if result != tt.expected {
				t.Errorf("getClientIP() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestNewServer_CacheInitFailure tests NewServer when cache initialization fails
func TestNewServer_CacheInitFailure(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			ListenAddress: ":5353",
		},
		Cache: config.CacheConfig{
			Enabled:    true,
			MaxEntries: -1, // Invalid value that might cause cache.New to fail
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

	// Should not panic even if cache init fails
	server := NewServer(cfg, handler, logger, metrics)

	if server == nil {
		t.Error("Expected NewServer to return non-nil server even on cache error")
	}

	// Server should still work without cache
	if server.handler == nil {
		t.Error("Expected handler to be set")
	}
}

// TestNewServer_NoUpstreamServers tests NewServer without upstream servers
func TestNewServer_NoUpstreamServers(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			ListenAddress: ":5353",
		},
		Cache: config.CacheConfig{
			Enabled: false,
		},
		UpstreamDNSServers: []string{}, // No upstream servers
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
		t.Error("Expected NewServer to return non-nil server")
	}

	// Forwarder should not be set
	if handler.Forwarder != nil {
		t.Error("Expected forwarder to be nil when no upstream servers configured")
	}
}

// TestServer_StartAlreadyRunning tests starting an already running server
func TestServer_StartAlreadyRunning(t *testing.T) {
	// This test verifies the check in Start() that prevents double-starting
	// We don't actually start the server, just manipulate the internal state

	cfg := &config.Config{
		Server: config.ServerConfig{
			ListenAddress: "127.0.0.1:15357",
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
	server := NewServer(cfg, handler, logger, metrics)

	// Manually set running flag to simulate already running server
	server.mu.Lock()
	server.running = true
	server.mu.Unlock()

	// Try to start - should return error
	err = server.Start(ctx)
	if err == nil {
		t.Error("Expected error when starting already running server")
	}
	if err != nil && err.Error() != "server already running" {
		t.Errorf("Expected 'server already running' error, got: %v", err)
	}

	// Reset state
	server.mu.Lock()
	server.running = false
	server.mu.Unlock()
}

// TestServeDNS_NilStorage tests ServeDNS with nil storage (no panic)
func TestServeDNS_NilStorage(t *testing.T) {
	handler := NewHandler()
	// Don't set storage - it should be nil

	msg := new(dns.Msg)
	msg.SetQuestion("example.com.", dns.TypeA)

	writer := &mockResponseWriter{remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345}}
	ctx := context.Background()

	// Should not panic with nil storage
	handler.ServeDNS(ctx, writer, msg)

	// Response should be SERVFAIL since there's no forwarder either
	if writer.msg == nil {
		t.Error("Expected response to be written")
	}
}

// TestServeDNS_MalformedQuery tests ServeDNS with malformed queries
func TestServeDNS_MalformedQuery(t *testing.T) {
	handler := NewHandler()

	// Create a message with invalid question
	msg := new(dns.Msg)
	msg.Question = []dns.Question{
		{
			Name:   "invalid..domain..", // Invalid domain with consecutive dots
			Qtype:  dns.TypeA,
			Qclass: dns.ClassINET,
		},
	}

	writer := &mockResponseWriter{remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345}}
	ctx := context.Background()

	// Should not panic with malformed query
	handler.ServeDNS(ctx, writer, msg)

	// Should return some response (likely SERVFAIL)
	if writer.msg == nil {
		t.Error("Expected response to be written even for malformed query")
	}
}

// Mock types for testing

// errorResponseWriter is a mock that simulates WriteMsg failures
type errorResponseWriter struct {
	remoteAddr      net.Addr
	writeAttempted  bool
}

func (w *errorResponseWriter) WriteMsg(msg *dns.Msg) error {
	w.writeAttempted = true
	return errors.New("simulated write error")
}

func (w *errorResponseWriter) Write(b []byte) (int, error) {
	return 0, errors.New("simulated write error")
}

func (w *errorResponseWriter) Close() error {
	return nil
}

func (w *errorResponseWriter) TsigStatus() error {
	return nil
}

func (w *errorResponseWriter) TsigTimersOnly(b bool) {}

func (w *errorResponseWriter) Hijack() {}

func (w *errorResponseWriter) LocalAddr() net.Addr {
	return &net.UDPAddr{IP: net.ParseIP("0.0.0.0"), Port: 53}
}

func (w *errorResponseWriter) RemoteAddr() net.Addr {
	return w.remoteAddr
}

// badAddr is a net.Addr that doesn't have a port (to test SplitHostPort failure)
type badAddr struct {
	addr string
}

func (a *badAddr) Network() string {
	return "bad"
}

func (a *badAddr) String() string {
	return a.addr
}
