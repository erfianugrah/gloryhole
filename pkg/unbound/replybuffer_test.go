package unbound

import (
	"testing"
	"time"
)

func TestReplyBuffer_AddAndFind(t *testing.T) {
	buf := NewReplyBuffer(100)

	entry := &UnboundQueryLog{
		Timestamp:       time.Now(),
		MessageType:     "CLIENT_RESPONSE",
		Domain:          "example.com.",
		QueryType:       "A",
		CachedInUnbound: false,
		DurationMs:      45.2,
		DNSSECValidated: true,
		ResponseSize:    128,
	}
	buf.Add(entry)

	match := buf.FindReply("example.com.", "A", 2*time.Second)
	if match == nil {
		t.Fatal("expected match, got nil")
	}
	if match.CachedInUnbound {
		t.Error("expected cached=false")
	}
	if match.DurationMs != 45.2 {
		t.Errorf("duration = %f, want 45.2", match.DurationMs)
	}
	if !match.DNSSECValidated {
		t.Error("expected dnssec=true")
	}
	if match.ResponseSize != 128 {
		t.Errorf("response_size = %d, want 128", match.ResponseSize)
	}
}

func TestReplyBuffer_CaseInsensitive(t *testing.T) {
	buf := NewReplyBuffer(100)

	buf.Add(&UnboundQueryLog{
		Timestamp:   time.Now(),
		MessageType: "CLIENT_RESPONSE",
		Domain:      "Example.COM.",
		QueryType:   "AAAA",
		DurationMs:  1.5,
	})

	match := buf.FindReply("example.com.", "AAAA", 2*time.Second)
	if match == nil {
		t.Fatal("expected case-insensitive match")
	}
}

func TestReplyBuffer_NoMatchWrongType(t *testing.T) {
	buf := NewReplyBuffer(100)

	buf.Add(&UnboundQueryLog{
		Timestamp:   time.Now(),
		MessageType: "CLIENT_RESPONSE",
		Domain:      "example.com.",
		QueryType:   "A",
	})

	match := buf.FindReply("example.com.", "AAAA", 2*time.Second)
	if match != nil {
		t.Error("expected no match for wrong query type")
	}
}

func TestReplyBuffer_Expiry(t *testing.T) {
	buf := NewReplyBuffer(100)

	buf.Add(&UnboundQueryLog{
		Timestamp:   time.Now().Add(-5 * time.Second),
		MessageType: "CLIENT_RESPONSE",
		Domain:      "example.com.",
		QueryType:   "A",
	})

	match := buf.FindReply("example.com.", "A", 1*time.Second)
	if match != nil {
		t.Error("expected nil for expired entry")
	}
}

func TestReplyBuffer_Wraparound(t *testing.T) {
	buf := NewReplyBuffer(3) // tiny buffer

	for i := 0; i < 5; i++ {
		buf.Add(&UnboundQueryLog{
			Timestamp:   time.Now(),
			MessageType: "CLIENT_RESPONSE",
			Domain:      "old.com.",
			QueryType:   "A",
			DurationMs:  float64(i),
		})
	}

	// Add the one we want to find
	buf.Add(&UnboundQueryLog{
		Timestamp:       time.Now(),
		MessageType:     "CLIENT_RESPONSE",
		Domain:          "new.com.",
		QueryType:       "A",
		CachedInUnbound: true,
	})

	match := buf.FindReply("new.com.", "A", 2*time.Second)
	if match == nil {
		t.Fatal("expected match after wraparound")
	}
	if !match.CachedInUnbound {
		t.Error("expected cached=true")
	}
}

func TestReplyBuffer_IgnoresNonResponse(t *testing.T) {
	buf := NewReplyBuffer(100)

	buf.Add(&UnboundQueryLog{
		Timestamp:   time.Now(),
		MessageType: "CLIENT_QUERY", // Not a response
		Domain:      "example.com.",
		QueryType:   "A",
	})

	match := buf.FindReply("example.com.", "A", 2*time.Second)
	if match != nil {
		t.Error("expected nil for non-response entry")
	}
}
