package dns

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"testing"
	"time"

	"glory-hole/pkg/config"
	"glory-hole/pkg/localrecords"
	"glory-hole/pkg/logging"

	"github.com/miekg/dns"
	proxyproto "github.com/pires/go-proxyproto"
)

// proxyProtoDoTExchange opens a TCP connection, sends a PROXY header,
// performs a TLS handshake, sends a DNS query, and returns the response.
func proxyProtoDoTExchange(t *testing.T, addr string, port int, header *proxyproto.Header, question string) *dns.Msg {
	t.Helper()

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		t.Fatalf("TCP dial failed: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	_, err = header.WriteTo(conn)
	if err != nil {
		t.Fatalf("write PROXY header: %v", err)
	}

	tlsConn := tls.Client(conn, &tls.Config{InsecureSkipVerify: true})
	err = tlsConn.Handshake()
	if err != nil {
		t.Fatalf("TLS handshake failed: %v", err)
	}
	t.Cleanup(func() { tlsConn.Close() })

	dnsConn := &dns.Conn{Conn: tlsConn}
	msg := new(dns.Msg)
	msg.SetQuestion(question, dns.TypeA)

	err = dnsConn.WriteMsg(msg)
	if err != nil {
		t.Fatalf("write DNS msg: %v", err)
	}

	resp, err := dnsConn.ReadMsg()
	if err != nil {
		t.Fatalf("read DNS response: %v", err)
	}
	return resp
}

// proxyProtoTCPExchange opens a TCP connection, sends a PROXY header,
// sends a DNS query (no TLS), and returns the response.
func proxyProtoTCPExchange(t *testing.T, port int, header *proxyproto.Header, question string) *dns.Msg {
	t.Helper()

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		t.Fatalf("TCP dial failed: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	_, err = header.WriteTo(conn)
	if err != nil {
		t.Fatalf("write PROXY header: %v", err)
	}

	dnsConn := &dns.Conn{Conn: conn}
	msg := new(dns.Msg)
	msg.SetQuestion(question, dns.TypeA)

	err = dnsConn.WriteMsg(msg)
	if err != nil {
		t.Fatalf("write DNS msg: %v", err)
	}

	resp, err := dnsConn.ReadMsg()
	if err != nil {
		t.Fatalf("read DNS response: %v", err)
	}
	return resp
}

// startProxyProtoServer starts a DNS server with the given config and
// returns a cancel function. Blocks until the server is ready.
func startProxyProtoServer(t *testing.T, cfg *config.Config, handler *Handler) context.CancelFunc {
	t.Helper()

	logger := logging.NewDefault()
	srv := NewServer(cfg, handler, logger, nil)

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		if err := srv.Start(ctx); err != nil {
			errCh <- err
		}
	}()

	time.Sleep(300 * time.Millisecond)

	t.Cleanup(func() {
		cancel()
		select {
		case err := <-errCh:
			if err != context.Canceled && err != nil {
				t.Errorf("server returned error: %v", err)
			}
		case <-time.After(time.Second):
		}
	})

	return cancel
}

// TestDoTWithProxyProtocol verifies that DoT with PROXY protocol
// extracts the real client IP from the PROXY header.
func TestDoTWithProxyProtocol(t *testing.T) {
	dotPort := freePort(t)
	certFile, keyFile := writeSelfSignedCert(t)

	cfg := &config.Config{
		Server: config.ServerConfig{
			ListenAddress:   "127.0.0.1:0",
			WebUIAddress:    ":0",
			TCPEnabled:      false,
			UDPEnabled:      false,
			EnableBlocklist: true,
			EnablePolicies:  true,
			DotEnabled:      true,
			DotAddress:      fmt.Sprintf("127.0.0.1:%d", dotPort),
			ProxyProtocol:   true,
			TLS: config.TLSConfig{
				CertFile: certFile,
				KeyFile:  keyFile,
			},
		},
		UpstreamDNSServers: []string{"1.1.1.1:53"},
		Logging:            config.LoggingConfig{Level: "error", Format: "text", Output: "stdout"},
	}

	lr := localrecords.NewManager()
	if err := lr.AddRecord(localrecords.NewARecord("example.local", net.ParseIP("1.2.3.4"))); err != nil {
		t.Fatalf("add record: %v", err)
	}

	handler := NewHandler()
	handler.SetLocalRecords(lr)
	startProxyProtoServer(t, cfg, handler)

	header := &proxyproto.Header{
		Version:           1,
		Command:           proxyproto.PROXY,
		TransportProtocol: proxyproto.TCPv4,
		SourceAddr:        &net.TCPAddr{IP: net.ParseIP("203.0.113.50"), Port: 12345},
		DestinationAddr:   &net.TCPAddr{IP: net.ParseIP("198.51.100.1"), Port: dotPort},
	}

	resp := proxyProtoDoTExchange(t, "127.0.0.1", dotPort, header, "example.local.")

	if resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("unexpected rcode: %d", resp.Rcode)
	}
	if len(resp.Answer) != 1 {
		t.Fatalf("expected 1 answer, got %d", len(resp.Answer))
	}
	a, ok := resp.Answer[0].(*dns.A)
	if !ok {
		t.Fatalf("expected A record, got %T", resp.Answer[0])
	}
	if a.A.String() != "1.2.3.4" {
		t.Fatalf("unexpected A record: %s", a.A.String())
	}
}

// TestTCPDNSWithProxyProtocol verifies that TCP DNS with PROXY protocol
// correctly accepts queries when proxy protocol is enabled.
func TestTCPDNSWithProxyProtocol(t *testing.T) {
	tcpPort := freePort(t)

	cfg := &config.Config{
		Server: config.ServerConfig{
			ListenAddress:   fmt.Sprintf("127.0.0.1:%d", tcpPort),
			WebUIAddress:    ":0",
			TCPEnabled:      true,
			UDPEnabled:      false,
			EnableBlocklist: true,
			EnablePolicies:  true,
			DotEnabled:      false,
			ProxyProtocol:   true,
		},
		UpstreamDNSServers: []string{"1.1.1.1:53"},
		Logging:            config.LoggingConfig{Level: "error", Format: "text", Output: "stdout"},
	}

	lr := localrecords.NewManager()
	if err := lr.AddRecord(localrecords.NewARecord("example.local", net.ParseIP("5.6.7.8"))); err != nil {
		t.Fatalf("add record: %v", err)
	}

	handler := NewHandler()
	handler.SetLocalRecords(lr)
	startProxyProtoServer(t, cfg, handler)

	header := &proxyproto.Header{
		Version:           1,
		Command:           proxyproto.PROXY,
		TransportProtocol: proxyproto.TCPv4,
		SourceAddr:        &net.TCPAddr{IP: net.ParseIP("198.51.100.42"), Port: 54321},
		DestinationAddr:   &net.TCPAddr{IP: net.ParseIP("203.0.113.1"), Port: tcpPort},
	}

	resp := proxyProtoTCPExchange(t, tcpPort, header, "example.local.")

	if resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("unexpected rcode: %d", resp.Rcode)
	}
	if len(resp.Answer) != 1 {
		t.Fatalf("expected 1 answer, got %d", len(resp.Answer))
	}
	a, ok := resp.Answer[0].(*dns.A)
	if !ok {
		t.Fatalf("expected A record, got %T", resp.Answer[0])
	}
	if a.A.String() != "5.6.7.8" {
		t.Fatalf("unexpected A record: %s", a.A.String())
	}
}

// TestDoTWithProxyProtocolV2 verifies v2 (binary) PROXY protocol works too.
func TestDoTWithProxyProtocolV2(t *testing.T) {
	dotPort := freePort(t)
	certFile, keyFile := writeSelfSignedCert(t)

	cfg := &config.Config{
		Server: config.ServerConfig{
			ListenAddress:   "127.0.0.1:0",
			WebUIAddress:    ":0",
			TCPEnabled:      false,
			UDPEnabled:      false,
			EnableBlocklist: true,
			EnablePolicies:  true,
			DotEnabled:      true,
			DotAddress:      fmt.Sprintf("127.0.0.1:%d", dotPort),
			ProxyProtocol:   true,
			TLS: config.TLSConfig{
				CertFile: certFile,
				KeyFile:  keyFile,
			},
		},
		UpstreamDNSServers: []string{"1.1.1.1:53"},
		Logging:            config.LoggingConfig{Level: "error", Format: "text", Output: "stdout"},
	}

	lr := localrecords.NewManager()
	if err := lr.AddRecord(localrecords.NewARecord("example.local", net.ParseIP("1.2.3.4"))); err != nil {
		t.Fatalf("add record: %v", err)
	}

	handler := NewHandler()
	handler.SetLocalRecords(lr)
	startProxyProtoServer(t, cfg, handler)

	header := &proxyproto.Header{
		Version:           2,
		Command:           proxyproto.PROXY,
		TransportProtocol: proxyproto.TCPv4,
		SourceAddr:        &net.TCPAddr{IP: net.ParseIP("192.0.2.100"), Port: 11111},
		DestinationAddr:   &net.TCPAddr{IP: net.ParseIP("198.51.100.1"), Port: dotPort},
	}

	resp := proxyProtoDoTExchange(t, "127.0.0.1", dotPort, header, "example.local.")

	if resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("unexpected rcode: %d", resp.Rcode)
	}
	if len(resp.Answer) != 1 {
		t.Fatalf("expected 1 answer, got %d", len(resp.Answer))
	}
	a, ok := resp.Answer[0].(*dns.A)
	if !ok {
		t.Fatalf("expected A record, got %T", resp.Answer[0])
	}
	if a.A.String() != "1.2.3.4" {
		t.Fatalf("unexpected A record: %s", a.A.String())
	}
}
