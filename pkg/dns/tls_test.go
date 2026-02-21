package dns

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCertCoversHosts(t *testing.T) {
	leaf := &x509.Certificate{
		DNSNames: []string{"dot.example.com", "dot2.example.com"},
	}

	tests := []struct {
		name  string
		hosts []string
		want  bool
	}{
		{"exact match single", []string{"dot.example.com"}, true},
		{"exact match both", []string{"dot.example.com", "dot2.example.com"}, true},
		{"case insensitive", []string{"DOT.Example.COM"}, true},
		{"missing host", []string{"other.example.com"}, false},
		{"partial overlap", []string{"dot.example.com", "other.example.com"}, false},
		{"empty hosts", []string{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := certCoversHosts(leaf, tt.hosts); got != tt.want {
				t.Errorf("certCoversHosts() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoadCachedRejectsMismatchedSANs(t *testing.T) {
	dir := t.TempDir()

	certPEM, keyPEM := generateSelfSignedPEMWithExpiry(t, "old.example.com", -time.Hour, 24*time.Hour)
	if err := os.WriteFile(filepath.Join(dir, "cert.pem"), certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "key.pem"), keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}

	mgr := &acmeManager{
		cacheDir: dir,
		hosts:    []string{"new.example.com"},
	}

	_, err := mgr.loadCached()
	if err == nil {
		t.Fatal("expected loadCached to reject cert with mismatched SANs")
	}
	if !strings.Contains(err.Error(), "do not match configured hosts") {
		t.Errorf("unexpected error: %s", err)
	}
}

func TestLoadCachedAcceptsMatchingSANs(t *testing.T) {
	dir := t.TempDir()

	certPEM, keyPEM := generateSelfSignedPEMWithExpiry(t, "dot.example.com", -time.Hour, 24*time.Hour)
	if err := os.WriteFile(filepath.Join(dir, "cert.pem"), certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "key.pem"), keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}

	mgr := &acmeManager{
		cacheDir: dir,
		hosts:    []string{"dot.example.com"},
	}

	cert, err := mgr.loadCached()
	if err != nil {
		t.Fatalf("expected loadCached to accept matching cert, got: %v", err)
	}
	if cert.Leaf == nil {
		t.Fatal("expected leaf to be parsed")
	}
}

func TestLoadCachedRejectsExpiredCert(t *testing.T) {
	dir := t.TempDir()

	// Certificate that expired 1 hour ago
	certPEM, keyPEM := generateSelfSignedPEMWithExpiry(t, "dot.example.com", -48*time.Hour, -time.Hour)
	if err := os.WriteFile(filepath.Join(dir, "cert.pem"), certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "key.pem"), keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}

	mgr := &acmeManager{
		cacheDir: dir,
		hosts:    []string{"dot.example.com"},
	}

	_, err := mgr.loadCached()
	if err == nil {
		t.Fatal("expected loadCached to reject expired cert")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Errorf("unexpected error: %s", err)
	}
}

// generateSelfSignedPEM creates a self-signed cert+key PEM for the given DNS name.
func generateSelfSignedPEM(t *testing.T, dnsName string) (certPEM, keyPEM []byte) {
	t.Helper()
	return generateSelfSignedPEMWithExpiry(t, dnsName, -time.Hour, 24*time.Hour)
}

// generateSelfSignedPEMWithExpiry creates a self-signed cert+key PEM with explicit
// NotBefore (now + notBeforeOffset) and NotAfter (now + notAfterOffset).
func generateSelfSignedPEMWithExpiry(t *testing.T, dnsName string, notBeforeOffset, notAfterOffset time.Duration) (certPEM, keyPEM []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: dnsName},
		DNSNames:     []string{dnsName},
		NotBefore:    time.Now().Add(notBeforeOffset),
		NotAfter:     time.Now().Add(notAfterOffset),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM
}
