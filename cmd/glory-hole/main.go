package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"glory-hole/pkg/blocklist"
	"glory-hole/pkg/config"
	"glory-hole/pkg/dns"
	"glory-hole/pkg/localrecords"
	"glory-hole/pkg/logging"
	"glory-hole/pkg/storage"
	"glory-hole/pkg/telemetry"
)

var (
	configPath = flag.String("config", "config.yml", "Path to configuration file")
	version    = "dev"
	buildTime  = "unknown"
)

func main() {
	flag.Parse()

	// Parse configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	logger, err := logging.New(&cfg.Logging)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	logging.SetGlobal(logger)

	logger.Info("Glory Hole DNS starting",
		"version", version,
		"build_time", buildTime,
	)

	// Initialize telemetry
	ctx := context.Background()
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

	// Initialize blocklist manager if configured (lock-free, high performance)
	var blocklistMgr *blocklist.Manager
	if len(cfg.Blocklists) > 0 {
		logger.Info("Initializing blocklist manager", "sources", len(cfg.Blocklists))
		blocklistMgr = blocklist.NewManager(cfg, logger)

		// Start blocklist manager (downloads lists and starts auto-update)
		if err := blocklistMgr.Start(ctx); err != nil {
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

	// Create DNS server
	server := dns.NewServer(cfg, handler, logger, metrics)

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start server in background
	serverCtx, serverCancel := context.WithCancel(ctx)
	defer serverCancel()

	errChan := make(chan error, 1)
	go func() {
		if err := server.Start(serverCtx); err != nil {
			errChan <- err
		}
	}()

	logger.Info("Glory Hole DNS server is running",
		"address", cfg.Server.ListenAddress,
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

		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("Error during server shutdown", "error", err)
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
