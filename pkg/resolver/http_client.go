package resolver

import (
	"net/http"
	"time"
)

// NewHTTPClient creates an HTTP client that uses the custom DNS resolver
// for all hostname resolution, ensuring consistent DNS behavior across the application.
//
// Example:
//   resolver := resolver.New([]string{"1.1.1.1:53"}, logger)
//   client := resolver.NewHTTPClient(60 * time.Second)
func (r *Resolver) NewHTTPClient(timeout time.Duration) *http.Client {
	// If no upstreams configured, use default HTTP client
	if len(r.upstreams) == 0 {
		r.logger.Debug("Creating HTTP client with system default DNS resolver")
		return &http.Client{
			Timeout: timeout,
		}
	}

	r.logger.Debug("Creating HTTP client with custom DNS resolver",
		"upstream", r.upstreams[0],
		"timeout", timeout,
	)

	// Create transport with custom DNS resolution
	transport := &http.Transport{
		DialContext:           r.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}
}
