package dns

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"glory-hole/pkg/cache"
	"glory-hole/pkg/config"
	"glory-hole/pkg/forwarder"
	"glory-hole/pkg/logging"
	"glory-hole/pkg/telemetry"

	"github.com/miekg/dns"
	proxyproto "github.com/pires/go-proxyproto"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Server is the DNS server
type Server struct {
	cfg            *config.Config
	handler        *Handler
	logger         *logging.Logger
	metrics        *telemetry.Metrics
	clientACL      *ClientACL
	udpServer      *dns.Server
	tcpServer      *dns.Server
	dotServer      *dns.Server
	acmeHTTPServer *http.Server
	tlsConfig      *tls.Config
	acmeRenew      *acmeManager
	running        bool
	mu             sync.RWMutex
}

// NewServer creates a new DNS server
func NewServer(cfg *config.Config, handler *Handler, logger *logging.Logger, metrics *telemetry.Metrics) *Server {
	// Initialize cache if enabled
	if cfg.Cache.Enabled {
		dnsCache, err := cache.New(&cfg.Cache, logger, metrics)
		if err != nil {
			logger.Error("Failed to initialize cache, continuing without cache", "error", err)
		} else {
			handler.SetCache(dnsCache)
			logger.Info("DNS cache enabled",
				"max_entries", cfg.Cache.MaxEntries,
				"min_ttl", cfg.Cache.MinTTL,
				"max_ttl", cfg.Cache.MaxTTL)
		}
	} else {
		logger.Info("DNS cache disabled")
	}

	// Initialize forwarder if upstream servers are configured
	if len(cfg.UpstreamDNSServers) > 0 {
		fwd := forwarder.NewForwarder(cfg, logger)
		handler.SetForwarder(fwd)
	}

	// Prepare TLS resources for DoT (if enabled)
	res, err := buildTLSResources(&cfg.Server, cfg.UpstreamDNSServers, logger)
	if err != nil {
		logger.Error("Failed to prepare TLS for DoT", "error", err)
	}
	if res == nil {
		// Ensure we never panic when TLS prep fails; DoT will be skipped.
		res = &tlsResources{}
	}

	// Build client ACL for plain DNS (port 53)
	acl := NewClientACL(cfg.Server.AllowedClients)
	if !acl.IsOpen() {
		logger.Info("DNS client ACL enabled",
			"entries", len(cfg.Server.AllowedClients),
			"note", "DoT and DoH bypass ACL")
	}

	return &Server{
		cfg:            cfg,
		handler:        handler,
		logger:         logger,
		metrics:        metrics,
		clientACL:      acl,
		tlsConfig:      res.TLSConfig,
		acmeHTTPServer: res.ACMEHTTPServer,
		acmeRenew:      res.ACMERenewer,
	}
}

// Start starts the DNS server (UDP and TCP)
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("server already running")
	}
	s.running = true

	// Each listener gets its own handler with the correct transport label.
	// Plain DNS (port 53) enforces the client ACL; DoT does not.
	udpHandler := &wrappedHandler{
		handler: s.handler, logger: s.logger, metrics: s.metrics,
		clientACL: s.clientACL, transport: "udp",
	}
	tcpHandler := &wrappedHandler{
		handler: s.handler, logger: s.logger, metrics: s.metrics,
		clientACL: s.clientACL, transport: "tcp",
	}
	dotHandler := &wrappedHandler{
		handler: s.handler, logger: s.logger, metrics: s.metrics,
		transport: "dot", // no clientACL — DoT bypasses ACL
	}

	errChan := make(chan error, 4)

	// Create and assign UDP server
	if s.cfg.Server.UDPEnabled {
		s.udpServer = &dns.Server{
			Addr:    s.cfg.Server.UDPAddr(),
			Net:     "udp",
			Handler: dns.HandlerFunc(udpHandler.serveDNS),
		}
	}

	// Create and assign TCP server
	if s.cfg.Server.TCPEnabled {
		if s.cfg.Server.ProxyProtocol {
			// PROXY protocol: create raw TCP listener wrapped with proxyproto
			rawLn, err := net.Listen("tcp", s.cfg.Server.TCPAddr())
			if err != nil {
				s.mu.Unlock()
				return fmt.Errorf("TCP DNS listen: %w", err)
			}
			proxyLn := &proxyproto.Listener{
				Listener:          rawLn,
				ReadHeaderTimeout: 5 * time.Second,
			}
			s.tcpServer = &dns.Server{
				Listener: proxyLn,
				Net:      "tcp",
				Handler:  dns.HandlerFunc(tcpHandler.serveDNS),
			}
		} else {
			s.tcpServer = &dns.Server{
				Addr:    s.cfg.Server.TCPAddr(),
				Net:     "tcp",
				Handler: dns.HandlerFunc(tcpHandler.serveDNS),
			}
		}
	}

	// Create DoT server if enabled and TLS is available
	if s.cfg.Server.DotEnabled && s.tlsConfig != nil {
		if s.cfg.Server.ProxyProtocol {
			// PROXY protocol + TLS: raw TCP → proxyproto → TLS
			// Fly.io sends PROXY header before TLS ClientHello, so
			// the proxy layer must sit between raw TCP and TLS.
			rawLn, err := net.Listen("tcp", s.cfg.Server.DotAddress)
			if err != nil {
				s.mu.Unlock()
				return fmt.Errorf("DoT listen: %w", err)
			}
			proxyLn := &proxyproto.Listener{
				Listener:          rawLn,
				ReadHeaderTimeout: 5 * time.Second,
			}
			tlsLn := tls.NewListener(proxyLn, s.tlsConfig)
			s.dotServer = &dns.Server{
				Listener: tlsLn,
				Net:      "tcp-tls",
				Handler:  dns.HandlerFunc(dotHandler.serveDNS),
			}
		} else {
			s.dotServer = &dns.Server{
				Addr:      s.cfg.Server.DotAddress,
				Net:       "tcp-tls",
				Handler:   dns.HandlerFunc(dotHandler.serveDNS),
				TLSConfig: s.tlsConfig,
			}
		}
	}

	// Unlock before starting goroutines
	s.mu.Unlock()

	// Start UDP server in goroutine
	if s.cfg.Server.UDPEnabled {
		go func() {
			s.logger.Info("Starting UDP DNS server", "address", s.cfg.Server.UDPAddr())
			s.mu.RLock()
			udpSrv := s.udpServer
			s.mu.RUnlock()
			if err := udpSrv.ListenAndServe(); err != nil {
				errChan <- fmt.Errorf("UDP server failed: %w", err)
			}
		}()
	}

	// Start TCP server in goroutine
	if s.cfg.Server.TCPEnabled {
		go func() {
			s.logger.Info("Starting TCP DNS server",
				"address", s.cfg.Server.TCPAddr(),
				"proxy_protocol", s.cfg.Server.ProxyProtocol)
			s.mu.RLock()
			tcpSrv := s.tcpServer
			s.mu.RUnlock()
			var err error
			if s.cfg.Server.ProxyProtocol {
				err = tcpSrv.ActivateAndServe()
			} else {
				err = tcpSrv.ListenAndServe()
			}
			if err != nil {
				errChan <- fmt.Errorf("TCP server failed: %w", err)
			}
		}()
	}

	// Start DoT server
	if s.dotServer != nil {
		go func() {
			s.logger.Info("Starting DoT server",
				"address", s.cfg.Server.DotAddress,
				"proxy_protocol", s.cfg.Server.ProxyProtocol)
			s.mu.RLock()
			dotSrv := s.dotServer
			s.mu.RUnlock()
			var err error
			if s.cfg.Server.ProxyProtocol {
				err = dotSrv.ActivateAndServe()
			} else {
				err = dotSrv.ListenAndServe()
			}
			if err != nil {
				errChan <- fmt.Errorf("DoT server failed: %w", err)
			}
		}()
	}

	// Start ACME HTTP-01 challenge server if configured
	if s.acmeHTTPServer != nil {
		go func() {
			s.logger.Info("Starting ACME HTTP-01 challenge server", "address", s.acmeHTTPServer.Addr)
			if err := s.acmeHTTPServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				errChan <- fmt.Errorf("ACME HTTP server failed: %w", err)
			}
		}()
	}

	s.logger.Info("DNS server started",
		"udp_address", s.cfg.Server.UDPAddr(),
		"tcp_address", s.cfg.Server.TCPAddr(),
		"udp", s.cfg.Server.UDPEnabled,
		"tcp", s.cfg.Server.TCPEnabled,
	)

	// Wait for context cancellation or server error
	select {
	case <-ctx.Done():
		s.logger.Info("DNS server shutting down")
		return s.Shutdown(context.Background())
	case err := <-errChan:
		s.logger.Error("DNS server error", "error", err)
		return err
	}
}

// Shutdown gracefully shuts down the DNS server
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	s.logger.Info("Shutting down DNS server")

	var errs []error

	// Shutdown UDP server
	if s.udpServer != nil {
		if err := s.udpServer.ShutdownContext(ctx); err != nil {
			errs = append(errs, fmt.Errorf("UDP shutdown: %w", err))
		}
	}

	// Shutdown TCP server
	if s.tcpServer != nil {
		if err := s.tcpServer.ShutdownContext(ctx); err != nil {
			errs = append(errs, fmt.Errorf("TCP shutdown: %w", err))
		}
	}

	// Shutdown DoT server
	if s.dotServer != nil {
		if err := s.dotServer.ShutdownContext(ctx); err != nil {
			errs = append(errs, fmt.Errorf("DoT shutdown: %w", err))
		}
	}

	// Shutdown ACME HTTP server
	if s.acmeHTTPServer != nil {
		if err := s.acmeHTTPServer.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("ACME HTTP shutdown: %w", err))
		}
	}

	if s.acmeRenew != nil {
		s.acmeRenew.Stop()
	}

	s.running = false

	if len(errs) > 0 {
		return fmt.Errorf("shutdown errors: %v", errs)
	}

	s.logger.Info("DNS server shut down successfully")
	return nil
}

// UpdateClientACL replaces the client ACL entries (hot-reload safe).
func (s *Server) UpdateClientACL(entries []string) {
	if s.clientACL != nil {
		s.clientACL.Update(entries)
		s.logger.Info("DNS client ACL updated", "entries", len(entries))
	}
}

// ClientACL returns the server's client ACL for API inspection.
func (s *Server) ClientACL() *ClientACL {
	return s.clientACL
}

// IsRunning returns whether the server is running
func (s *Server) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// wrappedHandler wraps the DNS handler with logging, metrics, and ACL.
// Each DNS listener (UDP, TCP, DoT) gets its own instance with the
// correct transport label and ACL configuration.
type wrappedHandler struct {
	handler   *Handler
	logger    *logging.Logger
	metrics   *telemetry.Metrics
	clientACL *ClientACL
	transport string // "udp", "tcp", or "dot" — set at creation, not inferred
}

// serveDNS is the DNS request handler wrapper that adds observability.
// It wraps the core DNS handler (Handler.ServeDNS) with:
// - Request logging (domain, query type, client IP)
// - Prometheus metrics collection (query counts by type)
// - Query duration measurement
//
// This wrapper sits between the miekg/dns library and our Handler,
// providing a single point for cross-cutting concerns without
// polluting the core DNS resolution logic.
//
// Metrics collected:
// - DNSQueriesTotal: Counter of total queries
// - DNSQueriesByType: Counter per query type (A, AAAA, MX, etc.)
// - DNSQueryDuration: Histogram of query latencies
// - ActiveClients: Gauge of concurrent queries being processed
func (w *wrappedHandler) serveDNS(rw dns.ResponseWriter, r *dns.Msg) {
	startTime := time.Now()
	ctx := context.Background()

	// Client ACL: enforce only when set (plain DNS handlers have it, DoT does not).
	if w.clientACL != nil {
		clientIP := getClientIP(rw)
		if !w.clientACL.IsAllowed(clientIP) {
			msg := new(dns.Msg)
			msg.SetRcode(r, dns.RcodeRefused)
			_ = rw.WriteMsg(msg)
			w.logger.Warn("DNS query refused by client ACL",
				"client", clientIP,
				"transport", w.transport,
			)
			return
		}
	}

	// Track active clients (concurrent queries)
	if w.metrics != nil {
		w.metrics.ActiveClients.Add(ctx, 1)
		defer w.metrics.ActiveClients.Add(ctx, -1)
	}

	// Extract query information
	var domain string
	var qtype uint16
	queryTypeName := "UNKNOWN"
	if len(r.Question) > 0 {
		domain = r.Question[0].Name
		qtype = r.Question[0].Qtype
		if label := dns.TypeToString[qtype]; label != "" {
			queryTypeName = label
		} else {
			queryTypeName = fmt.Sprintf("TYPE%d", qtype)
		}
	}

	clientIP := getClientIP(rw)

	// Log the query
	w.logger.Info("DNS query received",
		"domain", domain,
		"type", queryTypeName,
		"client", clientIP,
		"transport", w.transportLabel(rw),
	)

	// Record metrics
	if w.metrics != nil {
		attrs := []attribute.KeyValue{attribute.String("transport", w.transportLabel(rw))}
		w.metrics.DNSQueriesTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
		if queryTypeName != "" {
			w.metrics.DNSQueriesByType.Add(ctx, 1, metric.WithAttributes(
				append(attrs, attribute.String("type", queryTypeName))...),
			)
		} else {
			w.metrics.DNSQueriesByType.Add(ctx, 1, metric.WithAttributes(attrs...))
		}
	}

	// Call the actual handler
	w.handler.ServeDNS(ctx, rw, r)

	// Record query duration
	duration := time.Since(startTime)
	if w.metrics != nil {
		w.metrics.DNSQueryDuration.Record(ctx, float64(duration.Milliseconds()),
			metric.WithAttributes(attribute.String("transport", w.transportLabel(rw))))
	}

	w.logger.Info("DNS query processed",
		"domain", domain,
		"duration_ms", duration.Milliseconds(),
		"transport", w.transportLabel(rw),
	)
}

// transportLabel returns the transport for metrics/logs.
// Uses the label set at handler creation — network-level inference
// can't distinguish plain TCP from DoT (both report "tcp").
func (w *wrappedHandler) transportLabel(_ dns.ResponseWriter) string {
	if w.transport != "" {
		return w.transport
	}
	return "unknown"
}

// getClientIP extracts the client IP address from the DNS ResponseWriter.
// It handles both UDP and TCP connections by parsing the RemoteAddr(),
// which can return different address formats depending on the protocol:
// - UDP: *net.UDPAddr
// - TCP: *net.TCPAddr
//
// The function attempts to extract just the IP portion (without port)
// using net.SplitHostPort. If that fails (e.g., IPv6 without brackets),
// it falls back to returning the full address string.
//
// Returns "unknown" if RemoteAddr() is nil, which shouldn't happen in
// normal operation but provides a safe default.
func getClientIP(w dns.ResponseWriter) string {
	if w.RemoteAddr() != nil {
		host, _, err := net.SplitHostPort(w.RemoteAddr().String())
		if err == nil {
			return host
		}
		return w.RemoteAddr().String()
	}
	return "unknown"
}
