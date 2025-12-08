package dns

import (
    "context"
    "fmt"
    "strings"

    "glory-hole/pkg/blocklist"
    "glory-hole/pkg/storage"

    "go.opentelemetry.io/otel/attribute"
    "go.opentelemetry.io/otel/metric"
)

// blockMetadata contains metadata for recording blocked query metrics.
type blockMetadata struct {
    reason     string
    qtypeLabel string
    stage      string
    rule       string
    source     string
}

// recordRateLimit captures rate limit violations and drops with consistent attributes.
func (h *Handler) recordRateLimit(ctx context.Context, clientIP, qtypeLabel, action string, dropped bool) {
    if h.Metrics == nil {
        return
    }
    attrs := make([]attribute.KeyValue, 0, 3)
    if clientIP != "" {
        attrs = append(attrs, attribute.String("client", clientIP))
    }
    if qtypeLabel != "" {
        attrs = append(attrs, attribute.String("type", qtypeLabel))
    }
    if action != "" {
        attrs = append(attrs, attribute.String("action", action))
    }
    h.Metrics.RateLimitViolations.Add(ctx, 1, metric.WithAttributes(attrs...))
    if dropped {
        h.Metrics.RateLimitDropped.Add(ctx, 1, metric.WithAttributes(attrs...))
    }
}

// recordBlockedQuery increments the blocked-query counter with contextual attributes for better observability.
func (h *Handler) recordBlockedQuery(ctx context.Context, meta blockMetadata) {
    if h.Metrics == nil {
        return
    }
    attrs := make([]attribute.KeyValue, 0, 5)

    if meta.reason != "" {
        attrs = append(attrs, attribute.String("reason", meta.reason))
    }
    if meta.qtypeLabel != "" {
        attrs = append(attrs, attribute.String("type", meta.qtypeLabel))
    }
    if meta.stage != "" {
        attrs = append(attrs, attribute.String("stage", meta.stage))
    }
    if meta.rule != "" {
        attrs = append(attrs, attribute.String("rule", meta.rule))
    }
    if meta.source != "" {
        attrs = append(attrs, attribute.String("source", meta.source))
    }

    if len(attrs) == 0 {
        h.Metrics.DNSBlockedQueries.Add(ctx, 1)
        return
    }
    h.Metrics.DNSBlockedQueries.Add(ctx, 1, metric.WithAttributes(attrs...))
}

func blocklistTraceSource(match blocklist.MatchResult) string {
    if len(match.Sources) > 0 {
        return match.Sources[0]
    }
    if match.Kind != "" {
        return match.Kind
    }
    return ""
}

func describeBlockMatch(match blocklist.MatchResult) string {
    if !match.Blocked {
        return ""
    }
    kind := titleCase(match.Kind)
    switch {
    case match.Pattern != "" && kind != "":
        return fmt.Sprintf("Matched %s pattern %s", strings.ToLower(kind), match.Pattern)
    case match.Pattern != "":
        return fmt.Sprintf("Matched pattern %s", match.Pattern)
    case kind != "":
        return fmt.Sprintf("Matched %s entry", strings.ToLower(kind))
    case len(match.Sources) > 0:
        return fmt.Sprintf("Blocked by %s", match.Sources[0])
    default:
        return ""
    }
}

func applyBlockMatchMetadata(entry *storage.BlockTraceEntry, match blocklist.MatchResult) {
    if !match.Blocked {
        return
    }
    if entry.Metadata == nil {
        entry.Metadata = make(map[string]string)
    }
    if len(match.Sources) > 0 {
        entry.Metadata["lists"] = strings.Join(match.Sources, ", ")
    }
    if match.Kind != "" {
        entry.Metadata["match_kind"] = titleCase(match.Kind)
    }
    if match.Pattern != "" {
        entry.Metadata["pattern"] = match.Pattern
    }
    if len(entry.Metadata) == 0 {
        entry.Metadata = nil
    }
}

func titleCase(value string) string {
    if value == "" {
        return ""
    }
    if len(value) == 1 {
        return strings.ToUpper(value)
    }
    return strings.ToUpper(value[:1]) + strings.ToLower(value[1:])
}

// recordForwardedQuery increments the forwarded-query counter tagged with path/upstream metadata.
func (h *Handler) recordForwardedQuery(ctx context.Context, path, qtypeLabel, upstream string) {
    if h.Metrics == nil {
        return
    }
    attrs := make([]attribute.KeyValue, 0, 3)
    if path != "" {
        attrs = append(attrs, attribute.String("path", path))
    }
    if qtypeLabel != "" {
        attrs = append(attrs, attribute.String("type", qtypeLabel))
    }
    if upstream != "" {
        attrs = append(attrs, attribute.String("upstream", upstream))
    }
    if len(attrs) == 0 {
        h.Metrics.DNSForwardedQueries.Add(ctx, 1)
        return
    }
    h.Metrics.DNSForwardedQueries.Add(ctx, 1, metric.WithAttributes(attrs...))
}
