package unbound

import (
	"strings"
	"sync"
	"time"
)

// ReplyBuffer is a fixed-size circular buffer that stores recent
// CLIENT_RESPONSE dnstap entries for inline enrichment of Glory-Hole's
// query log. The DNS handler checks this buffer after receiving a
// response from Unbound to populate Unbound-specific fields.
type ReplyBuffer struct {
	mu      sync.Mutex
	entries []replyEntry
	head    int
	size    int
}

type replyEntry struct {
	timestamp       time.Time
	domain          string
	queryType       string
	cachedInUnbound bool
	durationMs      float64
	dnssecValidated bool
	responseSize    int
}

// ReplyMatch contains Unbound resolution details matched from the ring buffer.
type ReplyMatch struct {
	CachedInUnbound bool
	DurationMs      float64
	DNSSECValidated bool
	ResponseSize    int
}

// NewReplyBuffer creates a ring buffer with the given capacity.
func NewReplyBuffer(capacity int) *ReplyBuffer {
	return &ReplyBuffer{
		entries: make([]replyEntry, capacity),
	}
}

// Add inserts a CLIENT_RESPONSE entry into the buffer.
func (b *ReplyBuffer) Add(entry *UnboundQueryLog) {
	if entry == nil || entry.MessageType != "CLIENT_RESPONSE" {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.entries[b.head] = replyEntry{
		timestamp:       entry.Timestamp,
		domain:          strings.ToLower(entry.Domain),
		queryType:       entry.QueryType,
		cachedInUnbound: entry.CachedInUnbound,
		durationMs:      entry.DurationMs,
		dnssecValidated: entry.DNSSECValidated,
		responseSize:    entry.ResponseSize,
	}
	b.head = (b.head + 1) % len(b.entries)
	if b.size < len(b.entries) {
		b.size++
	}
}

// FindReply searches backward for a matching entry within maxAge.
// Returns nil if no match found. Matches on (domain, queryType).
func (b *ReplyBuffer) FindReply(domain, queryType string, maxAge time.Duration) *ReplyMatch {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.size == 0 {
		return nil
	}

	cutoff := time.Now().Add(-maxAge)
	domainLower := strings.ToLower(domain)

	// Scan backward from the most recent entry
	for i := 0; i < b.size; i++ {
		idx := (b.head - 1 - i + len(b.entries)) % len(b.entries)
		e := &b.entries[idx]

		// Skip expired entries — everything older is also expired
		if e.timestamp.Before(cutoff) {
			break
		}

		if e.domain == domainLower && e.queryType == queryType {
			return &ReplyMatch{
				CachedInUnbound: e.cachedInUnbound,
				DurationMs:      e.durationMs,
				DNSSECValidated: e.dnssecValidated,
				ResponseSize:    e.responseSize,
			}
		}
	}

	return nil
}
