package dns

import "github.com/miekg/dns"

func (h *Handler) serveFromLocalRecords(w dns.ResponseWriter, msg *dns.Msg, domain string, qtype uint16, outcome *serveDNSOutcome) bool {
	if h.LocalRecords == nil {
		return false
	}

	switch qtype {
	case dns.TypeA:
		if h.appendLocalARecords(msg, domain) {
			outcome.responseCode = dns.RcodeSuccess
			h.writeMsg(w, msg)
			return true
		}
		if h.resolveLocalCNAMEAsA(msg, domain) {
			outcome.responseCode = dns.RcodeSuccess
			h.writeMsg(w, msg)
			return true
		}
	case dns.TypeAAAA:
		if h.appendLocalAAAARecords(msg, domain) {
			outcome.responseCode = dns.RcodeSuccess
			h.writeMsg(w, msg)
			return true
		}
		if h.resolveLocalCNAMEAsAAAA(msg, domain) {
			outcome.responseCode = dns.RcodeSuccess
			h.writeMsg(w, msg)
			return true
		}
	case dns.TypeCNAME:
		if target, ttl, found := h.LocalRecords.LookupCNAME(domain); found {
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
			outcome.responseCode = dns.RcodeSuccess
			h.writeMsg(w, msg)
			return true
		}
	case dns.TypeTXT:
		if records := h.LocalRecords.LookupTXT(domain); len(records) > 0 {
			for _, rec := range records {
				rr := &dns.TXT{
					Hdr: dns.RR_Header{
						Name:   domain,
						Rrtype: dns.TypeTXT,
						Class:  dns.ClassINET,
						Ttl:    rec.TTL,
					},
					Txt: rec.TxtRecords,
				}
				msg.Answer = append(msg.Answer, rr)
			}
			outcome.responseCode = dns.RcodeSuccess
			h.writeMsg(w, msg)
			return true
		}
	case dns.TypeMX:
		if records := h.LocalRecords.LookupMX(domain); len(records) > 0 {
			for _, rec := range records {
				rr := &dns.MX{
					Hdr: dns.RR_Header{
						Name:   domain,
						Rrtype: dns.TypeMX,
						Class:  dns.ClassINET,
						Ttl:    rec.TTL,
					},
					Preference: rec.Priority,
					Mx:         rec.Target,
				}
				msg.Answer = append(msg.Answer, rr)
			}
			outcome.responseCode = dns.RcodeSuccess
			h.writeMsg(w, msg)
			return true
		}
	case dns.TypePTR:
		if records := h.LocalRecords.LookupPTR(domain); len(records) > 0 {
			for _, rec := range records {
				rr := &dns.PTR{
					Hdr: dns.RR_Header{
						Name:   domain,
						Rrtype: dns.TypePTR,
						Class:  dns.ClassINET,
						Ttl:    rec.TTL,
					},
					Ptr: rec.Target,
				}
				msg.Answer = append(msg.Answer, rr)
			}
			outcome.responseCode = dns.RcodeSuccess
			h.writeMsg(w, msg)
			return true
		}
	case dns.TypeSRV:
		if records := h.LocalRecords.LookupSRV(domain); len(records) > 0 {
			for _, rec := range records {
				rr := &dns.SRV{
					Hdr: dns.RR_Header{
						Name:   domain,
						Rrtype: dns.TypeSRV,
						Class:  dns.ClassINET,
						Ttl:    rec.TTL,
					},
					Priority: rec.Priority,
					Weight:   rec.Weight,
					Port:     rec.Port,
					Target:   rec.Target,
				}
				msg.Answer = append(msg.Answer, rr)
			}
			outcome.responseCode = dns.RcodeSuccess
			h.writeMsg(w, msg)
			return true
		}
	case dns.TypeNS:
		if records := h.LocalRecords.LookupNS(domain); len(records) > 0 {
			for _, rec := range records {
				rr := &dns.NS{
					Hdr: dns.RR_Header{
						Name:   domain,
						Rrtype: dns.TypeNS,
						Class:  dns.ClassINET,
						Ttl:    rec.TTL,
					},
					Ns: rec.Target,
				}
				msg.Answer = append(msg.Answer, rr)
			}
			outcome.responseCode = dns.RcodeSuccess
			h.writeMsg(w, msg)
			return true
		}
	case dns.TypeSOA:
		if records := h.LocalRecords.LookupSOA(domain); len(records) > 0 {
			rec := records[0]
			rr := &dns.SOA{
				Hdr: dns.RR_Header{
					Name:   domain,
					Rrtype: dns.TypeSOA,
					Class:  dns.ClassINET,
					Ttl:    rec.TTL,
				},
				Ns:      rec.Ns,
				Mbox:    rec.Mbox,
				Serial:  rec.Serial,
				Refresh: rec.Refresh,
				Retry:   rec.Retry,
				Expire:  rec.Expire,
				Minttl:  rec.Minttl,
			}
			msg.Answer = append(msg.Answer, rr)
			outcome.responseCode = dns.RcodeSuccess
			h.writeMsg(w, msg)
			return true
		}
	case dns.TypeCAA:
		if records := h.LocalRecords.LookupCAA(domain); len(records) > 0 {
			for _, rec := range records {
				rr := &dns.CAA{
					Hdr: dns.RR_Header{
						Name:   domain,
						Rrtype: dns.TypeCAA,
						Class:  dns.ClassINET,
						Ttl:    rec.TTL,
					},
					Flag:  rec.CaaFlag,
					Tag:   rec.CaaTag,
					Value: rec.CaaValue,
				}
				msg.Answer = append(msg.Answer, rr)
			}
			outcome.responseCode = dns.RcodeSuccess
			h.writeMsg(w, msg)
			return true
		}
	}

	return false
}

func (h *Handler) appendLocalARecords(msg *dns.Msg, domain string) bool {
	ips, ttl, found := h.LocalRecords.LookupA(domain)
	if !found {
		return false
	}
	for _, ip := range ips {
		if ip.To4() == nil {
			continue
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
	return len(msg.Answer) > 0
}

func (h *Handler) resolveLocalCNAMEAsA(msg *dns.Msg, domain string) bool {
	ips, ttl, found := h.LocalRecords.ResolveCNAME(domain, 10)
	if !found {
		return false
	}
	for _, ip := range ips {
		if ip.To4() == nil {
			continue
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
	return len(msg.Answer) > 0
}

func (h *Handler) appendLocalAAAARecords(msg *dns.Msg, domain string) bool {
	ips, ttl, found := h.LocalRecords.LookupAAAA(domain)
	if !found {
		return false
	}
	for _, ip := range ips {
		if ip.To4() != nil {
			continue
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
	return len(msg.Answer) > 0
}

func (h *Handler) resolveLocalCNAMEAsAAAA(msg *dns.Msg, domain string) bool {
	ips, ttl, found := h.LocalRecords.ResolveCNAME(domain, 10)
	if !found {
		return false
	}
	for _, ip := range ips {
		if ip.To4() != nil {
			continue
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
	return len(msg.Answer) > 0
}
