// Package resolver centralizes outbound DNS resolution so other packages avoid
// relying on the host resolver.
package resolver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"glory-hole/pkg/logging"
)

// Resolver provides DNS resolution using configured upstream servers
// instead of the system's default resolver (/etc/resolv.conf).
// This ensures all components in the application use consistent DNS infrastructure.
type Resolver struct {
	logger    *logging.Logger
	dialer    *net.Dialer
	upstreams []string
	strict    bool // when true, never fall back to system resolver
}

// New creates a new DNS resolver that uses the specified upstream DNS servers.
// If upstreams is empty or nil, it falls back to the system's default resolver.
//
// Example:
//
//	resolver := resolver.New([]string{"1.1.1.1:53", "8.8.8.8:53"}, logger)
func New(upstreams []string, logger *logging.Logger) *Resolver {
	return newWithOptions(upstreams, logger, false)
}

// NewStrict creates a resolver that will NOT fall back to the system resolver when upstreams fail.
// This is useful for environments where the host resolver is blocked or untrusted (e.g., ACME/Cloudflare).
func NewStrict(upstreams []string, logger *logging.Logger) *Resolver {
	return newWithOptions(upstreams, logger, true)
}

func newWithOptions(upstreams []string, logger *logging.Logger, strict bool) *Resolver {
	if len(upstreams) == 0 {
		logger.Warn("No upstream DNS servers configured, using system default resolver")
	} else {
		logger.Info("DNS resolver initialized", "upstreams", upstreams, "strict", strict)
	}

	return &Resolver{
		upstreams: upstreams,
		logger:    logger,
		strict:    strict,
		dialer: &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		},
	}
}

// LookupIP resolves a hostname to IP addresses using configured upstream DNS servers.
// It tries each upstream server until one succeeds or all fail.
func (r *Resolver) LookupIP(ctx context.Context, network, host string) ([]net.IP, error) {
	// If no upstreams configured, use system default
	if len(r.upstreams) == 0 {
		return net.DefaultResolver.LookupIP(ctx, network, host)
	}

	var lastErr error
	for idx, upstream := range r.upstreams {
		// RFC 1035 §7.2 requires resolvers to retry alternate name servers on failure.
		netResolver := &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				return r.dialer.DialContext(ctx, "udp", upstream)
			},
		}

		ips, err := netResolver.LookupIP(ctx, network, host)
		if err != nil {
			lastErr = err
			r.logger.Warn("DNS resolution attempt failed",
				"host", host,
				"upstream", upstream,
				"attempt", idx+1,
				"error", err,
			)
			continue
		}

		r.logger.Debug("DNS resolution successful",
			"host", host,
			"upstream", upstream,
			"ips", ips,
		)
		return ips, nil
	}

	// All upstreams failed
	if r.strict && len(r.upstreams) > 0 {
		return nil, fmt.Errorf("failed to resolve %s via configured upstreams (strict mode): %w", host, lastErr)
	}

	r.logger.Warn("All upstream DNS servers failed, falling back to system resolver",
		"host", host,
		"attempts", len(r.upstreams),
		"error", lastErr,
	)
	ips, err := net.DefaultResolver.LookupIP(ctx, network, host)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve %s via configured upstreams: %w", host, errors.Join(lastErr, err))
	}
	return ips, nil
}

// DialContext dials a network address, resolving hostnames using configured upstream DNS.
// This is compatible with http.Transport.DialContext.
func (r *Resolver) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	// Split host:port
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("invalid address %s: %w", addr, err)
	}

	// If it's already an IP, dial directly
	if net.ParseIP(host) != nil {
		return r.dialer.DialContext(ctx, network, addr)
	}

	// Resolve hostname using configured upstream DNS
	ips, err := r.LookupIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}

	if len(ips) == 0 {
		return nil, fmt.Errorf("no IP addresses found for %s", host)
	}

	// Dial using first resolved IP
	resolvedAddr := net.JoinHostPort(ips[0].String(), port)
	return r.dialer.DialContext(ctx, network, resolvedAddr)
}

// Upstreams returns the configured upstream DNS servers
func (r *Resolver) Upstreams() []string {
	return r.upstreams
}
