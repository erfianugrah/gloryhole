package forwarder

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"glory-hole/pkg/config"
	"glory-hole/pkg/logging"

	"github.com/miekg/dns"
)

// Forwarder handles forwarding DNS queries to upstream servers
type Forwarder struct {
	upstreams []string
	index     atomic.Uint32
	timeout   time.Duration
	retries   int
	logger    *logging.Logger

	// Connection pool
	clientPool sync.Pool
}

// NewForwarder creates a new DNS forwarder
func NewForwarder(cfg *config.Config, logger *logging.Logger) *Forwarder {
	if len(cfg.UpstreamDNSServers) == 0 {
		// Default to Cloudflare and Google DNS
		cfg.UpstreamDNSServers = []string{"1.1.1.1:53", "8.8.8.8:53"}
	}

	// Normalize upstream addresses (add :53 if port is missing)
	upstreams := make([]string, len(cfg.UpstreamDNSServers))
	for i, upstream := range cfg.UpstreamDNSServers {
		if _, _, err := net.SplitHostPort(upstream); err != nil {
			// No port specified, add default DNS port
			upstreams[i] = net.JoinHostPort(upstream, "53")
		} else {
			upstreams[i] = upstream
		}
	}

	f := &Forwarder{
		upstreams: upstreams,
		timeout:   2 * time.Second, // Default 2 second timeout
		retries:   2,               // Try up to 2 different upstreams
		logger:    logger,
	}

	// Initialize connection pool
	f.clientPool.New = func() any {
		return &dns.Client{
			Net:     "udp",
			Timeout: f.timeout,
		}
	}

	logger.Info("Forwarder initialized",
		"upstreams", upstreams,
		"timeout", f.timeout,
		"retries", f.retries,
	)

	return f
}

// Forward forwards a DNS query to upstream servers
func (f *Forwarder) Forward(ctx context.Context, r *dns.Msg) (*dns.Msg, error) {
	if len(f.upstreams) == 0 {
		return nil, fmt.Errorf("no upstream DNS servers configured")
	}

	// Try multiple upstreams with round-robin selection
	attempts := min(f.retries, len(f.upstreams))
	var lastErr error

	for i := 0; i < attempts; i++ {
		// Select upstream using round-robin
		upstream := f.selectUpstream()

		// Get client from pool
		client := f.clientPool.Get().(*dns.Client)
		defer f.clientPool.Put(client)

		// Log the forward attempt
		f.logger.Debug("Forwarding DNS query",
			"domain", r.Question[0].Name,
			"type", dns.TypeToString[r.Question[0].Qtype],
			"upstream", upstream,
			"attempt", i+1,
		)

		// Forward the query
		resp, rtt, err := client.ExchangeContext(ctx, r, upstream)
		if err != nil {
			f.logger.Warn("Upstream query failed",
				"upstream", upstream,
				"error", err,
				"attempt", i+1,
			)
			lastErr = err
			continue
		}

		// Check if response is valid
		if resp == nil {
			lastErr = fmt.Errorf("received nil response from %s", upstream)
			continue
		}

		if resp.Rcode == dns.RcodeServerFailure {
			f.logger.Warn("Upstream returned SERVFAIL",
				"upstream", upstream,
				"domain", r.Question[0].Name,
			)
			lastErr = fmt.Errorf("upstream %s returned SERVFAIL", upstream)
			continue
		}

		// Success!
		f.logger.Debug("Upstream query succeeded",
			"upstream", upstream,
			"domain", r.Question[0].Name,
			"rtt", rtt,
			"answers", len(resp.Answer),
		)

		return resp, nil
	}

	// All attempts failed
	if lastErr != nil {
		return nil, fmt.Errorf("all upstream servers failed: %w", lastErr)
	}
	return nil, fmt.Errorf("all upstream servers failed")
}

// ForwardTCP forwards a DNS query using TCP
func (f *Forwarder) ForwardTCP(ctx context.Context, r *dns.Msg) (*dns.Msg, error) {
	if len(f.upstreams) == 0 {
		return nil, fmt.Errorf("no upstream DNS servers configured")
	}

	// Try multiple upstreams
	attempts := min(f.retries, len(f.upstreams))
	var lastErr error

	for i := 0; i < attempts; i++ {
		upstream := f.selectUpstream()

		// Create TCP client
		client := &dns.Client{
			Net:     "tcp",
			Timeout: f.timeout,
		}

		f.logger.Debug("Forwarding DNS query via TCP",
			"domain", r.Question[0].Name,
			"upstream", upstream,
			"attempt", i+1,
		)

		resp, rtt, err := client.ExchangeContext(ctx, r, upstream)
		if err != nil {
			f.logger.Warn("TCP upstream query failed",
				"upstream", upstream,
				"error", err,
			)
			lastErr = err
			continue
		}

		if resp == nil {
			lastErr = fmt.Errorf("received nil response from %s", upstream)
			continue
		}

		if resp.Rcode == dns.RcodeServerFailure {
			lastErr = fmt.Errorf("upstream %s returned SERVFAIL", upstream)
			continue
		}

		f.logger.Debug("TCP upstream query succeeded",
			"upstream", upstream,
			"rtt", rtt,
		)

		return resp, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("all TCP upstream servers failed: %w", lastErr)
	}
	return nil, fmt.Errorf("all TCP upstream servers failed")
}

// selectUpstream selects the next upstream server using round-robin
func (f *Forwarder) selectUpstream() string {
	idx := f.index.Add(1) % uint32(len(f.upstreams))
	return f.upstreams[idx]
}

// SetTimeout sets the query timeout duration
func (f *Forwarder) SetTimeout(timeout time.Duration) {
	f.timeout = timeout
}

// SetRetries sets the number of retry attempts
func (f *Forwarder) SetRetries(retries int) {
	f.retries = retries
}

// Upstreams returns the list of configured upstream servers
func (f *Forwarder) Upstreams() []string {
	return f.upstreams
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
