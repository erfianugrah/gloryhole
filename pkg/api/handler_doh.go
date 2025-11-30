package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/miekg/dns"
)

// DNS-over-HTTPS (DoH) implementation
// Compatible with Cloudflare's DNS-over-HTTPS API and RFC 8484

// dohResponseWriter captures DNS responses for HTTP conversion
type dohResponseWriter struct {
	msg      *dns.Msg
	clientIP string
}

func (w *dohResponseWriter) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 443}
}

func (w *dohResponseWriter) RemoteAddr() net.Addr {
	if w.clientIP != "" {
		return &net.TCPAddr{IP: net.ParseIP(w.clientIP), Port: 0}
	}
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0}
}

func (w *dohResponseWriter) WriteMsg(m *dns.Msg) error {
	w.msg = m.Copy() // Make a copy to avoid issues with message pooling
	return nil
}

func (w *dohResponseWriter) Write(b []byte) (int, error) {
	// Parse the wire format message
	msg := new(dns.Msg)
	if err := msg.Unpack(b); err != nil {
		return 0, err
	}
	w.msg = msg
	return len(b), nil
}

func (w *dohResponseWriter) Close() error {
	return nil
}

func (w *dohResponseWriter) TsigStatus() error {
	return nil
}

func (w *dohResponseWriter) TsigTimersOnly(b bool) {}

func (w *dohResponseWriter) Hijack() {}

// DNSQuestion represents a DNS question in JSON format
type DNSQuestion struct {
	Name string `json:"name"`
	Type uint16 `json:"type"`
}

// DNSAnswer represents a DNS answer in JSON format
type DNSAnswer struct {
	Name string `json:"name"`
	Type uint16 `json:"type"`
	TTL  uint32 `json:"TTL"`
	Data string `json:"data"`
}

// DNSJSONResponse represents the JSON response format (compatible with Cloudflare/Google)
type DNSJSONResponse struct {
	Status           int           `json:"Status"`
	TC               bool          `json:"TC"`
	RD               bool          `json:"RD"`
	RA               bool          `json:"RA"`
	AD               bool          `json:"AD"`
	CD               bool          `json:"CD"`
	Question         []DNSQuestion `json:"Question,omitempty"`
	Answer           []DNSAnswer   `json:"Answer,omitempty"`
	Authority        []DNSAnswer   `json:"Authority,omitempty"`
	Additional       []DNSAnswer   `json:"Additional,omitempty"`
	Comment          string        `json:"Comment,omitempty"`
	EdnsClientSubnet string        `json:"edns_client_subnet,omitempty"`
}

// handleDNSQuery handles DNS-over-HTTPS requests
// Supports GET (with query parameters), POST (wire format), and HEAD (health check)
// Compatible with Cloudflare's DNS-over-HTTPS API
func (s *Server) handleDNSQuery(w http.ResponseWriter, r *http.Request) {
	// HEAD request: Simple health check
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}

	var dnsMsg *dns.Msg
	var err error

	// Parse request based on method
	switch r.Method {
	case http.MethodGet:
		dnsMsg, err = s.parseDNSQueryGET(r)
	case http.MethodPost:
		dnsMsg, err = s.parseDNSQueryPOST(r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err != nil {
		s.handleDOHError(w, err, http.StatusBadRequest)
		return
	}

	if dnsMsg == nil {
		s.handleDOHError(w, fmt.Errorf("query not specified"), http.StatusBadRequest)
		return
	}

	// Validate message
	if len(dnsMsg.Question) == 0 {
		s.handleDOHError(w, fmt.Errorf("query not specified or too small"), http.StatusBadRequest)
		return
	}

	// Check message size (RFC 8484 recommends 512 bytes for GET)
	if r.Method == http.MethodGet {
		packed, _ := dnsMsg.Pack()
		if len(packed) > 512 {
			s.handleDOHError(w, fmt.Errorf("query exceeds maximum size"), http.StatusRequestEntityTooLarge)
			return
		}
	}

	// Get client IP from request
	clientIP := s.getClientIP(r)

	// Create custom response writer to capture DNS response
	dohWriter := &dohResponseWriter{clientIP: clientIP}

	// Use DNS handler if available, otherwise return SERVFAIL
	if s.dnsHandler == nil {
		s.logger.Warn("DNS handler not configured for DoH requests")
		msg := new(dns.Msg)
		msg.SetReply(dnsMsg)
		msg.SetRcode(dnsMsg, dns.RcodeServerFailure)
		dohWriter.msg = msg
	} else {
		// Process DNS query through our DNS handler
		ctx := context.Background()
		s.dnsHandler.ServeDNS(ctx, dohWriter, dnsMsg)
	}

	if dohWriter.msg == nil {
		s.handleDOHError(w, fmt.Errorf("no response from DNS resolver"), http.StatusGatewayTimeout)
		return
	}

	// Determine response format based on Accept header
	acceptHeader := r.Header.Get("Accept")
	contentType := r.Header.Get("Content-Type")

	// Default to JSON if no preference specified
	useJSON := true
	if strings.Contains(acceptHeader, "application/dns-message") {
		useJSON = false
	} else if strings.Contains(contentType, "application/dns-message") && r.Method == http.MethodPost {
		// For POST with dns-message content-type, default to wire format response
		useJSON = false
	}

	// Override with accept header if explicitly set
	if strings.Contains(acceptHeader, "application/dns-json") {
		useJSON = true
	}

	if useJSON {
		s.writeDNSJSON(w, dohWriter.msg)
	} else {
		s.writeDNSWireFormat(w, dohWriter.msg)
	}
}

// parseDNSQueryGET parses DNS query from GET request query parameters
func (s *Server) parseDNSQueryGET(r *http.Request) (*dns.Msg, error) {
	query := r.URL.Query()

	// Check for dns parameter (base64-encoded wire format)
	if dnsParam := query.Get("dns"); dnsParam != "" {
		// Decode base64 URL-safe encoding
		decoded, err := base64.RawURLEncoding.DecodeString(dnsParam)
		if err != nil {
			return nil, fmt.Errorf("invalid dns parameter: %w", err)
		}

		msg := new(dns.Msg)
		if err := msg.Unpack(decoded); err != nil {
			return nil, fmt.Errorf("invalid DNS message: %w", err)
		}
		return msg, nil
	}

	// Check for name parameter (simple query)
	name := query.Get("name")
	if name == "" {
		return nil, fmt.Errorf("missing 'name' or 'dns' parameter")
	}

	// Parse type parameter (default to A)
	qtype := dns.TypeA
	if typeStr := query.Get("type"); typeStr != "" {
		// Try parsing as string first
		if qt, ok := dns.StringToType[strings.ToUpper(typeStr)]; ok {
			qtype = qt
		} else {
			// Try parsing as number
			if typeNum, err := strconv.Atoi(typeStr); err == nil {
				qtype = uint16(typeNum)
			}
		}
	}

	// Build DNS message
	msg := new(dns.Msg)
	msg.SetQuestion(dns.Fqdn(name), qtype)
	msg.RecursionDesired = true

	// Handle DNSSEC flags
	if cd := query.Get("cd"); cd == "1" || cd == "true" {
		msg.CheckingDisabled = true
	}

	if do := query.Get("do"); do == "1" || do == "true" {
		msg.SetEdns0(4096, true)
	}

	return msg, nil
}

// parseDNSQueryPOST parses DNS query from POST request body (wire format)
func (s *Server) parseDNSQueryPOST(r *http.Request) (*dns.Msg, error) {
	contentType := r.Header.Get("Content-Type")
	if !strings.Contains(contentType, "application/dns-message") {
		return nil, fmt.Errorf("unsupported content type: %s", contentType)
	}

	// Read body with size limit (64KB max)
	body, err := io.ReadAll(io.LimitReader(r.Body, 65536))
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}

	if len(body) == 0 {
		return nil, fmt.Errorf("empty request body")
	}

	if len(body) > 4096 {
		return nil, fmt.Errorf("query exceeds maximum size")
	}

	msg := new(dns.Msg)
	if err := msg.Unpack(body); err != nil {
		return nil, fmt.Errorf("invalid DNS message: %w", err)
	}

	return msg, nil
}

// writeDNSJSON writes DNS response in JSON format (Cloudflare/Google compatible)
func (s *Server) writeDNSJSON(w http.ResponseWriter, msg *dns.Msg) {
	response := DNSJSONResponse{
		Status: msg.Rcode,
		TC:     msg.Truncated,
		RD:     msg.RecursionDesired,
		RA:     msg.RecursionAvailable,
		AD:     msg.AuthenticatedData,
		CD:     msg.CheckingDisabled,
	}

	// Add questions
	for _, q := range msg.Question {
		response.Question = append(response.Question, DNSQuestion{
			Name: strings.TrimSuffix(q.Name, "."),
			Type: q.Qtype,
		})
	}

	// Add answers
	for _, rr := range msg.Answer {
		response.Answer = append(response.Answer, s.rrToJSON(rr))
	}

	// Add authority
	for _, rr := range msg.Ns {
		response.Authority = append(response.Authority, s.rrToJSON(rr))
	}

	// Add additional
	for _, rr := range msg.Extra {
		// Skip OPT records (EDNS0) in JSON output
		if rr.Header().Rrtype != dns.TypeOPT {
			response.Additional = append(response.Additional, s.rrToJSON(rr))
		}
	}

	w.Header().Set("Content-Type", "application/dns-json")
	w.Header().Set("Cache-Control", fmt.Sprintf("max-age=%d", s.calculateTTL(msg)))
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.logger.Error("Failed to encode DNS JSON response", "error", err)
	}
}

// writeDNSWireFormat writes DNS response in wire format (binary)
func (s *Server) writeDNSWireFormat(w http.ResponseWriter, msg *dns.Msg) {
	packed, err := msg.Pack()
	if err != nil {
		s.handleDOHError(w, fmt.Errorf("failed to pack DNS message: %w", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/dns-message")
	w.Header().Set("Cache-Control", fmt.Sprintf("max-age=%d", s.calculateTTL(msg)))
	w.WriteHeader(http.StatusOK)

	if _, err := w.Write(packed); err != nil {
		s.logger.Error("Failed to write DNS wire format response", "error", err)
	}
}

// rrToJSON converts a DNS resource record to JSON format
func (s *Server) rrToJSON(rr dns.RR) DNSAnswer {
	header := rr.Header()
	return DNSAnswer{
		Name: strings.TrimSuffix(header.Name, "."),
		Type: header.Rrtype,
		TTL:  header.Ttl,
		Data: s.extractRRData(rr),
	}
}

// extractRRData extracts the data field from a resource record
func (s *Server) extractRRData(rr dns.RR) string {
	switch r := rr.(type) {
	case *dns.A:
		return r.A.String()
	case *dns.AAAA:
		return r.AAAA.String()
	case *dns.CNAME:
		return strings.TrimSuffix(r.Target, ".")
	case *dns.MX:
		return fmt.Sprintf("%d %s", r.Preference, strings.TrimSuffix(r.Mx, "."))
	case *dns.NS:
		return strings.TrimSuffix(r.Ns, ".")
	case *dns.PTR:
		return strings.TrimSuffix(r.Ptr, ".")
	case *dns.SOA:
		return fmt.Sprintf("%s %s %d %d %d %d %d",
			strings.TrimSuffix(r.Ns, "."),
			strings.TrimSuffix(r.Mbox, "."),
			r.Serial, r.Refresh, r.Retry, r.Expire, r.Minttl)
	case *dns.SRV:
		return fmt.Sprintf("%d %d %d %s",
			r.Priority, r.Weight, r.Port, strings.TrimSuffix(r.Target, "."))
	case *dns.TXT:
		return strings.Join(r.Txt, " ")
	case *dns.CAA:
		return fmt.Sprintf("%d %s \"%s\"", r.Flag, r.Tag, r.Value)
	default:
		// Fallback to string representation
		return strings.TrimPrefix(rr.String(), rr.Header().String())
	}
}

// calculateTTL calculates the minimum TTL from DNS response for caching
func (s *Server) calculateTTL(msg *dns.Msg) uint32 {
	if msg == nil || len(msg.Answer) == 0 {
		return 60 // Default 60 seconds for negative responses
	}

	minTTL := uint32(86400) // Start with 24 hours
	for _, rr := range msg.Answer {
		if rr.Header().Ttl < minTTL {
			minTTL = rr.Header().Ttl
		}
	}

	// Enforce minimum of 1 second
	if minTTL == 0 {
		minTTL = 1
	}

	return minTTL
}

// handleDOHError writes a DoH error response
func (s *Server) handleDOHError(w http.ResponseWriter, err error, statusCode int) {
	s.logger.Error("DoH request error", "error", err, "status", statusCode)

	// Return JSON error for better client debugging
	response := map[string]interface{}{
		"error":  err.Error(),
		"status": statusCode,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.logger.Error("Failed to encode DoH error response", "error", err)
	}
}
