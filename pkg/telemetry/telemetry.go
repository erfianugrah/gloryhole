// Package telemetry wires up Prometheus + OpenTelemetry exporters used across
// the project.
package telemetry

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"glory-hole/pkg/config"
	"glory-hole/pkg/logging"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

// Telemetry holds telemetry providers and exporters
type Telemetry struct {
	cfg                *config.TelemetryConfig
	meterProvider      metric.MeterProvider
	tracerProvider     trace.TracerProvider
	prometheusExporter *prometheus.Exporter
	prometheusServer   *http.Server
	logger             *logging.Logger
}

// Metrics holds all application metrics
type Metrics struct {
	// DNS Query metrics
	DNSQueriesTotal       metric.Int64Counter
	DNSQueriesByType      metric.Int64Counter
	DNSQueryDuration      metric.Float64Histogram
	DNSCacheHits          metric.Int64Counter
	DNSCacheMisses        metric.Int64Counter
	DNSBlockedQueries     metric.Int64Counter
	DNSForwardedQueries   metric.Int64Counter
	DNSWhitelistedQueries metric.Int64Counter

	// Rate limiting metrics
	RateLimitViolations metric.Int64Counter
	RateLimitDropped    metric.Int64Counter

	// System metrics
	ActiveClients metric.Int64UpDownCounter
	BlocklistSize metric.Int64UpDownCounter
	CacheSize     metric.Int64UpDownCounter

	// Storage metrics
	StorageQueriesDropped metric.Int64Counter
}

// New creates a new telemetry instance
func New(ctx context.Context, cfg *config.TelemetryConfig, logger *logging.Logger) (*Telemetry, error) {
	if !cfg.Enabled {
		logger.Info("Telemetry disabled")
		return &Telemetry{
			cfg:            cfg,
			meterProvider:  noop.NewMeterProvider(),
			tracerProvider: tracenoop.NewTracerProvider(),
			logger:         logger,
		}, nil
	}

	t := &Telemetry{
		cfg:    cfg,
		logger: logger,
	}

	// Create resource with service information
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(cfg.ServiceName),
			semconv.ServiceVersionKey.String(cfg.ServiceVersion),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Setup metrics
	if err := t.setupMetrics(ctx, res); err != nil {
		return nil, fmt.Errorf("failed to setup metrics: %w", err)
	}

	// Setup tracing if enabled
	if cfg.TracingEnabled {
		if err := t.setupTracing(ctx, res); err != nil {
			return nil, fmt.Errorf("failed to setup tracing: %w", err)
		}
	} else {
		t.tracerProvider = tracenoop.NewTracerProvider()
	}

	logger.Info("Telemetry initialized",
		"service", cfg.ServiceName,
		"version", cfg.ServiceVersion,
		"prometheus", cfg.PrometheusEnabled,
		"tracing", cfg.TracingEnabled,
	)

	return t, nil
}

// setupMetrics initializes the metrics provider
func (t *Telemetry) setupMetrics(ctx context.Context, res *resource.Resource) error {
	if t.cfg.PrometheusEnabled {
		// Create Prometheus exporter
		exporter, err := prometheus.New()
		if err != nil {
			return fmt.Errorf("failed to create prometheus exporter: %w", err)
		}

		// Store the exporter for use in HTTP handler
		t.prometheusExporter = exporter

		// Create meter provider
		provider := sdkmetric.NewMeterProvider(
			sdkmetric.WithResource(res),
			sdkmetric.WithReader(exporter),
		)

		t.meterProvider = provider
		otel.SetMeterProvider(provider)

		// Start Prometheus HTTP server
		if err := t.startPrometheusServer(); err != nil {
			return fmt.Errorf("failed to start prometheus server: %w", err)
		}

		t.logger.Info("Prometheus metrics enabled", "port", t.cfg.PrometheusPort)
	} else {
		t.meterProvider = noop.NewMeterProvider()
	}

	return nil
}

// setupTracing initializes the tracer provider
func (t *Telemetry) setupTracing(ctx context.Context, res *resource.Resource) error {
	// For now, we'll use a no-op tracer
	// In production, you would configure OTLP exporter here
	t.tracerProvider = tracenoop.NewTracerProvider()
	otel.SetTracerProvider(t.tracerProvider)

	t.logger.Info("Tracing enabled", "endpoint", t.cfg.TracingEndpoint)
	return nil
}

// startPrometheusServer starts the Prometheus metrics HTTP server
func (t *Telemetry) startPrometheusServer() error {
	mux := http.NewServeMux()

	// Use promhttp.Handler() to serve Prometheus metrics
	// This works with the OpenTelemetry Prometheus exporter
	mux.Handle("/metrics", promhttp.Handler())

	t.prometheusServer = &http.Server{
		Addr:              fmt.Sprintf(":%d", t.cfg.PrometheusPort),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second, // Prevent Slowloris attacks
	}

	go func() {
		if err := t.prometheusServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			t.logger.Error("Prometheus server failed", "error", err)
		}
	}()

	return nil
}

// InitMetrics initializes and returns all application metrics
func (t *Telemetry) InitMetrics() (*Metrics, error) {
	meter := t.meterProvider.Meter("glory-hole")

	// Create all metrics
	queriesTotal, err := meter.Int64Counter(
		"dns.queries.total",
		metric.WithDescription("Total number of DNS queries received"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create queries counter: %w", err)
	}

	queriesByType, err := meter.Int64Counter(
		"dns.queries.by_type",
		metric.WithDescription("DNS queries by query type"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create queries by type counter: %w", err)
	}

	queryDuration, err := meter.Float64Histogram(
		"dns.query.duration",
		metric.WithDescription("DNS query processing duration in milliseconds"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create query duration histogram: %w", err)
	}

	cacheHits, err := meter.Int64Counter(
		"dns.cache.hits",
		metric.WithDescription("Number of DNS cache hits"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create cache hits counter: %w", err)
	}

	cacheMisses, err := meter.Int64Counter(
		"dns.cache.misses",
		metric.WithDescription("Number of DNS cache misses"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create cache misses counter: %w", err)
	}

	blockedQueries, err := meter.Int64Counter(
		"dns.queries.blocked",
		metric.WithDescription("Number of blocked DNS queries"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create blocked queries counter: %w", err)
	}

	forwardedQueries, err := meter.Int64Counter(
		"dns.queries.forwarded",
		metric.WithDescription("Number of forwarded DNS queries"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create forwarded queries counter: %w", err)
	}

	whitelistedQueries, err := meter.Int64Counter(
		"dns.queries.whitelisted",
		metric.WithDescription("Number of whitelisted DNS queries"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create whitelisted queries counter: %w", err)
	}

	rateLimitViolations, err := meter.Int64Counter(
		"rate_limit.violations",
		metric.WithDescription("Number of rate limit violations"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create rate limit violations counter: %w", err)
	}

	rateLimitDropped, err := meter.Int64Counter(
		"rate_limit.dropped",
		metric.WithDescription("Number of dropped requests due to rate limiting"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create rate limit dropped counter: %w", err)
	}

	activeClients, err := meter.Int64UpDownCounter(
		"clients.active",
		metric.WithDescription("Number of active clients"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create active clients gauge: %w", err)
	}

	blocklistSize, err := meter.Int64UpDownCounter(
		"blocklist.size",
		metric.WithDescription("Number of domains in blocklist"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create blocklist size gauge: %w", err)
	}

	cacheSize, err := meter.Int64UpDownCounter(
		"cache.size",
		metric.WithDescription("Number of entries in DNS cache"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create cache size gauge: %w", err)
	}

	storageQueriesDropped, err := meter.Int64Counter(
		"storage.queries.dropped",
		metric.WithDescription("Number of queries dropped due to full buffer"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage queries dropped counter: %w", err)
	}

	return &Metrics{
		DNSQueriesTotal:       queriesTotal,
		DNSQueriesByType:      queriesByType,
		DNSQueryDuration:      queryDuration,
		DNSCacheHits:          cacheHits,
		DNSCacheMisses:        cacheMisses,
		DNSBlockedQueries:     blockedQueries,
		DNSForwardedQueries:   forwardedQueries,
		DNSWhitelistedQueries: whitelistedQueries,
		RateLimitViolations:   rateLimitViolations,
		RateLimitDropped:      rateLimitDropped,
		ActiveClients:         activeClients,
		BlocklistSize:         blocklistSize,
		CacheSize:             cacheSize,
		StorageQueriesDropped: storageQueriesDropped,
	}, nil
}

// MeterProvider returns the meter provider
func (t *Telemetry) MeterProvider() metric.MeterProvider {
	return t.meterProvider
}

// TracerProvider returns the tracer provider
func (t *Telemetry) TracerProvider() trace.TracerProvider {
	return t.tracerProvider
}

// AddDroppedQuery implements storage.MetricsRecorder interface
// This allows Metrics to be passed to storage without creating import cycles
func (m *Metrics) AddDroppedQuery(ctx context.Context, count int64) {
	if m != nil && m.StorageQueriesDropped != nil {
		m.StorageQueriesDropped.Add(ctx, count)
	}
}

// Shutdown gracefully shuts down telemetry
func (t *Telemetry) Shutdown(ctx context.Context) error {
	var errs []error

	// Shutdown Prometheus server
	if t.prometheusServer != nil {
		if err := t.prometheusServer.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("prometheus server shutdown: %w", err))
		}
	}

	// Shutdown meter provider if it's the SDK implementation
	if provider, ok := t.meterProvider.(*sdkmetric.MeterProvider); ok {
		if err := provider.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("meter provider shutdown: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("telemetry shutdown errors: %v", errs)
	}

	t.logger.Info("Telemetry shut down")
	return nil
}
