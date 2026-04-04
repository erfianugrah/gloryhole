package unbound

import "time"

// UnboundQueryLog represents a single dnstap event from Unbound.
// Each DNS interaction produces multiple events (e.g., CLIENT_QUERY +
// CLIENT_RESPONSE, and optionally RESOLVER_QUERY + RESOLVER_RESPONSE
// if Unbound went recursive).
type UnboundQueryLog struct {
	ID              int64     `json:"id"`
	Timestamp       time.Time `json:"timestamp"`
	MessageType     string    `json:"message_type"` // CLIENT_QUERY, CLIENT_RESPONSE, RESOLVER_QUERY, RESOLVER_RESPONSE
	Domain          string    `json:"domain"`
	QueryType       string    `json:"query_type"`
	ResponseCode    string    `json:"response_code,omitempty"`
	DurationMs      float64   `json:"duration_ms,omitempty"` // Query-to-response time (CLIENT_RESPONSE only)
	DNSSECValidated bool      `json:"dnssec_validated"`
	AnswerCount     int       `json:"answer_count,omitempty"`
	ResponseSize    int       `json:"response_size,omitempty"`
	ClientIP        string    `json:"client_ip"`           // 127.0.0.1 for CLIENT_*, real NS IP for RESOLVER_*
	ServerIP        string    `json:"server_ip,omitempty"` // Authoritative NS IP for RESOLVER_*
	CachedInUnbound bool      `json:"cached_in_unbound"`   // True when CLIENT_RESPONSE with no RESOLVER_QUERY
}

// UnboundQueryFilter holds filtering options for querying the unbound_queries table.
type UnboundQueryFilter struct {
	Domain      string `json:"domain,omitempty"`
	QueryType   string `json:"query_type,omitempty"`
	MessageType string `json:"message_type,omitempty"`
	RCode       string `json:"rcode,omitempty"`
	Cached      *bool  `json:"cached,omitempty"`
	Start       string `json:"start,omitempty"`
	End         string `json:"end,omitempty"`
}

// UnboundQueryStats holds aggregated statistics from the unbound_queries table.
type UnboundQueryStats struct {
	TotalQueries       int64            `json:"total_queries"`
	CacheHits          int64            `json:"cache_hits"`
	CacheHitRate       float64          `json:"cache_hit_rate"`
	RecursiveQueries   int64            `json:"recursive_queries"`
	AvgRecursiveMs     float64          `json:"avg_recursive_ms"`
	AvgCachedMs        float64          `json:"avg_cached_ms"`
	DNSSECValidatedPct float64          `json:"dnssec_validated_pct"`
	ResponseCodes      map[string]int64 `json:"response_codes"`
}
