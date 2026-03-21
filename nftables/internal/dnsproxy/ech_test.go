package dnsproxy

import (
	"net"
	"testing"

	"github.com/miekg/dns"
)

func TestStripECHFromSVCB_SVCBWithECH(t *testing.T) {
	msg := new(dns.Msg)
	msg.Answer = []dns.RR{
		&dns.SVCB{
			Hdr:      dns.RR_Header{Name: "example.com.", Rrtype: dns.TypeSVCB, Class: dns.ClassINET},
			Priority: 1,
			Target:   ".",
			Value: []dns.SVCBKeyValue{
				&dns.SVCBAlpn{Alpn: []string{"h2", "h3"}},
				&dns.SVCBECHConfig{ECH: []byte{0x00, 0x01, 0x02}},
				&dns.SVCBIPv4Hint{Hint: []net.IP{net.ParseIP("1.2.3.4")}},
			},
		},
	}

	StripECHFromSVCB(msg)

	svcb := msg.Answer[0].(*dns.SVCB)
	if len(svcb.Value) != 2 {
		t.Fatalf("expected 2 params after stripping ECH, got %d", len(svcb.Value))
	}
	for _, kv := range svcb.Value {
		if kv.Key() == dns.SVCB_ECHCONFIG {
			t.Fatal("ECH param should have been stripped")
		}
	}
	if svcb.Value[0].Key() != dns.SVCB_ALPN {
		t.Errorf("expected ALPN param, got key %d", svcb.Value[0].Key())
	}
	if svcb.Value[1].Key() != dns.SVCB_IPV4HINT {
		t.Errorf("expected IPV4HINT param, got key %d", svcb.Value[1].Key())
	}
}

func TestStripECHFromSVCB_HTTPSWithECH(t *testing.T) {
	msg := new(dns.Msg)
	msg.Answer = []dns.RR{
		&dns.HTTPS{
			SVCB: dns.SVCB{
				Hdr:      dns.RR_Header{Name: "example.com.", Rrtype: dns.TypeHTTPS, Class: dns.ClassINET},
				Priority: 1,
				Target:   ".",
				Value: []dns.SVCBKeyValue{
					&dns.SVCBECHConfig{ECH: []byte{0xDE, 0xAD}},
					&dns.SVCBAlpn{Alpn: []string{"h2"}},
					&dns.SVCBIPv4Hint{Hint: []net.IP{net.ParseIP("10.0.0.1")}},
				},
			},
		},
	}

	StripECHFromSVCB(msg)

	https := msg.Answer[0].(*dns.HTTPS)
	if len(https.Value) != 2 {
		t.Fatalf("expected 2 params after stripping ECH, got %d", len(https.Value))
	}
	for _, kv := range https.Value {
		if kv.Key() == dns.SVCB_ECHCONFIG {
			t.Fatal("ECH param should have been stripped from HTTPS record")
		}
	}
	if https.Value[0].Key() != dns.SVCB_ALPN {
		t.Errorf("expected ALPN param first, got key %d", https.Value[0].Key())
	}
	if https.Value[1].Key() != dns.SVCB_IPV4HINT {
		t.Errorf("expected IPV4HINT param second, got key %d", https.Value[1].Key())
	}
}

func TestStripECHFromSVCB_NoSVCBRecords(t *testing.T) {
	msg := new(dns.Msg)
	msg.Answer = []dns.RR{
		&dns.A{
			Hdr: dns.RR_Header{Name: "example.com.", Rrtype: dns.TypeA, Class: dns.ClassINET},
			A:   net.ParseIP("93.184.216.34"),
		},
	}

	StripECHFromSVCB(msg)

	if len(msg.Answer) != 1 {
		t.Fatalf("expected 1 answer, got %d", len(msg.Answer))
	}
	if _, ok := msg.Answer[0].(*dns.A); !ok {
		t.Fatal("expected A record to be unchanged")
	}
}

func TestStripECHFromSVCB_SVCBWithoutECH(t *testing.T) {
	msg := new(dns.Msg)
	msg.Answer = []dns.RR{
		&dns.SVCB{
			Hdr:      dns.RR_Header{Name: "example.com.", Rrtype: dns.TypeSVCB, Class: dns.ClassINET},
			Priority: 1,
			Target:   ".",
			Value: []dns.SVCBKeyValue{
				&dns.SVCBAlpn{Alpn: []string{"h2", "h3"}},
				&dns.SVCBIPv4Hint{Hint: []net.IP{net.ParseIP("1.2.3.4")}},
			},
		},
	}

	StripECHFromSVCB(msg)

	svcb := msg.Answer[0].(*dns.SVCB)
	if len(svcb.Value) != 2 {
		t.Fatalf("expected 2 params unchanged, got %d", len(svcb.Value))
	}
	if svcb.Value[0].Key() != dns.SVCB_ALPN {
		t.Errorf("expected ALPN, got key %d", svcb.Value[0].Key())
	}
	if svcb.Value[1].Key() != dns.SVCB_IPV4HINT {
		t.Errorf("expected IPV4HINT, got key %d", svcb.Value[1].Key())
	}
}

func TestStripECHFromSVCB_MultipleSVCBRecords(t *testing.T) {
	msg := new(dns.Msg)
	msg.Answer = []dns.RR{
		&dns.SVCB{
			Hdr:      dns.RR_Header{Name: "a.example.com.", Rrtype: dns.TypeSVCB, Class: dns.ClassINET},
			Priority: 1,
			Target:   ".",
			Value: []dns.SVCBKeyValue{
				&dns.SVCBECHConfig{ECH: []byte{0x01}},
				&dns.SVCBAlpn{Alpn: []string{"h3"}},
			},
		},
		&dns.SVCB{
			Hdr:      dns.RR_Header{Name: "b.example.com.", Rrtype: dns.TypeSVCB, Class: dns.ClassINET},
			Priority: 1,
			Target:   ".",
			Value: []dns.SVCBKeyValue{
				&dns.SVCBAlpn{Alpn: []string{"h2"}},
			},
		},
		&dns.SVCB{
			Hdr:      dns.RR_Header{Name: "c.example.com.", Rrtype: dns.TypeSVCB, Class: dns.ClassINET},
			Priority: 2,
			Target:   ".",
			Value: []dns.SVCBKeyValue{
				&dns.SVCBECHConfig{ECH: []byte{0x02, 0x03}},
			},
		},
	}

	StripECHFromSVCB(msg)

	// First record: ECH stripped, ALPN preserved
	svcb0 := msg.Answer[0].(*dns.SVCB)
	if len(svcb0.Value) != 1 {
		t.Fatalf("record 0: expected 1 param, got %d", len(svcb0.Value))
	}
	if svcb0.Value[0].Key() != dns.SVCB_ALPN {
		t.Errorf("record 0: expected ALPN, got key %d", svcb0.Value[0].Key())
	}

	// Second record: unchanged (no ECH)
	svcb1 := msg.Answer[1].(*dns.SVCB)
	if len(svcb1.Value) != 1 {
		t.Fatalf("record 1: expected 1 param, got %d", len(svcb1.Value))
	}
	if svcb1.Value[0].Key() != dns.SVCB_ALPN {
		t.Errorf("record 1: expected ALPN, got key %d", svcb1.Value[0].Key())
	}

	// Third record: ECH stripped, no params left
	svcb2 := msg.Answer[2].(*dns.SVCB)
	if len(svcb2.Value) != 0 {
		t.Fatalf("record 2: expected 0 params, got %d", len(svcb2.Value))
	}
}

func TestStripECHFromSVCB_ExtraSection(t *testing.T) {
	msg := new(dns.Msg)
	msg.Extra = []dns.RR{
		&dns.HTTPS{
			SVCB: dns.SVCB{
				Hdr:      dns.RR_Header{Name: "example.com.", Rrtype: dns.TypeHTTPS, Class: dns.ClassINET},
				Priority: 1,
				Target:   ".",
				Value: []dns.SVCBKeyValue{
					&dns.SVCBECHConfig{ECH: []byte{0xFF}},
				},
			},
		},
	}

	StripECHFromSVCB(msg)

	https := msg.Extra[0].(*dns.HTTPS)
	if len(https.Value) != 0 {
		t.Fatalf("expected ECH stripped from extra section, got %d params", len(https.Value))
	}
}
