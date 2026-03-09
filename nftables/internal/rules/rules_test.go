package rules

import (
	"encoding/binary"
	"net"
	"testing"

	"github.com/google/nftables/expr"
	"golang.org/x/sys/unix"
)

// --- Expression helper tests (no root required) ---

func TestEstablished(t *testing.T) {
	exprs := matchEstablished()

	if len(exprs) != 3 {
		t.Fatalf("matchEstablished: got %d expressions, want 3", len(exprs))
	}

	// [0] Ct state load
	ct, ok := exprs[0].(*expr.Ct)
	if !ok {
		t.Fatalf("exprs[0]: got %T, want *expr.Ct", exprs[0])
	}
	if ct.Register != 1 {
		t.Errorf("Ct.Register = %d, want 1", ct.Register)
	}
	if ct.Key != expr.CtKeySTATE {
		t.Errorf("Ct.Key = %v, want CtKeySTATE", ct.Key)
	}

	// [1] Bitwise mask for established|related
	bw, ok := exprs[1].(*expr.Bitwise)
	if !ok {
		t.Fatalf("exprs[1]: got %T, want *expr.Bitwise", exprs[1])
	}
	if bw.Len != 4 {
		t.Errorf("Bitwise.Len = %d, want 4", bw.Len)
	}
	wantMask := nativeEndianUint32(expr.CtStateBitESTABLISHED | expr.CtStateBitRELATED)
	if !bytesEqual(bw.Mask, wantMask) {
		t.Errorf("Bitwise.Mask = %v, want %v", bw.Mask, wantMask)
	}

	// [2] Cmp neq 0
	cmp, ok := exprs[2].(*expr.Cmp)
	if !ok {
		t.Fatalf("exprs[2]: got %T, want *expr.Cmp", exprs[2])
	}
	if cmp.Op != expr.CmpOpNeq {
		t.Errorf("Cmp.Op = %v, want CmpOpNeq", cmp.Op)
	}
	if !bytesEqual(cmp.Data, nativeEndianUint32(0)) {
		t.Errorf("Cmp.Data = %v, want zero", cmp.Data)
	}
}

func TestMatchProtoVerdict(t *testing.T) {
	tests := []struct {
		name  string
		proto byte
		verd  expr.VerdictKind
	}{
		{"ICMP drop", unix.IPPROTO_ICMP, expr.VerdictDrop},
		{"UDP drop", unix.IPPROTO_UDP, expr.VerdictDrop},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exprs := append(matchIPv4Proto(tt.proto), verdict(tt.verd))

			// Payload (proto) + Cmp + Verdict = 3
			if len(exprs) != 3 {
				t.Fatalf("got %d expressions, want 3", len(exprs))
			}

			// [0] Payload: network header offset 9, len 1
			p, ok := exprs[0].(*expr.Payload)
			if !ok {
				t.Fatalf("exprs[0]: got %T, want *expr.Payload", exprs[0])
			}
			if p.Base != expr.PayloadBaseNetworkHeader {
				t.Errorf("Payload.Base = %v, want NetworkHeader", p.Base)
			}
			if p.Offset != ipv4OffsetProtocol {
				t.Errorf("Payload.Offset = %d, want %d", p.Offset, ipv4OffsetProtocol)
			}
			if p.Len != 1 {
				t.Errorf("Payload.Len = %d, want 1", p.Len)
			}

			// [1] Cmp eq proto
			cmp, ok := exprs[1].(*expr.Cmp)
			if !ok {
				t.Fatalf("exprs[1]: got %T, want *expr.Cmp", exprs[1])
			}
			if cmp.Op != expr.CmpOpEq {
				t.Errorf("Cmp.Op = %v, want CmpOpEq", cmp.Op)
			}
			if len(cmp.Data) != 1 || cmp.Data[0] != tt.proto {
				t.Errorf("Cmp.Data = %v, want [%d]", cmp.Data, tt.proto)
			}

			// [2] Verdict
			v, ok := exprs[2].(*expr.Verdict)
			if !ok {
				t.Fatalf("exprs[2]: got %T, want *expr.Verdict", exprs[2])
			}
			if v.Kind != tt.verd {
				t.Errorf("Verdict.Kind = %v, want %v", v.Kind, tt.verd)
			}
		})
	}
}

func TestMatchDstPort(t *testing.T) {
	// TCP port 853 (DoT) drop
	proto := byte(unix.IPPROTO_TCP)
	port := uint16(853)

	exprs := make([]expr.Any, 0, 5)
	exprs = append(exprs, matchL4Proto(proto)...)
	exprs = append(exprs, matchDstPort(port)...)
	exprs = append(exprs, verdict(expr.VerdictDrop))

	// Meta(L4PROTO) + Cmp(TCP) + Payload(dport) + Cmp(853) + Verdict = 5
	if len(exprs) != 5 {
		t.Fatalf("got %d expressions, want 5", len(exprs))
	}

	// [0] Meta L4PROTO
	meta, ok := exprs[0].(*expr.Meta)
	if !ok {
		t.Fatalf("exprs[0]: got %T, want *expr.Meta", exprs[0])
	}
	if meta.Key != expr.MetaKeyL4PROTO {
		t.Errorf("Meta.Key = %v, want MetaKeyL4PROTO", meta.Key)
	}
	if meta.Register != 1 {
		t.Errorf("Meta.Register = %d, want 1", meta.Register)
	}

	// [1] Cmp TCP
	cmp, ok := exprs[1].(*expr.Cmp)
	if !ok {
		t.Fatalf("exprs[1]: got %T, want *expr.Cmp", exprs[1])
	}
	if len(cmp.Data) != 1 || cmp.Data[0] != unix.IPPROTO_TCP {
		t.Errorf("Cmp.Data = %v, want [%d]", cmp.Data, unix.IPPROTO_TCP)
	}

	// [2] Payload transport header offset 2, len 2
	p, ok := exprs[2].(*expr.Payload)
	if !ok {
		t.Fatalf("exprs[2]: got %T, want *expr.Payload", exprs[2])
	}
	if p.Base != expr.PayloadBaseTransportHeader {
		t.Errorf("Payload.Base = %v, want TransportHeader", p.Base)
	}
	if p.Offset != 2 || p.Len != 2 {
		t.Errorf("Payload offset=%d len=%d, want offset=2 len=2", p.Offset, p.Len)
	}

	// [3] Cmp port 853
	cmpPort, ok := exprs[3].(*expr.Cmp)
	if !ok {
		t.Fatalf("exprs[3]: got %T, want *expr.Cmp", exprs[3])
	}
	wantPort := bigEndianUint16(853)
	if !bytesEqual(cmpPort.Data, wantPort) {
		t.Errorf("Cmp.Data = %v, want %v (port 853)", cmpPort.Data, wantPort)
	}

	// [4] Verdict drop
	v, ok := exprs[4].(*expr.Verdict)
	if !ok {
		t.Fatalf("exprs[4]: got %T, want *expr.Verdict", exprs[4])
	}
	if v.Kind != expr.VerdictDrop {
		t.Errorf("Verdict.Kind = %v, want VerdictDrop", v.Kind)
	}
}

func TestMatchIface(t *testing.T) {
	iif := "veth0"
	oif := "eth0"
	exprs := append(matchIface(iif, oif), verdict(expr.VerdictAccept))

	// Meta(IIFNAME) + Cmp + Meta(OIFNAME) + Cmp + Verdict = 5
	if len(exprs) != 5 {
		t.Fatalf("got %d expressions, want 5", len(exprs))
	}

	// [0] Meta IIFNAME
	metaIif, ok := exprs[0].(*expr.Meta)
	if !ok {
		t.Fatalf("exprs[0]: got %T, want *expr.Meta", exprs[0])
	}
	if metaIif.Key != expr.MetaKeyIIFNAME {
		t.Errorf("Meta.Key = %v, want MetaKeyIIFNAME", metaIif.Key)
	}

	// [1] Cmp iifname padded to 16 bytes
	cmpIif, ok := exprs[1].(*expr.Cmp)
	if !ok {
		t.Fatalf("exprs[1]: got %T, want *expr.Cmp", exprs[1])
	}
	if len(cmpIif.Data) != ifnameSize {
		t.Errorf("iifname Cmp.Data length = %d, want %d", len(cmpIif.Data), ifnameSize)
	}
	if string(cmpIif.Data[:len(iif)]) != iif {
		t.Errorf("iifname prefix = %q, want %q", string(cmpIif.Data[:len(iif)]), iif)
	}
	// Verify padding bytes are zero
	for i := len(iif); i < ifnameSize; i++ {
		if cmpIif.Data[i] != 0 {
			t.Errorf("iifname padding byte %d = %d, want 0", i, cmpIif.Data[i])
		}
	}

	// [2] Meta OIFNAME
	metaOif, ok := exprs[2].(*expr.Meta)
	if !ok {
		t.Fatalf("exprs[2]: got %T, want *expr.Meta", exprs[2])
	}
	if metaOif.Key != expr.MetaKeyOIFNAME {
		t.Errorf("Meta.Key = %v, want MetaKeyOIFNAME", metaOif.Key)
	}

	// [3] Cmp oifname padded to 16 bytes
	cmpOif, ok := exprs[3].(*expr.Cmp)
	if !ok {
		t.Fatalf("exprs[3]: got %T, want *expr.Cmp", exprs[3])
	}
	if len(cmpOif.Data) != ifnameSize {
		t.Errorf("oifname Cmp.Data length = %d, want %d", len(cmpOif.Data), ifnameSize)
	}
	if string(cmpOif.Data[:len(oif)]) != oif {
		t.Errorf("oifname prefix = %q, want %q", string(cmpOif.Data[:len(oif)]), oif)
	}

	// [4] Verdict accept
	v, ok := exprs[4].(*expr.Verdict)
	if !ok {
		t.Fatalf("exprs[4]: got %T, want *expr.Verdict", exprs[4])
	}
	if v.Kind != expr.VerdictAccept {
		t.Errorf("Verdict.Kind = %v, want VerdictAccept", v.Kind)
	}
}

func TestMatchDstCIDR(t *testing.T) {
	network, prefixLen, err := parseCIDRv4("198.19.0.0/16")
	if err != nil {
		t.Fatalf("parseCIDRv4: %v", err)
	}

	exprs := append(matchCIDR(network, prefixLen), verdict(expr.VerdictDrop))

	// Payload(dst addr) + Bitwise(mask) + Cmp(network) + Verdict = 4
	if len(exprs) != 4 {
		t.Fatalf("got %d expressions, want 4", len(exprs))
	}

	// [0] Payload: network header, dst addr offset, 4 bytes
	p, ok := exprs[0].(*expr.Payload)
	if !ok {
		t.Fatalf("exprs[0]: got %T, want *expr.Payload", exprs[0])
	}
	if p.Base != expr.PayloadBaseNetworkHeader {
		t.Errorf("Payload.Base = %v, want NetworkHeader", p.Base)
	}
	if p.Offset != IPv4OffsetDstAddr {
		t.Errorf("Payload.Offset = %d, want %d", p.Offset, IPv4OffsetDstAddr)
	}
	if p.Len != 4 {
		t.Errorf("Payload.Len = %d, want 4", p.Len)
	}

	// [1] Bitwise: mask = /16
	bw, ok := exprs[1].(*expr.Bitwise)
	if !ok {
		t.Fatalf("exprs[1]: got %T, want *expr.Bitwise", exprs[1])
	}
	wantMask := cidrMask(16) // 255.255.0.0
	if !bytesEqual(bw.Mask, wantMask) {
		t.Errorf("Bitwise.Mask = %v, want %v", bw.Mask, wantMask)
	}

	// [2] Cmp: network address = 198.19.0.0
	cmp, ok := exprs[2].(*expr.Cmp)
	if !ok {
		t.Fatalf("exprs[2]: got %T, want *expr.Cmp", exprs[2])
	}
	wantNet := net.IP{198, 19, 0, 0}
	if !bytesEqual(cmp.Data, wantNet) {
		t.Errorf("Cmp.Data = %v, want %v", cmp.Data, wantNet)
	}

	// [3] Verdict
	v, ok := exprs[3].(*expr.Verdict)
	if !ok {
		t.Fatalf("exprs[3]: got %T, want *expr.Verdict", exprs[3])
	}
	if v.Kind != expr.VerdictDrop {
		t.Errorf("Verdict.Kind = %v, want VerdictDrop", v.Kind)
	}
}

func TestDNAT(t *testing.T) {
	dstIP := net.IP{198, 19, 0, 2}.To4()
	exprs := dnatExprs(unix.IPPROTO_TCP, 8080, dstIP, 80)

	// Meta(L4PROTO) + Cmp(TCP) + Payload(dport) + Cmp(8080) + Immediate(addr) + Immediate(port) + NAT = 7
	if len(exprs) != 7 {
		t.Fatalf("got %d expressions, want 7", len(exprs))
	}

	// [0] Meta L4PROTO
	meta, ok := exprs[0].(*expr.Meta)
	if !ok {
		t.Fatalf("exprs[0]: got %T, want *expr.Meta", exprs[0])
	}
	if meta.Key != expr.MetaKeyL4PROTO {
		t.Errorf("Meta.Key = %v, want MetaKeyL4PROTO", meta.Key)
	}

	// [1] Cmp TCP
	cmp, ok := exprs[1].(*expr.Cmp)
	if !ok {
		t.Fatalf("exprs[1]: got %T, want *expr.Cmp", exprs[1])
	}
	if len(cmp.Data) != 1 || cmp.Data[0] != unix.IPPROTO_TCP {
		t.Errorf("Cmp.Data = %v, want [%d]", cmp.Data, unix.IPPROTO_TCP)
	}

	// [2] Payload transport header dport
	p, ok := exprs[2].(*expr.Payload)
	if !ok {
		t.Fatalf("exprs[2]: got %T, want *expr.Payload", exprs[2])
	}
	if p.Base != expr.PayloadBaseTransportHeader {
		t.Errorf("Payload.Base = %v, want TransportHeader", p.Base)
	}

	// [3] Cmp source port 8080
	cmpPort, ok := exprs[3].(*expr.Cmp)
	if !ok {
		t.Fatalf("exprs[3]: got %T, want *expr.Cmp", exprs[3])
	}
	if !bytesEqual(cmpPort.Data, bigEndianUint16(8080)) {
		t.Errorf("Cmp.Data = %v, want port 8080", cmpPort.Data)
	}

	// [4] Immediate addr
	immAddr, ok := exprs[4].(*expr.Immediate)
	if !ok {
		t.Fatalf("exprs[4]: got %T, want *expr.Immediate", exprs[4])
	}
	if immAddr.Register != 1 {
		t.Errorf("Immediate.Register = %d, want 1", immAddr.Register)
	}
	if !bytesEqual(immAddr.Data, dstIP) {
		t.Errorf("Immediate.Data = %v, want %v", immAddr.Data, dstIP)
	}

	// [5] Immediate port
	immPort, ok := exprs[5].(*expr.Immediate)
	if !ok {
		t.Fatalf("exprs[5]: got %T, want *expr.Immediate", exprs[5])
	}
	if immPort.Register != 2 {
		t.Errorf("Immediate.Register = %d, want 2", immPort.Register)
	}
	if !bytesEqual(immPort.Data, bigEndianUint16(80)) {
		t.Errorf("Immediate.Data = %v, want port 80", immPort.Data)
	}

	// [6] NAT
	nat, ok := exprs[6].(*expr.NAT)
	if !ok {
		t.Fatalf("exprs[6]: got %T, want *expr.NAT", exprs[6])
	}
	if nat.Type != expr.NATTypeDestNAT {
		t.Errorf("NAT.Type = %v, want NATTypeDestNAT", nat.Type)
	}
	if nat.Family != 2 {
		t.Errorf("NAT.Family = %d, want 2 (NFPROTO_IPV4)", nat.Family)
	}
	if nat.RegAddrMin != 1 {
		t.Errorf("NAT.RegAddrMin = %d, want 1", nat.RegAddrMin)
	}
	if nat.RegProtoMin != 2 {
		t.Errorf("NAT.RegProtoMin = %d, want 2", nat.RegProtoMin)
	}
}

func TestMasquerade(t *testing.T) {
	exprs := []expr.Any{&expr.Masq{}}

	if len(exprs) != 1 {
		t.Fatalf("got %d expressions, want 1", len(exprs))
	}
	if _, ok := exprs[0].(*expr.Masq); !ok {
		t.Fatalf("exprs[0]: got %T, want *expr.Masq", exprs[0])
	}
}

func TestDoHDropRules(t *testing.T) {
	want := 11
	if len(DoHResolverIPs) != want {
		t.Fatalf("DoHResolverIPs count = %d, want %d", len(DoHResolverIPs), want)
	}

	// Verify every IP is parseable as IPv4.
	for _, ip := range DoHResolverIPs {
		parsed := net.ParseIP(ip)
		if parsed == nil {
			t.Errorf("invalid IP in DoHResolverIPs: %s", ip)
			continue
		}
		if parsed.To4() == nil {
			t.Errorf("non-IPv4 IP in DoHResolverIPs: %s", ip)
		}
	}

	// Verify expression structure for each DoH drop rule.
	for _, ipStr := range DoHResolverIPs {
		ip := net.ParseIP(ipStr).To4()
		exprs := make([]expr.Any, 0, 7)
		exprs = append(exprs, matchDstIP(ip)...)
		exprs = append(exprs, matchL4Proto(unix.IPPROTO_TCP)...)
		exprs = append(exprs, matchDstPort(443)...)
		exprs = append(exprs, verdict(expr.VerdictDrop))

		// matchDstIP(2) + matchL4Proto(2) + matchDstPort(2) + verdict(1) = 7
		if len(exprs) != 7 {
			t.Errorf("DoH drop rule for %s: got %d exprs, want 7", ipStr, len(exprs))
		}
	}
}

func TestCrossVMIsolation(t *testing.T) {
	want := 4
	if len(CrossVMSubnets) != want {
		t.Fatalf("CrossVMSubnets count = %d, want %d", len(CrossVMSubnets), want)
	}

	// Verify every subnet is parseable as an IPv4 CIDR and produces valid expressions.
	for _, cidr := range CrossVMSubnets {
		network, prefixLen, err := parseCIDRv4(cidr)
		if err != nil {
			t.Errorf("parseCIDRv4(%q): %v", cidr, err)
			continue
		}
		if len(network) != 4 {
			t.Errorf("parseCIDRv4(%q): network length = %d, want 4", cidr, len(network))
		}
		if prefixLen < 1 || prefixLen > 32 {
			t.Errorf("parseCIDRv4(%q): prefixLen = %d, want 1-32", cidr, prefixLen)
		}

		// Verify expression structure: Payload + Bitwise + Cmp + Verdict = 4
		exprs := append(matchCIDR(network, prefixLen), verdict(expr.VerdictDrop))
		if len(exprs) != 4 {
			t.Errorf("CIDR rule for %s: got %d exprs, want 4", cidr, len(exprs))
		}
	}
}

func TestDNSRules(t *testing.T) {
	resolvers := []string{"1.1.1.1", "8.8.8.8"}

	// For each resolver: UDP allow + TCP allow = 2 rules per resolver.
	// Plus 2 block rules (UDP + TCP port 53).
	// Total: 2*2 + 2 = 6 rules.
	//
	// Verify the expression structure for each DNS allow rule:
	// matchL4Proto(2) + matchDstIP(2) + matchDstPort(2) + verdict(1) = 7
	for _, r := range resolvers {
		ip, err := ParseIPv4(r)
		if err != nil {
			t.Fatalf("ParseIPv4(%q): %v", r, err)
		}

		for _, proto := range []byte{unix.IPPROTO_UDP, unix.IPPROTO_TCP} {
			exprs := make([]expr.Any, 0, 7)
			exprs = append(exprs, matchL4Proto(proto)...)
			exprs = append(exprs, matchDstIP(ip)...)
			exprs = append(exprs, matchDstPort(53)...)
			exprs = append(exprs, verdict(expr.VerdictAccept))

			if len(exprs) != 7 {
				t.Errorf("DNS allow rule for %s proto %d: got %d exprs, want 7", r, proto, len(exprs))
			}
		}
	}

	// Block rules: matchL4Proto(2) + matchDstPort(2) + verdict(1) = 5
	for _, proto := range []byte{unix.IPPROTO_UDP, unix.IPPROTO_TCP} {
		exprs := make([]expr.Any, 0, 5)
		exprs = append(exprs, matchL4Proto(proto)...)
		exprs = append(exprs, matchDstPort(53)...)
		exprs = append(exprs, verdict(expr.VerdictDrop))

		if len(exprs) != 5 {
			t.Errorf("DNS block rule proto %d: got %d exprs, want 5", proto, len(exprs))
		}
	}
}

// --- Byte encoding helper tests ---

func TestParseCIDRv4(t *testing.T) {
	tests := []struct {
		cidr      string
		wantNet   net.IP
		wantBits  int
		wantError bool
	}{
		{"198.19.0.0/16", net.IP{198, 19, 0, 0}, 16, false},
		{"10.0.0.0/8", net.IP{10, 0, 0, 0}, 8, false},
		{"192.168.1.0/24", net.IP{192, 168, 1, 0}, 24, false},
		{"invalid", nil, 0, true},
		{"::1/128", nil, 0, true}, // IPv6 should fail
	}

	for _, tt := range tests {
		t.Run(tt.cidr, func(t *testing.T) {
			network, bits, err := parseCIDRv4(tt.cidr)
			if (err != nil) != tt.wantError {
				t.Fatalf("parseCIDRv4(%q) error = %v, wantError = %v", tt.cidr, err, tt.wantError)
			}
			if tt.wantError {
				return
			}
			if !network.Equal(tt.wantNet) {
				t.Errorf("network = %v, want %v", network, tt.wantNet)
			}
			if bits != tt.wantBits {
				t.Errorf("prefixLen = %d, want %d", bits, tt.wantBits)
			}
		})
	}
}

func TestParseIPv4(t *testing.T) {
	tests := []struct {
		input     string
		wantIP    net.IP
		wantError bool
	}{
		{"1.1.1.1", net.IP{1, 1, 1, 1}, false},
		{"255.255.255.255", net.IP{255, 255, 255, 255}, false},
		{"0.0.0.0", net.IP{0, 0, 0, 0}, false},
		{"not-an-ip", nil, true},
		{"::1", nil, true}, // IPv6
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			ip, err := ParseIPv4(tt.input)
			if (err != nil) != tt.wantError {
				t.Fatalf("ParseIPv4(%q) error = %v, wantError = %v", tt.input, err, tt.wantError)
			}
			if tt.wantError {
				return
			}
			if !ip.Equal(tt.wantIP) {
				t.Errorf("ip = %v, want %v", ip, tt.wantIP)
			}
		})
	}
}

func TestProtoNum(t *testing.T) {
	tests := []struct {
		input string
		want  byte
	}{
		{"tcp", 6},
		{"udp", 17},
		{"unknown", 0},
		{"", 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ProtoNum(tt.input)
			if got != tt.want {
				t.Errorf("ProtoNum(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// --- Additional encoding helper tests ---

func TestPadIfname(t *testing.T) {
	b := padIfname("eth0")
	if len(b) != ifnameSize {
		t.Fatalf("padIfname length = %d, want %d", len(b), ifnameSize)
	}
	if string(b[:4]) != "eth0" {
		t.Errorf("padIfname prefix = %q, want %q", string(b[:4]), "eth0")
	}
	for i := 4; i < ifnameSize; i++ {
		if b[i] != 0 {
			t.Errorf("padding byte %d = %d, want 0", i, b[i])
		}
	}
}

func TestCidrMask(t *testing.T) {
	tests := []struct {
		ones int
		want []byte
	}{
		{8, []byte{255, 0, 0, 0}},
		{16, []byte{255, 255, 0, 0}},
		{24, []byte{255, 255, 255, 0}},
		{32, []byte{255, 255, 255, 255}},
	}

	for _, tt := range tests {
		got := cidrMask(tt.ones)
		if !bytesEqual(got, tt.want) {
			t.Errorf("cidrMask(%d) = %v, want %v", tt.ones, got, tt.want)
		}
	}
}

func TestBigEndianUint16(t *testing.T) {
	b := bigEndianUint16(853)
	v := binary.BigEndian.Uint16(b)
	if v != 853 {
		t.Errorf("bigEndianUint16(853) round-trip = %d, want 853", v)
	}
}

func TestNativeEndianUint32(t *testing.T) {
	b := nativeEndianUint32(42)
	v := binary.NativeEndian.Uint32(b)
	if v != 42 {
		t.Errorf("nativeEndianUint32(42) round-trip = %d, want 42", v)
	}
}

// bytesEqual compares two byte slices.
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
