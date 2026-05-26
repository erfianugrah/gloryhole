package dns

import (
	"net"

	"github.com/miekg/dns"
)

const overrideTTL = 300

func addARecord(msg *dns.Msg, domain string, ip net.IP, ttl uint32) {
	if ip == nil || ip.To4() == nil {
		return
	}
	rr := &dns.A{
		Hdr: dns.RR_Header{
			Name:   domain,
			Rrtype: dns.TypeA,
			Class:  dns.ClassINET,
			Ttl:    ttl,
		},
		A: ip.To4(),
	}
	msg.Answer = append(msg.Answer, rr)
}

func addAAAARecord(msg *dns.Msg, domain string, ip net.IP, ttl uint32) {
	if ip == nil || ip.To16() == nil || ip.To4() != nil {
		return
	}
	rr := &dns.AAAA{
		Hdr: dns.RR_Header{
			Name:   domain,
			Rrtype: dns.TypeAAAA,
			Class:  dns.ClassINET,
			Ttl:    ttl,
		},
		AAAA: ip.To16(),
	}
	msg.Answer = append(msg.Answer, rr)
}

func addCNAMERecord(msg *dns.Msg, domain, target string, ttl uint32) {
	rr := &dns.CNAME{
		Hdr: dns.RR_Header{
			Name:   domain,
			Rrtype: dns.TypeCNAME,
			Class:  dns.ClassINET,
			Ttl:    ttl,
		},
		Target: target,
	}
	msg.Answer = append(msg.Answer, rr)
}
