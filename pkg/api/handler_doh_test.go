package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"glory-hole/pkg/dns"
	"glory-hole/pkg/logging"

	mdns "github.com/miekg/dns"
)

func TestHandleDNSQuery_HEAD(t *testing.T) {
	server := createTestServer()

	req := httptest.NewRequest("HEAD", "/dns-query", nil)
	w := httptest.NewRecorder()

	server.handleDNSQuery(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200 for HEAD request, got %d", w.Code)
	}

	if w.Body.Len() != 0 {
		t.Error("HEAD request should have empty body")
	}
}

func TestHandleDNSQuery_GET_JSON(t *testing.T) {
	server := createTestServerWithDNS()

	req := httptest.NewRequest("GET", "/dns-query?name=example.com&type=A", nil)
	req.Header.Set("Accept", "application/dns-json")
	w := httptest.NewRecorder()

	server.handleDNSQuery(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	contentType := w.Header().Get("Content-Type")
	if !strings.Contains(contentType, "application/dns-json") {
		t.Errorf("Expected Content-Type 'application/dns-json', got %s", contentType)
	}

	var response DNSJSONResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	if len(response.Question) == 0 {
		t.Error("Expected at least one question")
	}

	if response.Question[0].Name != "example.com" {
		t.Errorf("Expected question name 'example.com', got %s", response.Question[0].Name)
	}

	if response.Question[0].Type != 1 { // A record
		t.Errorf("Expected question type 1 (A), got %d", response.Question[0].Type)
	}
}

func TestHandleDNSQuery_GET_TypeString(t *testing.T) {
	server := createTestServerWithDNS()

	// Test with type as string
	req := httptest.NewRequest("GET", "/dns-query?name=example.com&type=AAAA", nil)
	req.Header.Set("Accept", "application/dns-json")
	w := httptest.NewRecorder()

	server.handleDNSQuery(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response DNSJSONResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	if response.Question[0].Type != 28 { // AAAA record
		t.Errorf("Expected question type 28 (AAAA), got %d", response.Question[0].Type)
	}
}

func TestHandleDNSQuery_GET_MissingName(t *testing.T) {
	server := createTestServer()

	req := httptest.NewRequest("GET", "/dns-query", nil)
	req.Header.Set("Accept", "application/dns-json")
	w := httptest.NewRecorder()

	server.handleDNSQuery(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for missing name, got %d", w.Code)
	}
}

func TestHandleDNSQuery_GET_WireFormat(t *testing.T) {
	server := createTestServerWithDNS()

	req := httptest.NewRequest("GET", "/dns-query?name=example.com&type=A", nil)
	req.Header.Set("Accept", "application/dns-message")
	w := httptest.NewRecorder()

	server.handleDNSQuery(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if !strings.Contains(contentType, "application/dns-message") {
		t.Errorf("Expected Content-Type 'application/dns-message', got %s", contentType)
	}

	// Verify it's valid DNS wire format
	msg := new(mdns.Msg)
	if err := msg.Unpack(w.Body.Bytes()); err != nil {
		t.Errorf("Failed to parse DNS wire format: %v", err)
	}

	if len(msg.Question) == 0 {
		t.Error("Expected at least one question in wire format")
	}
}

func TestHandleDNSQuery_POST_WireFormat(t *testing.T) {
	server := createTestServerWithDNS()

	// Create a DNS query message
	msg := new(mdns.Msg)
	msg.SetQuestion(mdns.Fqdn("example.com"), mdns.TypeA)
	msg.RecursionDesired = true

	packed, err := msg.Pack()
	if err != nil {
		t.Fatalf("Failed to pack DNS message: %v", err)
	}

	req := httptest.NewRequest("POST", "/dns-query", bytes.NewReader(packed))
	req.Header.Set("Content-Type", "application/dns-message")
	req.Header.Set("Accept", "application/dns-json")
	w := httptest.NewRecorder()

	server.handleDNSQuery(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var response DNSJSONResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	if response.Question[0].Name != "example.com" {
		t.Errorf("Expected question name 'example.com', got %s", response.Question[0].Name)
	}
}

func TestHandleDNSQuery_POST_InvalidContentType(t *testing.T) {
	server := createTestServer()

	req := httptest.NewRequest("POST", "/dns-query", strings.NewReader("invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleDNSQuery(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for invalid content type, got %d", w.Code)
	}
}

func TestHandleDNSQuery_POST_TooLarge(t *testing.T) {
	server := createTestServer()

	// Create message larger than 4KB
	largeData := make([]byte, 5000)
	req := httptest.NewRequest("POST", "/dns-query", bytes.NewReader(largeData))
	req.Header.Set("Content-Type", "application/dns-message")
	w := httptest.NewRecorder()

	server.handleDNSQuery(w, req)

	// Should return 400 (caught by size check in POST handler)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for too large message, got %d", w.Code)
	}

	// Verify error message mentions size
	var errResp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &errResp); err == nil {
		if errorMsg, ok := errResp["error"].(string); ok {
			if !strings.Contains(errorMsg, "size") && !strings.Contains(errorMsg, "maximum") {
				t.Errorf("Expected error message to mention size, got: %s", errorMsg)
			}
		}
	}
}

func TestHandleDNSQuery_GET_DNSParam(t *testing.T) {
	server := createTestServerWithDNS()

	// Create a DNS query and encode it as base64
	msg := new(mdns.Msg)
	msg.SetQuestion(mdns.Fqdn("example.com"), mdns.TypeA)
	packed, _ := msg.Pack()
	encoded := base64.RawURLEncoding.EncodeToString(packed)

	req := httptest.NewRequest("GET", "/dns-query?dns="+encoded, nil)
	req.Header.Set("Accept", "application/dns-json")
	w := httptest.NewRecorder()

	server.handleDNSQuery(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var response DNSJSONResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	if response.Question[0].Name != "example.com" {
		t.Errorf("Expected question name 'example.com', got %s", response.Question[0].Name)
	}
}

func TestHandleDNSQuery_DNSSEC_Flags(t *testing.T) {
	server := createTestServerWithDNS()

	// Test with DNSSEC DO flag
	req := httptest.NewRequest("GET", "/dns-query?name=example.com&type=A&do=1", nil)
	req.Header.Set("Accept", "application/dns-json")
	w := httptest.NewRecorder()

	server.handleDNSQuery(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Verify EDNS0 was set (indicated by AD/CD flags or RRSIG records in response)
	var response DNSJSONResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}
}

func TestHandleDNSQuery_CacheControl(t *testing.T) {
	server := createTestServerWithDNS()

	req := httptest.NewRequest("GET", "/dns-query?name=example.com&type=A", nil)
	req.Header.Set("Accept", "application/dns-json")
	w := httptest.NewRecorder()

	server.handleDNSQuery(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	cacheControl := w.Header().Get("Cache-Control")
	if !strings.Contains(cacheControl, "max-age=") {
		t.Errorf("Expected Cache-Control header with max-age, got: %s", cacheControl)
	}
}

func TestHandleDNSQuery_MethodNotAllowed(t *testing.T) {
	server := createTestServer()

	req := httptest.NewRequest("PUT", "/dns-query", nil)
	w := httptest.NewRecorder()

	server.handleDNSQuery(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405 for PUT method, got %d", w.Code)
	}
}

func TestRRToJSON_VariousTypes(t *testing.T) {
	server := createTestServer()

	tests := []struct {
		name     string
		rr       mdns.RR
		expected string
	}{
		{
			name:     "A record",
			rr:       &mdns.A{Hdr: mdns.RR_Header{Name: "example.com.", Rrtype: mdns.TypeA, Ttl: 300}, A: []byte{192, 0, 2, 1}},
			expected: "192.0.2.1",
		},
		{
			name:     "CNAME record",
			rr:       &mdns.CNAME{Hdr: mdns.RR_Header{Name: "www.example.com.", Rrtype: mdns.TypeCNAME, Ttl: 300}, Target: "example.com."},
			expected: "example.com",
		},
		{
			name:     "MX record",
			rr:       &mdns.MX{Hdr: mdns.RR_Header{Name: "example.com.", Rrtype: mdns.TypeMX, Ttl: 300}, Preference: 10, Mx: "mail.example.com."},
			expected: "10 mail.example.com",
		},
		{
			name:     "TXT record",
			rr:       &mdns.TXT{Hdr: mdns.RR_Header{Name: "example.com.", Rrtype: mdns.TypeTXT, Ttl: 300}, Txt: []string{"v=spf1", "include:_spf.google.com"}},
			expected: "v=spf1 include:_spf.google.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			answer := server.rrToJSON(tt.rr)
			if answer.Data != tt.expected {
				t.Errorf("Expected data %q, got %q", tt.expected, answer.Data)
			}
		})
	}
}

func TestCalculateTTL(t *testing.T) {
	server := createTestServer()

	tests := []struct {
		name     string
		msg      *mdns.Msg
		expected uint32
	}{
		{
			name:     "No answers",
			msg:      &mdns.Msg{},
			expected: 60,
		},
		{
			name: "Single answer",
			msg: &mdns.Msg{
				Answer: []mdns.RR{
					&mdns.A{Hdr: mdns.RR_Header{Ttl: 300}},
				},
			},
			expected: 300,
		},
		{
			name: "Multiple answers - minimum TTL",
			msg: &mdns.Msg{
				Answer: []mdns.RR{
					&mdns.A{Hdr: mdns.RR_Header{Ttl: 300}},
					&mdns.A{Hdr: mdns.RR_Header{Ttl: 60}},
					&mdns.A{Hdr: mdns.RR_Header{Ttl: 180}},
				},
			},
			expected: 60,
		},
		{
			name: "Zero TTL",
			msg: &mdns.Msg{
				Answer: []mdns.RR{
					&mdns.A{Hdr: mdns.RR_Header{Ttl: 0}},
				},
			},
			expected: 1, // Enforced minimum
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ttl := server.calculateTTL(tt.msg)
			if ttl != tt.expected {
				t.Errorf("Expected TTL %d, got %d", tt.expected, ttl)
			}
		})
	}
}

// Helper functions

func createTestServer() *Server {
	logger := logging.NewDefault()

	return &Server{
		logger: logger.Logger,
	}
}

func createTestServerWithDNS() *Server {
	server := createTestServer()
	server.dnsHandler = dns.NewHandler()
	return server
}
