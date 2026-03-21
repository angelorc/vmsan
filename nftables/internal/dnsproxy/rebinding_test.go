package dnsproxy

import (
	"net"
	"testing"

	"github.com/miekg/dns"
)

func makeARecord(name string, ip string) *dns.A {
	return &dns.A{
		Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeA, Class: dns.ClassINET},
		A:   net.ParseIP(ip),
	}
}

func makeAAAARecord(name string, ip string) *dns.AAAA {
	return &dns.AAAA{
		Hdr:  dns.RR_Header{Name: name, Rrtype: dns.TypeAAAA, Class: dns.ClassINET},
		AAAA: net.ParseIP(ip),
	}
}

func TestCheckDNSRebinding_RFC1918_192168(t *testing.T) {
	msg := new(dns.Msg)
	msg.Answer = []dns.RR{makeARecord("evil.com.", "192.168.1.1")}

	blocked, ip, domain := CheckDNSRebinding(msg)
	if !blocked {
		t.Fatal("expected 192.168.1.1 to be blocked")
	}
	if !ip.Equal(net.ParseIP("192.168.1.1")) {
		t.Errorf("expected blocked IP 192.168.1.1, got %s", ip)
	}
	if domain != "evil.com." {
		t.Errorf("expected domain evil.com., got %s", domain)
	}
}

func TestCheckDNSRebinding_RFC1918_10(t *testing.T) {
	msg := new(dns.Msg)
	msg.Answer = []dns.RR{makeARecord("evil.com.", "10.0.0.1")}

	blocked, _, _ := CheckDNSRebinding(msg)
	if !blocked {
		t.Fatal("expected 10.0.0.1 to be blocked")
	}
}

func TestCheckDNSRebinding_RFC1918_172(t *testing.T) {
	msg := new(dns.Msg)
	msg.Answer = []dns.RR{makeARecord("evil.com.", "172.16.0.1")}

	blocked, _, _ := CheckDNSRebinding(msg)
	if !blocked {
		t.Fatal("expected 172.16.0.1 to be blocked")
	}
}

func TestCheckDNSRebinding_VmsanLink(t *testing.T) {
	msg := new(dns.Msg)
	msg.Answer = []dns.RR{makeARecord("evil.com.", "198.19.0.1")}

	blocked, _, _ := CheckDNSRebinding(msg)
	if !blocked {
		t.Fatal("expected 198.19.0.1 (vmsan link-local) to be blocked")
	}
}

func TestCheckDNSRebinding_Loopback(t *testing.T) {
	msg := new(dns.Msg)
	msg.Answer = []dns.RR{makeARecord("evil.com.", "127.0.0.1")}

	blocked, _, _ := CheckDNSRebinding(msg)
	if !blocked {
		t.Fatal("expected 127.0.0.1 to be blocked")
	}
}

func TestCheckDNSRebinding_PublicIP_8888(t *testing.T) {
	msg := new(dns.Msg)
	msg.Answer = []dns.RR{makeARecord("dns.google.", "8.8.8.8")}

	blocked, _, _ := CheckDNSRebinding(msg)
	if blocked {
		t.Fatal("expected 8.8.8.8 to be allowed")
	}
}

func TestCheckDNSRebinding_PublicIP_1111(t *testing.T) {
	msg := new(dns.Msg)
	msg.Answer = []dns.RR{makeARecord("one.one.one.one.", "1.1.1.1")}

	blocked, _, _ := CheckDNSRebinding(msg)
	if blocked {
		t.Fatal("expected 1.1.1.1 to be allowed")
	}
}

func TestCheckDNSRebinding_MultipleRecordsOnePrivate(t *testing.T) {
	msg := new(dns.Msg)
	msg.Answer = []dns.RR{
		makeARecord("cdn.evil.com.", "8.8.4.4"),
		makeARecord("cdn.evil.com.", "192.168.0.1"),
		makeARecord("cdn.evil.com.", "1.0.0.1"),
	}

	blocked, ip, _ := CheckDNSRebinding(msg)
	if !blocked {
		t.Fatal("expected blocked when one of multiple records is private")
	}
	if !ip.Equal(net.ParseIP("192.168.0.1")) {
		t.Errorf("expected blocked IP 192.168.0.1, got %s", ip)
	}
}

func TestCheckDNSRebinding_AAAARecord(t *testing.T) {
	msg := new(dns.Msg)
	msg.Answer = []dns.RR{makeAAAARecord("example.com.", "2001:db8::1")}

	blocked, _, _ := CheckDNSRebinding(msg)
	if blocked {
		t.Fatal("expected pure IPv6 AAAA record to not be blocked")
	}
}

func TestCheckDNSRebinding_NoARecords(t *testing.T) {
	msg := new(dns.Msg)
	msg.Answer = []dns.RR{
		&dns.CNAME{
			Hdr:    dns.RR_Header{Name: "www.example.com.", Rrtype: dns.TypeCNAME, Class: dns.ClassINET},
			Target: "example.com.",
		},
	}

	blocked, _, _ := CheckDNSRebinding(msg)
	if blocked {
		t.Fatal("expected no blocking when no A/AAAA records present")
	}
}

func TestCheckDNSRebinding_VmsanMeshRange(t *testing.T) {
	msg := new(dns.Msg)
	msg.Answer = []dns.RR{makeARecord("evil.com.", "10.90.0.1")}

	blocked, _, _ := CheckDNSRebinding(msg)
	if !blocked {
		t.Fatal("expected 10.90.0.1 (vmsan mesh range) to be blocked")
	}
}

func TestIsPrivateIP_VmsanVethTransit(t *testing.T) {
	ip := net.ParseIP("10.200.0.1")
	if !IsPrivateIP(ip) {
		t.Fatal("expected 10.200.0.1 (vmsan veth transit) to be private")
	}
}

func TestIsPrivateIP_PublicAddresses(t *testing.T) {
	publicIPs := []string{
		"8.8.8.8",
		"1.1.1.1",
		"93.184.216.34",
		"198.18.0.1",  // just outside vmsan link-local (198.19.0.0/16)
		"172.32.0.1",  // just outside 172.16.0.0/12
		"11.0.0.1",    // outside 10.0.0.0/8... wait, 11 is outside
	}
	for _, s := range publicIPs {
		ip := net.ParseIP(s)
		if IsPrivateIP(ip) {
			t.Errorf("expected %s to be public, but was classified as private", s)
		}
	}
}

func TestIsPrivateIP_IPv6NotBlocked(t *testing.T) {
	ip := net.ParseIP("::1")
	if IsPrivateIP(ip) {
		t.Fatal("expected IPv6 loopback to not be blocked (IPv6 not checked)")
	}
}
