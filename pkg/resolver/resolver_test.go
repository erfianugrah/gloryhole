package resolver

import (
	"context"
	"net/http"
	"testing"
	"time"

	"glory-hole/pkg/config"
	"glory-hole/pkg/logging"
)

func getTestLogger() *logging.Logger {
	logger, _ := logging.New(&config.LoggingConfig{
		Level:  "error", // Suppress logs during tests
		Format: "text",
		Output: "stdout",
	})
	return logger
}

func TestNew(t *testing.T) {
	logger := getTestLogger()

	tests := []struct {
		name      string
		upstreams []string
		wantNil   bool
	}{
		{
			name:      "with upstreams",
			upstreams: []string{"1.1.1.1:53", "8.8.8.8:53"},
			wantNil:   false,
		},
		{
			name:      "without upstreams",
			upstreams: []string{},
			wantNil:   false,
		},
		{
			name:      "nil upstreams",
			upstreams: nil,
			wantNil:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := New(tt.upstreams, logger)
			if resolver == nil {
				t.Error("New() returned nil")
			}
			if len(tt.upstreams) > 0 && len(resolver.Upstreams()) != len(tt.upstreams) {
				t.Errorf("Upstreams() = %v, want %v", resolver.Upstreams(), tt.upstreams)
			}
		})
	}
}

func TestResolver_LookupIP_SystemDefault(t *testing.T) {
	logger := getTestLogger()
	resolver := New([]string{}, logger) // No upstreams = use system default

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ips, err := resolver.LookupIP(ctx, "ip", "google.com")
	if err != nil {
		t.Fatalf("LookupIP() with system default failed: %v", err)
	}

	if len(ips) == 0 {
		t.Error("LookupIP() returned no IPs")
	}

	t.Logf("Resolved google.com to %v using system default", ips)
}

func TestResolver_LookupIP_CustomUpstream(t *testing.T) {
	logger := getTestLogger()
	resolver := New([]string{"1.1.1.1:53"}, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ips, err := resolver.LookupIP(ctx, "ip", "google.com")
	if err != nil {
		t.Fatalf("LookupIP() with custom upstream failed: %v", err)
	}

	if len(ips) == 0 {
		t.Error("LookupIP() returned no IPs")
	}

	t.Logf("Resolved google.com to %v using 1.1.1.1:53", ips)
}

func TestResolver_DialContext(t *testing.T) {
	logger := getTestLogger()
	resolver := New([]string{"1.1.1.1:53"}, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Test dialing with hostname
	conn, err := resolver.DialContext(ctx, "tcp", "google.com:80")
	if err != nil {
		t.Fatalf("DialContext() failed: %v", err)
	}
	defer conn.Close()

	if conn == nil {
		t.Error("DialContext() returned nil connection")
	}

	t.Logf("Successfully connected to google.com:80 via custom resolver")
}

func TestResolver_DialContext_WithIP(t *testing.T) {
	logger := getTestLogger()
	resolver := New([]string{"1.1.1.1:53"}, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Test dialing with IP address (should skip resolution)
	conn, err := resolver.DialContext(ctx, "tcp", "8.8.8.8:53")
	if err != nil {
		t.Fatalf("DialContext() with IP failed: %v", err)
	}
	defer conn.Close()

	if conn == nil {
		t.Error("DialContext() returned nil connection")
	}

	t.Logf("Successfully connected to 8.8.8.8:53 (direct IP)")
}

func TestResolver_DialContext_InvalidAddress(t *testing.T) {
	logger := getTestLogger()
	resolver := New([]string{"1.1.1.1:53"}, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := resolver.DialContext(ctx, "tcp", "invalid-address")
	if err == nil {
		t.Error("DialContext() should fail with invalid address")
	}
}

func TestResolver_NewHTTPClient(t *testing.T) {
	logger := getTestLogger()

	tests := []struct {
		name      string
		upstreams []string
	}{
		{
			name:      "with upstreams",
			upstreams: []string{"1.1.1.1:53"},
		},
		{
			name:      "without upstreams",
			upstreams: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := New(tt.upstreams, logger)
			client := resolver.NewHTTPClient(30 * time.Second)

			if client == nil {
				t.Fatal("NewHTTPClient() returned nil")
			}

			if client.Timeout != 30*time.Second {
				t.Errorf("Client timeout = %v, want 30s", client.Timeout)
			}

			// Test HTTP request
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			req, _ := http.NewRequestWithContext(ctx, "HEAD", "https://google.com", nil)
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("HTTP request failed: %v", err)
			}
			defer resp.Body.Close()

			t.Logf("HTTP request successful: %s", resp.Status)
		})
	}
}

func TestResolver_Upstreams(t *testing.T) {
	logger := getTestLogger()
	upstreams := []string{"1.1.1.1:53", "8.8.8.8:53"}
	resolver := New(upstreams, logger)

	got := resolver.Upstreams()
	if len(got) != len(upstreams) {
		t.Errorf("Upstreams() length = %d, want %d", len(got), len(upstreams))
	}

	for i, upstream := range got {
		if upstream != upstreams[i] {
			t.Errorf("Upstreams()[%d] = %s, want %s", i, upstream, upstreams[i])
		}
	}
}
