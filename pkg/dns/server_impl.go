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
		dnsCache, err := cache.New(&cfg.Cache, logger)
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
	s.mu.Unlock()

	// Wrap handler with telemetry and logging
	wrappedHandler := &wrappedHandler{
		handler: s.handler,
		logger:  s.logger,
		metrics: s.metrics,
	}

	errChan := make(chan error, 2)

	// Start UDP server
	if s.cfg.Server.UDPEnabled {
		s.udpServer = &dns.Server{
			Addr:    s.cfg.Server.ListenAddress,
			Net:     "udp",
			Handler: dns.HandlerFunc(wrappedHandler.serveDNS),
		}

		go func() {
			s.logger.Info("Starting UDP DNS server", "address", s.cfg.Server.ListenAddress)
			if err := s.udpServer.ListenAndServe(); err != nil {
				errChan <- fmt.Errorf("UDP server failed: %w", err)
			}
		}()
	}

	// Start TCP server
	if s.cfg.Server.TCPEnabled {
		s.tcpServer = &dns.Server{
			Addr:    s.cfg.Server.ListenAddress,
			Net:     "tcp",
			Handler: dns.HandlerFunc(wrappedHandler.serveDNS),
		}

		go func() {
			s.logger.Info("Starting TCP DNS server", "address", s.cfg.Server.ListenAddress)
			if err := s.tcpServer.ListenAndServe(); err != nil {
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

// serveDNS handles DNS requests with logging and metrics
func (w *wrappedHandler) serveDNS(rw dns.ResponseWriter, r *dns.Msg) {
	startTime := time.Now()
	ctx := context.Background()

	// Extract query information
	var domain string
	var qtype uint16
	if len(r.Question) > 0 {
		domain = r.Question[0].Name
		qtype = r.Question[0].Qtype
	}

	clientIP := getClientIP(rw)

	// Log the query
	w.logger.Debug("DNS query received",
		"domain", domain,
		"type", dns.TypeToString[qtype],
		"client", clientIP,
	)

	// Record metrics
	if w.metrics != nil {
		w.metrics.DNSQueriesTotal.Add(ctx, 1)
		w.metrics.DNSQueriesByType.Add(ctx, 1)// attribute.String("type", dns.TypeToString[qtype]),

	}

	// Call the actual handler
	w.handler.ServeDNS(ctx, rw, r)

	// Record query duration
	duration := time.Since(startTime)
	if w.metrics != nil {
		w.metrics.DNSQueryDuration.Record(ctx, float64(duration.Milliseconds()))
	}

	w.logger.Debug("DNS query processed",
		"domain", domain,
		"duration_ms", duration.Milliseconds(),
	)
}

// getClientIP extracts the client IP from the DNS request
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
