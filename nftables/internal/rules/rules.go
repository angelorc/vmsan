// Package rules provides nftables rule composition primitives.
//
// It wraps the low-level google/nftables expression types into a Builder
// that simplifies constructing complete rules for VM firewall chains.
// All expressions are private helpers — callers interact only through Builder methods.
package rules

import (
	"encoding/binary"
	"fmt"
	"net"

	"github.com/google/nftables"
	"github.com/google/nftables/expr"
	"golang.org/x/sys/unix"
)

// NftablesClient defines the interface for nftables operations.
// This abstraction allows for dependency injection and easier testing.
type NftablesClient interface {
	AddTable(*nftables.Table) *nftables.Table
	DelTable(*nftables.Table)
	AddChain(*nftables.Chain) *nftables.Chain
	DelChain(*nftables.Chain)
	AddRule(*nftables.Rule) *nftables.Rule
	DelRule(*nftables.Rule) error
	ListTables() ([]*nftables.Table, error)
	ListChains() ([]*nftables.Chain, error)
	GetRules(*nftables.Table, *nftables.Chain) ([]*nftables.Rule, error)
	Flush() error
	FlushRuleset()
}

// Compile-time check: ensure *nftables.Conn implements NftablesClient.
var _ NftablesClient = (*nftables.Conn)(nil)

// IPv4 header offsets (bytes from network header start).
const (
	ipv4OffsetProtocol = 9
	IPv4OffsetSrcAddr  = 12
	IPv4OffsetDstAddr  = 16
)

// nftables interface name length (IFNAMSIZ).
const ifnameSize = 16

// DoHResolverIPs are well-known DNS-over-HTTPS resolver IPs.
// Blocking TCP 443 to these prevents DoH bypass of DNS filtering.
var DoHResolverIPs = []string{
	"8.8.8.8", "8.8.4.4", // Google
	"1.1.1.1", "1.0.0.1", // Cloudflare
	"9.9.9.9", "149.112.112.112", // Quad9
	"208.67.222.222", "208.67.220.220", // OpenDNS
	"94.140.14.14", "94.140.15.15", // AdGuard
	"185.228.168.168", // CleanBrowsing
}

// CrossVMSubnets are internal subnets that must be blocked in the FORWARD chain
// to prevent cross-VM lateral movement.
var CrossVMSubnets = []string{
	"198.19.0.0/16", // VM link subnet (guest/host IPs)
	"172.16.0.0/16", // Legacy VM address block
	"10.200.0.0/16", // Veth transit subnet
	"10.90.0.0/16",  // Future mesh subnet (defense-in-depth)
}

// --- Builder ---

// Builder composes nftables rules for a specific table and chain.
// It eliminates the need to pass conn/table/chain to every rule function.
type Builder struct {
	c     NftablesClient
	table *nftables.Table
	chain *nftables.Chain
}

// NewBuilder creates a Builder bound to the given connection, table, and chain.
func NewBuilder(c NftablesClient, table *nftables.Table, chain *nftables.Chain) *Builder {
	return &Builder{c: c, table: table, chain: chain}
}

// add appends a rule with the given expressions.
func (b *Builder) add(exprs []expr.Any) {
	b.c.AddRule(&nftables.Rule{
		Table: b.table,
		Chain: b.chain,
		Exprs: exprs,
	})
}

// Established adds: ct state established,related accept
func (b *Builder) Established() {
	b.add(append(matchEstablished(), verdict(expr.VerdictAccept)))
}

// MatchProtoVerdict adds: ip protocol <proto> <verdict>
func (b *Builder) MatchProtoVerdict(proto byte, v expr.VerdictKind) {
	b.add(append(matchIPv4Proto(proto), verdict(v)))
}

// MatchDstPort adds: <l4proto> dport <port> <verdict>
func (b *Builder) MatchDstPort(proto byte, port uint16, v expr.VerdictKind) {
	exprs := make([]expr.Any, 0, 5)
	exprs = append(exprs, matchL4Proto(proto)...)
	exprs = append(exprs, matchDstPort(port)...)
	exprs = append(exprs, verdict(v))
	b.add(exprs)
}

// MatchDstIPPort adds: ip daddr <ip> <l4proto> dport <port> <verdict>
func (b *Builder) MatchDstIPPort(ip net.IP, proto byte, port uint16, v expr.VerdictKind) {
	exprs := make([]expr.Any, 0, 7)
	exprs = append(exprs, matchDstIP(ip)...)
	exprs = append(exprs, matchL4Proto(proto)...)
	exprs = append(exprs, matchDstPort(port)...)
	exprs = append(exprs, verdict(v))
	b.add(exprs)
}

// MatchIface adds: iifname <iif> oifname <oif> accept
func (b *Builder) MatchIface(iifname, oifname string) {
	b.add(append(matchIface(iifname, oifname), verdict(expr.VerdictAccept)))
}

// MatchDstCIDR adds: ip daddr <cidr> <verdict>
func (b *Builder) MatchDstCIDR(cidr string, v expr.VerdictKind) error {
	network, prefixLen, err := parseCIDRv4(cidr)
	if err != nil {
		return err
	}
	b.add(append(matchCIDR(network, prefixLen), verdict(v)))
	return nil
}

// MatchIPAddr adds: ip <src|dst>addr <ip> accept
func (b *Builder) MatchIPAddr(ip net.IP, offset uint32) {
	b.add(append(matchIPAddr(ip, offset), verdict(expr.VerdictAccept)))
}

// DNSForwardAccept adds: <proto> ip daddr <resolverIP> dport 53 accept
func (b *Builder) DNSForwardAccept(resolverIP string, proto byte) error {
	ip, err := ParseIPv4(resolverIP)
	if err != nil {
		return fmt.Errorf("invalid resolver IP: %w", err)
	}

	exprs := make([]expr.Any, 0, 7)
	exprs = append(exprs, matchL4Proto(proto)...)
	exprs = append(exprs, matchDstIP(ip)...)
	exprs = append(exprs, matchDstPort(53)...)
	exprs = append(exprs, verdict(expr.VerdictAccept))
	b.add(exprs)
	return nil
}

// DNAT adds a DNAT rule: <proto> dport <srcPort> dnat to <dstIP>:<dstPort>
func (b *Builder) DNAT(proto byte, srcPort uint16, dstIP net.IP, dstPort uint16) {
	b.add(dnatExprs(proto, srcPort, dstIP, dstPort))
}

// Masquerade adds: masquerade
func (b *Builder) Masquerade() {
	b.add([]expr.Any{&expr.Masq{}})
}

// Accept adds an unconditional accept verdict.
func (b *Builder) Accept() {
	b.add([]expr.Any{verdict(expr.VerdictAccept)})
}

// TLSDNAT adds a DNAT rule to redirect TLS traffic (tcp dport 443) to the SNI proxy.
func (b *Builder) TLSDNAT(dstIP net.IP, dstPort uint16) {
	b.DNAT(unix.IPPROTO_TCP, 443, dstIP, dstPort)
}

// HTTPDNAT adds a DNAT rule to redirect HTTP traffic (tcp dport 80) to the HTTP proxy.
func (b *Builder) HTTPDNAT(dstIP net.IP, dstPort uint16) {
	b.DNAT(unix.IPPROTO_TCP, 80, dstIP, dstPort)
}

// DNSDNAT adds a DNAT rule to redirect DNS traffic (udp dport 53) to the mesh DNS handler.
func (b *Builder) DNSDNAT(dstIP net.IP, dstPort uint16) {
	b.DNAT(unix.IPPROTO_UDP, 53, dstIP, dstPort)
}

// LogAndDrop adds a log rule followed by a drop rule. The log prefix
// identifies the reason for the drop (visible in kern.log / journalctl).
func (b *Builder) LogAndDrop(prefix string, matchExprs []expr.Any) {
	// Log rule: match expressions + log action (no verdict — continues to next rule).
	logExprs := make([]expr.Any, len(matchExprs), len(matchExprs)+1)
	copy(logExprs, matchExprs)
	logExprs = append(logExprs, logExpr(prefix))
	b.add(logExprs)

	// Drop rule: same match expressions + drop verdict.
	dropExprs := make([]expr.Any, len(matchExprs), len(matchExprs)+1)
	copy(dropExprs, matchExprs)
	dropExprs = append(dropExprs, verdict(expr.VerdictDrop))
	b.add(dropExprs)
}

// LogDropProto adds a log+drop rule pair matching an IP protocol.
func (b *Builder) LogDropProto(prefix string, proto byte) {
	b.LogAndDrop(prefix, matchIPv4Proto(proto))
}

// LogDropDstPort adds a log+drop rule pair matching a destination port.
func (b *Builder) LogDropDstPort(prefix string, proto byte, port uint16) {
	exprs := make([]expr.Any, 0, 4)
	exprs = append(exprs, matchL4Proto(proto)...)
	exprs = append(exprs, matchDstPort(port)...)
	b.LogAndDrop(prefix, exprs)
}

// LogDropDstIPPort adds a log+drop rule pair matching destination IP + port.
func (b *Builder) LogDropDstIPPort(prefix string, ip net.IP, proto byte, port uint16) {
	exprs := make([]expr.Any, 0, 6)
	exprs = append(exprs, matchDstIP(ip)...)
	exprs = append(exprs, matchL4Proto(proto)...)
	exprs = append(exprs, matchDstPort(port)...)
	b.LogAndDrop(prefix, exprs)
}

// --- Composite helpers ---

// DoHDropRules blocks TCP 443 to well-known DoH resolver IPs with logging.
func (b *Builder) DoHDropRules() error {
	for _, ipStr := range DoHResolverIPs {
		ip, err := ParseIPv4(ipStr)
		if err != nil {
			return fmt.Errorf("invalid DoH resolver IP %q: %w", ipStr, err)
		}
		b.LogDropDstIPPort("vmsan-deny-doh: ", ip, unix.IPPROTO_TCP, 443)
	}
	return nil
}

// CrossVMIsolation blocks traffic to all internal VM subnets.
func (b *Builder) CrossVMIsolation() error {
	for _, subnet := range CrossVMSubnets {
		if err := b.MatchDstCIDR(subnet, expr.VerdictDrop); err != nil {
			return fmt.Errorf("cross-VM isolation %s: %w", subnet, err)
		}
	}
	return nil
}

// DNSRules allows DNS to configured resolvers and blocks DNS to all others.
func (b *Builder) DNSRules(resolvers []string) error {
	if len(resolvers) == 0 {
		return nil
	}

	for _, resolver := range resolvers {
		if err := b.DNSForwardAccept(resolver, unix.IPPROTO_UDP); err != nil {
			return fmt.Errorf("dns forward accept (udp) %s: %w", resolver, err)
		}
		if err := b.DNSForwardAccept(resolver, unix.IPPROTO_TCP); err != nil {
			return fmt.Errorf("dns forward accept (tcp) %s: %w", resolver, err)
		}
	}

	// Block all non-resolver DNS
	b.MatchDstPort(unix.IPPROTO_UDP, 53, expr.VerdictDrop)
	b.MatchDstPort(unix.IPPROTO_TCP, 53, expr.VerdictDrop)
	return nil
}

// --- Chain constructors ---

// AddNATChain creates a NAT chain with the given hook and priority.
func AddNATChain(c NftablesClient, table *nftables.Table, name string, hook *nftables.ChainHook, priority *nftables.ChainPriority) *nftables.Chain {
	return c.AddChain(&nftables.Chain{
		Name:     name,
		Table:    table,
		Type:     nftables.ChainTypeNAT,
		Hooknum:  hook,
		Priority: priority,
	})
}

// AddFilterChain creates a filter chain with policy DROP.
func AddFilterChain(c NftablesClient, table *nftables.Table, name string, hook *nftables.ChainHook, priority *nftables.ChainPriority) *nftables.Chain {
	policyDrop := nftables.ChainPolicyDrop
	return c.AddChain(&nftables.Chain{
		Name:     name,
		Table:    table,
		Type:     nftables.ChainTypeFilter,
		Hooknum:  hook,
		Priority: priority,
		Policy:   &policyDrop,
	})
}

// --- Low-level expression helpers (private) ---

// matchPayload loads a field from the packet into register 1.
func matchPayload(base expr.PayloadBase, offset, length uint32) expr.Any {
	return &expr.Payload{
		DestRegister: 1,
		Base:         base,
		Offset:       offset,
		Len:          length,
	}
}

// matchCmp compares register 1 against the given data.
func matchCmp(op expr.CmpOp, data []byte) expr.Any {
	return &expr.Cmp{Op: op, Register: 1, Data: data}
}

// matchCmpEq is shorthand for an equality comparison on register 1.
func matchCmpEq(data []byte) expr.Any {
	return matchCmp(expr.CmpOpEq, data)
}

// verdict returns a verdict expression.
func verdict(kind expr.VerdictKind) expr.Any {
	return &expr.Verdict{Kind: kind}
}

// matchL4Proto loads the L4 protocol meta key into register 1 and compares.
func matchL4Proto(proto byte) []expr.Any {
	return []expr.Any{
		&expr.Meta{Key: expr.MetaKeyL4PROTO, Register: 1},
		matchCmpEq([]byte{proto}),
	}
}

// matchDstPort matches the destination port (transport header offset 2).
func matchDstPort(port uint16) []expr.Any {
	return []expr.Any{
		matchPayload(expr.PayloadBaseTransportHeader, 2, 2),
		matchCmpEq(bigEndianUint16(port)),
	}
}

// matchIPv4Proto matches the IPv4 protocol field (network header offset 9).
func matchIPv4Proto(proto byte) []expr.Any {
	return []expr.Any{
		matchPayload(expr.PayloadBaseNetworkHeader, ipv4OffsetProtocol, 1),
		matchCmpEq([]byte{proto}),
	}
}

// matchDstIP matches the destination IPv4 address.
func matchDstIP(ip net.IP) []expr.Any {
	return []expr.Any{
		matchPayload(expr.PayloadBaseNetworkHeader, IPv4OffsetDstAddr, 4),
		matchCmpEq(ip.To4()),
	}
}

// matchIPAddr matches an IPv4 address at the given offset (src or dst).
func matchIPAddr(ip net.IP, offset uint32) []expr.Any {
	return []expr.Any{
		matchPayload(expr.PayloadBaseNetworkHeader, offset, 4),
		matchCmpEq(ip.To4()),
	}
}

// matchIface matches input and output interface names.
func matchIface(iifname, oifname string) []expr.Any {
	return []expr.Any{
		&expr.Meta{Key: expr.MetaKeyIIFNAME, Register: 1},
		matchCmpEq(padIfname(iifname)),
		&expr.Meta{Key: expr.MetaKeyOIFNAME, Register: 1},
		matchCmpEq(padIfname(oifname)),
	}
}

// matchEstablished matches ct state established,related.
func matchEstablished() []expr.Any {
	return []expr.Any{
		&expr.Ct{Register: 1, Key: expr.CtKeySTATE},
		&expr.Bitwise{
			SourceRegister: 1,
			DestRegister:   1,
			Len:            4,
			Mask:           nativeEndianUint32(expr.CtStateBitESTABLISHED | expr.CtStateBitRELATED),
			Xor:            nativeEndianUint32(0),
		},
		&expr.Cmp{
			Op:       expr.CmpOpNeq,
			Register: 1,
			Data:     nativeEndianUint32(0),
		},
	}
}

// matchCIDR matches a destination CIDR (network + prefix mask).
func matchCIDR(network net.IP, prefixLen int) []expr.Any {
	return []expr.Any{
		matchPayload(expr.PayloadBaseNetworkHeader, IPv4OffsetDstAddr, 4),
		&expr.Bitwise{
			SourceRegister: 1,
			DestRegister:   1,
			Len:            4,
			Mask:           cidrMask(prefixLen),
			Xor:            []byte{0, 0, 0, 0},
		},
		matchCmpEq(network),
	}
}

// dnatExprs returns expressions for a DNAT rule: match proto + dport, then DNAT.
func dnatExprs(proto byte, srcPort uint16, dstIP net.IP, dstPort uint16) []expr.Any {
	exprs := make([]expr.Any, 0, 8)
	exprs = append(exprs, matchL4Proto(proto)...)
	exprs = append(exprs, matchDstPort(srcPort)...)
	exprs = append(exprs,
		&expr.Immediate{Register: 1, Data: dstIP.To4()},
		&expr.Immediate{Register: 2, Data: bigEndianUint16(dstPort)},
		&expr.NAT{
			Type:        expr.NATTypeDestNAT,
			Family:      2, // unix.NFPROTO_IPV4
			RegAddrMin:  1,
			RegProtoMin: 2,
		},
	)
	return exprs
}

// logExpr returns a log expression with the given prefix.
func logExpr(prefix string) expr.Any {
	return &expr.Log{
		Key:  (1 << unix.NFTA_LOG_PREFIX) | (1 << unix.NFTA_LOG_LEVEL),
		Data: []byte(prefix),
		Level: expr.LogLevelWarning,
	}
}

// --- Byte encoding helpers ---

// parseCIDRv4 parses a CIDR string and returns the IPv4 network address and prefix length.
func parseCIDRv4(cidr string) (network net.IP, prefixLen int, err error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, 0, fmt.Errorf("invalid CIDR %q: %w", cidr, err)
	}
	ip4 := ipNet.IP.To4()
	if ip4 == nil {
		return nil, 0, fmt.Errorf("not an IPv4 CIDR: %s", cidr)
	}
	ones, _ := ipNet.Mask.Size()
	return ip4, ones, nil
}

// ParseIPv4 parses an IPv4 address string.
func ParseIPv4(s string) (net.IP, error) {
	ip := net.ParseIP(s)
	if ip == nil {
		return nil, fmt.Errorf("invalid IP address: %s", s)
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return nil, fmt.Errorf("not an IPv4 address: %s", s)
	}
	return ip4, nil
}

// ProtoNum converts a protocol string to its IP protocol number.
func ProtoNum(proto string) byte {
	switch proto {
	case "tcp":
		return 6 // unix.IPPROTO_TCP
	case "udp":
		return 17 // unix.IPPROTO_UDP
	default:
		return 0
	}
}

// padIfname pads an interface name to IFNAMSIZ (16 bytes) for nftables comparison.
func padIfname(name string) []byte {
	b := make([]byte, ifnameSize)
	copy(b, name)
	return b
}

// cidrMask converts a prefix length to a 4-byte network mask.
func cidrMask(ones int) []byte {
	mask := make([]byte, 4)
	binary.BigEndian.PutUint32(mask, ^uint32(0)<<(32-ones))
	return mask
}

// bigEndianUint16 encodes a uint16 in big-endian (network byte order).
func bigEndianUint16(v uint16) []byte {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, v)
	return b
}

// nativeEndianUint32 encodes a uint32 in native byte order.
// Used for nftables ct state bitmask comparisons.
func nativeEndianUint32(v uint32) []byte {
	b := make([]byte, 4)
	binary.NativeEndian.PutUint32(b, v)
	return b
}
