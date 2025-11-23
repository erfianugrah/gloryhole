package main

import (
	"context"
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
	"glory-hole/pkg/config"
	"glory-hole/pkg/dns"
	"glory-hole/pkg/forwarder"
	"glory-hole/pkg/localrecords"
	"glory-hole/pkg/logging"
	"glory-hole/pkg/policy"
	"glory-hole/pkg/storage"
	"glory-hole/pkg/telemetry"
)

var (
	configPath  = flag.String("config", "config.yml", "Path to configuration file")
	showVersion = flag.Bool("version", false, "Show version information and exit")
	healthCheck = flag.Bool("health-check", false, "Perform health check and exit (for Docker HEALTHCHECK)")
	apiAddress  = flag.String("api-address", "", "Override API address for health check (default: from config)")

	// Build-time variables set via ldflags
	// Example: go build -ldflags "-X main.version=0.7.2 -X main.buildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
	version   = "dev"     // Set via -ldflags "-X main.version=x.y.z"
	buildTime = "unknown" // Set via -ldflags "-X main.buildTime=$(date)"
	gitCommit = "unknown" // Set via -ldflags "-X main.gitCommit=$(git rev-parse --short HEAD)"
)

func main() {
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

	// Create DNS handler
	handler := dns.NewHandler()

	// Set config watcher for kill-switch feature (hot-reload access)
	handler.ConfigWatcher = cfgWatcher

	// Initialize blocklist manager if configured (lock-free, high performance)
	var blocklistMgr *blocklist.Manager
	if len(cfg.Blocklists) > 0 {
		logger.Info("Initializing blocklist manager", "sources", len(cfg.Blocklists))
		blocklistMgr = blocklist.NewManager(cfg, logger, metrics)

		// Start blocklist manager (downloads lists and starts auto-update)
		err = blocklistMgr.Start(ctx)
		if err != nil {
			logger.Error("Failed to start blocklist manager", "error", err)
			// Continue anyway - server can run without blocklists
		} else {
			handler.SetBlocklistManager(blocklistMgr)
			logger.Info("Blocklist manager started",
				"domains", blocklistMgr.Size(),
				"auto_update", cfg.AutoUpdateBlocklists,
			)
		}
	}

	// Load whitelist if configured
	if len(cfg.Whitelist) > 0 {
		for _, domain := range cfg.Whitelist {
			handler.Whitelist[domain] = struct{}{}
		}
		logger.Info("Whitelist loaded", "domains", len(cfg.Whitelist))
	}

	// Initialize storage (database for query logging)
	var stor storage.Storage
	if cfg.Database.Enabled {
		logger.Info("Initializing storage",
			"backend", cfg.Database.Backend,
			"path", cfg.Database.SQLite.Path,
		)
		stor, err = storage.New(&cfg.Database)
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
		}
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
			if err := localMgr.AddRecord(record); err != nil {
				logger.Error("Failed to add local record",
					"domain", entry.Domain,
					"type", entry.Type,
					"error", err,
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

	// Initialize policy engine if configured
	var policyEngine *policy.Engine
	if cfg.Policy.Enabled && len(cfg.Policy.Rules) > 0 {
		logger.Info("Initializing policy engine", "rules", len(cfg.Policy.Rules))
		policyEngine = policy.NewEngine()

		for _, entry := range cfg.Policy.Rules {
			rule := &policy.Rule{
				Name:       entry.Name,
				Logic:      entry.Logic,
				Action:     entry.Action,
				ActionData: entry.ActionData,
				Enabled:    entry.Enabled,
			}

			if err := policyEngine.AddRule(rule); err != nil {
				logger.Error("Failed to add policy rule",
					"name", entry.Name,
					"error", err,
				)
				continue
			}

			logger.Debug("Added policy rule",
				"name", entry.Name,
				"action", entry.Action,
				"enabled", entry.Enabled,
			)
		}

		handler.SetPolicyEngine(policyEngine)
		logger.Info("Policy engine initialized",
			"total_rules", policyEngine.Count(),
		)
	}

	// Initialize conditional forwarding rule evaluator
	if cfg.ConditionalForwarding.Enabled {
		ruleEvaluator, err := forwarder.NewRuleEvaluator(&cfg.ConditionalForwarding)
		if err != nil {
			logger.Error("Failed to initialize conditional forwarding",
				"error", err,
			)
		} else {
			handler.RuleEvaluator = ruleEvaluator
			logger.Info("Conditional forwarding initialized",
				"total_rules", ruleEvaluator.Count(),
			)
		}
	}

	// Set metrics collector for Prometheus metrics recording
	handler.SetMetrics(metrics)

	// Create DNS server
	server := dns.NewServer(cfg, handler, logger, metrics)

	// Create API server
	apiServer := api.New(&api.Config{
		ListenAddress:    cfg.Server.WebUIAddress,
		Storage:          stor,
		BlocklistManager: blocklistMgr,
		PolicyEngine:     policyEngine,
		Logger:           logger.Logger, // Get underlying slog.Logger
		Version:          version,
		ConfigWatcher:    cfgWatcher, // For kill-switch feature
		ConfigPath:       *configPath, // For persisting kill-switch changes
	})

	// Setup config change callback now that all components are created
	// This enables hot-reload for configuration changes
	cfgWatcher.OnChange(func(newCfg *config.Config) {
		logger.Info("Configuration reloaded",
			"dns_address", newCfg.Server.ListenAddress,
			"api_address", newCfg.Server.WebUIAddress,
		)

		// Hot-reload blocklists if sources changed
		if blocklistMgr != nil && !equalBlocklistConfig(cfg.Blocklists, newCfg.Blocklists) {
			logger.Info("Blocklist configuration changed, triggering reload")
			if err := blocklistMgr.Update(ctx); err != nil {
				logger.Error("Failed to reload blocklists", "error", err)
			} else {
				logger.Info("Blocklists reloaded", "domains", blocklistMgr.Size())
			}
		}

		// Hot-reload policy engine if rules changed
		if policyEngine != nil && !equalPolicyConfig(&cfg.Policy, &newCfg.Policy) {
			logger.Info("Policy configuration changed, triggering reload")
			policyEngine.Clear()
			for _, entry := range newCfg.Policy.Rules {
				rule := &policy.Rule{
					Name:       entry.Name,
					Logic:      entry.Logic,
					Action:     entry.Action,
					ActionData: entry.ActionData,
					Enabled:    entry.Enabled,
				}
				if err := policyEngine.AddRule(rule); err != nil {
					logger.Error("Failed to add policy rule during hot-reload",
						"name", entry.Name,
						"error", err,
					)
				}
			}
			logger.Info("Policy engine reloaded", "total_rules", policyEngine.Count())
		}

		// Hot-reload conditional forwarding if changed
		if !equalConditionalForwardingConfig(&cfg.ConditionalForwarding, &newCfg.ConditionalForwarding) {
			logger.Info("Conditional forwarding configuration changed")
			if newCfg.ConditionalForwarding.Enabled {
				ruleEvaluator, err := forwarder.NewRuleEvaluator(&newCfg.ConditionalForwarding)
				if err != nil {
					logger.Error("Failed to reload conditional forwarding", "error", err)
				} else {
					handler.RuleEvaluator = ruleEvaluator
					logger.Info("Conditional forwarding reloaded", "total_rules", ruleEvaluator.Count())
				}
			} else {
				handler.RuleEvaluator = nil
				logger.Info("Conditional forwarding disabled")
			}
		}

		// Hot-reload whitelist if changed
		if !equalStringSlice(cfg.Whitelist, newCfg.Whitelist) {
			logger.Info("Whitelist configuration changed")
			handler.Whitelist = make(map[string]struct{})
			for _, domain := range newCfg.Whitelist {
				handler.Whitelist[domain] = struct{}{}
			}
			logger.Info("Whitelist reloaded", "domains", len(newCfg.Whitelist))
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

		// Update the cfg reference for next comparison
		cfg = newCfg

		// Note: Some config changes require server restart:
		// - ListenAddress (DNS/API bind addresses)
		// - Database settings (connection strings)
		// - Upstream DNS servers (forwarder recreation)
		// - Cache settings (cache recreation)
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

		// Shutdown blocklist manager
		if blocklistMgr != nil {
			blocklistMgr.Stop()
		}

		// Shutdown storage
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

// equalPolicyConfig compares two policy configurations
func equalPolicyConfig(a, b *config.PolicyConfig) bool {
	if a.Enabled != b.Enabled || len(a.Rules) != len(b.Rules) {
		return false
	}
	for i := range a.Rules {
		if a.Rules[i].Name != b.Rules[i].Name ||
			a.Rules[i].Logic != b.Rules[i].Logic ||
			a.Rules[i].Action != b.Rules[i].Action ||
			a.Rules[i].ActionData != b.Rules[i].ActionData ||
			a.Rules[i].Enabled != b.Rules[i].Enabled {
			return false
		}
	}
	return true
}

// equalConditionalForwardingConfig compares two conditional forwarding configurations
func equalConditionalForwardingConfig(a, b *config.ConditionalForwardingConfig) bool {
	if a.Enabled != b.Enabled || len(a.Rules) != len(b.Rules) {
		return false
	}
	for i := range a.Rules {
		if a.Rules[i].Name != b.Rules[i].Name ||
			!equalStringSlice(a.Rules[i].Domains, b.Rules[i].Domains) ||
			!equalStringSlice(a.Rules[i].ClientCIDRs, b.Rules[i].ClientCIDRs) ||
			!equalStringSlice(a.Rules[i].QueryTypes, b.Rules[i].QueryTypes) ||
			!equalStringSlice(a.Rules[i].Upstreams, b.Rules[i].Upstreams) ||
			a.Rules[i].Priority != b.Rules[i].Priority ||
			a.Rules[i].Enabled != b.Rules[i].Enabled {
			return false
		}
	}
	return true
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
