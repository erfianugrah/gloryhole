package dns

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"glory-hole/pkg/config"
	"glory-hole/pkg/localrecords"
	"glory-hole/pkg/logging"

	"github.com/miekg/dns"
)

func TestDoTServerServesLocalRecord(t *testing.T) {
	dotPort := freePort(t)

	certFile, keyFile := writeSelfSignedCert(t)

	cfg := &config.Config{
		Server: config.ServerConfig{
			ListenAddress:   "127.0.0.1:0", // unused (UDP/TCP disabled)
			WebUIAddress:    ":0",
			TCPEnabled:      false,
			UDPEnabled:      false,
			EnableBlocklist: true,
			EnablePolicies:  true,
			DecisionTrace:   false,
			DotEnabled:      true,
			DotAddress:      fmt.Sprintf("127.0.0.1:%d", dotPort),
			TLS: config.TLSConfig{
				CertFile: certFile,
				KeyFile:  keyFile,
			},
		},
		UpstreamDNSServers: []string{"1.1.1.1:53"},
		Logging:            config.LoggingConfig{Level: "error", Format: "text", Output: "stdout"},
	}

	// Local record
	lr := localrecords.NewManager()
	rec := localrecords.NewARecord("example.local", net.ParseIP("1.2.3.4"))
	if err := lr.AddRecord(rec); err != nil {
		t.Fatalf("add record: %v", err)
	}

	handler := NewHandler()
	handler.SetLocalRecords(lr)

	logger := logging.NewDefault()

	srv := NewServer(cfg, handler, logger, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		if err := srv.Start(ctx); err != nil {
			errCh <- err
		}
	}()

	// Give the server a moment to start
	time.Sleep(300 * time.Millisecond)

	client := &dns.Client{
		Net:       "tcp-tls",
		TLSConfig: &tls.Config{InsecureSkipVerify: true},
		Timeout:   2 * time.Second,
	}

	msg := new(dns.Msg)
	msg.SetQuestion("example.local.", dns.TypeA)

	resp, _, err := client.Exchange(msg, fmt.Sprintf("127.0.0.1:%d", dotPort))
	if err != nil {
		cancel()
		t.Fatalf("DoT query failed: %v", err)
	}

	if resp.Rcode != dns.RcodeSuccess {
		cancel()
		t.Fatalf("unexpected rcode: %d", resp.Rcode)
	}

	if len(resp.Answer) != 1 {
		cancel()
		t.Fatalf("expected 1 answer, got %d", len(resp.Answer))
	}

	a, ok := resp.Answer[0].(*dns.A)
	if !ok {
		cancel()
		t.Fatalf("expected A record, got %T", resp.Answer[0])
	}
	if a.A.String() != "1.2.3.4" {
		cancel()
		t.Fatalf("unexpected A record: %s", a.A.String())
	}

	cancel()
	select {
	case err := <-errCh:
		if err != context.Canceled && err != nil {
			t.Fatalf("server returned error: %v", err)
		}
	case <-time.After(time.Second):
		// best effort
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to allocate port: %v", err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

func writeSelfSignedCert(t *testing.T) (string, string) {
	t.Helper()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "example.local"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		DNSNames:     []string{"example.local"},
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}

	certOut := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyBytes, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	keyOut := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})

	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certFile, certOut, 0600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyFile, keyOut, 0600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	return certFile, keyFile
}
