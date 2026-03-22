package dnsproxy

import "github.com/miekg/dns"

// StripECHFromSVCB removes the ECH (Encrypted Client Hello) SvcParam
// from all SVCB/HTTPS resource records in a DNS message.
// This prevents ECH bypass of SNI filtering while preserving all other
// HTTPS record functionality (ALPN, port hints, IPv4/IPv6 hints).
func StripECHFromSVCB(msg *dns.Msg) {
	for _, rr := range msg.Answer {
		stripECHFromRR(rr)
	}
	for _, rr := range msg.Extra {
		stripECHFromRR(rr)
	}
}

func stripECHFromRR(rr dns.RR) {
	// SVCB (type 64) and HTTPS (type 65) both embed the same SVCB struct.
	switch v := rr.(type) {
	case *dns.SVCB:
		v.Value = filterECH(v.Value)
	case *dns.HTTPS:
		v.Value = filterECH(v.Value)
	}
}

func filterECH(params []dns.SVCBKeyValue) []dns.SVCBKeyValue {
	filtered := make([]dns.SVCBKeyValue, 0, len(params))
	for _, kv := range params {
		if kv.Key() != dns.SVCB_ECHCONFIG {
			filtered = append(filtered, kv)
		}
	}
	return filtered
}
