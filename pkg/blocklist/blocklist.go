package blocklist

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"glory-hole/pkg/logging"

	"github.com/miekg/dns"
)

// Downloader downloads and parses blocklists
type Downloader struct {
	client  *http.Client
	logger  *logging.Logger
	timeout time.Duration
}

// NewDownloader creates a new blocklist downloader with a custom HTTP client.
// The HTTP client should be configured with appropriate DNS resolution (e.g., using pkg/resolver).
// If client is nil, a default HTTP client with 60s timeout will be created.
func NewDownloader(logger *logging.Logger, client *http.Client) *Downloader {
	if client == nil {
		logger.Warn("No HTTP client provided, using default client with system DNS resolver")
		client = &http.Client{
			Timeout: 60 * time.Second, // Long timeout for large files
		}
	}

	return &Downloader{
		client:  client,
		logger:  logger,
		timeout: 60 * time.Second,
	}
}

// Download downloads a blocklist from a URL and returns a map of blocked domains
func (d *Downloader) Download(ctx context.Context, url string) (map[string]struct{}, error) {
	d.logger.Info("Downloading blocklist", "url", url)
	startTime := time.Now()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download blocklist: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	domains, err := d.parseHostsFile(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse blocklist: %w", err)
	}

	elapsed := time.Since(startTime)
	d.logger.Info("Blocklist downloaded",
		"url", url,
		"domains", len(domains),
		"duration", elapsed)

	return domains, nil
}

// parseHostsFile parses a hosts file format blocklist
// Supports formats:
// - 0.0.0.0 domain.com
// - 127.0.0.1 domain.com
// - domain.com (plain list)
// - ||domain.com^ (adblock format)
func (d *Downloader) parseHostsFile(r io.Reader) (map[string]struct{}, error) {
	domains := make(map[string]struct{})
	scanner := bufio.NewScanner(r)
	lineCount := 0

	for scanner.Scan() {
		lineCount++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse the domain
		domain := d.extractDomain(line)
		if domain == "" {
			continue
		}

		// Ensure FQDN format (with trailing dot)
		if !strings.HasSuffix(domain, ".") {
			domain = dns.Fqdn(domain)
		}

		domains[domain] = struct{}{}

		// Log progress for large files
		if lineCount%100000 == 0 {
			d.logger.Debug("Parsing blocklist", "lines", lineCount, "domains", len(domains))
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading blocklist: %w", err)
	}

	return domains, nil
}

// extractDomain extracts a domain from various blocklist formats
func (d *Downloader) extractDomain(line string) string {
	// Adblock format: ||domain.com^
	if strings.HasPrefix(line, "||") && strings.Contains(line, "^") {
		domain := strings.TrimPrefix(line, "||")
		domain = strings.Split(domain, "^")[0]
		return strings.TrimSpace(domain)
	}

	// Hosts file format: 0.0.0.0 domain.com or 127.0.0.1 domain.com
	fields := strings.Fields(line)
	if len(fields) >= 2 {
		// Check if first field is an IP address
		if strings.Contains(fields[0], ".") || strings.Contains(fields[0], ":") {
			// It's likely "IP domain" format
			domain := fields[1]
			// Skip localhost entries
			if domain == "localhost" || domain == "localhost.localdomain" {
				return ""
			}
			return domain
		}
	}

	// Plain domain list format
	if len(fields) == 1 {
		domain := fields[0]
		// Skip localhost
		if domain == "localhost" || domain == "localhost.localdomain" {
			return ""
		}
		return domain
	}

	return ""
}

// DownloadAll downloads multiple blocklists and merges them
func (d *Downloader) DownloadAll(ctx context.Context, urls []string) (map[string]struct{}, error) {
	if len(urls) == 0 {
		return make(map[string]struct{}), nil
	}

	d.logger.Info("Downloading blocklists", "count", len(urls))
	startTime := time.Now()

	merged := make(map[string]struct{})

	for i, url := range urls {
		d.logger.Info("Downloading blocklist", "index", i+1, "total", len(urls), "url", url)

		domains, err := d.Download(ctx, url)
		if err != nil {
			d.logger.Error("Failed to download blocklist", "url", url, "error", err)
			// Continue with other lists even if one fails
			continue
		}

		// Merge domains
		for domain := range domains {
			merged[domain] = struct{}{}
		}
	}

	elapsed := time.Since(startTime)
	d.logger.Info("All blocklists downloaded",
		"total_domains", len(merged),
		"duration", elapsed)

	return merged, nil
}
