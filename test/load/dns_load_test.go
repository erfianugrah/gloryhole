package load

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"net"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"glory-hole/pkg/blocklist"
	"glory-hole/pkg/cache"
	"glory-hole/pkg/config"
	"glory-hole/pkg/dns"
	"glory-hole/pkg/localrecords"
	"glory-hole/pkg/logging"
	"glory-hole/pkg/policy"

	dnslib "github.com/miekg/dns"
)

// LoadTestConfig holds configuration for load tests
type LoadTestConfig struct {
	ConcurrentClients int           // Number of concurrent clients
	QueriesPerClient  int           // Queries each client should make
	Duration          time.Duration // Test duration (alternative to queries per client)
	BlocklistSize     int           // Number of domains in blocklist
	CacheEnabled      bool          // Enable DNS cache
	RampUpTime        time.Duration // Time to gradually increase load
}

// LoadTestResult holds results from a load test
type LoadTestResult struct {
	TotalQueries     int64
	SuccessfulQueries int64
	FailedQueries    int64
	BlockedQueries   int64
	CachedQueries    int64
	Duration         time.Duration
	QueriesPerSecond float64

	// Latency statistics (in milliseconds)
	MinLatency    float64
	MaxLatency    float64
	AvgLatency    float64
	P50Latency    float64
	P95Latency    float64
	P99Latency    float64
	P999Latency   float64

	// Resource usage
	StartMemory  uint64
	EndMemory    uint64
	MemoryDelta  uint64
	Goroutines   int

	// Cache statistics
	CacheHits     uint64
	CacheMisses   uint64
	CacheHitRate  float64
}

// mockResponseWriter implements dns.ResponseWriter for testing
type mockResponseWriter struct {
	msg        *dnslib.Msg
	remoteAddr net.Addr
}

func (m *mockResponseWriter) LocalAddr() net.Addr {
	return &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 53}
}

func (m *mockResponseWriter) RemoteAddr() net.Addr {
	if m.remoteAddr != nil {
		return m.remoteAddr
	}
	return &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345}
}

func (m *mockResponseWriter) WriteMsg(msg *dnslib.Msg) error {
	// Copy the message to avoid races with pooled messages
	// Real DNS writers serialize to bytes, so this simulates that behavior
	m.msg = msg.Copy()
	return nil
}

func (m *mockResponseWriter) Write(b []byte) (int, error) {
	return len(b), nil
}

func (m *mockResponseWriter) Close() error {
	return nil
}

func (m *mockResponseWriter) TsigStatus() error {
	return nil
}

func (m *mockResponseWriter) TsigTimersOnly(b bool) {}

func (m *mockResponseWriter) Hijack() {}

// TestDNSLoadBasic performs a basic load test with concurrent queries
func TestDNSLoadBasic(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	cfg := LoadTestConfig{
		ConcurrentClients: 100,
		QueriesPerClient:  100,
		BlocklistSize:     10000,
		CacheEnabled:      true,
		RampUpTime:        time.Second,
	}

	result := runLoadTest(t, cfg)
	printLoadTestResults(t, "Basic Load Test", result)

	// Assertions
	if result.QueriesPerSecond < 1000 {
		t.Logf("WARNING: QPS is low (%0.2f), expected > 1000", result.QueriesPerSecond)
	}

	if result.P99Latency > 100 {
		t.Logf("WARNING: P99 latency is high (%0.2fms), expected < 100ms", result.P99Latency)
	}
}

// TestDNSLoadHeavy performs a heavy load test with many concurrent clients
func TestDNSLoadHeavy(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping heavy load test in short mode")
	}

	cfg := LoadTestConfig{
		ConcurrentClients: 1000,
		QueriesPerClient:  100,
		BlocklistSize:     100000,
		CacheEnabled:      true,
		RampUpTime:        2 * time.Second,
	}

	result := runLoadTest(t, cfg)
	printLoadTestResults(t, "Heavy Load Test", result)

	// Check for failures
	if result.FailedQueries > int64(float64(result.TotalQueries)*0.01) {
		t.Errorf("Too many failed queries: %d (%.2f%%)",
			result.FailedQueries,
			float64(result.FailedQueries)/float64(result.TotalQueries)*100)
	}
}

// TestDNSLoadLargeBlocklist tests performance with a massive blocklist
func TestDNSLoadLargeBlocklist(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large blocklist test in short mode")
	}

	cfg := LoadTestConfig{
		ConcurrentClients: 500,
		QueriesPerClient:  200,
		BlocklistSize:     1000000, // 1 million domains
		CacheEnabled:      true,
		RampUpTime:        3 * time.Second,
	}

	result := runLoadTest(t, cfg)
	printLoadTestResults(t, "Large Blocklist Load Test", result)

	// Memory should be reasonable even with 1M domains
	memoryMB := float64(result.EndMemory) / 1024 / 1024
	t.Logf("Memory usage with 1M blocklist: %.2f MB", memoryMB)

	if memoryMB > 500 {
		t.Logf("WARNING: Memory usage is high (%.2fMB), expected < 500MB", memoryMB)
	}
}

// TestDNSCachePerformance tests cache hit rates and performance
func TestDNSCachePerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping cache performance test in short mode")
	}

	cfg := LoadTestConfig{
		ConcurrentClients: 200,
		QueriesPerClient:  500,
		BlocklistSize:     10000,
		CacheEnabled:      true,
		RampUpTime:        time.Second,
	}

	result := runLoadTest(t, cfg)
	printLoadTestResults(t, "Cache Performance Test", result)

	// With repeated queries, we should have a good cache hit rate
	if result.CacheHitRate < 0.5 {
		t.Logf("WARNING: Cache hit rate is low (%.2f%%), expected > 50%%", result.CacheHitRate*100)
	}

	t.Logf("Cache efficiency: %.2f%% hit rate", result.CacheHitRate*100)
}

// TestDNSLoadSustained performs a sustained load test over time
func TestDNSLoadSustained(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping sustained load test in short mode")
	}

	cfg := LoadTestConfig{
		ConcurrentClients: 100,
		Duration:          30 * time.Second,
		BlocklistSize:     50000,
		CacheEnabled:      true,
		RampUpTime:        2 * time.Second,
	}

	result := runLoadTestDuration(t, cfg)
	printLoadTestResults(t, "Sustained Load Test (30s)", result)

	// Check for stability over time
	if result.FailedQueries > 0 {
		failureRate := float64(result.FailedQueries) / float64(result.TotalQueries)
		if failureRate > 0.001 { // More than 0.1% failures
			t.Errorf("Sustained test has high failure rate: %.4f%%", failureRate*100)
		}
	}
}

// TestDNSMemoryProfile performs memory profiling under load
func TestDNSMemoryProfile(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory profile test in short mode")
	}

	cfg := LoadTestConfig{
		ConcurrentClients: 500,
		QueriesPerClient:  1000,
		BlocklistSize:     100000,
		CacheEnabled:      true,
		RampUpTime:        2 * time.Second,
	}

	// Force GC before test
	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	result := runLoadTest(t, cfg)
	printLoadTestResults(t, "Memory Profile Test", result)

	// Check for memory leaks
	memGrowthMB := float64(result.MemoryDelta) / 1024 / 1024
	t.Logf("Memory growth during test: %.2f MB", memGrowthMB)

	// Memory growth should be reasonable
	if memGrowthMB > 200 {
		t.Logf("WARNING: Significant memory growth detected: %.2fMB", memGrowthMB)
	}

	// Check goroutine count
	if result.Goroutines > 1000 {
		t.Logf("WARNING: High goroutine count: %d", result.Goroutines)
	}
}

// runLoadTest executes a load test with the given configuration
func runLoadTest(t *testing.T, cfg LoadTestConfig) LoadTestResult {
	// Setup handler with test configuration
	handler := setupTestHandler(t, cfg)

	// Track metrics
	var (
		totalQueries      int64
		successfulQueries int64
		failedQueries     int64
		blockedQueries    int64
		latencies         []float64
		latenciesMu       sync.Mutex
	)

	// Memory snapshot before test
	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	// Test domains (mix of blocked and allowed)
	testDomains := generateTestDomains(cfg.BlocklistSize)

	startTime := time.Now()

	// Launch concurrent clients
	var wg sync.WaitGroup
	clientDelay := time.Duration(0)
	if cfg.RampUpTime > 0 && cfg.ConcurrentClients > 0 {
		clientDelay = cfg.RampUpTime / time.Duration(cfg.ConcurrentClients)
	}

	for i := 0; i < cfg.ConcurrentClients; i++ {
		wg.Add(1)

		// Stagger client start times for ramp-up
		if clientDelay > 0 {
			time.Sleep(clientDelay)
		}

		go func(clientID int) {
			defer wg.Done()

			// Each client makes multiple queries
			for j := 0; j < cfg.QueriesPerClient; j++ {
				// Pick a random domain
				domain := testDomains[rand.Intn(len(testDomains))]

				// Create DNS query
				msg := new(dnslib.Msg)
				msg.SetQuestion(dnslib.Fqdn(domain), dnslib.TypeA)

				// Mock response writer
				writer := &mockResponseWriter{
					remoteAddr: &net.UDPAddr{
						IP:   net.ParseIP(fmt.Sprintf("192.168.%d.%d", clientID%256, (clientID/256)%256)),
						Port: 12345 + clientID,
					},
				}

				// Measure latency
				queryStart := time.Now()
				ctx := context.Background()
				handler.ServeDNS(ctx, writer, msg)
				queryDuration := time.Since(queryStart)

				// Record metrics
				atomic.AddInt64(&totalQueries, 1)

				if writer.msg != nil {
					atomic.AddInt64(&successfulQueries, 1)

					// Check if blocked (NXDOMAIN)
					if writer.msg.Rcode == dnslib.RcodeNameError {
						atomic.AddInt64(&blockedQueries, 1)
					}
				} else {
					atomic.AddInt64(&failedQueries, 1)
				}

				// Record latency
				latenciesMu.Lock()
				latencies = append(latencies, float64(queryDuration.Microseconds())/1000.0)
				latenciesMu.Unlock()
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(startTime)

	// Memory snapshot after test
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)

	// Get cache statistics if enabled
	var cacheHits, cacheMisses uint64
	var cacheHitRate float64
	if cfg.CacheEnabled && handler.Cache != nil {
		stats := handler.Cache.Stats()
		cacheHits = stats.Hits
		cacheMisses = stats.Misses
		cacheHitRate = stats.HitRate
	}

	// Calculate latency percentiles
	sort.Float64s(latencies)

	return LoadTestResult{
		TotalQueries:      totalQueries,
		SuccessfulQueries: successfulQueries,
		FailedQueries:     failedQueries,
		BlockedQueries:    blockedQueries,
		Duration:          duration,
		QueriesPerSecond:  float64(totalQueries) / duration.Seconds(),

		MinLatency:  latencies[0],
		MaxLatency:  latencies[len(latencies)-1],
		AvgLatency:  average(latencies),
		P50Latency:  percentile(latencies, 0.50),
		P95Latency:  percentile(latencies, 0.95),
		P99Latency:  percentile(latencies, 0.99),
		P999Latency: percentile(latencies, 0.999),

		StartMemory: memBefore.Alloc,
		EndMemory:   memAfter.Alloc,
		MemoryDelta: memAfter.Alloc - memBefore.Alloc,
		Goroutines:  runtime.NumGoroutine(),

		CacheHits:    cacheHits,
		CacheMisses:  cacheMisses,
		CacheHitRate: cacheHitRate,
	}
}

// runLoadTestDuration executes a duration-based load test
func runLoadTestDuration(t *testing.T, cfg LoadTestConfig) LoadTestResult {
	// Setup handler with test configuration
	handler := setupTestHandler(t, cfg)

	// Track metrics
	var (
		totalQueries      int64
		successfulQueries int64
		failedQueries     int64
		blockedQueries    int64
		latencies         []float64
		latenciesMu       sync.Mutex
	)

	// Memory snapshot before test
	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	// Test domains
	testDomains := generateTestDomains(cfg.BlocklistSize)

	startTime := time.Now()
	endTime := startTime.Add(cfg.Duration)

	// Launch concurrent clients
	var wg sync.WaitGroup
	clientDelay := time.Duration(0)
	if cfg.RampUpTime > 0 && cfg.ConcurrentClients > 0 {
		clientDelay = cfg.RampUpTime / time.Duration(cfg.ConcurrentClients)
	}

	for i := 0; i < cfg.ConcurrentClients; i++ {
		wg.Add(1)

		// Stagger client start times for ramp-up
		if clientDelay > 0 {
			time.Sleep(clientDelay)
		}

		go func(clientID int) {
			defer wg.Done()

			// Keep querying until duration expires
			for time.Now().Before(endTime) {
				// Pick a random domain
				domain := testDomains[rand.Intn(len(testDomains))]

				// Create DNS query
				msg := new(dnslib.Msg)
				msg.SetQuestion(dnslib.Fqdn(domain), dnslib.TypeA)

				// Mock response writer
				writer := &mockResponseWriter{
					remoteAddr: &net.UDPAddr{
						IP:   net.ParseIP(fmt.Sprintf("192.168.%d.%d", clientID%256, (clientID/256)%256)),
						Port: 12345 + clientID,
					},
				}

				// Measure latency
				queryStart := time.Now()
				ctx := context.Background()
				handler.ServeDNS(ctx, writer, msg)
				queryDuration := time.Since(queryStart)

				// Record metrics
				atomic.AddInt64(&totalQueries, 1)

				if writer.msg != nil {
					atomic.AddInt64(&successfulQueries, 1)

					if writer.msg.Rcode == dnslib.RcodeNameError {
						atomic.AddInt64(&blockedQueries, 1)
					}
				} else {
					atomic.AddInt64(&failedQueries, 1)
				}

				// Record latency (with sampling to avoid memory issues)
				if rand.Float32() < 0.1 { // Sample 10% of queries
					latenciesMu.Lock()
					latencies = append(latencies, float64(queryDuration.Microseconds())/1000.0)
					latenciesMu.Unlock()
				}
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(startTime)

	// Memory snapshot after test
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)

	// Get cache statistics
	var cacheHits, cacheMisses uint64
	var cacheHitRate float64
	if cfg.CacheEnabled && handler.Cache != nil {
		stats := handler.Cache.Stats()
		cacheHits = stats.Hits
		cacheMisses = stats.Misses
		cacheHitRate = stats.HitRate
	}

	// Calculate latency percentiles
	if len(latencies) > 0 {
		sort.Float64s(latencies)
	}

	result := LoadTestResult{
		TotalQueries:      totalQueries,
		SuccessfulQueries: successfulQueries,
		FailedQueries:     failedQueries,
		BlockedQueries:    blockedQueries,
		Duration:          duration,
		QueriesPerSecond:  float64(totalQueries) / duration.Seconds(),

		StartMemory: memBefore.Alloc,
		EndMemory:   memAfter.Alloc,
		MemoryDelta: memAfter.Alloc - memBefore.Alloc,
		Goroutines:  runtime.NumGoroutine(),

		CacheHits:    cacheHits,
		CacheMisses:  cacheMisses,
		CacheHitRate: cacheHitRate,
	}

	if len(latencies) > 0 {
		result.MinLatency = latencies[0]
		result.MaxLatency = latencies[len(latencies)-1]
		result.AvgLatency = average(latencies)
		result.P50Latency = percentile(latencies, 0.50)
		result.P95Latency = percentile(latencies, 0.95)
		result.P99Latency = percentile(latencies, 0.99)
		result.P999Latency = percentile(latencies, 0.999)
	}

	return result
}

// setupTestHandler creates a DNS handler configured for load testing
func setupTestHandler(t *testing.T, cfg LoadTestConfig) *dns.Handler {
	handler := dns.NewHandler()

	// Setup logger
	logger, err := logging.New(&config.LoggingConfig{
		Level:  "error", // Reduce logging overhead during load tests
		Format: "text",
		Output: "stdout",
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Setup blocklist manager with test domains
	testCfg := &config.Config{
		Blocklists:           []string{}, // No external blocklists for tests
		AutoUpdateBlocklists: false,
	}
	blocklistMgr := blocklist.NewManager(testCfg, logger, nil, nil)

	// Manually populate the blocklist for testing
	handler.SetBlocklistManager(blocklistMgr)

	// Add domains to the handler's legacy blocklist for testing
	for i := 0; i < cfg.BlocklistSize; i++ {
		domain := fmt.Sprintf("blocked%d.test.", i)
		handler.Blocklist[domain] = struct{}{}
	}

	// Setup cache if enabled
	if cfg.CacheEnabled {
		dnsCache, err := cache.New(&config.CacheConfig{
			Enabled:     true,
			MaxEntries:  10000,
			MinTTL:      5 * time.Second,
			MaxTTL:      1 * time.Hour,
			NegativeTTL: 5 * time.Second,
		}, logger, nil)
		if err != nil {
			t.Fatalf("Failed to create cache: %v", err)
		}
		handler.SetCache(dnsCache)
	}

	// Setup local records for fast responses
	localMgr := localrecords.NewManager()
	for i := 0; i < 100; i++ {
		domain := fmt.Sprintf("local%d.test.", i)
		ip := net.ParseIP(fmt.Sprintf("192.168.1.%d", i%256))
		_ = localMgr.AddRecord(localrecords.NewARecord(domain, ip))
	}
	handler.SetLocalRecords(localMgr)

	// Setup policy engine with test rules
	policyEngine := policy.NewEngine()
	handler.SetPolicyEngine(policyEngine)

	return handler
}

// generateTestDomains creates a list of test domains
func generateTestDomains(blocklistSize int) []string {
	domains := make([]string, 0, blocklistSize+200)

	// Add local domains (fast responses)
	for i := 0; i < 100; i++ {
		domains = append(domains, fmt.Sprintf("local%d.test", i))
	}

	// Add blocked domains
	for i := 0; i < blocklistSize && i < 1000; i++ {
		domains = append(domains, fmt.Sprintf("blocked%d.test", i))
	}

	// Add regular domains
	regularDomains := []string{
		"example.com", "test.com", "google.com", "github.com",
		"cloudflare.com", "amazon.com", "microsoft.com", "apple.com",
	}
	domains = append(domains, regularDomains...)

	return domains
}

// percentile calculates the nth percentile of sorted data
func percentile(data []float64, p float64) float64 {
	if len(data) == 0 {
		return 0
	}
	index := int(math.Ceil(float64(len(data)) * p))
	if index >= len(data) {
		index = len(data) - 1
	}
	return data[index]
}

// average calculates the average of a slice of float64
func average(data []float64) float64 {
	if len(data) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range data {
		sum += v
	}
	return sum / float64(len(data))
}

// printLoadTestResults prints formatted load test results
func printLoadTestResults(t *testing.T, testName string, result LoadTestResult) {
	separator := strings.Repeat("=", 80)
	t.Logf("\n%s", separator)
	t.Logf("%s Results", testName)
	t.Logf("%s", separator)
	t.Logf("Total Queries:      %d", result.TotalQueries)
	t.Logf("Successful:         %d (%.2f%%)", result.SuccessfulQueries,
		float64(result.SuccessfulQueries)/float64(result.TotalQueries)*100)
	t.Logf("Failed:             %d (%.2f%%)", result.FailedQueries,
		float64(result.FailedQueries)/float64(result.TotalQueries)*100)
	t.Logf("Blocked:            %d (%.2f%%)", result.BlockedQueries,
		float64(result.BlockedQueries)/float64(result.TotalQueries)*100)
	t.Logf("Duration:           %v", result.Duration)
	t.Logf("Queries/Second:     %.2f", result.QueriesPerSecond)
	t.Logf("")
	t.Logf("Latency Statistics (ms):")
	t.Logf("  Min:    %8.3f", result.MinLatency)
	t.Logf("  Avg:    %8.3f", result.AvgLatency)
	t.Logf("  P50:    %8.3f", result.P50Latency)
	t.Logf("  P95:    %8.3f", result.P95Latency)
	t.Logf("  P99:    %8.3f", result.P99Latency)
	t.Logf("  P99.9:  %8.3f", result.P999Latency)
	t.Logf("  Max:    %8.3f", result.MaxLatency)
	t.Logf("")
	t.Logf("Resource Usage:")
	t.Logf("  Start Memory:  %10d bytes (%.2f MB)", result.StartMemory, float64(result.StartMemory)/1024/1024)
	t.Logf("  End Memory:    %10d bytes (%.2f MB)", result.EndMemory, float64(result.EndMemory)/1024/1024)
	t.Logf("  Memory Delta:  %10d bytes (%.2f MB)", result.MemoryDelta, float64(result.MemoryDelta)/1024/1024)
	t.Logf("  Goroutines:    %d", result.Goroutines)
	t.Logf("")
	if result.CacheHits > 0 || result.CacheMisses > 0 {
		t.Logf("Cache Statistics:")
		t.Logf("  Hits:      %d", result.CacheHits)
		t.Logf("  Misses:    %d", result.CacheMisses)
		t.Logf("  Hit Rate:  %.2f%%", result.CacheHitRate*100)
		t.Logf("")
	}
	t.Logf("%s\n", separator)
}
