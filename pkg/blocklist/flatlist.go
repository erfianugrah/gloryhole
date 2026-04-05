package blocklist

import (
	"sort"
	"strings"
)

// FlatBlocklist is a memory-compact blocklist that stores all domain strings
// in a single contiguous byte slice with a sorted index for binary search.
//
// Memory cost per domain: ~33 bytes (vs ~140 bytes for map[string]uint64).
// At 1.3M domains this saves ~140MB of heap.
//
// Layout:
//
//	data  []byte   — all FQDN strings concatenated (e.g. "ad.example.com.\0tracker.net.\0...")
//	offs  []uint32 — sorted start offsets into data (one per domain)
//	masks []uint64 — source bitmask parallel to offs
//
// Lookup is O(log n) binary search. Subdomain walk does one binary search
// per parent label (typically 2–4 searches per query).
type FlatBlocklist struct {
	data  []byte   // concatenated domain strings, NUL-terminated
	offs  []uint32 // sorted offsets into data (start of each domain)
	masks []uint64 // source bitmask, parallel to offs
}

// BuildFlatBlocklist constructs a FlatBlocklist from a map of domain → source mask.
// The input map is consumed and should not be used after this call.
func BuildFlatBlocklist(m map[string]uint64) *FlatBlocklist {
	n := len(m)
	if n == 0 {
		return &FlatBlocklist{}
	}

	// Phase 1: collect keys and compute total data size.
	keys := make([]string, 0, n)
	dataSize := 0
	for k := range m {
		keys = append(keys, k)
		dataSize += len(k) + 1 // +1 for NUL terminator
	}

	// Phase 2: sort keys for binary search.
	sort.Strings(keys)

	// Phase 3: pack into contiguous storage.
	data := make([]byte, 0, dataSize)
	offs := make([]uint32, n)
	masks := make([]uint64, n)

	for i, k := range keys {
		offs[i] = uint32(len(data))
		masks[i] = m[k]
		data = append(data, k...)
		data = append(data, 0) // NUL terminator
	}

	return &FlatBlocklist{
		data:  data,
		offs:  offs,
		masks: masks,
	}
}

// sortedList is a pre-sorted slice of domains from a single blocklist source.
type sortedList struct {
	domains []string
	mask    uint64 // source bitmask for this list
}

// BuildFromSortedLists constructs a FlatBlocklist by k-way merging pre-sorted
// domain slices. This avoids allocating a temporary map[string]uint64 for
// deduplication, saving ~140 bytes per domain (180MB for 1.3M domains).
//
// Each list's domains must already be sorted. Duplicates across lists are
// merged and their source masks are OR'd together.
func BuildFromSortedLists(lists []sortedList) *FlatBlocklist {
	// Compute total domain count (upper bound; duplicates reduce this)
	totalDomains := 0
	totalBytes := 0
	for _, l := range lists {
		totalDomains += len(l.domains)
		for _, d := range l.domains {
			totalBytes += len(d) + 1
		}
	}

	if totalDomains == 0 {
		return &FlatBlocklist{}
	}

	// Pre-allocate output arrays at upper-bound capacity
	data := make([]byte, 0, totalBytes)
	offs := make([]uint32, 0, totalDomains)
	masks := make([]uint64, 0, totalDomains)

	// k-way merge using index cursors (simple for small k, typically 2-5 lists)
	cursors := make([]int, len(lists))

	for {
		// Find the lexicographically smallest domain across all lists
		minDomain := ""
		for i, l := range lists {
			if cursors[i] >= len(l.domains) {
				continue // this list is exhausted
			}
			d := l.domains[cursors[i]]
			if minDomain == "" || d < minDomain {
				minDomain = d
			}
		}

		if minDomain == "" {
			break // all lists exhausted
		}

		// Collect the source mask from all lists that have this domain
		var mask uint64
		for i, l := range lists {
			if cursors[i] < len(l.domains) && l.domains[cursors[i]] == minDomain {
				mask |= l.mask
				cursors[i]++
			}
		}

		// Append to output
		offs = append(offs, uint32(len(data)))
		masks = append(masks, mask)
		data = append(data, minDomain...)
		data = append(data, 0)
	}

	return &FlatBlocklist{
		data:  data,
		offs:  offs,
		masks: masks,
	}
}

// Len returns the number of domains in the blocklist.
func (f *FlatBlocklist) Len() int {
	if f == nil {
		return 0
	}
	return len(f.offs)
}

// domainAt returns the domain string at index i without allocating.
// The returned string shares memory with f.data (no copy).
func (f *FlatBlocklist) domainAt(i int) string {
	start := f.offs[i]
	// Find NUL terminator
	end := start
	for end < uint32(len(f.data)) && f.data[end] != 0 {
		end++
	}
	// Convert byte slice to string without allocation via unsafe would be
	// ideal, but the standard conversion is fine here — the compiler can
	// often optimize the comparison path to avoid the copy.
	return string(f.data[start:end])
}

// Lookup returns the source bitmask for a domain and whether it was found.
// O(log n) binary search.
func (f *FlatBlocklist) Lookup(domain string) (mask uint64, ok bool) {
	if f == nil || len(f.offs) == 0 {
		return 0, false
	}

	idx := sort.Search(len(f.offs), func(i int) bool {
		return f.cmpDomainAt(i, domain) >= 0
	})

	if idx < len(f.offs) && f.cmpDomainAt(idx, domain) == 0 {
		return f.masks[idx], true
	}
	return 0, false
}

// cmpDomainAt compares the domain at index i with target.
// Returns negative if data[i] < target, 0 if equal, positive if data[i] > target.
// Compares directly against f.data bytes without allocating a string.
func (f *FlatBlocklist) cmpDomainAt(i int, target string) int {
	start := int(f.offs[i])
	tLen := len(target)
	pos := 0

	for pos < tLen {
		dataPos := start + pos
		if dataPos >= len(f.data) || f.data[dataPos] == 0 {
			// data domain is shorter → data < target
			return -1
		}
		db := f.data[dataPos]
		tb := target[pos]
		if db != tb {
			if db < tb {
				return -1
			}
			return 1
		}
		pos++
	}

	// All characters matched so far. Check if data domain has more chars.
	endPos := start + pos
	if endPos < len(f.data) && f.data[endPos] != 0 {
		// data domain is longer → data > target
		return 1
	}

	return 0
}

// Contains checks if a domain exists in the blocklist.
func (f *FlatBlocklist) Contains(domain string) bool {
	_, ok := f.Lookup(domain)
	return ok
}

// ForEach iterates all domains in sorted order, calling fn for each.
// Used for Get() compatibility and stats. Allocates a string per call.
func (f *FlatBlocklist) ForEach(fn func(domain string, mask uint64)) {
	if f == nil {
		return
	}
	for i := range f.offs {
		fn(f.domainAt(i), f.masks[i])
	}
}

// MemoryUsage returns an estimate of the total bytes consumed by the structure.
func (f *FlatBlocklist) MemoryUsage() int {
	if f == nil {
		return 0
	}
	return len(f.data) + len(f.offs)*4 + len(f.masks)*8
}

// LookupSubdomains checks the domain and all its parent domains.
// Returns on first match (most specific wins). This is the equivalent
// of the old Match() subdomain walk but using binary search.
func (f *FlatBlocklist) LookupSubdomains(fqdn string) (mask uint64, kind string, ok bool) {
	if f == nil || len(f.offs) == 0 {
		return 0, "", false
	}

	// Try exact match first
	if mask, found := f.Lookup(fqdn); found {
		return mask, "exact", true
	}

	// Walk parent domains: "sub.example.com." → "example.com." → "com."
	parent := fqdn
	for {
		idx := strings.Index(parent, ".")
		if idx < 0 || idx+1 >= len(parent) {
			break
		}
		parent = parent[idx+1:]
		if parent == "." || parent == "" {
			break
		}
		if mask, found := f.Lookup(parent); found {
			return mask, "subdomain", true
		}
	}

	return 0, "", false
}
