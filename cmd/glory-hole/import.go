package main

import (
	"archive/zip"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"glory-hole/pkg/config"
	"glory-hole/pkg/storage"

	"gopkg.in/yaml.v3"
	_ "modernc.org/sqlite"
)

// PiholeImporter handles importing Pi-hole configurations
type PiholeImporter struct {
	zipPath      string
	gravityDB    string
	piholeConfig string
	customList   string
	output       string
	tempDir      string
	dryRun       bool
	validate     bool
	merge        bool
}

// NewPiholeImporter creates a new Pi-hole importer
func NewPiholeImporter() *PiholeImporter {
	return &PiholeImporter{
		validate: true, // Validation enabled by default
	}
}

// Import performs the Pi-hole configuration import
func (i *PiholeImporter) Import() (*config.Config, error) {
	// Step 0: Extract ZIP if provided
	if i.zipPath != "" {
		fmt.Println("Extracting Pi-hole Teleporter backup...")
		if err := i.extractZip(); err != nil {
			return nil, fmt.Errorf("failed to extract ZIP: %w", err)
		}
		defer i.cleanup()
		fmt.Printf("  ✓ Extracted to: %s\n", i.tempDir)
		fmt.Printf("  ✓ Found gravity.db\n")
		if i.piholeConfig != "" {
			fmt.Printf("  ✓ Found pihole.toml\n")
		}
		if i.customList != "" {
			fmt.Printf("  ✓ Found custom.list\n")
		}
		fmt.Println()
	}

	// Verify required files
	if i.gravityDB == "" {
		return nil, fmt.Errorf("gravity.db not provided (use --zip or --gravity-db)")
	}

	if _, err := os.Stat(i.gravityDB); os.IsNotExist(err) {
		return nil, fmt.Errorf("gravity.db not found at: %s", i.gravityDB)
	}

	fmt.Println("Pi-hole Import Summary")
	fmt.Println("======================")
	fmt.Println()

	// Step 1: Import blocklists from gravity.db
	blocklists, err := i.importBlocklists()
	if err != nil {
		return nil, fmt.Errorf("failed to import blocklists: %w", err)
	}

	// Step 2: Import whitelist/blacklist from domainlist table
	whitelist, blacklist, err := i.importDomainLists()
	if err != nil {
		return nil, fmt.Errorf("failed to import domain lists: %w", err)
	}

	// Step 3: Import local DNS records (if custom.list provided)
	var localRecords []config.LocalRecordEntry
	if i.customList != "" {
		records, err := i.importLocalDNS()
		if err != nil {
			fmt.Printf("⚠ Warning: Failed to import custom.list: %v\n\n", err)
		} else {
			localRecords = records
		}
	} else {
		fmt.Println("Local DNS Records:")
		fmt.Println("  ⚠ No custom.list provided (skipped)")
		fmt.Println()
	}

	// Step 4: Import upstream DNS (if pihole.toml provided)
	var upstreams []string
	if i.piholeConfig != "" {
		ups, err := i.importUpstreams()
		if err != nil {
			fmt.Printf("⚠ Warning: Failed to import upstreams: %v\n\n", err)
		} else {
			upstreams = ups
		}
	} else {
		// Use default upstreams (Cloudflare and Google)
		upstreams = []string{"1.1.1.1:53", "8.8.8.8:53"}
		fmt.Println("Upstream DNS:")
		fmt.Println("  ⚠ No pihole.toml provided, using defaults:")
		fmt.Println("    - 1.1.1.1:53 (Cloudflare)")
		fmt.Println("    - 8.8.8.8:53 (Google)")
		fmt.Println()
	}

	// Build Glory-Hole configuration
	cfg := &config.Config{
		Server: config.ServerConfig{
			ListenAddress:   ":53",
			TCPEnabled:      true,
			UDPEnabled:      true,
			WebUIAddress:    ":8080",
			EnablePolicies:  true,
			EnableBlocklist: true,
		},
		Blocklists:           blocklists,
		Whitelist:            whitelist,
		UpdateInterval:       24 * time.Hour,
		AutoUpdateBlocklists: true,
		UpstreamDNSServers:   upstreams,
		Logging: config.LoggingConfig{
			Level:  "info",
			Format: "text",
			Output: "stdout",
		},
		Database: storage.Config{
			Enabled: true,
			Backend: "sqlite",
			SQLite: storage.SQLiteConfig{
				Path:        "./glory-hole.db",
				BusyTimeout: 5000,
				WALMode:     true,
			},
			RetentionDays: 91, // Match Pi-hole default
		},
		Cache: config.CacheConfig{
			Enabled:     true,
			MaxEntries:  10000,
			MinTTL:      60 * time.Second,
			MaxTTL:      24 * time.Hour,
			NegativeTTL: 5 * time.Minute,
		},
	}

	// Add local records if imported
	if len(localRecords) > 0 {
		cfg.LocalRecords.Enabled = true
		cfg.LocalRecords.Records = localRecords
	}

	// Note: Blacklist is not part of the config structure yet
	// This would require adding blacklist support to Glory-Hole
	if len(blacklist) > 0 {
		fmt.Printf("⚠ Note: Found %d blacklist entries (not yet supported by Glory-Hole)\n", len(blacklist))
		fmt.Println()
	}

	fmt.Println("Cleanup:")
	if i.tempDir != "" {
		fmt.Println("  ✓ Removed temporary files")
	}
	fmt.Println()

	// Validate if requested
	if i.validate {
		if err := cfg.Validate(); err != nil {
			return nil, fmt.Errorf("validation failed: %w", err)
		}
	}

	return cfg, nil
}

// extractZip extracts the Pi-hole Teleporter ZIP file
func (i *PiholeImporter) extractZip() error {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "pihole-import-*")
	if err != nil {
		return err
	}
	i.tempDir = tempDir

	// Open ZIP file
	r, err := zip.OpenReader(i.zipPath)
	if err != nil {
		return fmt.Errorf("failed to open ZIP: %w", err)
	}
	defer func() { _ = r.Close() }()

	// Extract all files
	for _, f := range r.File {
		if err := i.extractFile(f, tempDir); err != nil {
			return fmt.Errorf("failed to extract %s: %w", f.Name, err)
		}

		// Auto-detect file paths
		baseName := filepath.Base(f.Name)
		// #nosec G305 - Path is validated against tempDir immediately below to prevent traversal
		cleanedPath := filepath.Clean(filepath.Join(tempDir, f.Name))
		// Verify path is within tempDir
		if !strings.HasPrefix(cleanedPath, filepath.Clean(tempDir)+string(os.PathSeparator)) &&
			cleanedPath != filepath.Clean(tempDir) {
			continue // Skip files with invalid paths
		}

		switch {
		case strings.Contains(baseName, "gravity.db"):
			i.gravityDB = cleanedPath
		case strings.Contains(baseName, "pihole.toml"):
			i.piholeConfig = cleanedPath
		case strings.Contains(baseName, "custom.list"):
			i.customList = cleanedPath
		}
	}

	// Verify required files
	if i.gravityDB == "" {
		return fmt.Errorf("gravity.db not found in ZIP")
	}

	return nil
}

// extractFile extracts a single file from the ZIP archive
func (i *PiholeImporter) extractFile(f *zip.File, dest string) error {
	// Create destination path
	// #nosec G305 - Path is validated against dest directory immediately below to prevent traversal
	path := filepath.Join(dest, f.Name)

	// Validate path to prevent directory traversal (G305)
	cleanPath := filepath.Clean(path)
	if !strings.HasPrefix(cleanPath, filepath.Clean(dest)+string(os.PathSeparator)) &&
		cleanPath != filepath.Clean(dest) {
		return fmt.Errorf("invalid file path: %s", f.Name)
	}

	// Create directory if needed
	if f.FileInfo().IsDir() {
		return os.MkdirAll(cleanPath, f.Mode()&0750) // G301: Restrict directory permissions
	}

	// Create parent directories
	if err := os.MkdirAll(filepath.Dir(cleanPath), 0750); err != nil { // G301: Use 0750 instead of 0755
		return err
	}

	// Open source file
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()

	// Create destination file with safe permissions (G304)
	outFile, err := os.OpenFile(cleanPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode()&0640)
	if err != nil {
		return err
	}
	defer func() { _ = outFile.Close() }()

	// Copy contents with size limit to prevent decompression bombs (G110)
	// Pi-hole databases are typically <100MB, limit to 500MB for safety
	const maxSize = 500 * 1024 * 1024 // 500MB
	limitedReader := io.LimitReader(rc, maxSize)
	n, err := io.Copy(outFile, limitedReader)
	if err != nil {
		return err
	}

	// Check if we hit the limit
	if n == maxSize {
		// Check if there's more data
		buf := make([]byte, 1)
		if _, err := rc.Read(buf); err == nil {
			return fmt.Errorf("file too large (>500MB): %s", f.Name)
		}
	}

	return nil
}

// cleanup removes temporary directory
func (i *PiholeImporter) cleanup() {
	if i.tempDir != "" {
		_ = os.RemoveAll(i.tempDir)
	}
}

// importBlocklists imports blocklist URLs from gravity.db
func (i *PiholeImporter) importBlocklists() ([]string, error) {
	db, err := sql.Open("sqlite", i.gravityDB)
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()

	rows, err := db.Query(`
		SELECT address, enabled
		FROM adlist
		WHERE enabled = 1
		ORDER BY id
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var blocklists []string
	for rows.Next() {
		var address string
		var enabled int
		if scanErr := rows.Scan(&address, &enabled); scanErr != nil {
			return nil, scanErr
		}
		blocklists = append(blocklists, address)
	}

	// Calculate total domains (optional, can be slow)
	var totalDomains int
	err = db.QueryRow(`SELECT COUNT(*) FROM gravity`).Scan(&totalDomains)
	if err != nil {
		totalDomains = 0 // Non-fatal
	}

	fmt.Println("Blocklists:")
	fmt.Printf("  ✓ Found %d enabled blocklists\n", len(blocklists))
	if totalDomains > 0 {
		fmt.Printf("  ✓ Total domains: %s\n", formatNumber(totalDomains))
	}
	fmt.Println()

	return blocklists, nil
}

// importDomainLists imports whitelist/blacklist from domainlist table
func (i *PiholeImporter) importDomainLists() (whitelist []string, blacklist []string, err error) {
	db, err := sql.Open("sqlite", i.gravityDB)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = db.Close() }()

	// Pi-hole domain types:
	// 0 = whitelist exact
	// 1 = blacklist exact
	// 2 = whitelist regex
	// 3 = blacklist regex
	rows, err := db.Query(`
		SELECT type, domain, enabled
		FROM domainlist
		WHERE enabled = 1
		ORDER BY id
	`)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rows.Close() }()

	var exactWhitelist, regexWhitelist int
	var exactBlacklist, regexBlacklist int

	for rows.Next() {
		var domainType int
		var domain string
		var enabled int

		if err := rows.Scan(&domainType, &domain, &enabled); err != nil {
			return nil, nil, err
		}

		switch domainType {
		case 0: // Whitelist exact
			whitelist = append(whitelist, domain)
			exactWhitelist++
		case 1: // Blacklist exact
			blacklist = append(blacklist, domain)
			exactBlacklist++
		case 2: // Whitelist regex
			whitelist = append(whitelist, domain)
			regexWhitelist++
		case 3: // Blacklist regex
			blacklist = append(blacklist, domain)
			regexBlacklist++
		}
	}

	fmt.Println("Whitelist:")
	fmt.Printf("  ✓ Found %d patterns (%d exact, %d regex)\n",
		len(whitelist), exactWhitelist, regexWhitelist)
	fmt.Println()

	if len(blacklist) > 0 {
		fmt.Println("Blacklist:")
		fmt.Printf("  ✓ Found %d patterns (%d exact, %d regex)\n",
			len(blacklist), exactBlacklist, regexBlacklist)
		fmt.Println()
	}

	return whitelist, blacklist, nil
}

// importLocalDNS imports local DNS records from custom.list
func (i *PiholeImporter) importLocalDNS() ([]config.LocalRecordEntry, error) {
	data, err := os.ReadFile(i.customList)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(data), "\n")
	var records []config.LocalRecordEntry

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Format: IP domain
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		ip := parts[0]
		domain := parts[1]

		records = append(records, config.LocalRecordEntry{
			Domain: domain,
			Type:   "A",
			IPs:    []string{ip},
			TTL:    300,
		})
	}

	fmt.Println("Local DNS Records:")
	fmt.Printf("  ✓ Found %d custom DNS entries\n", len(records))
	fmt.Println()

	return records, nil
}

// importUpstreams imports upstream DNS servers from pihole.toml
func (i *PiholeImporter) importUpstreams() ([]string, error) {
	data, err := os.ReadFile(i.piholeConfig)
	if err != nil {
		return nil, err
	}

	// Simple TOML parsing for upstreams array
	lines := strings.Split(string(data), "\n")
	var upstreams []string
	inDNSSection := false
	inUpstreamsArray := false

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Check if we're in [dns] section
		if strings.HasPrefix(line, "[dns]") {
			inDNSSection = true
			continue
		}

		// Check if we left [dns] section
		if strings.HasPrefix(line, "[") && !strings.HasPrefix(line, "[dns") {
			inDNSSection = false
			inUpstreamsArray = false
			continue
		}

		if inDNSSection {
			// Look for upstreams = [
			if strings.Contains(line, "upstreams") && strings.Contains(line, "[") {
				inUpstreamsArray = true
				// Check if it's a single-line array
				if strings.Contains(line, "]") {
					// Extract upstreams from single line: upstreams = [ "1.1.1.1" ]
					start := strings.Index(line, "[")
					end := strings.Index(line, "]")
					if start != -1 && end != -1 {
						content := line[start+1 : end]
						for _, upstream := range strings.Split(content, ",") {
							upstream = strings.TrimSpace(upstream)
							upstream = strings.Trim(upstream, "\"'")
							if upstream != "" {
								// Add port if not specified
								if !strings.Contains(upstream, ":") {
									upstream += ":53"
								}
								upstreams = append(upstreams, upstream)
							}
						}
					}
					inUpstreamsArray = false
				}
				continue
			}

			// Multi-line array elements
			if inUpstreamsArray {
				if strings.Contains(line, "]") {
					inUpstreamsArray = false
					continue
				}
				// Extract quoted string
				line = strings.Trim(line, " \t,")
				line = strings.Trim(line, "\"'")
				if line != "" && !strings.HasPrefix(line, "#") {
					// Add port if not specified
					if !strings.Contains(line, ":") {
						line += ":53"
					}
					upstreams = append(upstreams, line)
				}
			}
		}
	}

	fmt.Println("Upstream DNS:")
	if len(upstreams) > 0 {
		fmt.Printf("  ✓ Found %d upstream(s)\n", len(upstreams))
		for _, upstream := range upstreams {
			fmt.Printf("    - %s\n", upstream)
		}
	} else {
		fmt.Println("  ⚠ No upstreams found in pihole.toml")
	}
	fmt.Println()

	return upstreams, nil
}

// WriteConfig writes the configuration to a file or stdout
func (i *PiholeImporter) WriteConfig(cfg *config.Config) error {
	if i.dryRun {
		fmt.Println("Dry run mode - configuration not written")
		return nil
	}

	// Marshal to YAML with custom header
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	// Prepare output with header
	var output strings.Builder
	output.WriteString("# Glory-Hole Configuration\n")
	output.WriteString("# Imported from Pi-hole\n")
	output.WriteString(fmt.Sprintf("# Import date: %s\n", time.Now().Format(time.RFC3339)))
	output.WriteString("\n")
	if i.zipPath != "" {
		output.WriteString(fmt.Sprintf("# Import source: %s\n", filepath.Base(i.zipPath)))
	} else {
		output.WriteString(fmt.Sprintf("# Import source: %s\n", i.gravityDB))
	}
	output.WriteString("\n")
	output.Write(data)

	// Write to file or stdout
	if i.output == "" || i.output == "-" {
		fmt.Print(output.String())
	} else {
		// G306: Use 0600 for config file (owner read/write only)
		if err := os.WriteFile(i.output, []byte(output.String()), 0600); err != nil {
			return err
		}
		fmt.Printf("Config written to: %s\n", i.output)
		fmt.Println()
	}

	// Print next steps
	fmt.Println("Next steps:")
	fmt.Println("  1. Review generated config.yml")
	fmt.Println("  2. Test with: glory-hole -config=config.yml")
	fmt.Println("  3. Deploy: sudo systemctl restart glory-hole")
	fmt.Println()

	return nil
}

// formatNumber formats a number with thousand separators
func formatNumber(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}

	var result strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result.WriteString(",")
		}
		result.WriteRune(c)
	}
	return result.String()
}
