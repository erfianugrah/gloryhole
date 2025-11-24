package cache

import (
	"context"

	"glory-hole/pkg/storage"

	"github.com/miekg/dns"
)

// Interface defines the common operations for all cache implementations.
// Both Cache and ShardedCache implement this interface.
type Interface interface {
	// Get retrieves a cached DNS response for the given request
	Get(ctx context.Context, r *dns.Msg) *dns.Msg

	// GetWithTrace returns the cached response and any associated block trace metadata
	GetWithTrace(ctx context.Context, r *dns.Msg) (*dns.Msg, []storage.BlockTraceEntry)

	// Set stores a DNS response in the cache with appropriate TTL
	Set(ctx context.Context, r *dns.Msg, resp *dns.Msg)

	// SetBlocked stores a blocked domain response in the cache with BlockedTTL
	SetBlocked(ctx context.Context, r *dns.Msg, resp *dns.Msg, trace []storage.BlockTraceEntry)

	// Stats returns current cache statistics
	Stats() Stats

	// Clear removes all entries from the cache
	Clear()

	// Close stops the cache and cleanup goroutine
	Close() error
}

// Verify that both implementations satisfy the interface
var (
	_ Interface = (*Cache)(nil)
	_ Interface = (*ShardedCache)(nil)
)
