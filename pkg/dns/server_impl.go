package dns

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"glory-hole/pkg/cache"
	"glory-hole/pkg/config"
	"glory-hole/pkg/forwarder"
	"glory-hole/pkg/logging"
	"glory-hole/pkg/telemetry"

	"github.com/miekg/dns"
)

// Server is the DNS server
type Server struct {
	cfg       *config.Config
	handler   *Handler
	logger    *logging.Logger
	metrics   *telemetry.Metrics
	udpServer *dns.Server
	tcpServer *dns.Server
	running   bool
	mu        sync.RWMutex
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

	return &Server{
		cfg:     cfg,
		handler: handler,
		logger:  logger,
		metrics: metrics,
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

	// Wrap handler with telemetry and logging
	wrappedHandler := &wrappedHandler{
		handler: s.handler,
		logger:  s.logger,
		metrics: s.metrics,
	}

	errChan := make(chan error, 2)

	// Create and assign UDP server
	if s.cfg.Server.UDPEnabled {
		s.udpServer = &dns.Server{
			Addr:    s.cfg.Server.ListenAddress,
			Net:     "udp",
			Handler: dns.HandlerFunc(wrappedHandler.serveDNS),
		}
	}

	// Create and assign TCP server
	if s.cfg.Server.TCPEnabled {
		s.tcpServer = &dns.Server{
			Addr:    s.cfg.Server.ListenAddress,
			Net:     "tcp",
			Handler: dns.HandlerFunc(wrappedHandler.serveDNS),
		}
	}

	// Unlock before starting goroutines
	s.mu.Unlock()

	// Start UDP server in goroutine
	if s.cfg.Server.UDPEnabled {
		go func() {
			s.logger.Info("Starting UDP DNS server", "address", s.cfg.Server.ListenAddress)
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
			s.logger.Info("Starting TCP DNS server", "address", s.cfg.Server.ListenAddress)
			s.mu.RLock()
			tcpSrv := s.tcpServer
			s.mu.RUnlock()
			if err := tcpSrv.ListenAndServe(); err != nil {
				errChan <- fmt.Errorf("TCP server failed: %w", err)
			}
		}()
	}

	s.logger.Info("DNS server started",
		"address", s.cfg.Server.ListenAddress,
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

	s.running = false

	if len(errs) > 0 {
		return fmt.Errorf("shutdown errors: %v", errs)
	}

	s.logger.Info("DNS server shut down successfully")
	return nil
}

// IsRunning returns whether the server is running
func (s *Server) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// wrappedHandler wraps the DNS handler with logging and metrics
type wrappedHandler struct {
	handler *Handler
	logger  *logging.Logger
	metrics *telemetry.Metrics
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

	// Track active clients (concurrent queries)
	if w.metrics != nil {
		w.metrics.ActiveClients.Add(ctx, 1)
		defer w.metrics.ActiveClients.Add(ctx, -1)
	}

	// Extract query information
	var domain string
	var qtype uint16
	if len(r.Question) > 0 {
		domain = r.Question[0].Name
		qtype = r.Question[0].Qtype
	}

	clientIP := getClientIP(rw)

	// Log the query
	w.logger.Info("DNS query received",
		"domain", domain,
		"type", dns.TypeToString[qtype],
		"client", clientIP,
	)

	// Record metrics
	if w.metrics != nil {
		w.metrics.DNSQueriesTotal.Add(ctx, 1)
		w.metrics.DNSQueriesByType.Add(ctx, 1) // attribute.String("type", dns.TypeToString[qtype]),
	}

	// Call the actual handler
	w.handler.ServeDNS(ctx, rw, r)

	// Record query duration
	duration := time.Since(startTime)
	if w.metrics != nil {
		w.metrics.DNSQueryDuration.Record(ctx, float64(duration.Milliseconds()))
	}

	w.logger.Info("DNS query processed",
		"domain", domain,
		"duration_ms", duration.Milliseconds(),
	)
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
