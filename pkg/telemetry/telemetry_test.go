package telemetry

import (
	"context"
	"testing"
	"time"

	"glory-hole/pkg/config"
	"glory-hole/pkg/logging"

	"go.opentelemetry.io/otel/metric"
)

func TestNew(t *testing.T) {
	logger := logging.NewDefault()

	tests := []struct {
		cfg     *config.TelemetryConfig
		name    string
		wantErr bool
	}{
		{
			name: "disabled telemetry",
			cfg: &config.TelemetryConfig{
				Enabled: false,
			},
			wantErr: false,
		},
		{
			name: "prometheus enabled",
			cfg: &config.TelemetryConfig{
				Enabled:           true,
				ServiceName:       "test-service",
				ServiceVersion:    "1.0.0",
				PrometheusEnabled: true,
				PrometheusPort:    9091, // Use different port to avoid conflicts
			},
			wantErr: false,
		},
		{
			name: "only metrics",
			cfg: &config.TelemetryConfig{
				Enabled:           true,
				ServiceName:       "test-service",
				ServiceVersion:    "1.0.0",
				PrometheusEnabled: false,
				TracingEnabled:    false,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			tel, err := New(ctx, tt.cfg, logger)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tel == nil {
				t.Error("New() returned nil telemetry")
			}

			// Cleanup
			if tel != nil && tel.prometheusServer != nil {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = tel.Shutdown(ctx)
			}
		})
	}
}

func TestInitMetrics(t *testing.T) {
	logger := logging.NewDefault()
	cfg := &config.TelemetryConfig{
		Enabled:     true,
		ServiceName: "test-service",
	}

	ctx := context.Background()
	tel, err := New(ctx, cfg, logger)
	if err != nil {
		t.Fatalf("Failed to create telemetry: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tel.Shutdown(ctx)
	}()

	metrics, err := tel.InitMetrics()
	if err != nil {
		t.Fatalf("InitMetrics() failed: %v", err)
	}

	// Test that all metrics are initialized
	if metrics.DNSQueriesTotal == nil {
		t.Error("DNSQueriesTotal not initialized")
	}
	if metrics.DNSQueryDuration == nil {
		t.Error("DNSQueryDuration not initialized")
	}
	if metrics.DNSCacheHits == nil {
		t.Error("DNSCacheHits not initialized")
	}
	if metrics.ActiveClients == nil {
		t.Error("ActiveClients not initialized")
	}
}

func TestMetricsRecording(t *testing.T) {
	logger := logging.NewDefault()
	cfg := &config.TelemetryConfig{
		Enabled:     true,
		ServiceName: "test-service",
	}

	ctx := context.Background()
	tel, err := New(ctx, cfg, logger)
	if err != nil {
		t.Fatalf("Failed to create telemetry: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tel.Shutdown(ctx)
	}()

	metrics, err := tel.InitMetrics()
	if err != nil {
		t.Fatalf("InitMetrics() failed: %v", err)
	}

	// Test recording metrics
	metrics.DNSQueriesTotal.Add(ctx, 1, metric.WithAttributes())
	metrics.DNSCacheHits.Add(ctx, 1, metric.WithAttributes())
	metrics.DNSQueryDuration.Record(ctx, 5.5, metric.WithAttributes())
	metrics.ActiveClients.Add(ctx, 1, metric.WithAttributes())

	// If we got here without panicking, the test passes
}

func TestMeterProvider(t *testing.T) {
	logger := logging.NewDefault()
	cfg := &config.TelemetryConfig{
		Enabled:     true,
		ServiceName: "test-service",
	}

	ctx := context.Background()
	tel, err := New(ctx, cfg, logger)
	if err != nil {
		t.Fatalf("Failed to create telemetry: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tel.Shutdown(ctx)
	}()

	provider := tel.MeterProvider()
	if provider == nil {
		t.Error("MeterProvider() returned nil")
	}
}

func TestTracerProvider(t *testing.T) {
	logger := logging.NewDefault()
	cfg := &config.TelemetryConfig{
		Enabled:     true,
		ServiceName: "test-service",
	}

	ctx := context.Background()
	tel, err := New(ctx, cfg, logger)
	if err != nil {
		t.Fatalf("Failed to create telemetry: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tel.Shutdown(ctx)
	}()

	provider := tel.TracerProvider()
	if provider == nil {
		t.Error("TracerProvider() returned nil")
	}

	// Verify we can get a tracer
	tracer := provider.Tracer("test-tracer")
	if tracer == nil {
		t.Error("Tracer() returned nil")
	}
}

func TestShutdown(t *testing.T) {
	logger := logging.NewDefault()
	cfg := &config.TelemetryConfig{
		Enabled:           true,
		ServiceName:       "test-service",
		PrometheusEnabled: true,
		PrometheusPort:    9092, // Use different port
	}

	ctx := context.Background()
	tel, err := New(ctx, cfg, logger)
	if err != nil {
		t.Fatalf("Failed to create telemetry: %v", err)
	}

	// Test shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = tel.Shutdown(shutdownCtx)
	if err != nil {
		t.Errorf("Shutdown() error = %v", err)
	}
}

func TestDisabledTelemetry(t *testing.T) {
	logger := logging.NewDefault()
	cfg := &config.TelemetryConfig{
		Enabled: false,
	}

	ctx := context.Background()
	tel, err := New(ctx, cfg, logger)
	if err != nil {
		t.Fatalf("Failed to create telemetry: %v", err)
	}

	// Even with disabled telemetry, we should get valid providers
	if tel.MeterProvider() == nil {
		t.Error("Disabled telemetry should still return a noop meter provider")
	}
	if tel.TracerProvider() == nil {
		t.Error("Disabled telemetry should still return a noop tracer provider")
	}

	// Should be able to init metrics without error
	metrics, err := tel.InitMetrics()
	if err != nil {
		t.Errorf("InitMetrics() with disabled telemetry failed: %v", err)
	}
	if metrics == nil {
		t.Error("InitMetrics() returned nil metrics")
	}
}
