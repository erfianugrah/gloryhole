package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"glory-hole/pkg/blocklist"
	"glory-hole/pkg/config"
	"glory-hole/pkg/dns"
	"glory-hole/pkg/logging"
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

		if err := telem.Shutdown(shutdownCtx); err != nil {
			logger.Error("Error during telemetry shutdown", "error", err)
		}

		logger.Info("Glory Hole DNS stopped")

	case err := <-errChan:
		logger.Error("Server error", "error", err)
		os.Exit(1)
	}
}
