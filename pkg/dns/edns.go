package dns

import (
	"github.com/miekg/dns"
)

// EDNS0 constants
const (
	// DefaultEDNSBufferSize is the default UDP buffer size for EDNS0 responses
	// RFC 6891 recommends 4096 bytes as a safe default
	DefaultEDNSBufferSize = 4096

	// MaxEDNSBufferSize is the maximum UDP buffer size we'll advertise
	// Conservative limit to avoid fragmentation issues
	MaxEDNSBufferSize = 4096

	// MinEDNSBufferSize is the minimum buffer size we'll accept
	MinEDNSBufferSize = 512
)

// EDNSInfo holds EDNS0 information from a DNS request
type EDNSInfo struct {
	Present    bool   // Whether EDNS0 is present in the request
	Version    uint8  // EDNS version (should be 0)
	BufferSize uint16 // Requested UDP payload size
	DO         bool   // DNSSEC OK bit
}

// GetEDNSInfo extracts EDNS0 information from a DNS request
func GetEDNSInfo(req *dns.Msg) *EDNSInfo {
	info := &EDNSInfo{
		Present: false,
	}

	if req == nil {
		return info
	}

	// Check for OPT record (EDNS0)
	if opt := req.IsEdns0(); opt != nil {
		info.Present = true
		info.Version = opt.Version()
		info.BufferSize = opt.UDPSize()
		info.DO = opt.Do()
	}

	return info
}

// SetEDNS0 adds an EDNS0 OPT record to the response message
// Only adds EDNS0 if the request had EDNS0
func SetEDNS0(resp *dns.Msg, reqInfo *EDNSInfo) {
	if resp == nil || reqInfo == nil || !reqInfo.Present {
		return
	}

	// Determine buffer size
	bufferSize := negotiateBufferSize(reqInfo.BufferSize)

	// Check if response already has an OPT record (e.g., from cache or upstream)
	if resp.IsEdns0() != nil {
		// Already has EDNS0, don't add another one
		return
	}

	// Create OPT record
	// Note: Do NOT set Class field manually - it represents UDP payload size for OPT records
	// and is automatically set by SetUDPSize()
	opt := &dns.OPT{
		Hdr: dns.RR_Header{
			Name:   ".",
			Rrtype: dns.TypeOPT,
		},
	}

	// Set UDP payload size (this sets the Class field internally)
	opt.SetUDPSize(bufferSize)

	// Preserve DNSSEC OK bit from request
	if reqInfo.DO {
		opt.SetDo()
	}

	// Add OPT record to additional section
	resp.Extra = append(resp.Extra, opt)
}

// negotiateBufferSize determines the appropriate buffer size for EDNS0
// Takes the smaller of the requested size and our maximum
func negotiateBufferSize(requested uint16) uint16 {
	if requested == 0 {
		return DefaultEDNSBufferSize
	}

	if requested < MinEDNSBufferSize {
		return MinEDNSBufferSize
	}

	if requested > MaxEDNSBufferSize {
		return MaxEDNSBufferSize
	}

	return requested
}

// HandleEDNS0 is a convenience function that extracts EDNS info from request
// and applies it to the response
func HandleEDNS0(req *dns.Msg, resp *dns.Msg) {
	ednsInfo := GetEDNSInfo(req)
	SetEDNS0(resp, ednsInfo)
}

// ExtractEDE extracts the Extended DNS Error (RFC 8914) from a DNS response.
// Returns the info code, human-readable text, and whether EDE was present.
func ExtractEDE(resp *dns.Msg) (uint16, string, bool) {
	if resp == nil {
		return 0, "", false
	}
	opt := resp.IsEdns0()
	if opt == nil {
		return 0, "", false
	}
	for _, o := range opt.Option {
		if ede, ok := o.(*dns.EDNS0_EDE); ok {
			return ede.InfoCode, ede.ExtraText, true
		}
	}
	return 0, "", false
}

// EDECodeToString returns a human-readable name for an EDE info code (RFC 8914).
func EDECodeToString(code uint16) string {
	switch code {
	case 0:
		return "Other"
	case 1:
		return "Unsupported DNSKEY Algorithm"
	case 2:
		return "Unsupported DS Digest Type"
	case 3:
		return "Stale Answer"
	case 4:
		return "Forged Answer"
	case 5:
		return "DNSSEC Indeterminate"
	case 6:
		return "DNSSEC Bogus"
	case 7:
		return "Signature Expired"
	case 8:
		return "Signature Not Yet Valid"
	case 9:
		return "DNSKEY Missing"
	case 10:
		return "RRSIGs Missing"
	case 11:
		return "No Zone Key Bit Set"
	case 12:
		return "NSEC Missing"
	case 13:
		return "Cached Error"
	case 14:
		return "Not Ready"
	case 15:
		return "Blocked"
	case 16:
		return "Censored"
	case 17:
		return "Filtered"
	case 18:
		return "Prohibited"
	case 19:
		return "Stale NXDOMAIN Answer"
	case 20:
		return "Not Authoritative"
	case 21:
		return "Not Supported"
	case 22:
		return "No Reachable Authority"
	case 23:
		return "Network Error"
	case 24:
		return "Invalid Data"
	default:
		return "Unknown"
	}
}
