package dns

import (
	"testing"

	"github.com/miekg/dns"
)

func TestGetEDNSInfo_NoEDNS(t *testing.T) {
	req := new(dns.Msg)
	req.SetQuestion("example.com.", dns.TypeA)

	info := GetEDNSInfo(req)

	if info.Present {
		t.Error("expected EDNS0 to not be present")
	}
}

func TestGetEDNSInfo_WithEDNS(t *testing.T) {
	req := new(dns.Msg)
	req.SetQuestion("example.com.", dns.TypeA)

	// Add EDNS0 OPT record
	opt := &dns.OPT{
		Hdr: dns.RR_Header{
			Name:   ".",
			Rrtype: dns.TypeOPT,
		},
	}
	opt.SetUDPSize(4096)
	opt.SetDo() // Set DNSSEC OK bit
	req.Extra = append(req.Extra, opt)

	info := GetEDNSInfo(req)

	if !info.Present {
		t.Fatal("expected EDNS0 to be present")
	}

	if info.BufferSize != 4096 {
		t.Errorf("expected buffer size 4096, got %d", info.BufferSize)
	}

	if !info.DO {
		t.Error("expected DNSSEC OK bit to be set")
	}
}

func TestGetEDNSInfo_NilRequest(t *testing.T) {
	info := GetEDNSInfo(nil)

	if info.Present {
		t.Error("expected EDNS0 to not be present for nil request")
	}
}

func TestSetEDNS0_NoEDNSInRequest(t *testing.T) {
	resp := new(dns.Msg)
	reqInfo := &EDNSInfo{
		Present: false,
	}

	SetEDNS0(resp, reqInfo)

	// Check that no OPT record was added
	if opt := resp.IsEdns0(); opt != nil {
		t.Error("expected no EDNS0 in response when request had no EDNS0")
	}
}

func TestSetEDNS0_WithEDNS(t *testing.T) {
	resp := new(dns.Msg)
	reqInfo := &EDNSInfo{
		Present:    true,
		BufferSize: 4096,
		DO:         true,
	}

	SetEDNS0(resp, reqInfo)

	// Check that OPT record was added
	opt := resp.IsEdns0()
	if opt == nil {
		t.Fatal("expected EDNS0 in response")
	}

	if opt.UDPSize() != 4096 {
		t.Errorf("expected buffer size 4096, got %d", opt.UDPSize())
	}

	if !opt.Do() {
		t.Error("expected DNSSEC OK bit to be set")
	}
}

func TestSetEDNS0_NilResponse(t *testing.T) {
	reqInfo := &EDNSInfo{
		Present: true,
	}

	// Should not panic
	SetEDNS0(nil, reqInfo)
}

func TestSetEDNS0_NilReqInfo(t *testing.T) {
	resp := new(dns.Msg)

	// Should not panic and not add EDNS0
	SetEDNS0(resp, nil)

	if opt := resp.IsEdns0(); opt != nil {
		t.Error("expected no EDNS0 when reqInfo is nil")
	}
}

func TestNegotiateBufferSize_Default(t *testing.T) {
	size := negotiateBufferSize(0)
	if size != DefaultEDNSBufferSize {
		t.Errorf("expected default buffer size %d, got %d", DefaultEDNSBufferSize, size)
	}
}

func TestNegotiateBufferSize_TooSmall(t *testing.T) {
	size := negotiateBufferSize(256)
	if size != MinEDNSBufferSize {
		t.Errorf("expected minimum buffer size %d, got %d", MinEDNSBufferSize, size)
	}
}

func TestNegotiateBufferSize_TooLarge(t *testing.T) {
	size := negotiateBufferSize(65535)
	if size != MaxEDNSBufferSize {
		t.Errorf("expected maximum buffer size %d, got %d", MaxEDNSBufferSize, size)
	}
}

func TestNegotiateBufferSize_Valid(t *testing.T) {
	requested := uint16(2048)
	size := negotiateBufferSize(requested)
	if size != requested {
		t.Errorf("expected buffer size %d, got %d", requested, size)
	}
}

func TestHandleEDNS0_RequestWithoutEDNS(t *testing.T) {
	req := new(dns.Msg)
	req.SetQuestion("example.com.", dns.TypeA)

	resp := new(dns.Msg)
	resp.SetReply(req)

	HandleEDNS0(req, resp)

	// Response should not have EDNS0
	if opt := resp.IsEdns0(); opt != nil {
		t.Error("expected no EDNS0 in response when request had no EDNS0")
	}
}

func TestHandleEDNS0_RequestWithEDNS(t *testing.T) {
	req := new(dns.Msg)
	req.SetQuestion("example.com.", dns.TypeA)

	// Add EDNS0 to request
	opt := &dns.OPT{
		Hdr: dns.RR_Header{
			Name:   ".",
			Rrtype: dns.TypeOPT,
		},
	}
	opt.SetUDPSize(2048)
	opt.SetDo()
	req.Extra = append(req.Extra, opt)

	resp := new(dns.Msg)
	resp.SetReply(req)

	HandleEDNS0(req, resp)

	// Response should have EDNS0
	respOpt := resp.IsEdns0()
	if respOpt == nil {
		t.Fatal("expected EDNS0 in response")
	}

	if respOpt.UDPSize() != 2048 {
		t.Errorf("expected buffer size 2048, got %d", respOpt.UDPSize())
	}

	if !respOpt.Do() {
		t.Error("expected DNSSEC OK bit to be set")
	}
}

func TestHandleEDNS0_PreservesDOBit(t *testing.T) {
	req := new(dns.Msg)
	req.SetQuestion("example.com.", dns.TypeA)

	// Add EDNS0 without DO bit
	opt := &dns.OPT{
		Hdr: dns.RR_Header{
			Name:   ".",
			Rrtype: dns.TypeOPT,
		},
	}
	opt.SetUDPSize(4096)
	// Explicitly not setting DO bit
	req.Extra = append(req.Extra, opt)

	resp := new(dns.Msg)
	resp.SetReply(req)

	HandleEDNS0(req, resp)

	// Response should have EDNS0 but DO bit should not be set
	respOpt := resp.IsEdns0()
	if respOpt == nil {
		t.Fatal("expected EDNS0 in response")
	}

	if respOpt.Do() {
		t.Error("expected DNSSEC OK bit to not be set")
	}
}

func TestHandleEDNS0_BufferSizeNegotiation(t *testing.T) {
	testCases := []struct {
		name           string
		requestedSize  uint16
		expectedSize   uint16
	}{
		{
			name:          "Zero size",
			requestedSize: 0,
			expectedSize:  DefaultEDNSBufferSize,
		},
		{
			name:          "Below minimum",
			requestedSize: 256,
			expectedSize:  MinEDNSBufferSize,
		},
		{
			name:          "Above maximum",
			requestedSize: 65535,
			expectedSize:  MaxEDNSBufferSize,
		},
		{
			name:          "Valid size",
			requestedSize: 1024,
			expectedSize:  1024,
		},
		{
			name:          "At maximum",
			requestedSize: MaxEDNSBufferSize,
			expectedSize:  MaxEDNSBufferSize,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := new(dns.Msg)
			req.SetQuestion("example.com.", dns.TypeA)

			// Add EDNS0 with specified buffer size
			opt := &dns.OPT{
				Hdr: dns.RR_Header{
					Name:   ".",
					Rrtype: dns.TypeOPT,
				},
			}
			opt.SetUDPSize(tc.requestedSize)
			req.Extra = append(req.Extra, opt)

			resp := new(dns.Msg)
			resp.SetReply(req)

			HandleEDNS0(req, resp)

			respOpt := resp.IsEdns0()
			if respOpt == nil {
				t.Fatal("expected EDNS0 in response")
			}

			if respOpt.UDPSize() != tc.expectedSize {
				t.Errorf("expected buffer size %d, got %d", tc.expectedSize, respOpt.UDPSize())
			}
		})
	}
}
