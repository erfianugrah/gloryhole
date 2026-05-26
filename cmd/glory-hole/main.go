package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"glory-hole/pkg/api"
	"glory-hole/pkg/blocklist"
	"glory-hole/pkg/cache"
	"glory-hole/pkg/config"
	"glory-hole/pkg/dns"
	"glory-hole/pkg/forwarder"
	"glory-hole/pkg/localrecords"
	"glory-hole/pkg/logging"
	"glory-hole/pkg/policy"
	"glory-hole/pkg/resolver"
	"glory-hole/pkg/storage"
	"glory-hole/pkg/telemetry"
	"glory-hole/pkg/unbound"

	"golang.org/x/crypto/bcrypt"
)

var (
	configPath     = flag.String("config", "config.yml", "Path to configuration file")
	showVersion    = flag.Bool("version", false, "Show version information and exit")
	validateConfig = flag.Bool("validate-config", false, "Validate configuration file and exit")
	healthCheck    = flag.Bool("health-check", false, "Perform health check and exit (for Docker HEALTHCHECK)")
	apiAddress     = flag.String("api-address", "", "Override API address for health check (default: from config)")

	// Build-time variables set via ldflags
	// Example: go build -ldflags "-X main.version=$(git describe --tags) -X main.buildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
	version   = "dev"     // Set via -ldflags "-X main.version=x.y.z"
	buildTime = "unknown" // Set via -ldflags "-X main.buildTime=$(date)"
	gitCommit = "unknown" // Set via -ldflags "-X main.gitCommit=$(git rev-parse --short HEAD)"
)

// whitelistMigratedSentinel is the dynamic_config key set after the one-shot
// whitelist → policy migration runs successfully. Presence (any non-empty
// value) means: do NOT re-run, even if the YAML still has whitelist entries
// (those will be cleaned up by the YAML-write step on first run).
const whitelistMigratedSentinel = "whitelist_migrated_at"

// conditionalForwardingMigratedSentinel mirrors whitelistMigratedSentinel for
// the v0.26 Conditional Forwarding → Policy FORWARD migration. Same three
// guards (sentinel, UNIQUE constraint, YAML persist) apply.
const conditionalForwardingMigratedSentinel = "conditional_forwarding_migrated_at"

// conditionalForwardingPolicyBand is the sort_order base for migrated CF rules.
// Placed at 1000+ so they sort after any hand-curated low-numbered policies
// the user added directly to the policy engine. Within the band, original
// CF priority is preserved by inverting (CF Priority DESC → SortOrder ASC).
const conditionalForwardingPolicyBand = 1000

// migrateWhitelistToPolicies converts whitelist entries to ALLOW policies.
// Idempotent: checks dynamic_config sentinel + relies on UNIQUE(name) index
// on policy_rules (migration v15) + persists cfg.Whitelist = nil back to YAML.
//
// Three independent guards prevent the v0.25-and-earlier duplicate-row
// accumulation bug:
//   1. Sentinel skip — if dynamic_config[whitelist_migrated_at] is set, return.
//   2. UNIQUE constraint — even if sentinel was somehow lost, duplicate inserts
//      fail at the DB layer.
//   3. YAML persist — cfg.Whitelist = nil is written back to disk so the
//      source-of-truth for next boot is empty.
func migrateWhitelistToPolicies(cfg *config.Config, stor storage.Storage, configPath string, logger *logging.Logger) bool {
	if len(cfg.Whitelist) == 0 {
		return false
	}

	ctx := context.Background()

	// Guard 1: sentinel check. Already ran — the YAML still has entries because
	// either an old binary couldn't write back, the volume was restored from a
	// backup, or the user manually re-added a whitelist: block. Either way:
	// log the entries we're skipping (for human triage) and bail.
	if stor != nil {
		if marker, err := stor.GetDynamicConfig(ctx, whitelistMigratedSentinel); err == nil && marker != "" {
			logger.Warn("Whitelist migration already ran; YAML still contains whitelist entries (likely YAML edited or restored from backup). Skipping re-migration; remove the whitelist: block from the YAML to silence this warning.",
				"sentinel_set_at", marker,
				"yaml_entry_count", len(cfg.Whitelist),
			)
			// Still nil out in-memory so the now-deprecated field doesn't leak
			// into API responses; YAML is the user's problem to clean up.
			cfg.Whitelist = nil
			return false
		}
	}

	logger.Info("Migrating whitelist entries to policies", "count", len(cfg.Whitelist))

	var migratedCount int
	var duplicateSkipped int

	for i, entry := range cfg.Whitelist {
		var logic string
		var name string

		if strings.HasPrefix(entry, "*.") {
			baseDomain := strings.TrimPrefix(entry, "*.")
			logic = fmt.Sprintf(`Domain == "%s" || DomainEndsWith(Domain, ".%s")`, baseDomain, baseDomain)
			name = fmt.Sprintf("Allow *.%s (migrated)", baseDomain)
		} else if strings.ContainsAny(entry, "()[]{}^$|\\+?") {
			logic = fmt.Sprintf(`DomainMatches(Domain, "%s")`, entry)
			name = fmt.Sprintf("Allow pattern %s (migrated)", entry)
		} else {
			logic = fmt.Sprintf(`Domain == "%s"`, entry)
			name = fmt.Sprintf("Allow %s (migrated)", entry)
		}

		if stor != nil {
			// Guard 2: UNIQUE(name) on policy_rules (migration v15) makes this
			// insert fail safely on duplicate — we count + skip rather than abort.
			_, err := stor.CreatePolicyRule(ctx, &storage.PolicyRule{
				Name:      name,
				Logic:     logic,
				Action:    "ALLOW",
				Enabled:   true,
				SortOrder: i,
			})
			if err != nil {
				if strings.Contains(err.Error(), "UNIQUE constraint failed") {
					duplicateSkipped++
					continue
				}
				logger.Error("Failed to migrate whitelist entry to DB",
					"entry", entry, "error", err)
				continue
			}
		} else {
			// No storage — append to config for YAML seed path
			cfg.Policy.Rules = append(cfg.Policy.Rules, config.PolicyRuleEntry{
				Name:    name,
				Logic:   logic,
				Action:  "ALLOW",
				Enabled: true,
			})
		}
		migratedCount++
	}

	cfg.Policy.Enabled = true
	cfg.Whitelist = nil

	// Guard 3: persist YAML so source-of-truth doesn't keep triggering migration.
	// Best-effort — if the file is read-only (e.g. baked into image), the sentinel
	// alone keeps idempotency.
	if configPath != "" {
		if err := config.Save(configPath, cfg); err != nil {
			logger.Warn("Failed to persist YAML after whitelist migration; sentinel + UNIQUE index still prevent duplicates on next boot.",
				"path", configPath, "error", err,
			)
		} else {
			logger.Info("Persisted YAML after whitelist migration", "path", configPath)
		}
	}

	// Set sentinel last — only if we got far enough to attempt all entries.
	if stor != nil {
		if err := stor.SetDynamicConfig(ctx, whitelistMigratedSentinel, time.Now().UTC().Format(time.RFC3339)); err != nil {
			logger.Warn("Failed to set whitelist migration sentinel; UNIQUE index still prevents duplicates on next boot.",
				"error", err,
			)
		}
	}

	logger.Info("Whitelist migration complete",
		"migrated", migratedCount,
		"duplicate_skipped", duplicateSkipped,
	)

	return migratedCount > 0
}

// migrateConditionalForwardingToPolicies converts cfg.ConditionalForwarding.Rules
// into Policy FORWARD rules in SQLite. v0.26 deprecation step — the
// conditional-forwarding code path is removed in v0.27.
//
// Same three idempotency guards as migrateWhitelistToPolicies:
//
//  1. Sentinel skip: dynamic_config[conditional_forwarding_migrated_at]
//  2. UNIQUE(name) on policy_rules (migration v15) catches duplicates
//  3. YAML persist (cfg.ConditionalForwarding.Rules = nil written back to disk)
//
// Each CF rule's matchers (Domains / ClientCIDRs / QueryTypes) AND-join into
// a single Policy DSL expression. Priority direction is inverted to match
// the policy engine's sort_order ASC convention.
func migrateConditionalForwardingToPolicies(cfg *config.Config, stor storage.Storage, configPath string, logger *logging.Logger) bool {
	if !cfg.ConditionalForwarding.Enabled || len(cfg.ConditionalForwarding.Rules) == 0 {
		return false
	}

	ctx := context.Background()

	// Guard 1: sentinel
	if stor != nil {
		if marker, err := stor.GetDynamicConfig(ctx, conditionalForwardingMigratedSentinel); err == nil && marker != "" {
			logger.Warn("Conditional forwarding migration already ran; YAML still contains rules. Remove the conditional_forwarding: block from the YAML to silence this warning.",
				"sentinel_set_at", marker,
				"yaml_rule_count", len(cfg.ConditionalForwarding.Rules),
			)
			// Drain in-memory so deprecated rules don't show up at /api/conditionalforwarding
			cfg.ConditionalForwarding.Rules = nil
			cfg.ConditionalForwarding.Enabled = false
			return false
		}
	}

	logger.Info("Migrating conditional forwarding rules to policies",
		"count", len(cfg.ConditionalForwarding.Rules))

	var migratedCount, duplicateSkipped, invalidSkipped int

	for _, rule := range cfg.ConditionalForwarding.Rules {
		if !rule.Enabled {
			continue
		}
		logic := buildPolicyLogicFromCFRule(rule)
		if logic == "" {
			logger.Warn("Conditional forwarding rule has no matchers — skipping (would silently match all queries)",
				"rule", rule.Name)
			invalidSkipped++
			continue
		}
		if len(rule.Upstreams) == 0 {
			logger.Warn("Conditional forwarding rule has no upstreams — skipping",
				"rule", rule.Name)
			invalidSkipped++
			continue
		}

		name := fmt.Sprintf("Forward %s (migrated)", rule.Name)
		actionData := strings.Join(rule.Upstreams, ",")

		// Priority direction inversion: CF uses Priority DESC range 1-100,
		// policy uses sort_order ASC. Invert + offset into the migrated band.
		prio := rule.Priority
		if prio == 0 {
			prio = 50 // CF default
		}
		sortOrder := conditionalForwardingPolicyBand + (100 - prio)

		if stor != nil {
			_, err := stor.CreatePolicyRule(ctx, &storage.PolicyRule{
				Name:       name,
				Logic:      logic,
				Action:     "FORWARD",
				ActionData: actionData,
				Enabled:    true,
				SortOrder:  sortOrder,
			})
			if err != nil {
				if strings.Contains(err.Error(), "UNIQUE constraint failed") {
					duplicateSkipped++
					continue
				}
				logger.Error("Failed to migrate conditional forwarding rule",
					"rule", rule.Name, "error", err)
				continue
			}
		} else {
			cfg.Policy.Rules = append(cfg.Policy.Rules, config.PolicyRuleEntry{
				Name:       name,
				Logic:      logic,
				Action:     "FORWARD",
				ActionData: actionData,
				Enabled:    true,
			})
		}
		migratedCount++
	}

	cfg.Policy.Enabled = true
	cfg.ConditionalForwarding.Rules = nil
	cfg.ConditionalForwarding.Enabled = false

	// Guard 3: YAML persist
	if configPath != "" {
		if err := config.Save(configPath, cfg); err != nil {
			logger.Warn("Failed to persist YAML after conditional forwarding migration; sentinel + UNIQUE index still prevent duplicates on next boot.",
				"path", configPath, "error", err)
		} else {
			logger.Info("Persisted YAML after conditional forwarding migration", "path", configPath)
		}
	}

	if stor != nil {
		if err := stor.SetDynamicConfig(ctx, conditionalForwardingMigratedSentinel, time.Now().UTC().Format(time.RFC3339)); err != nil {
			logger.Warn("Failed to set conditional forwarding migration sentinel", "error", err)
		}
	}

	logger.Info("Conditional forwarding migration complete",
		"migrated", migratedCount,
		"duplicate_skipped", duplicateSkipped,
		"invalid_skipped", invalidSkipped,
	)

	return migratedCount > 0
}

// buildPolicyLogicFromCFRule synthesizes a Policy DSL expression from a CF
// rule's matchers. Returns "" when the rule has zero matchers (caller should
// reject this — a no-matcher rule would expand silently to "match all").
func buildPolicyLogicFromCFRule(rule config.ForwardingRule) string {
	var parts []string

	if len(rule.Domains) > 0 {
		domainParts := make([]string, 0, len(rule.Domains))
		for _, d := range rule.Domains {
			switch {
			case strings.HasPrefix(d, "*."):
				base := strings.TrimPrefix(d, "*.")
				domainParts = append(domainParts, fmt.Sprintf(`(Domain == %q || DomainEndsWith(Domain, %q))`, base, "."+base))
			case strings.ContainsAny(d, "()[]{}^$|\\+?"):
				domainParts = append(domainParts, fmt.Sprintf(`DomainMatches(Domain, %q)`, d))
			default:
				domainParts = append(domainParts, fmt.Sprintf(`Domain == %q`, d))
			}
		}
		if len(domainParts) == 1 {
			parts = append(parts, domainParts[0])
		} else {
			parts = append(parts, "("+strings.Join(domainParts, " || ")+")")
		}
	}

	if len(rule.ClientCIDRs) > 0 {
		cidrParts := make([]string, 0, len(rule.ClientCIDRs))
		for _, c := range rule.ClientCIDRs {
			cidrParts = append(cidrParts, fmt.Sprintf(`IPInCIDR(ClientIP, %q)`, c))
		}
		if len(cidrParts) == 1 {
			parts = append(parts, cidrParts[0])
		} else {
			parts = append(parts, "("+strings.Join(cidrParts, " || ")+")")
		}
	}

	if len(rule.QueryTypes) > 0 {
		qts := make([]string, 0, len(rule.QueryTypes))
		for _, q := range rule.QueryTypes {
			qts = append(qts, fmt.Sprintf("%q", q))
		}
		parts = append(parts, fmt.Sprintf(`QueryTypeIn(QueryType, %s)`, strings.Join(qts, ", ")))
	}

	return strings.Join(parts, " && ")
}

func main() {
	// Check for subcommands before flag parsing
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "import-pihole":
			runImportPihole(os.Args[2:])
			return
		case "hash-password":
			runHashPassword(os.Args[2:])
			return
		}
	}

	flag.Parse()

	// Handle --version flag
	if *showVersion {
		fmt.Printf("Glory-Hole DNS Server\n")
		fmt.Printf("Version:     %s\n", version)
		fmt.Printf("Git Commit:  %s\n", gitCommit)
		fmt.Printf("Build Time:  %s\n", buildTime)
		fmt.Printf("Go Version:  %s\n", runtime.Version())
		os.Exit(0)
	}

	// Handle --validate-config flag
	if *validateConfig {
		if _, err := config.Load(*configPath); err != nil {
			fmt.Fprintf(os.Stderr, "Configuration invalid: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Configuration valid.")
		return
	}

	// Handle --health-check flag
	if *healthCheck {
		os.Exit(performHealthCheck(*apiAddress, *configPath))
	}

	// Create context for application lifecycle
	ctx := context.Background()

	// Initialize config watcher for hot-reload support
	cfgWatcher, err := config.NewWatcher(*configPath, nil) // Logger set after initialization
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize config watcher: %v\n", err)
		os.Exit(1)
	}
	cfg := cfgWatcher.Config()

	// Initialize logger
	logger, err := logging.New(&cfg.Logging)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	logging.SetGlobal(logger)

	// Update watcher with logger
	cfgWatcher, err = config.NewWatcher(*configPath, logger.Logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to reinitialize config watcher with logger: %v\n", err)
		os.Exit(1)
	}
	cfg = cfgWatcher.Config()

	// Warn if config file is world-readable (may contain secrets)
	if info, statErr := os.Stat(*configPath); statErr == nil {
		mode := info.Mode().Perm()
		if mode&0o044 != 0 {
			logger.Warn("Config file is readable by group/others — consider chmod 600",
				"path", *configPath, "mode", fmt.Sprintf("%04o", mode))
		}
	}

	// Note: OnChange callback will be set after components are created
	// This allows the callback to update components when config changes

	// Start config watcher in background
	watcherCtx, watcherCancel := context.WithCancel(ctx)
	defer watcherCancel()

	go func() {
		if watcherErr := cfgWatcher.Start(watcherCtx); watcherErr != nil {
			logger.Error("Config watcher stopped", "error", watcherErr)
		}
	}()

	logger.Info("Glory Hole DNS starting",
		"version", version,
		"build_time", buildTime,
	)

	// Always use the build-time version for telemetry, not the config file value.
	// This ensures Prometheus/OTel labels match the actual binary version.
	cfg.Telemetry.ServiceVersion = version

	// Initialize telemetry
	telem, err := telemetry.New(ctx, &cfg.Telemetry, logger)
	if err != nil {
		logger.Error("Failed to initialize telemetry", "error", err)
		os.Exit(1)
	}

	// Initialize metrics
	metrics, err := telem.InitMetrics()
	if err != nil {
		logger.Error("Failed to initialize metrics", "error", err)
		os.Exit(1)
	}

	// Initialize DNS resolver with configured upstream servers
	// This ensures all HTTP clients use consistent DNS resolution
	dnsResolver := resolver.New(cfg.UpstreamDNSServers, logger)

	// Create HTTP client with custom DNS resolver for blocklist downloads
	// This prevents blocklist downloads from using system DNS (/etc/resolv.conf)
	httpClient := dnsResolver.NewHTTPClient(60 * time.Second)

	// Create DNS handler
	handler := dns.NewHandler()
	handler.SetDecisionTrace(cfg.Server.DecisionTrace)
	if cfg.BlockPage.Enabled && cfg.BlockPage.BlockIP != "" {
		handler.SetBlockPageIP(cfg.BlockPage.BlockIP)
		logger.Info("Block page enabled", "block_ip", cfg.BlockPage.BlockIP)
	}

	// Set config watcher for kill-switch feature (hot-reload access)
	handler.SetConfigWatcher(cfgWatcher)

	// Initialize blocklist manager (create early so handler can reference it,
	// but defer download until after Unbound is ready to avoid DNS resolution failures)
	var blocklistMgr *blocklist.Manager
	var dnsCache cache.Interface
	if len(cfg.Blocklists) > 0 {
		logger.Info("Initializing blocklist manager", "sources", len(cfg.Blocklists))
		blocklistMgr = blocklist.NewManager(cfg, logger, metrics, httpClient)
		blocklistMgr.UpdateConfig(cfg)
		handler.SetBlocklistManager(blocklistMgr)
		// Download deferred to after Unbound startup (see below)
	}

	// Initialize storage (database for query logging)
	// Must happen before whitelist migration since it writes to SQLite.
	var stor storage.Storage
	if cfg.Database.Enabled {
		logger.Info("Initializing storage",
			"backend", cfg.Database.Backend,
			"path", cfg.Database.SQLite.Path,
		)
		stor, err = storage.New(&cfg.Database, metrics)
		if err != nil {
			logger.Error("Failed to initialize storage", "error", err)
			// Continue anyway - server can run without query logging
		} else {
			handler.SetStorage(stor)
			logger.Info("Storage initialized successfully",
				"backend", cfg.Database.Backend,
				"buffer_size", cfg.Database.BufferSize,
				"retention_days", cfg.Database.RetentionDays,
			)

			// Start retention cleanup goroutine
			if cfg.Database.RetentionDays > 0 {
				retentionDays := cfg.Database.RetentionDays
				go func() {
					// Run immediately on startup, then every hour
					ticker := time.NewTicker(1 * time.Hour)
					defer ticker.Stop()
					for {
						cutoff := time.Now().AddDate(0, 0, -retentionDays)
						cleanupCtx, cleanupCancel := context.WithTimeout(ctx, 5*time.Minute)
						if cleanupErr := stor.Cleanup(cleanupCtx, cutoff); cleanupErr != nil {
							logger.Error("Retention cleanup failed", "error", cleanupErr, "retention_days", retentionDays)
						} else {
							logger.Debug("Retention cleanup completed", "cutoff", cutoff.Format(time.RFC3339))
						}
						cleanupCancel()

						select {
						case <-ticker.C:
						case <-ctx.Done():
							return
						}
					}
				}()
				logger.Info("Retention cleanup scheduled", "retention_days", retentionDays, "interval", "1h")
			}

			// Initialize query logger worker pool (if enabled)
			if cfg.Server.QueryLogger.Enabled || (cfg.Server.QueryLogger.BufferSize == 0 && cfg.Server.QueryLogger.Workers == 0) {
				// Apply defaults if not configured
				bufferSize := cfg.Server.QueryLogger.BufferSize
				if bufferSize == 0 {
					bufferSize = 5000 // Default: 5K queries — sufficient for typical home/small-office traffic
				}
				workers := cfg.Server.QueryLogger.Workers
				if workers == 0 {
					workers = 2 // Default: 2 workers — sufficient for single-core instances
				}

				queryLogger := dns.NewQueryLogger(stor, logger, bufferSize, workers)
				handler.SetQueryLogger(queryLogger)

				// Register cleanup on shutdown
				defer func() {
					if queryLogger != nil {
						logger.Info("Shutting down query logger")
						if closeErr := queryLogger.Close(); closeErr != nil {
							logger.Error("Failed to close query logger", "error", closeErr)
						}
					}
				}()

				logger.Info("Query logger worker pool initialized",
					"buffer_size", bufferSize,
					"workers", workers)
			}
		}
	}

	// Migrate whitelist entries to policies (one-time migration)
	if migrateWhitelistToPolicies(cfg, stor, *configPath, logger) {
		logger.Info("Whitelist migration complete — entries converted to ALLOW policies")
	}

	// Migrate conditional forwarding rules to policy FORWARD rules (v0.26 deprecation)
	if migrateConditionalForwardingToPolicies(cfg, stor, *configPath, logger) {
		logger.Info("Conditional forwarding migration complete — rules converted to FORWARD policies (use /api/policies)")
	}

	// Initialize local DNS records if configured
	if cfg.LocalRecords.Enabled && len(cfg.LocalRecords.Records) > 0 {
		logger.Info("Initializing local DNS records", "count", len(cfg.LocalRecords.Records))
		localMgr := localrecords.NewManager()

		for _, entry := range cfg.LocalRecords.Records {
			var record *localrecords.LocalRecord

			switch entry.Type {
			case "A":
				// Parse IPs and create A record
				if len(entry.IPs) == 0 {
					logger.Error("A record has no IPs", "domain", entry.Domain)
					continue
				}

				ips := make([]net.IP, 0, len(entry.IPs))
				for _, ipStr := range entry.IPs {
					ip := net.ParseIP(ipStr)
					if ip == nil || ip.To4() == nil {
						logger.Error("Invalid IPv4 address", "domain", entry.Domain, "ip", ipStr)
						continue
					}
					ips = append(ips, ip.To4())
				}

				if len(ips) == 0 {
					logger.Error("A record has no valid IPs", "domain", entry.Domain)
					continue
				}

				record = localrecords.NewARecord(entry.Domain, ips[0])
				if len(ips) > 1 {
					record.IPs = ips
				}

			case "AAAA":
				// Parse IPs and create AAAA record
				if len(entry.IPs) == 0 {
					logger.Error("AAAA record has no IPs", "domain", entry.Domain)
					continue
				}

				ips := make([]net.IP, 0, len(entry.IPs))
				for _, ipStr := range entry.IPs {
					ip := net.ParseIP(ipStr)
					if ip == nil || ip.To4() != nil {
						logger.Error("Invalid IPv6 address", "domain", entry.Domain, "ip", ipStr)
						continue
					}
					ips = append(ips, ip.To16())
				}

				if len(ips) == 0 {
					logger.Error("AAAA record has no valid IPs", "domain", entry.Domain)
					continue
				}

				record = localrecords.NewAAAARecord(entry.Domain, ips[0])
				if len(ips) > 1 {
					record.IPs = ips
				}

			case "CNAME":
				// Create CNAME record
				if entry.Target == "" {
					logger.Error("CNAME record has no target", "domain", entry.Domain)
					continue
				}
				record = localrecords.NewCNAMERecord(entry.Domain, entry.Target)

			case "TXT":
				// Create TXT record
				if len(entry.TxtRecords) == 0 {
					logger.Error("TXT record has no text data", "domain", entry.Domain)
					continue
				}
				record = localrecords.NewLocalRecord(entry.Domain, localrecords.RecordTypeTXT)
				record.TxtRecords = entry.TxtRecords

			case "MX":
				// Create MX record
				if entry.Target == "" {
					logger.Error("MX record has no target", "domain", entry.Domain)
					continue
				}
				var priority uint16 = 10 // Default priority
				if entry.Priority != nil {
					priority = *entry.Priority
				}
				record = localrecords.NewMXRecord(entry.Domain, entry.Target, priority)
			case "PTR":
				// Create PTR record
				if entry.Target == "" {
					logger.Error("PTR record has no target", "domain", entry.Domain)
					continue
				}
				record = localrecords.NewPTRRecord(entry.Domain, entry.Target)
			case "SRV":
				// Create SRV record
				if entry.Target == "" {
					logger.Error("SRV record has no target", "domain", entry.Domain)
					continue
				}
				if entry.Port == nil || *entry.Port == 0 {
					logger.Error("SRV record requires port", "domain", entry.Domain)
					continue
				}
				var priority uint16 = 0
				if entry.Priority != nil {
					priority = *entry.Priority
				}
				var weight uint16 = 0
				if entry.Weight != nil {
					weight = *entry.Weight
				}
				record = localrecords.NewSRVRecord(entry.Domain, entry.Target, priority, weight, *entry.Port)

			case "NS":
				// Create NS record
				if entry.Target == "" {
					logger.Error("NS record has no target", "domain", entry.Domain)
					continue
				}
				record = localrecords.NewNSRecord(entry.Domain, entry.Target)

			case "SOA":
				// Create SOA record
				if entry.Ns == "" || entry.Mbox == "" {
					logger.Error("SOA record requires ns and mbox fields", "domain", entry.Domain)
					continue
				}
				// Use defaults for optional fields if not specified
				var serial uint32 = 1
				if entry.Serial != nil {
					serial = *entry.Serial
				}
				var refresh uint32 = 3600 // 1 hour
				if entry.Refresh != nil {
					refresh = *entry.Refresh
				}
				var retry uint32 = 600 // 10 minutes
				if entry.Retry != nil {
					retry = *entry.Retry
				}
				var expire uint32 = 86400 // 1 day
				if entry.Expire != nil {
					expire = *entry.Expire
				}
				var minttl uint32 = 300 // 5 minutes
				if entry.Minttl != nil {
					minttl = *entry.Minttl
				}
				record = localrecords.NewSOARecord(entry.Domain, entry.Ns, entry.Mbox, serial, refresh, retry, expire, minttl)

			case "CAA":
				// Create CAA record
				if entry.CaaTag == "" || entry.CaaValue == "" {
					logger.Error("CAA record requires caa_tag and caa_value fields", "domain", entry.Domain)
					continue
				}
				var flag uint8 = 0 // Default flag is 0 (non-critical)
				if entry.CaaFlag != nil {
					flag = *entry.CaaFlag
				}
				record = localrecords.NewCAARecord(entry.Domain, entry.CaaTag, entry.CaaValue, flag)

			default:
				logger.Error("Unsupported record type", "domain", entry.Domain, "type", entry.Type)
				continue
			}

			// Apply custom TTL if specified
			if entry.TTL > 0 {
				record.TTL = entry.TTL
			}

			// Apply wildcard flag
			record.Wildcard = entry.Wildcard

			// Add record to manager
			if addErr := localMgr.AddRecord(record); addErr != nil {
				logger.Error("Failed to add local record",
					"domain", entry.Domain,
					"type", entry.Type,
					"error", addErr,
				)
				continue
			}

			logger.Debug("Added local DNS record",
				"domain", entry.Domain,
				"type", entry.Type,
				"wildcard", entry.Wildcard,
			)
		}

		handler.SetLocalRecords(localMgr)
		logger.Info("Local DNS records initialized",
			"total_records", localMgr.Count(),
		)
	}

	// Initialize policy engine from SQLite (source of truth for runtime state).
	// On first boot, seed from YAML config for backward compatibility.
	policyEngine := policy.NewEngine(logger)

	if stor != nil {
		dbRules, dbErr := stor.GetPolicyRules(ctx)
		if dbErr != nil {
			logger.Error("Failed to load policies from database", "error", dbErr)
		}

		// First-boot seed: import YAML rules into SQLite if DB is empty
		if len(dbRules) == 0 && len(cfg.Policy.Rules) > 0 {
			logger.Info("Seeding policies from config into database",
				"count", len(cfg.Policy.Rules))
			for i, entry := range cfg.Policy.Rules {
				_, seedErr := stor.CreatePolicyRule(ctx, &storage.PolicyRule{
					Name:       entry.Name,
					Logic:      entry.Logic,
					Action:     entry.Action,
					ActionData: entry.ActionData,
					Enabled:    entry.Enabled,
					SortOrder:  i,
				})
				if seedErr != nil {
					logger.Error("Failed to seed policy rule",
						"name", entry.Name, "error", seedErr)
				}
			}
			dbRules, _ = stor.GetPolicyRules(ctx)
		}

		// Build engine from DB rules
		for _, r := range dbRules {
			rule := &policy.Rule{
				Name:       r.Name,
				Logic:      r.Logic,
				Action:     r.Action,
				ActionData: r.ActionData,
				Enabled:    r.Enabled,
			}
			if addErr := policyEngine.AddRule(rule); addErr != nil {
				logger.Error("Failed to compile policy rule from DB",
					"id", r.ID, "name", r.Name, "error", addErr)
			}
		}
	} else {
		// No storage — load directly from YAML (tests, ephemeral runs)
		for _, entry := range cfg.Policy.Rules {
			rule := &policy.Rule{
				Name:       entry.Name,
				Logic:      entry.Logic,
				Action:     entry.Action,
				ActionData: entry.ActionData,
				Enabled:    entry.Enabled,
			}
			if addErr := policyEngine.AddRule(rule); addErr != nil {
				logger.Error("Failed to add policy rule",
					"name", entry.Name, "error", addErr)
			}
		}
	}

	handler.SetPolicyEngine(policyEngine)
	logger.Info("Policy engine initialized",
		"total_rules", policyEngine.Count(),
		"enabled", cfg.Server.EnablePolicies,
	)

	// Initialize the policy ClientGroupResolver. Backed by SQLite
	// client_profiles. The InClientGroup() DSL primitive resolves through
	// this; default before this point is a noop returning false.
	clientGroupResolver := policy.NewSQLiteResolver(stor)
	if err := clientGroupResolver.Reload(ctx); err != nil {
		logger.Warn("Initial client group cache build failed; InClientGroup() will return false until next reload",
			"error", err)
	}
	policy.SetClientGroupResolver(clientGroupResolver)

	// Load allowed_clients from SQLite (fallback to YAML for first boot)
	if stor != nil {
		aclJSON, aclErr := stor.GetDynamicConfig(ctx, "allowed_clients")
		if aclErr == nil && aclJSON != "" {
			var aclEntries []string
			if json.Unmarshal([]byte(aclJSON), &aclEntries) == nil {
				cfg.Server.AllowedClients = aclEntries
				logger.Info("Loaded client ACL from database", "entries", len(aclEntries))
			}
		} else if len(cfg.Server.AllowedClients) > 0 {
			// Seed from YAML on first boot
			data, _ := json.Marshal(cfg.Server.AllowedClients)
			_ = stor.SetDynamicConfig(ctx, "allowed_clients", string(data))
			logger.Info("Seeded client ACL from config into database",
				"entries", len(cfg.Server.AllowedClients))
		}
	}

	// Set metrics collector for Prometheus metrics recording
	handler.SetMetrics(metrics)

	// Set logger for enhanced visibility into DNS operations
	handler.SetLogger(logger)

	// Create kill-switch manager for duration-based temporary disabling (Pi-hole style)
	killSwitch := api.NewKillSwitchManager(logger.Logger) // Get underlying slog.Logger
	handler.SetKillSwitch(killSwitch)

	// When blocklist/policies auto-re-enable, invalidate cache entries that
	// hold upstream answers for domains that should now be blocked.
	// The closure captures dnsCache by reference — reassignments in OnChange
	// (cache reload) are observed via the variable, so we re-check on each call.
	killSwitch.SetOnReEnable(func() {
		if dnsCache != nil {
			dnsCache.ClearBlocklistDecisions()
		}
	})

	// Initialize Unbound recursive resolver (optional)
	var unboundSupervisor *unbound.Supervisor

	if cfg.Unbound.Enabled && cfg.Unbound.Managed {
		logger.Info("Starting Unbound recursive resolver",
			"port", cfg.Unbound.ListenPort,
			"config", cfg.Unbound.ConfigPath,
		)

		unboundSupervisor = unbound.NewSupervisor(&cfg.Unbound, logger)

		// Wire dnstap callback: writes to storage and populates the reply buffer
		replyBuffer := unbound.NewReplyBuffer(2000)
		handler.SetUnboundReplyBuffer(replyBuffer)

		unboundSupervisor.SetDnstapCallback(func(entry *unbound.UnboundQueryLog) {
			// Feed the reply buffer for inline enrichment of Glory-Hole's query log
			replyBuffer.Add(entry)

			// Persist to SQLite for the Unbound query log view
			if stor != nil {
				unboundQL := &storage.UnboundQueryLog{
					Timestamp:       entry.Timestamp,
					MessageType:     entry.MessageType,
					Domain:          entry.Domain,
					QueryType:       entry.QueryType,
					ResponseCode:    entry.ResponseCode,
					DurationMs:      entry.DurationMs,
					DNSSECValidated: entry.DNSSECValidated,
					AnswerCount:     entry.AnswerCount,
					ResponseSize:    entry.ResponseSize,
					ClientIP:        entry.ClientIP,
					ServerIP:        entry.ServerIP,
					CachedInUnbound: entry.CachedInUnbound,
				}
				if logErr := stor.LogUnboundQuery(context.Background(), unboundQL); logErr != nil {
					logger.Debug("Failed to log Unbound query", "error", logErr)
				}
			}
		})

		if startErr := unboundSupervisor.Start(ctx); startErr != nil {
			logger.Error("Unbound failed to start, falling back to direct forwarding",
				"error", startErr,
			)
			unboundSupervisor = nil
			// Fall through — Glory-Hole continues with configured upstream_dns_servers
		} else {
			// Override upstreams to point at local Unbound
			unboundAddr := unboundSupervisor.ListenAddr()
			cfg.UpstreamDNSServers = []string{unboundAddr}

			// Glory-Hole's cache stays enabled — it provides:
			// - API/UI cache purge support
			// - Prometheus cache metrics
			// - Blocklist-aware caching (SetBlocked with custom TTLs)
			// - Policy decisions are never cached (by design)
			// Unbound validates DNSSEC before returning, so cached responses
			// were already validated at insertion time.

			logger.Info("Unbound active, forwarding to local resolver",
				"addr", unboundAddr,
			)
		}
	}

	// Now that Unbound is ready (if enabled), start the blocklist download.
	// This avoids the race condition where blocklist download fails because
	// Unbound hasn't started yet and DNS resolution is unavailable.
	if blocklistMgr != nil {
		if blErr := blocklistMgr.Start(ctx); blErr != nil {
			logger.Error("Failed to start blocklist manager", "error", blErr)
			// Continue anyway - server can run without blocklists
		} else {
			logger.Info("Blocklist manager started",
				"domains", blocklistMgr.Size(),
				"auto_update", cfg.AutoUpdateBlocklists,
			)
		}
	}

	// Create DNS server
	server := dns.NewServer(cfg, handler, logger, metrics)
	// dnsCache starts nil; initialized in OnChange when cache config enables it

	// Create API server
	apiServer := api.New(&api.Config{
		ListenAddress:     cfg.Server.WebUIAddress,
		Storage:           stor,
		BlocklistManager:  blocklistMgr,
		PolicyEngine:      policyEngine,
		Cache:             dnsCache,          // DNS cache for purge operations
		DNSHandler:        handler,           // DNS handler for DNS-over-HTTPS (DoH) queries
		UnboundSupervisor: unboundSupervisor, // Unbound process supervisor (nil if disabled)
		Logger:            logger.Logger,     // Get underlying slog.Logger
		Version:           version,
		InitialConfig:     cfg,         // Pass initial config for auth/CORS setup
		ConfigWatcher:     cfgWatcher,  // For kill-switch feature
		ConfigPath:        *configPath, // For persisting kill-switch changes
		KillSwitch:        killSwitch,  // For duration-based temporary disabling
	})
	apiServer.SetCache(dnsCache)
	apiServer.SetDNSServer(server)
	apiServer.SetClientGroupReloader(clientGroupResolver.Reload)

	// Setup config change callback now that all components are created
	// This enables hot-reload for configuration changes
	cfgWatcher.OnChange(func(newCfg *config.Config) {
		logger.Info("Configuration reloaded",
			"dns_address", newCfg.Server.ListenAddress,
			"api_address", newCfg.Server.WebUIAddress,
		)

		apiServer.SetAuthConfig(newCfg.Auth)

		handler.SetDecisionTrace(newCfg.Server.DecisionTrace)

		// NOTE: Policy rules and allowed_clients are now in SQLite.
		// They are NOT hot-reloaded from YAML — the API/UI writes directly to the DB.

		if !equalStringSlice(cfg.UpstreamDNSServers, newCfg.UpstreamDNSServers) {
			logger.Info("Upstream DNS servers changed")
			dnsResolver = resolver.New(newCfg.UpstreamDNSServers, logger)
			httpClient = dnsResolver.NewHTTPClient(60 * time.Second)

			handler.SetForwarder(forwarder.NewForwarder(newCfg, logger, metrics))

			if blocklistMgr != nil {
				blocklistMgr.UpdateConfig(newCfg)
				blocklistMgr.SetHTTPClient(httpClient)
			}
		}

		// Hot-reload blocklists if sources changed.
		// Run asynchronously to avoid blocking the config watcher goroutine.
		// Synchronous downloads on a 512MB Fly.io VM caused GC pressure that
		// stalled health checks and triggered OOM-kill restarts.
		if blocklistMgr != nil && !equalBlocklistConfig(cfg.Blocklists, newCfg.Blocklists) {
			logger.Info("Blocklist configuration changed, triggering async reload")
			blocklistMgr.UpdateConfig(newCfg)
			go func() {
				reloadCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
				defer cancel()
				if err := blocklistMgr.Update(reloadCtx); err != nil {
					logger.Error("Failed to reload blocklists", "error", err)
				} else {
					logger.Info("Blocklists reloaded", "domains", blocklistMgr.Size())
				}
			}()
		}

		if !equalCacheConfig(&cfg.Cache, &newCfg.Cache) {
			logger.Info("Cache configuration changed")
			if dnsCache != nil {
				if err := dnsCache.Close(); err != nil {
					logger.Error("Failed to close old cache", "error", err)
				}
				dnsCache = nil
			}
			handler.SetCache(nil)
			apiServer.SetCache(nil)

			if newCfg.Cache.Enabled {
				newCache, err := cache.New(&newCfg.Cache, logger, metrics)
				if err != nil {
					logger.Error("Failed to initialize cache", "error", err)
				} else {
					dnsCache = newCache
					handler.SetCache(dnsCache)
					apiServer.SetCache(dnsCache)
					logger.Info("DNS cache reloaded",
						"max_entries", newCfg.Cache.MaxEntries,
						"min_ttl", newCfg.Cache.MinTTL,
						"max_ttl", newCfg.Cache.MaxTTL)
				}
			} else {
				logger.Info("DNS cache disabled")
			}
		}

		if !equalLoggingConfig(&cfg.Logging, &newCfg.Logging) {
			logger.Info("Logging configuration changed")
			newLogger, err := logging.New(&newCfg.Logging)
			if err != nil {
				logger.Error("Failed to reload logger", "error", err)
			} else {
				logging.SetGlobal(newLogger)
				logger = newLogger
				handler.SetLogger(newLogger)
				apiServer.SetLogger(newLogger.Logger)
				if blocklistMgr != nil {
					blocklistMgr.SetLogger(newLogger)
				}
			}
		}

		// Hot-reload local records if changed
		if !equalLocalRecordsConfig(&cfg.LocalRecords, &newCfg.LocalRecords) {
			logger.Info("Local records configuration changed")
			if newCfg.LocalRecords.Enabled && len(newCfg.LocalRecords.Records) > 0 {
				localMgr := localrecords.NewManager()
				for _, entry := range newCfg.LocalRecords.Records {
					var record *localrecords.LocalRecord
					switch entry.Type {
					case "A":
						if len(entry.IPs) == 0 {
							continue
						}
						ips := make([]net.IP, 0, len(entry.IPs))
						for _, ipStr := range entry.IPs {
							if ip := net.ParseIP(ipStr); ip != nil && ip.To4() != nil {
								ips = append(ips, ip.To4())
							}
						}
						if len(ips) > 0 {
							record = localrecords.NewARecord(entry.Domain, ips[0])
							if len(ips) > 1 {
								record.IPs = ips
							}
						}
					case "AAAA":
						if len(entry.IPs) == 0 {
							continue
						}
						ips := make([]net.IP, 0, len(entry.IPs))
						for _, ipStr := range entry.IPs {
							if ip := net.ParseIP(ipStr); ip != nil && ip.To4() == nil {
								ips = append(ips, ip.To16())
							}
						}
						if len(ips) > 0 {
							record = localrecords.NewAAAARecord(entry.Domain, ips[0])
							if len(ips) > 1 {
								record.IPs = ips
							}
						}
					case "CNAME":
						if entry.Target != "" {
							record = localrecords.NewCNAMERecord(entry.Domain, entry.Target)
						}
					case "TXT":
						if len(entry.TxtRecords) > 0 {
							record = localrecords.NewLocalRecord(entry.Domain, localrecords.RecordTypeTXT)
							record.TxtRecords = entry.TxtRecords
						}
					case "MX":
						if entry.Target != "" {
							var priority uint16 = 10
							if entry.Priority != nil {
								priority = *entry.Priority
							}
							record = localrecords.NewMXRecord(entry.Domain, entry.Target, priority)
						}
					case "PTR":
						if entry.Target != "" {
							record = localrecords.NewPTRRecord(entry.Domain, entry.Target)
						}
					case "SRV":
						if entry.Target != "" && entry.Port != nil && *entry.Port != 0 {
							var priority uint16 = 0
							if entry.Priority != nil {
								priority = *entry.Priority
							}
							var weight uint16 = 0
							if entry.Weight != nil {
								weight = *entry.Weight
							}
							record = localrecords.NewSRVRecord(entry.Domain, entry.Target, priority, weight, *entry.Port)
						}
					case "NS":
						if entry.Target != "" {
							record = localrecords.NewNSRecord(entry.Domain, entry.Target)
						}
					case "SOA":
						if entry.Ns != "" && entry.Mbox != "" {
							var serial uint32 = 1
							if entry.Serial != nil {
								serial = *entry.Serial
							}
							var refresh uint32 = 3600
							if entry.Refresh != nil {
								refresh = *entry.Refresh
							}
							var retry uint32 = 600
							if entry.Retry != nil {
								retry = *entry.Retry
							}
							var expire uint32 = 86400
							if entry.Expire != nil {
								expire = *entry.Expire
							}
							var minttl uint32 = 300
							if entry.Minttl != nil {
								minttl = *entry.Minttl
							}
							record = localrecords.NewSOARecord(entry.Domain, entry.Ns, entry.Mbox, serial, refresh, retry, expire, minttl)
						}
					case "CAA":
						if entry.CaaTag != "" && entry.CaaValue != "" {
							var flag uint8 = 0
							if entry.CaaFlag != nil {
								flag = *entry.CaaFlag
							}
							record = localrecords.NewCAARecord(entry.Domain, entry.CaaTag, entry.CaaValue, flag)
						}
					}

					if record != nil {
						if entry.TTL > 0 {
							record.TTL = entry.TTL
						}
						record.Wildcard = entry.Wildcard
						if err := localMgr.AddRecord(record); err != nil {
							logger.Error("Failed to add local record during hot-reload",
								"domain", entry.Domain,
								"error", err,
							)
						}
					}
				}
				handler.SetLocalRecords(localMgr)
				logger.Info("Local records reloaded", "total_records", localMgr.Count())
			} else {
				handler.SetLocalRecords(nil)
				logger.Info("Local records disabled")
			}
		}

		// Hot-reload Unbound resolver config
		if !equalUnboundConfig(&cfg.Unbound, &newCfg.Unbound) {
			logger.Info("Unbound configuration changed")

			// Case 1: Unbound disabled → enabled
			if !cfg.Unbound.Enabled && newCfg.Unbound.Enabled && newCfg.Unbound.Managed {
				logger.Info("Enabling Unbound resolver")
				sup := unbound.NewSupervisor(&newCfg.Unbound, logger)
				if err := sup.Start(ctx); err != nil {
					logger.Error("Failed to start Unbound", "error", err)
				} else {
					unboundSupervisor = sup
					newCfg.UpstreamDNSServers = []string{sup.ListenAddr()}
					handler.SetForwarder(forwarder.NewForwarder(newCfg, logger, metrics))
					apiServer.SetUnboundSupervisor(sup)
					logger.Info("Unbound started via hot-reload", "addr", sup.ListenAddr())
				}
			}

			// Case 2: Unbound enabled → disabled
			if cfg.Unbound.Enabled && !newCfg.Unbound.Enabled {
				logger.Info("Disabling Unbound resolver")
				if unboundSupervisor != nil {
					_ = unboundSupervisor.Stop()
					unboundSupervisor = nil
				}
				apiServer.SetUnboundSupervisor(nil)
				// Restore original upstreams from new config
				handler.SetForwarder(forwarder.NewForwarder(newCfg, logger, metrics))
				logger.Info("Unbound stopped, reverted to direct forwarding",
					"upstreams", newCfg.UpstreamDNSServers)
			}

			// Case 3: Still enabled but config changed (port, socket, etc.)
			if cfg.Unbound.Enabled && newCfg.Unbound.Enabled && unboundSupervisor != nil {
				if cfg.Unbound.ListenPort != newCfg.Unbound.ListenPort {
					logger.Warn("Unbound listen port changed — requires restart to take effect",
						"old", cfg.Unbound.ListenPort, "new", newCfg.Unbound.ListenPort)
				}
			}
		}

		// Update the cfg reference for next comparison
		cfg = newCfg

		// Note: Some config changes still require server restart:
		// - ListenAddress (DNS/API bind addresses)
		// - Database settings (connection strings)
		// - Unbound listen port changes
		// These will take effect on next server restart
	})

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start servers in background
	serverCtx, serverCancel := context.WithCancel(ctx)
	defer serverCancel()

	errChan := make(chan error, 2) // Buffer for both DNS and API errors

	// Start DNS server
	go func() {
		if err := server.Start(serverCtx); err != nil {
			errChan <- fmt.Errorf("DNS server error: %w", err)
		}
	}()

	// Start API server
	go func() {
		if err := apiServer.Start(serverCtx); err != nil {
			errChan <- fmt.Errorf("API server error: %w", err)
		}
	}()

	logger.Info("Glory Hole DNS server is running",
		"dns_address", cfg.Server.ListenAddress,
		"api_address", cfg.Server.WebUIAddress,
		"upstreams", cfg.UpstreamDNSServers,
	)

	// Wait for shutdown signal or error
	select {
	case sig := <-sigChan:
		logger.Info("Received shutdown signal", "signal", sig.String())
		serverCancel()

		// Graceful shutdown
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()

		// Shutdown DNS server
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("Error during DNS server shutdown", "error", err)
		}

		// Shutdown API server
		if err := apiServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("Error during API server shutdown", "error", err)
		}

		// Shutdown Unbound resolver
		if unboundSupervisor != nil {
			logger.Info("Stopping Unbound resolver")
			if err := unboundSupervisor.Stop(); err != nil {
				logger.Error("Error during Unbound shutdown", "error", err)
			}
		}

		// Shutdown blocklist manager
		if blocklistMgr != nil {
			blocklistMgr.Stop()
		}

		// Close DNS cache (stops cleanup goroutine, emits final stats)
		if dnsCache != nil {
			if err := dnsCache.Close(); err != nil {
				logger.Error("Error during cache shutdown", "error", err)
			}
		}

		// Shutdown storage (query logger defer runs before this via deferred stack)
		if stor != nil {
			if err := stor.Close(); err != nil {
				logger.Error("Error during storage shutdown", "error", err)
			}
		}

		if err := telem.Shutdown(shutdownCtx); err != nil {
			logger.Error("Error during telemetry shutdown", "error", err)
		}

		logger.Info("Glory Hole DNS stopped")

	case err := <-errChan:
		logger.Error("Server error", "error", err)
		os.Exit(1)
	}
}

// equalBlocklistConfig compares two blocklist configurations
func equalBlocklistConfig(a, b []string) bool {
	return equalStringSlice(a, b)
}

func equalCacheConfig(a, b *config.CacheConfig) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Enabled == b.Enabled &&
		a.MaxEntries == b.MaxEntries &&
		a.MinTTL == b.MinTTL &&
		a.MaxTTL == b.MaxTTL &&
		a.NegativeTTL == b.NegativeTTL &&
		a.BlockedTTL == b.BlockedTTL &&
		a.ShardCount == b.ShardCount
}

func equalLoggingConfig(a, b *config.LoggingConfig) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Level == b.Level &&
		a.Format == b.Format &&
		a.Output == b.Output &&
		a.FilePath == b.FilePath &&
		a.AddSource == b.AddSource &&
		a.MaxSize == b.MaxSize &&
		a.MaxBackups == b.MaxBackups &&
		a.MaxAge == b.MaxAge
}

// equalUnboundConfig compares two Unbound configurations
func equalUnboundConfig(a, b *config.UnboundConfig) bool {
	return a.Enabled == b.Enabled &&
		a.Managed == b.Managed &&
		a.ListenPort == b.ListenPort &&
		a.BinaryPath == b.BinaryPath &&
		a.ConfigPath == b.ConfigPath &&
		a.ControlSocket == b.ControlSocket
}

// equalStringSlice compares two string slices
func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// equalLocalRecordsConfig compares two local records configurations
func equalLocalRecordsConfig(a, b *config.LocalRecordsConfig) bool {
	if a.Enabled != b.Enabled || len(a.Records) != len(b.Records) {
		return false
	}
	for i := range a.Records {
		if a.Records[i].Domain != b.Records[i].Domain ||
			a.Records[i].Type != b.Records[i].Type ||
			a.Records[i].Target != b.Records[i].Target ||
			a.Records[i].TTL != b.Records[i].TTL ||
			a.Records[i].Wildcard != b.Records[i].Wildcard ||
			!equalStringSlice(a.Records[i].IPs, b.Records[i].IPs) {
			return false
		}
	}
	return true
}

// performHealthCheck performs a health check against the API server
// Returns exit code 0 if healthy, 1 if unhealthy
func performHealthCheck(apiAddr, configPath string) int {
	// If API address not provided, try to load from config
	if apiAddr == "" {
		cfg, err := config.Load(configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Health check failed: cannot load config: %v\n", err)
			return 1
		}
		apiAddr = cfg.Server.WebUIAddress

		// If WebUIAddress doesn't have http:// prefix, add it
		if apiAddr != "" && apiAddr[0] == ':' {
			apiAddr = "http://localhost" + apiAddr
		} else if !strings.HasPrefix(apiAddr, "http://") && !strings.HasPrefix(apiAddr, "https://") {
			apiAddr = "http://" + apiAddr
		}
	}

	// Make HTTP request to health endpoint
	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	healthURL := apiAddr + "/api/health"
	resp, err := client.Get(healthURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Health check failed: %v\n", err)
		return 1
	}
	defer func() { _ = resp.Body.Close() }()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "Health check failed: status code %d\n", resp.StatusCode)
		return 1
	}

	fmt.Println("Health check passed")
	return 0
}

// runImportPihole runs the Pi-hole import command
func runImportPihole(args []string) {
	// Create flagset for import command
	fs := flag.NewFlagSet("import-pihole", flag.ExitOnError)

	// Define import flags
	zipPath := fs.String("zip", "", "Path to Pi-hole Teleporter backup ZIP (recommended)")
	gravityDB := fs.String("gravity-db", "", "Path to gravity.db (alternative to --zip)")
	piholeConfig := fs.String("pihole-config", "", "Path to pihole.toml (optional)")
	customList := fs.String("custom-list", "", "Path to custom.list (optional)")
	output := fs.String("output", "", "Output file path (default: stdout)")
	dryRun := fs.Bool("dry-run", false, "Show what would be imported without writing")
	validate := fs.Bool("validate", true, "Validate config before writing")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: glory-hole import-pihole [options]\n\n")
		fmt.Fprintf(os.Stderr, "Import Pi-hole configuration to Glory-Hole format\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  # Import from Pi-hole Teleporter ZIP (recommended)\n")
		fmt.Fprintf(os.Stderr, "  glory-hole import-pihole --zip=pihole-teleporter-2025-11-23.zip\n\n")
		fmt.Fprintf(os.Stderr, "  # With output file\n")
		fmt.Fprintf(os.Stderr, "  glory-hole import-pihole --zip=backup.zip --output=config.yml\n\n")
		fmt.Fprintf(os.Stderr, "  # Dry run to preview\n")
		fmt.Fprintf(os.Stderr, "  glory-hole import-pihole --zip=backup.zip --dry-run\n\n")
		fmt.Fprintf(os.Stderr, "  # Alternative: Direct file paths\n")
		fmt.Fprintf(os.Stderr, "  glory-hole import-pihole --gravity-db=/etc/pihole/gravity.db --output=config.yml\n\n")
	}

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	// Validate inputs
	if *zipPath == "" && *gravityDB == "" {
		fmt.Fprintf(os.Stderr, "Error: Must provide either --zip or --gravity-db\n\n")
		fs.Usage()
		os.Exit(1)
	}

	// Create importer
	importer := NewPiholeImporter()
	importer.zipPath = *zipPath
	importer.gravityDB = *gravityDB
	importer.piholeConfig = *piholeConfig
	importer.customList = *customList
	importer.output = *output
	importer.dryRun = *dryRun
	importer.validate = *validate

	// Run import
	cfg, err := importer.Import()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Import failed: %v\n", err)
		os.Exit(1)
	}

	// Write configuration
	if err := importer.WriteConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write config: %v\n", err)
		os.Exit(1)
	}
}

func runHashPassword(args []string) {
	fs := flag.NewFlagSet("hash-password", flag.ExitOnError)
	cost := fs.Int("cost", 12, "Bcrypt cost parameter (10-14 recommended, higher = more secure but slower)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: glory-hole hash-password [OPTIONS] [PASSWORD]\n\n")
		fmt.Fprintf(os.Stderr, "Generate a bcrypt hash for a password to use in auth.password_hash.\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  glory-hole hash-password MySecretPassword\n")
		fmt.Fprintf(os.Stderr, "  glory-hole hash-password --cost 14 MySecretPassword\n")
		fmt.Fprintf(os.Stderr, "  echo -n 'MySecretPassword' | glory-hole hash-password\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse flags: %v\n", err)
		os.Exit(1)
	}

	var password string

	// Get password from argument or stdin
	if fs.NArg() > 0 {
		password = fs.Arg(0)
	} else {
		// Read from stdin
		fmt.Fprintf(os.Stderr, "Enter password: ")
		var input string
		if _, err := fmt.Scanln(&input); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read password: %v\n", err)
			os.Exit(1)
		}
		password = input
	}

	if password == "" {
		fmt.Fprintf(os.Stderr, "Error: Password cannot be empty\n")
		fs.Usage()
		os.Exit(1)
	}

	// Validate cost
	if *cost < 4 || *cost > 31 {
		fmt.Fprintf(os.Stderr, "Error: Cost must be between 4 and 31 (recommended: 10-14)\n")
		os.Exit(1)
	}

	// Generate hash
	fmt.Fprintf(os.Stderr, "Generating bcrypt hash with cost %d...\n", *cost)

	hash, err := bcrypt.GenerateFromPassword([]byte(password), *cost)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to generate hash: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Success! Hash generated.\n\n")
	fmt.Printf("# Add this to your config.yml:\n")
	fmt.Printf("auth:\n")
	fmt.Printf("  enabled: true\n")
	fmt.Printf("  username: \"admin\"\n")
	fmt.Printf("  password_hash: \"%s\"\n", string(hash))
	fmt.Printf("\n# IMPORTANT: Remove the plaintext 'password' field when using password_hash!\n")
	fmt.Printf("# The password_hash field takes precedence over password.\n")
}
