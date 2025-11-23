package load

import (
	"context"
	"fmt"
	"net"
	"testing"

	"glory-hole/pkg/blocklist"
	"glory-hole/pkg/cache"
	"glory-hole/pkg/config"
	"glory-hole/pkg/dns"
	"glory-hole/pkg/localrecords"
	"glory-hole/pkg/logging"
	"glory-hole/pkg/policy"

	dnslib "github.com/miekg/dns"
)

// Benchmark DNS query processing with various configurations

// BenchmarkDNSQuery_LocalRecord benchmarks local DNS record resolution
func BenchmarkDNSQuery_LocalRecord(b *testing.B) {
	handler := setupBenchmarkHandler(b, 0, true, false)

	msg := new(dnslib.Msg)
	msg.SetQuestion("local1.test.", dnslib.TypeA)

	writer := &mockResponseWriter{
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		handler.ServeDNS(ctx, writer, msg)
	}

	b.StopTimer()
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "qps")
}

// BenchmarkDNSQuery_BlocklistHit benchmarks blocklist lookup and blocking
func BenchmarkDNSQuery_BlocklistHit(b *testing.B) {
	handler := setupBenchmarkHandler(b, 100000, true, false)

	msg := new(dnslib.Msg)
	msg.SetQuestion("blocked1000.test.", dnslib.TypeA)

	writer := &mockResponseWriter{
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		handler.ServeDNS(ctx, writer, msg)
	}

	b.StopTimer()
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "qps")
}

// BenchmarkDNSQuery_CacheHit benchmarks cache hit performance
func BenchmarkDNSQuery_CacheHit(b *testing.B) {
	handler := setupBenchmarkHandler(b, 1000, true, false)

	msg := new(dnslib.Msg)
	msg.SetQuestion("cached.test.", dnslib.TypeA)

	// Pre-populate cache
	resp := new(dnslib.Msg)
	resp.SetReply(msg)
	resp.Answer = append(resp.Answer, &dnslib.A{
		Hdr: dnslib.RR_Header{
			Name:   "cached.test.",
			Rrtype: dnslib.TypeA,
			Class:  dnslib.ClassINET,
			Ttl:    300,
		},
		A: net.ParseIP("192.168.1.1"),
	})
	handler.Cache.Set(context.Background(), msg, resp)

	writer := &mockResponseWriter{
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		handler.ServeDNS(ctx, writer, msg)
	}

	b.StopTimer()
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "qps")
}

// BenchmarkDNSQuery_CacheMiss benchmarks cache miss and set performance
func BenchmarkDNSQuery_CacheMiss(b *testing.B) {
	handler := setupBenchmarkHandler(b, 1000, true, false)

	writer := &mockResponseWriter{
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Different domain each time to ensure cache miss
		msg := new(dnslib.Msg)
		msg.SetQuestion(fmt.Sprintf("unique%d.test.", i), dnslib.TypeA)
		handler.ServeDNS(ctx, writer, msg)
	}

	b.StopTimer()
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "qps")
}

// BenchmarkDNSQuery_PolicyEngine benchmarks policy engine evaluation
func BenchmarkDNSQuery_PolicyEngine(b *testing.B) {
	handler := setupBenchmarkHandler(b, 10000, true, true)

	msg := new(dnslib.Msg)
	msg.SetQuestion("test.example.com.", dnslib.TypeA)

	writer := &mockResponseWriter{
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("192.168.1.100"), Port: 12345},
	}
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		handler.ServeDNS(ctx, writer, msg)
	}

	b.StopTimer()
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "qps")
}

// BenchmarkDNSQuery_FullStack benchmarks complete DNS processing pipeline
func BenchmarkDNSQuery_FullStack(b *testing.B) {
	handler := setupBenchmarkHandler(b, 50000, true, true)

	msg := new(dnslib.Msg)
	msg.SetQuestion("example.com.", dnslib.TypeA)

	writer := &mockResponseWriter{
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("192.168.1.100"), Port: 12345},
	}
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		handler.ServeDNS(ctx, writer, msg)
	}

	b.StopTimer()
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "qps")
}

// BenchmarkBlocklistLookup benchmarks blocklist lookup performance with various sizes
func BenchmarkBlocklistLookup_1K(b *testing.B) {
	benchmarkBlocklistLookup(b, 1000)
}

func BenchmarkBlocklistLookup_10K(b *testing.B) {
	benchmarkBlocklistLookup(b, 10000)
}

func BenchmarkBlocklistLookup_100K(b *testing.B) {
	benchmarkBlocklistLookup(b, 100000)
}

func BenchmarkBlocklistLookup_1M(b *testing.B) {
	benchmarkBlocklistLookup(b, 1000000)
}

func benchmarkBlocklistLookup(b *testing.B, size int) {
	logger, _ := logging.New(&config.LoggingConfig{
		Level:  "error",
		Format: "text",
		Output: "stdout",
	})

	// Setup blocklist manager
	testCfg := &config.Config{
		Blocklists:           []string{},
		AutoUpdateBlocklists: false,
	}
	blocklistMgr := blocklist.NewManager(testCfg, logger, nil)

	// Build blocklist data
	blocklistData := make(map[string]struct{}, size)
	for i := 0; i < size; i++ {
		domain := fmt.Sprintf("blocked%d.test.", i)
		blocklistData[domain] = struct{}{}
	}

	// Manually set the blocklist using reflection-like approach
	// For testing, we'll use the handler's legacy blocklist instead
	handler := dns.NewHandler()
	handler.SetBlocklistManager(blocklistMgr)
	for domain := range blocklistData {
		handler.Blocklist[domain] = struct{}{}
	}

	// Test with domain in middle of list
	testDomain := fmt.Sprintf("blocked%d.test.", size/2)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = handler.Blocklist[testDomain]
	}

	b.StopTimer()
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "lookups/sec")
}

// BenchmarkCacheOperations benchmarks cache set and get operations
func BenchmarkCache_Set(b *testing.B) {
	logger, _ := logging.New(&config.LoggingConfig{
		Level:  "error",
		Format: "text",
		Output: "stdout",
	})

	dnsCache, _ := cache.New(&config.CacheConfig{
		Enabled:    true,
		MaxEntries: 10000,
	}, logger, nil)

	msg := new(dnslib.Msg)
	msg.SetQuestion("test.example.com.", dnslib.TypeA)

	resp := new(dnslib.Msg)
	resp.SetReply(msg)
	resp.Answer = append(resp.Answer, &dnslib.A{
		Hdr: dnslib.RR_Header{
			Name:   "test.example.com.",
			Rrtype: dnslib.TypeA,
			Class:  dnslib.ClassINET,
			Ttl:    300,
		},
		A: net.ParseIP("192.168.1.1"),
	})

	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		dnsCache.Set(ctx, msg, resp)
	}
}

func BenchmarkCache_Get(b *testing.B) {
	logger, _ := logging.New(&config.LoggingConfig{
		Level:  "error",
		Format: "text",
		Output: "stdout",
	})

	dnsCache, _ := cache.New(&config.CacheConfig{
		Enabled:    true,
		MaxEntries: 10000,
	}, logger, nil)

	msg := new(dnslib.Msg)
	msg.SetQuestion("test.example.com.", dnslib.TypeA)

	resp := new(dnslib.Msg)
	resp.SetReply(msg)
	resp.Answer = append(resp.Answer, &dnslib.A{
		Hdr: dnslib.RR_Header{
			Name:   "test.example.com.",
			Rrtype: dnslib.TypeA,
			Class:  dnslib.ClassINET,
			Ttl:    300,
		},
		A: net.ParseIP("192.168.1.1"),
	})

	ctx := context.Background()
	dnsCache.Set(ctx, msg, resp)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = dnsCache.Get(ctx, msg)
	}
}

func BenchmarkCache_Mixed(b *testing.B) {
	logger, _ := logging.New(&config.LoggingConfig{
		Level:  "error",
		Format: "text",
		Output: "stdout",
	})

	dnsCache, _ := cache.New(&config.CacheConfig{
		Enabled:    true,
		MaxEntries: 1000,
	}, logger, nil)

	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Alternate between get and set
		msg := new(dnslib.Msg)
		msg.SetQuestion(fmt.Sprintf("test%d.example.com.", i%100), dnslib.TypeA)

		if i%2 == 0 {
			// Get
			_ = dnsCache.Get(ctx, msg)
		} else {
			// Set
			resp := new(dnslib.Msg)
			resp.SetReply(msg)
			resp.Answer = append(resp.Answer, &dnslib.A{
				Hdr: dnslib.RR_Header{
					Name:   msg.Question[0].Name,
					Rrtype: dnslib.TypeA,
					Class:  dnslib.ClassINET,
					Ttl:    300,
				},
				A: net.ParseIP("192.168.1.1"),
			})
			dnsCache.Set(ctx, msg, resp)
		}
	}
}

// BenchmarkPolicyEngine benchmarks policy engine rule evaluation
func BenchmarkPolicyEngine_SingleRule(b *testing.B) {
	engine := policy.NewEngine()

	rule := &policy.Rule{
		Name:    "Block Social Media",
		Logic:   `DomainMatches(Domain, "facebook.com")`,
		Action:  policy.ActionBlock,
		Enabled: true,
	}
	_ = engine.AddRule(rule)

	ctx := policy.NewContext("www.facebook.com", "192.168.1.100", "A")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = engine.Evaluate(ctx)
	}
}

func BenchmarkPolicyEngine_MultipleRules(b *testing.B) {
	engine := policy.NewEngine()

	// Add 10 rules
	for i := 0; i < 10; i++ {
		rule := &policy.Rule{
			Name:    fmt.Sprintf("Rule %d", i),
			Logic:   fmt.Sprintf(`DomainMatches(Domain, "blocked%d.com")`, i),
			Action:  policy.ActionBlock,
			Enabled: true,
		}
		_ = engine.AddRule(rule)
	}

	ctx := policy.NewContext("www.example.com", "192.168.1.100", "A")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = engine.Evaluate(ctx)
	}
}

func BenchmarkPolicyEngine_ComplexRule(b *testing.B) {
	engine := policy.NewEngine()

	rule := &policy.Rule{
		Name: "Complex Time-Based Block",
		Logic: `DomainMatches(Domain, "social.com") &&
		        IPInCIDR(ClientIP, "192.168.0.0/16") &&
		        (Hour >= 9 && Hour <= 17) &&
		        !IsWeekend(Weekday)`,
		Action:  policy.ActionBlock,
		Enabled: true,
	}
	_ = engine.AddRule(rule)

	ctx := policy.NewContext("www.social.com", "192.168.1.100", "A")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = engine.Evaluate(ctx)
	}
}

// BenchmarkLocalRecords benchmarks local DNS record lookups
func BenchmarkLocalRecords_LookupA(b *testing.B) {
	localMgr := localrecords.NewManager()

	// Add 1000 local records
	for i := 0; i < 1000; i++ {
		domain := fmt.Sprintf("local%d.test.", i)
		ip := net.ParseIP(fmt.Sprintf("192.168.%d.%d", i/256, i%256))
		_ = localMgr.AddRecord(localrecords.NewARecord(domain, ip))
	}

	testDomain := "local500.test."

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _, _ = localMgr.LookupA(testDomain)
	}
}

func BenchmarkLocalRecords_LookupCNAME(b *testing.B) {
	localMgr := localrecords.NewManager()

	// Add CNAME records
	_ = localMgr.AddRecord(localrecords.NewCNAMERecord("alias.test.", "target.test."))
	_ = localMgr.AddRecord(localrecords.NewARecord("target.test.", net.ParseIP("192.168.1.100")))

	testDomain := "alias.test."

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _, _ = localMgr.LookupCNAME(testDomain)
	}
}

func BenchmarkLocalRecords_ResolveCNAME(b *testing.B) {
	localMgr := localrecords.NewManager()

	// Add CNAME chain
	_ = localMgr.AddRecord(localrecords.NewCNAMERecord("alias.test.", "target.test."))
	_ = localMgr.AddRecord(localrecords.NewARecord("target.test.", net.ParseIP("192.168.1.100")))

	testDomain := "alias.test."

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _, _ = localMgr.ResolveCNAME(testDomain, 10)
	}
}

// BenchmarkConcurrentQueries benchmarks concurrent DNS query processing
func BenchmarkConcurrentQueries_10(b *testing.B) {
	benchmarkConcurrentQueries(b, 10)
}

func BenchmarkConcurrentQueries_100(b *testing.B) {
	benchmarkConcurrentQueries(b, 100)
}

func BenchmarkConcurrentQueries_1000(b *testing.B) {
	benchmarkConcurrentQueries(b, 1000)
}

func benchmarkConcurrentQueries(b *testing.B, concurrency int) {
	handler := setupBenchmarkHandler(b, 10000, true, false)

	msg := new(dnslib.Msg)
	msg.SetQuestion("local1.test.", dnslib.TypeA)

	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		writer := &mockResponseWriter{
			remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
		}
		for pb.Next() {
			handler.ServeDNS(ctx, writer, msg)
		}
	})

	b.StopTimer()
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "qps")
}

// BenchmarkMemoryAllocation benchmarks memory allocation patterns
func BenchmarkMemoryAllocation_DNSMessage(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		msg := new(dnslib.Msg)
		msg.SetQuestion("example.com.", dnslib.TypeA)
		msg.Answer = append(msg.Answer, &dnslib.A{
			Hdr: dnslib.RR_Header{
				Name:   "example.com.",
				Rrtype: dnslib.TypeA,
				Class:  dnslib.ClassINET,
				Ttl:    300,
			},
			A: net.ParseIP("192.168.1.1"),
		})
		_ = msg
	}
}

func BenchmarkMemoryAllocation_ResponseWriter(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		writer := &mockResponseWriter{}
		_ = writer
	}
}

// setupBenchmarkHandler creates a DNS handler configured for benchmarking
func setupBenchmarkHandler(b *testing.B, blocklistSize int, enableCache, enablePolicy bool) *dns.Handler {
	handler := dns.NewHandler()

	logger, _ := logging.New(&config.LoggingConfig{
		Level:  "error",
		Format: "text",
		Output: "stdout",
	})

	// Setup blocklist
	if blocklistSize > 0 {
		testCfg := &config.Config{
			Blocklists:           []string{},
			AutoUpdateBlocklists: false,
		}
		blocklistMgr := blocklist.NewManager(testCfg, logger, nil)
		handler.SetBlocklistManager(blocklistMgr)

		// Add to legacy blocklist for testing
		for i := 0; i < blocklistSize; i++ {
			domain := fmt.Sprintf("blocked%d.test.", i)
			handler.Blocklist[domain] = struct{}{}
		}
	}

	// Setup cache
	if enableCache {
		dnsCache, _ := cache.New(&config.CacheConfig{
			Enabled:    true,
			MaxEntries: 10000,
		}, logger, nil)
		handler.SetCache(dnsCache)
	}

	// Setup local records
	localMgr := localrecords.NewManager()
	for i := 0; i < 10; i++ {
		domain := fmt.Sprintf("local%d.test.", i)
		ip := net.ParseIP(fmt.Sprintf("192.168.1.%d", i))
		_ = localMgr.AddRecord(localrecords.NewARecord(domain, ip))
	}
	handler.SetLocalRecords(localMgr)

	// Setup policy engine
	if enablePolicy {
		policyEngine := policy.NewEngine()
		rule := &policy.Rule{
			Name:    "Test Rule",
			Logic:   `DomainMatches(Domain, "blocked.test")`,
			Action:  policy.ActionBlock,
			Enabled: true,
		}
		_ = policyEngine.AddRule(rule)
		handler.SetPolicyEngine(policyEngine)
	}

	// Note: We don't setup forwarder for benchmarks to avoid network calls
	// and keep benchmarks deterministic and fast

	return handler
}

// Benchmark suite for comparing different configurations
func BenchmarkComparison_NoCacheNoBlocklist(b *testing.B) {
	handler := setupBenchmarkHandler(b, 0, false, false)
	benchmarkHandler(b, handler, "example.com.")
}

func BenchmarkComparison_CacheOnly(b *testing.B) {
	handler := setupBenchmarkHandler(b, 0, true, false)
	benchmarkHandler(b, handler, "local1.test.")
}

func BenchmarkComparison_BlocklistOnly(b *testing.B) {
	handler := setupBenchmarkHandler(b, 10000, false, false)
	benchmarkHandler(b, handler, "blocked1000.test.")
}

func BenchmarkComparison_CacheAndBlocklist(b *testing.B) {
	handler := setupBenchmarkHandler(b, 10000, true, false)
	benchmarkHandler(b, handler, "local1.test.")
}

func BenchmarkComparison_FullStack(b *testing.B) {
	handler := setupBenchmarkHandler(b, 50000, true, true)
	benchmarkHandler(b, handler, "local1.test.")
}

func benchmarkHandler(b *testing.B, handler *dns.Handler, domain string) {
	msg := new(dnslib.Msg)
	msg.SetQuestion(domain, dnslib.TypeA)

	writer := &mockResponseWriter{
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		handler.ServeDNS(ctx, writer, msg)
	}

	b.StopTimer()
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "qps")
}
