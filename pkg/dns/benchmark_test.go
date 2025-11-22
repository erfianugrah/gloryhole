package dns

import (
	"context"
	"net"
	"testing"

	"glory-hole/pkg/cache"
	"glory-hole/pkg/config"
	"glory-hole/pkg/forwarder"
	"glory-hole/pkg/localrecords"
	"glory-hole/pkg/logging"
	"glory-hole/pkg/policy"

	"github.com/miekg/dns"
)

// BenchmarkHandler_LocalRecord benchmarks local record resolution
func BenchmarkHandler_LocalRecord(b *testing.B) {
	handler := NewHandler()

	// Setup local records
	localMgr := localrecords.NewManager()
	_ = localMgr.AddRecord(localrecords.NewARecord("test.local.", net.ParseIP("192.168.1.100")))
	handler.SetLocalRecords(localMgr)

	msg := new(dns.Msg)
	msg.SetQuestion("test.local.", dns.TypeA)

	writer := &mockResponseWriter{remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345}}
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		handler.ServeDNS(ctx, writer, msg)
	}
}

// BenchmarkHandler_PolicyBlock benchmarks policy engine blocking
func BenchmarkHandler_PolicyBlock(b *testing.B) {
	handler := NewHandler()

	// Setup policy engine
	policyEngine := policy.NewEngine()
	rule := &policy.Rule{
		Name:    "Block Test",
		Logic:   `Domain == "blocked.test."`,
		Action:  policy.ActionBlock,
		Enabled: true,
	}
	_ = policyEngine.AddRule(rule)
	handler.SetPolicyEngine(policyEngine)

	msg := new(dns.Msg)
	msg.SetQuestion("blocked.test.", dns.TypeA)

	writer := &mockResponseWriter{remoteAddr: &net.UDPAddr{IP: net.ParseIP("192.168.1.100"), Port: 12345}}
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		handler.ServeDNS(ctx, writer, msg)
	}
}

// BenchmarkHandler_BlocklistBlock benchmarks blocklist blocking
func BenchmarkHandler_BlocklistBlock(b *testing.B) {
	handler := NewHandler()
	handler.Blocklist["blocked.test."] = struct{}{}

	msg := new(dns.Msg)
	msg.SetQuestion("blocked.test.", dns.TypeA)

	writer := &mockResponseWriter{remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345}}
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		handler.ServeDNS(ctx, writer, msg)
	}
}

// BenchmarkHandler_WhitelistBypass benchmarks whitelist bypass
func BenchmarkHandler_WhitelistBypass(b *testing.B) {
	handler := NewHandler()
	handler.Blocklist["test.com."] = struct{}{}
	handler.Whitelist["test.com."] = struct{}{}

	// Create minimal forwarder for bypass
	cfg := &config.Config{
		UpstreamDNSServers: []string{"1.1.1.1:53"},
	}
	logger, _ := logging.New(&config.LoggingConfig{
		Level:  "error",
		Format: "text",
		Output: "stdout",
	})
	fwd := forwarder.NewForwarder(cfg, logger)
	handler.SetForwarder(fwd)

	msg := new(dns.Msg)
	msg.SetQuestion("test.com.", dns.TypeA)

	writer := &mockResponseWriter{remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345}}
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		handler.ServeDNS(ctx, writer, msg)
	}
}

// BenchmarkHandler_CacheHit benchmarks cache hit performance
func BenchmarkHandler_CacheHit(b *testing.B) {
	handler := NewHandler()

	// Setup cache
	logger, _ := logging.New(&config.LoggingConfig{
		Level:  "error",
		Format: "text",
		Output: "stdout",
	})
	dnsCache, _ := cache.New(&config.CacheConfig{
		Enabled:    true,
		MaxEntries: 1000,
	}, logger)
	handler.SetCache(dnsCache)

	// Create and cache a response
	msg := new(dns.Msg)
	msg.SetQuestion("cached.test.", dns.TypeA)

	resp := new(dns.Msg)
	resp.SetReply(msg)
	resp.Answer = append(resp.Answer, &dns.A{
		Hdr: dns.RR_Header{
			Name:   "cached.test.",
			Rrtype: dns.TypeA,
			Class:  dns.ClassINET,
			Ttl:    300,
		},
		A: net.ParseIP("192.168.1.1"),
	})

	ctx := context.Background()
	dnsCache.Set(ctx, msg, resp)

	writer := &mockResponseWriter{remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345}}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		handler.ServeDNS(ctx, writer, msg)
	}
}

// BenchmarkHandler_FullStack benchmarks the complete handler stack
func BenchmarkHandler_FullStack(b *testing.B) {
	handler := NewHandler()

	// Setup all components
	logger, _ := logging.New(&config.LoggingConfig{
		Level:  "error",
		Format: "text",
		Output: "stdout",
	})

	// Cache
	dnsCache, _ := cache.New(&config.CacheConfig{
		Enabled:    true,
		MaxEntries: 1000,
	}, logger)
	handler.SetCache(dnsCache)

	// Local records
	localMgr := localrecords.NewManager()
	_ = localMgr.AddRecord(localrecords.NewARecord("local.test.", net.ParseIP("192.168.1.100")))
	handler.SetLocalRecords(localMgr)

	// Policy engine
	policyEngine := policy.NewEngine()
	rule := &policy.Rule{
		Name:    "Block Test",
		Logic:   `Domain == "blocked.test."`,
		Action:  policy.ActionBlock,
		Enabled: true,
	}
	_ = policyEngine.AddRule(rule)
	handler.SetPolicyEngine(policyEngine)

	// Blocklist
	handler.Blocklist["blocklist.test."] = struct{}{}

	// Forwarder
	cfg := &config.Config{
		UpstreamDNSServers: []string{"1.1.1.1:53"},
	}
	fwd := forwarder.NewForwarder(cfg, logger)
	handler.SetForwarder(fwd)

	msg := new(dns.Msg)
	msg.SetQuestion("local.test.", dns.TypeA)

	writer := &mockResponseWriter{remoteAddr: &net.UDPAddr{IP: net.ParseIP("192.168.1.100"), Port: 12345}}
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		handler.ServeDNS(ctx, writer, msg)
	}
}

// BenchmarkHandler_ConcurrentRequests benchmarks concurrent DNS requests
func BenchmarkHandler_ConcurrentRequests(b *testing.B) {
	handler := NewHandler()

	// Setup local records for fast response
	localMgr := localrecords.NewManager()
	_ = localMgr.AddRecord(localrecords.NewARecord("test.local.", net.ParseIP("192.168.1.100")))
	handler.SetLocalRecords(localMgr)

	msg := new(dns.Msg)
	msg.SetQuestion("test.local.", dns.TypeA)

	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		writer := &mockResponseWriter{remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345}}
		for pb.Next() {
			handler.ServeDNS(ctx, writer, msg)
		}
	})
}
