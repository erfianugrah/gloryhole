package dns

import "time"

// serveDNSOutcome captures the mutable fields that downstream helpers update
// while ServeDNS orchestrates the request lifecycle.
type serveDNSOutcome struct {
	blocked          bool
	cached           bool
	upstream         string
	responseCode     int
	upstreamDuration time.Duration
}
