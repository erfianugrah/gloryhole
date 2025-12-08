package dns

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"glory-hole/pkg/config"
	"glory-hole/pkg/logging"

	"golang.org/x/crypto/acme/autocert"
)

// buildTLSConfig prepares TLS settings for DoT and optional ACME HTTP-01 challenge server.
// Returns (tlsConfig, acmeHTTPServer, error).
func buildTLSConfig(cfg *config.ServerConfig, logger *logging.Logger) (*tls.Config, *http.Server, error) {
	if cfg == nil || !cfg.DotEnabled {
		return nil, nil, nil
	}

	// Manual certificate path
	if cfg.TLS.CertFile != "" && cfg.TLS.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(cfg.TLS.CertFile, cfg.TLS.KeyFile)
		if err != nil {
			return nil, nil, fmt.Errorf("load x509 key pair: %w", err)
		}
		return &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
			NextProtos:   []string{"dot", "h2", "http/1.1"},
		}, nil, nil
	}

	// Autocert path
	if cfg.TLS.Autocert.Enabled {
		cacheDir := cfg.TLS.Autocert.CacheDir
		if cacheDir == "" {
			if usrCache, err := os.UserCacheDir(); err == nil {
				cacheDir = filepath.Join(usrCache, "gloryhole-autocert")
			} else {
				cacheDir = "./.cache/autocert"
			}
		}

		m := &autocert.Manager{
			Cache:      autocert.DirCache(cacheDir),
			Prompt:     autocert.AcceptTOS,
			Email:      cfg.TLS.Autocert.Email,
			HostPolicy: autocert.HostWhitelist(cfg.TLS.Autocert.Hosts...),
		}

		acmeHTTP := &http.Server{
			Addr:    cfg.TLS.Autocert.HTTP01Address,
			Handler: m.HTTPHandler(nil),
		}

		tlsCfg := &tls.Config{
			GetCertificate: m.GetCertificate,
			MinVersion:     tls.VersionTLS12,
			NextProtos:     []string{"dot", "h2", "http/1.1", "acme-tls/1"},
		}

		logger.Info("Autocert enabled for DoT", "hosts", cfg.TLS.Autocert.Hosts, "cache", cacheDir)
		return tlsCfg, acmeHTTP, nil
	}

	return nil, nil, fmt.Errorf("DoT enabled but no TLS configuration provided")
}
