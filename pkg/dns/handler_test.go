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

func TestServeDNS_WhitelistedDomain(t *testing.T) {
	handler := NewHandler()
	handler.Blocklist["example.com."] = struct{}{}
	whitelist := map[string]struct{}{"example.com.": {}}
	handler.Whitelist.Store(&whitelist)

	w := &mockResponseWriter{
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}

	r := new(dns.Msg)
	r.SetQuestion("example.com.", dns.TypeA)

	handler.ServeDNS(context.Background(), w, r)

	if w.msg == nil {
		t.Fatal("Expected response message")
	}
	// Should not be blocked because it's whitelisted
	// Will return NXDOMAIN because no override is set (no upstream yet)
	if w.msg.Rcode != dns.RcodeNameError {
		t.Errorf("Expected RcodeNameError (no upstream), got %d", w.msg.Rcode)
	}
}

func TestServeDNS_LocalOverride_A(t *testing.T) {
	handler := NewHandler()
	handler.Overrides["nas.local."] = net.ParseIP("192.168.1.100")

	w := &mockResponseWriter{
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}

	r := new(dns.Msg)
	r.SetQuestion("nas.local.", dns.TypeA)

	handler.ServeDNS(context.Background(), w, r)

	if w.msg == nil {
		t.Fatal("Expected response message")
	}
	if w.msg.Rcode != dns.RcodeSuccess {
		t.Errorf("Expected RcodeSuccess, got %d", w.msg.Rcode)
	}
	if len(w.msg.Answer) != 1 {
		t.Fatalf("Expected 1 answer, got %d", len(w.msg.Answer))
	}

	aRecord, ok := w.msg.Answer[0].(*dns.A)
	if !ok {
		t.Fatalf("Expected A record, got %T", w.msg.Answer[0])
	}
	if !aRecord.A.Equal(net.ParseIP("192.168.1.100")) {
		t.Errorf("Expected IP 192.168.1.100, got %s", aRecord.A)
	}
}

func TestServeDNS_LocalOverride_AAAA(t *testing.T) {
	handler := NewHandler()
	handler.Overrides["nas.local."] = net.ParseIP("fe80::1")

	w := &mockResponseWriter{
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}

	r := new(dns.Msg)
	r.SetQuestion("nas.local.", dns.TypeAAAA)

	handler.ServeDNS(context.Background(), w, r)

	if w.msg == nil {
		t.Fatal("Expected response message")
	}
	if w.msg.Rcode != dns.RcodeSuccess {
		t.Errorf("Expected RcodeSuccess, got %d", w.msg.Rcode)
	}
	if len(w.msg.Answer) != 1 {
		t.Fatalf("Expected 1 answer, got %d", len(w.msg.Answer))
	}

	aaaaRecord, ok := w.msg.Answer[0].(*dns.AAAA)
	if !ok {
		t.Fatalf("Expected AAAA record, got %T", w.msg.Answer[0])
	}
	if !aaaaRecord.AAAA.Equal(net.ParseIP("fe80::1")) {
		t.Errorf("Expected IP fe80::1, got %s", aaaaRecord.AAAA)
	}
}

func TestServeDNS_CNAMEOverride(t *testing.T) {
	handler := NewHandler()
	handler.CNAMEOverrides["storage.local."] = "nas.local."

	w := &mockResponseWriter{
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}

	r := new(dns.Msg)
	r.SetQuestion("storage.local.", dns.TypeA)

	handler.ServeDNS(context.Background(), w, r)

	if w.msg == nil {
		t.Fatal("Expected response message")
	}
	if w.msg.Rcode != dns.RcodeSuccess {
		t.Errorf("Expected RcodeSuccess, got %d", w.msg.Rcode)
	}
	if len(w.msg.Answer) != 1 {
		t.Fatalf("Expected 1 answer, got %d", len(w.msg.Answer))
	}

	cnameRecord, ok := w.msg.Answer[0].(*dns.CNAME)
	if !ok {
		t.Fatalf("Expected CNAME record, got %T", w.msg.Answer[0])
	}
	if cnameRecord.Target != "nas.local." {
		t.Errorf("Expected target nas.local., got %s", cnameRecord.Target)
	}
}

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
	handler.Overrides["nas.local."] = net.ParseIP("192.168.1.100")

	// Test concurrent access to handler
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			w := &mockResponseWriter{
				remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
			}
			r := new(dns.Msg)
			r.SetQuestion("nas.local.", dns.TypeA)
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
	if handler.Whitelist.Load() == nil {
		t.Error("Whitelist not initialized")
	}
	if handler.Overrides == nil {
		t.Error("Overrides not initialized")
	}
	if handler.CNAMEOverrides == nil {
		t.Error("CNAMEOverrides not initialized")
	}
}
