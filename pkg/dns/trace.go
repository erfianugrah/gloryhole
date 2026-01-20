package dns

import (
	"sync"

	"glory-hole/pkg/storage"
)

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

// traceRecorderPool provides object pooling for blockTraceRecorder to reduce allocations.
// Pre-allocates slice capacity to avoid reallocations for typical trace sizes.
var traceRecorderPool = sync.Pool{
	New: func() interface{} {
		return &blockTraceRecorder{
			entries: make([]storage.BlockTraceEntry, 0, 4),
		}
	},
}

func newBlockTraceRecorder(enabled bool) *blockTraceRecorder {
	r := traceRecorderPool.Get().(*blockTraceRecorder)
	r.enabled = enabled
	r.entries = r.entries[:0] // Reset slice but keep capacity
	return r
}

// Release returns the recorder to the pool for reuse.
// Must be called when the recorder is no longer needed.
func (r *blockTraceRecorder) Release() {
	if r == nil {
		return
	}
	// Clear entries to avoid holding references
	for i := range r.entries {
		r.entries[i] = storage.BlockTraceEntry{}
	}
	r.entries = r.entries[:0]
	r.enabled = false
	traceRecorderPool.Put(r)
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
