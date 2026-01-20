package dns

import (
	"sync"
	"time"
)

// serveDNSOutcome captures the mutable fields that downstream helpers update
// while ServeDNS orchestrates the request lifecycle.
type serveDNSOutcome struct {
	blocked          bool
	cached           bool
	upstream         string
	responseCode     int
	upstreamDuration time.Duration
}

// outcomePool provides object pooling for serveDNSOutcome to reduce allocations.
var outcomePool = sync.Pool{
	New: func() interface{} {
		return &serveDNSOutcome{}
	},
}

// getOutcome retrieves an outcome from the pool and resets it.
func getOutcome() *serveDNSOutcome {
	o := outcomePool.Get().(*serveDNSOutcome)
	*o = serveDNSOutcome{} // Zero out all fields
	return o
}

// releaseOutcome returns an outcome to the pool for reuse.
func releaseOutcome(o *serveDNSOutcome) {
	if o == nil {
		return
	}
	// Clear string to avoid holding references
	o.upstream = ""
	outcomePool.Put(o)
}
