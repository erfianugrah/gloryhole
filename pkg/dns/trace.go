package dns

import "glory-hole/pkg/storage"

const (
	traceStagePolicy    = "policy"
	traceStageBlocklist = "blocklist"
	traceStageWhitelist = "whitelist"
	traceStageRateLimit = "rate_limit"
	traceStageCache     = "cache"
)

type blockTraceRecorder struct {
	enabled bool
	entries []storage.BlockTraceEntry
}

func newBlockTraceRecorder(enabled bool) *blockTraceRecorder {
	return &blockTraceRecorder{enabled: enabled}
}

func (r *blockTraceRecorder) Record(stage, action string, mutate func(*storage.BlockTraceEntry)) {
	if !r.enabled {
		return
	}

	entry := storage.BlockTraceEntry{
		Stage:  stage,
		Action: action,
	}

	if mutate != nil {
		mutate(&entry)
	}

	r.entries = append(r.entries, entry)
}

func (r *blockTraceRecorder) Entries() []storage.BlockTraceEntry {
	if !r.enabled || len(r.entries) == 0 {
		return nil
	}

	return cloneTraceEntries(r.entries)
}

func (r *blockTraceRecorder) Append(entries []storage.BlockTraceEntry) {
	if !r.enabled || len(entries) == 0 {
		return
	}

	r.entries = append(r.entries, cloneTraceEntries(entries)...)
}

func cloneTraceEntries(entries []storage.BlockTraceEntry) []storage.BlockTraceEntry {
	if len(entries) == 0 {
		return nil
	}

	out := make([]storage.BlockTraceEntry, len(entries))
	copy(out, entries)
	return out
}
