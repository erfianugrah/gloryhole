package dns

import (
	"context"
	"net"
	"testing"

	"github.com/miekg/dns"
)

// mockResponseWriter implements dns.ResponseWriter for testing
type mockResponseWriter struct {
	msg        *dns.Msg
	remoteAddr net.Addr
}

func (m *mockResponseWriter) LocalAddr() net.Addr  { return nil }
func (m *mockResponseWriter) RemoteAddr() net.Addr { return m.remoteAddr }
func (m *mockResponseWriter) WriteMsg(msg *dns.Msg) error {
	m.msg = msg
	return nil
}
func (m *mockResponseWriter) Write([]byte) (int, error) { return 0, nil }
func (m *mockResponseWriter) Close() error              { return nil }
func (m *mockResponseWriter) TsigStatus() error         { return nil }
func (m *mockResponseWriter) TsigTimersOnly(bool)       {}
func (m *mockResponseWriter) Hijack()                   {}

func TestServeDNS_EmptyQuestion(t *testing.T) {
	handler := NewHandler()
	w := &mockResponseWriter{
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}

	// Request with no questions
	r := new(dns.Msg)
	r.SetQuestion("", dns.TypeA)
	r.Question = nil // Clear questions

	handler.ServeDNS(context.Background(), w, r)

	if w.msg == nil {
		t.Fatal("Expected response message")
	}
	if w.msg.Rcode != dns.RcodeFormatError {
		t.Errorf("Expected RcodeFormatError, got %d", w.msg.Rcode)
	}
}

func TestServeDNS_BlockedDomain(t *testing.T) {
	handler := NewHandler()
	handler.Blocklist["ads.example.com."] = struct{}{}

	w := &mockResponseWriter{
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}

	r := new(dns.Msg)
	r.SetQuestion("ads.example.com.", dns.TypeA)

	handler.ServeDNS(context.Background(), w, r)

	if w.msg == nil {
		t.Fatal("Expected response message")
	}
	if w.msg.Rcode != dns.RcodeNameError {
		t.Errorf("Expected RcodeNameError for blocked domain, got %d", w.msg.Rcode)
	}
}

// TestServeDNS_LocalOverride_A / _AAAA / TestServeDNS_CNAMEOverride were
// removed in v0.26: they targeted Handler.Overrides / Handler.CNAMEOverrides
// which were never populated outside test code (see v0.26 plan §6b). Same
// functionality is now covered by Policy REDIRECT (single-IP) and
// LocalRecords (CNAME chains, TXT, MX, ...).

func TestServeDNS_NoMatch(t *testing.T) {
	handler := NewHandler()

	w := &mockResponseWriter{
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}

	r := new(dns.Msg)
	r.SetQuestion("unknown.example.com.", dns.TypeA)

	handler.ServeDNS(context.Background(), w, r)

	if w.msg == nil {
		t.Fatal("Expected response message")
	}
	// Should return NXDOMAIN (no upstream forwarding yet)
	if w.msg.Rcode != dns.RcodeNameError {
		t.Errorf("Expected RcodeNameError, got %d", w.msg.Rcode)
	}
}

func TestServeDNS_ConcurrentAccess(t *testing.T) {
	handler := NewHandler()
	handler.Blocklist["blocked.local."] = struct{}{}

	// Test concurrent access to handler
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			w := &mockResponseWriter{
				remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
			}
			r := new(dns.Msg)
			r.SetQuestion("blocked.local.", dns.TypeA)
			handler.ServeDNS(context.Background(), w, r)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestNewHandler(t *testing.T) {
	handler := NewHandler()
	if handler == nil {
		t.Fatal("NewHandler returned nil")
	}
	if handler.Blocklist == nil {
		t.Error("Blocklist not initialized")
	}
}
