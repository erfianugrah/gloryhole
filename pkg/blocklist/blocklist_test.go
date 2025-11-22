package blocklist

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"glory-hole/pkg/logging"
)

func TestNewDownloader(t *testing.T) {
	logger := logging.NewDefault()
	d := NewDownloader(logger)

	if d == nil {
		t.Fatal("Expected downloader, got nil")
	}

	if d.logger == nil {
		t.Error("Expected logger to be set")
	}

	if d.client == nil {
		t.Error("Expected HTTP client to be set")
	}

	if d.timeout != 60*time.Second {
		t.Errorf("Expected timeout 60s, got %v", d.timeout)
	}
}

func TestDownload_HostsFormat(t *testing.T) {
	// Create test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hosts := `# Comment line
0.0.0.0 ads.example.com
0.0.0.0 tracker.example.com
127.0.0.1 localhost
0.0.0.0 malware.example.com
`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(hosts))
	}))
	defer server.Close()

	logger := logging.NewDefault()
	d := NewDownloader(logger)

	ctx := context.Background()
	domains, err := d.Download(ctx, server.URL)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	expected := 3 // ads, tracker, malware (localhost should be skipped)
	if len(domains) != expected {
		t.Errorf("Expected %d domains, got %d", expected, len(domains))
	}

	// Check specific domains (with FQDN format)
	if _, ok := domains["ads.example.com."]; !ok {
		t.Error("Expected ads.example.com. to be in blocklist")
	}

	if _, ok := domains["tracker.example.com."]; !ok {
		t.Error("Expected tracker.example.com. to be in blocklist")
	}

	// localhost should not be in blocklist
	if _, ok := domains["localhost."]; ok {
		t.Error("localhost should not be in blocklist")
	}
}

func TestDownload_AdblockFormat(t *testing.T) {
	// Create test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		adblock := `! Comment line
||ads.example.com^
||tracking.example.com^
||malicious.example.com^
`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(adblock))
	}))
	defer server.Close()

	logger := logging.NewDefault()
	d := NewDownloader(logger)

	ctx := context.Background()
	domains, err := d.Download(ctx, server.URL)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(domains) != 3 {
		t.Errorf("Expected 3 domains, got %d", len(domains))
	}

	// Check specific domains
	if _, ok := domains["ads.example.com."]; !ok {
		t.Error("Expected ads.example.com. to be in blocklist")
	}

	if _, ok := domains["tracking.example.com."]; !ok {
		t.Error("Expected tracking.example.com. to be in blocklist")
	}
}

func TestDownload_PlainFormat(t *testing.T) {
	// Create test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		plain := `# Comment
ads.example.com
tracker.example.com

malware.example.com
`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(plain))
	}))
	defer server.Close()

	logger := logging.NewDefault()
	d := NewDownloader(logger)

	ctx := context.Background()
	domains, err := d.Download(ctx, server.URL)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(domains) != 3 {
		t.Errorf("Expected 3 domains, got %d", len(domains))
	}
}

func TestDownload_MixedFormats(t *testing.T) {
	// Create test HTTP server with mixed formats
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mixed := `# Mixed format blocklist
0.0.0.0 ads1.example.com
||ads2.example.com^
ads3.example.com
127.0.0.1 ads4.example.com
`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(mixed))
	}))
	defer server.Close()

	logger := logging.NewDefault()
	d := NewDownloader(logger)

	ctx := context.Background()
	domains, err := d.Download(ctx, server.URL)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(domains) != 4 {
		t.Errorf("Expected 4 domains, got %d", len(domains))
	}

	// Verify all formats were parsed
	for i := 1; i <= 4; i++ {
		domain := "ads" + string(rune('0'+i)) + ".example.com."
		if _, ok := domains[domain]; !ok {
			t.Errorf("Expected %s to be in blocklist", domain)
		}
	}
}

func TestDownload_HTTPError(t *testing.T) {
	// Create test HTTP server that returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	logger := logging.NewDefault()
	d := NewDownloader(logger)

	ctx := context.Background()
	_, err := d.Download(ctx, server.URL)

	if err == nil {
		t.Fatal("Expected error for HTTP 404, got nil")
	}

	if !strings.Contains(err.Error(), "unexpected status code") {
		t.Errorf("Expected 'unexpected status code' error, got: %v", err)
	}
}

func TestDownload_InvalidURL(t *testing.T) {
	logger := logging.NewDefault()
	d := NewDownloader(logger)

	ctx := context.Background()
	_, err := d.Download(ctx, "not-a-valid-url://example.com")

	if err == nil {
		t.Fatal("Expected error for invalid URL, got nil")
	}
}

func TestDownload_ContextCancellation(t *testing.T) {
	// Create server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	logger := logging.NewDefault()
	d := NewDownloader(logger)

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := d.Download(ctx, server.URL)

	if err == nil {
		t.Fatal("Expected timeout error, got nil")
	}
}

func TestDownloadAll_MultipleSources(t *testing.T) {
	// Create first blocklist server
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("0.0.0.0 ads1.example.com\n0.0.0.0 ads2.example.com\n"))
	}))
	defer server1.Close()

	// Create second blocklist server
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("||ads3.example.com^\n||ads4.example.com^\n"))
	}))
	defer server2.Close()

	logger := logging.NewDefault()
	d := NewDownloader(logger)

	ctx := context.Background()
	urls := []string{server1.URL, server2.URL}
	domains, err := d.DownloadAll(ctx, urls)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(domains) != 4 {
		t.Errorf("Expected 4 domains from merged lists, got %d", len(domains))
	}

	// Verify domains from both sources
	for i := 1; i <= 4; i++ {
		domain := "ads" + string(rune('0'+i)) + ".example.com."
		if _, ok := domains[domain]; !ok {
			t.Errorf("Expected %s to be in merged blocklist", domain)
		}
	}
}

func TestDownloadAll_DuplicateDomains(t *testing.T) {
	// Create servers with overlapping domains
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("0.0.0.0 duplicate.example.com\n0.0.0.0 unique1.example.com\n"))
	}))
	defer server1.Close()

	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("||duplicate.example.com^\n||unique2.example.com^\n"))
	}))
	defer server2.Close()

	logger := logging.NewDefault()
	d := NewDownloader(logger)

	ctx := context.Background()
	urls := []string{server1.URL, server2.URL}
	domains, err := d.DownloadAll(ctx, urls)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Should have 3 unique domains (duplicate appears in both)
	if len(domains) != 3 {
		t.Errorf("Expected 3 unique domains, got %d", len(domains))
	}

	// Verify all domains are present
	expectedDomains := []string{
		"duplicate.example.com.",
		"unique1.example.com.",
		"unique2.example.com.",
	}

	for _, domain := range expectedDomains {
		if _, ok := domains[domain]; !ok {
			t.Errorf("Expected %s to be in blocklist", domain)
		}
	}
}

func TestDownloadAll_OneSourceFails(t *testing.T) {
	// Create working server
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("0.0.0.0 ads1.example.com\n"))
	}))
	defer server1.Close()

	// Create failing server
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server2.Close()

	logger := logging.NewDefault()
	d := NewDownloader(logger)

	ctx := context.Background()
	urls := []string{server1.URL, server2.URL}
	domains, err := d.DownloadAll(ctx, urls)

	// Should not return error (continues with successful downloads)
	if err != nil {
		t.Fatalf("Expected no error when one source fails, got %v", err)
	}

	// Should still have domains from successful download
	if len(domains) != 1 {
		t.Errorf("Expected 1 domain from successful source, got %d", len(domains))
	}

	if _, ok := domains["ads1.example.com."]; !ok {
		t.Error("Expected ads1.example.com. to be in blocklist")
	}
}

func TestDownloadAll_EmptyList(t *testing.T) {
	logger := logging.NewDefault()
	d := NewDownloader(logger)

	ctx := context.Background()
	domains, err := d.DownloadAll(ctx, []string{})

	if err != nil {
		t.Fatalf("Expected no error for empty list, got %v", err)
	}

	if len(domains) != 0 {
		t.Errorf("Expected 0 domains for empty list, got %d", len(domains))
	}
}

func TestExtractDomain_VariousFormats(t *testing.T) {
	logger := logging.NewDefault()
	d := NewDownloader(logger)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"Adblock format", "||ads.example.com^", "ads.example.com"},
		{"Hosts format with 0.0.0.0", "0.0.0.0 ads.example.com", "ads.example.com"},
		{"Hosts format with 127.0.0.1", "127.0.0.1 ads.example.com", "ads.example.com"},
		{"Plain domain", "ads.example.com", "ads.example.com"},
		{"Comment", "# This is a comment", ""},
		{"Empty line", "", ""},
		{"Localhost", "0.0.0.0 localhost", ""},
		{"localhost.localdomain", "127.0.0.1 localhost.localdomain", ""},
		{"Adblock with subdomain", "||tracker.ads.example.com^", "tracker.ads.example.com"},
		{"Multiple spaces", "0.0.0.0     ads.example.com", "ads.example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := d.extractDomain(tt.input)
			if result != tt.expected {
				t.Errorf("extractDomain(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
