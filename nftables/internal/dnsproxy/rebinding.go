package dnsproxy

import (
	"net"

	"github.com/miekg/dns"
)

// privateRanges lists all IP ranges that should be blocked in DNS responses
// to prevent DNS rebinding attacks against the host or VMs.
var privateRanges = []net.IPNet{
	{IP: net.IP{10, 0, 0, 0}, Mask: net.CIDRMask(8, 32)},   // RFC 1918
	{IP: net.IP{172, 16, 0, 0}, Mask: net.CIDRMask(12, 32)}, // RFC 1918
	{IP: net.IP{192, 168, 0, 0}, Mask: net.CIDRMask(16, 32)},// RFC 1918
	{IP: net.IP{198, 19, 0, 0}, Mask: net.CIDRMask(16, 32)}, // vmsan link-local
	{IP: net.IP{10, 200, 0, 0}, Mask: net.CIDRMask(16, 32)}, // vmsan veth transit
	{IP: net.IP{10, 90, 0, 0}, Mask: net.CIDRMask(16, 32)},  // vmsan mesh (future 0.5.0)
	{IP: net.IP{127, 0, 0, 0}, Mask: net.CIDRMask(8, 32)},   // loopback
}

// IsPrivateIP checks whether the given IP falls within any blocked range.
func IsPrivateIP(ip net.IP) bool {
	ip4 := ip.To4()
	if ip4 == nil {
		return false // IPv6 not checked
	}
	for _, r := range privateRanges {
		if r.Contains(ip4) {
			return true
		}
	}
	return false
}

// CheckDNSRebinding inspects a DNS message for A/AAAA records pointing to
// private IP addresses. Returns (true, blockedIP, domain) if a rebinding
// attempt is detected.
func CheckDNSRebinding(msg *dns.Msg) (blocked bool, ip net.IP, domain string) {
	for _, rr := range msg.Answer {
		switch v := rr.(type) {
		case *dns.A:
			if IsPrivateIP(v.A) {
				return true, v.A, v.Hdr.Name
			}
		case *dns.AAAA:
			// Check for IPv4-mapped IPv6 addresses
			if ip4 := v.AAAA.To4(); ip4 != nil && IsPrivateIP(ip4) {
				return true, v.AAAA, v.Hdr.Name
			}
		}
	}
	return false, nil, ""
}
