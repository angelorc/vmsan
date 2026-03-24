package mesh

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// AddRoute adds a host-side route for a mesh IP via the transit guest IP.
// Uses "via <transitGuestIP>" so ARP resolves against the known transit IP
// (10.200.X.2) rather than the mesh IP (10.90.X.X) which would fail because
// the mesh IP isn't in the veth's subnet.
//
// Example: ip route add 10.90.0.1/32 via 10.200.0.2 dev veth-h-0
func AddRoute(meshIP string, vethHost string, transitGuestIP string) error {
	cmd := exec.Command("ip", "route", "add", meshIP+"/32", "via", transitGuestIP, "dev", vethHost)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("add mesh route %s via %s dev %s: %w: %s", meshIP, transitGuestIP, vethHost, err, out)
	}
	return nil
}

// RemoveRoute removes a mesh IP route from the host routing table.
//
// Example: ip route del 10.90.0.1/32
func RemoveRoute(meshIP string) error {
	cmd := exec.Command("ip", "route", "del", meshIP+"/32")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("remove mesh route %s: %w: %s", meshIP, err, out)
	}
	return nil
}

// AddGuestRoute sets up mesh IP routing inside the VM's network namespace.
//
// Two NAT rules are created in the vmsan_mesh nftables table:
//
// DNAT (prerouting, priority -101): rewrites dst from meshIP to guestIP so
// traffic is forwarded to the VM via the TAP device. Priority -101 fires
// before the per-VM prerouting chain at -100.
//
// SNAT (postrouting, priority 100): rewrites src to meshIP for outgoing
// traffic destined to the mesh subnet (10.90.0.0/16). This ensures the
// return path uses mesh host routes (10.90.X.X via 10.200.X.2) rather than
// requiring host routes for each guest subnet (198.19.X.0/30).
//
// IMPORTANT: we do NOT assign meshIP as a local address (ip addr add).
// A local address creates a "local" route in table local (priority 0),
// causing the kernel to deliver packets to INPUT and bypass NAT DNAT.
// The host route uses "via transitGuestIP" so ARP resolves against the
// transit IP (10.200.X.2) which IS assigned to veth-g — no mesh IP needed.
func AddGuestRoute(nsName string, meshIP string, guestIP string, _ string) error {
	nftRules := fmt.Sprintf(`add table ip vmsan_mesh
add chain ip vmsan_mesh mesh_pre { type nat hook prerouting priority -101 ; }
add rule ip vmsan_mesh mesh_pre ip daddr %s counter dnat to %s
add chain ip vmsan_mesh mesh_post { type nat hook postrouting priority 100 ; }
add rule ip vmsan_mesh mesh_post ip daddr 10.90.0.0/16 counter snat to %s
`, meshIP, guestIP, meshIP)

	cmd := exec.Command("ip", "netns", "exec", nsName, "nft", "-f", "-")
	cmd.Stdin = strings.NewReader(nftRules)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("add mesh NAT %s↔%s in ns %s: %w: %s", meshIP, guestIP, nsName, err, string(out))
	}

	fmt.Fprintf(os.Stderr, "[mesh] DNAT %s→%s + SNAT→%s in ns %s\n", meshIP, guestIP, meshIP, nsName)
	return nil
}

// AddMeshPortDNAT adds a DNAT rule in the namespace so that mesh traffic
// arriving at meshIP:port is forwarded to the VM's guest IP (on the TAP device).
// Without this, traffic to the mesh IP is delivered locally in the namespace
// and never reaches the VM behind the TAP.
//
// Example: ip netns exec <ns> nft add rule ip nat prerouting ip daddr 10.90.0.2 tcp dport 5432 dnat to 198.19.0.2
func AddMeshPortDNAT(nsName, meshIP, guestIP, proto string, port int) error {
	// Ensure nat table and prerouting chain exist in the namespace.
	setup := "add table ip nat\nadd chain ip nat mesh_prerouting { type nat hook prerouting priority -100 ; }\n"
	setupCmd := exec.Command("ip", "netns", "exec", nsName, "nft", "-f", "-")
	setupCmd.Stdin = strings.NewReader(setup)
	setupCmd.Run() // best-effort, may already exist

	// Port 0 means DNAT all TCP traffic to this mesh IP (any port).
	var rule string
	if port > 0 {
		rule = fmt.Sprintf("add rule ip nat mesh_prerouting ip daddr %s %s dport %d dnat to %s\n", meshIP, proto, port, guestIP)
	} else {
		rule = fmt.Sprintf("add rule ip nat mesh_prerouting ip daddr %s ip protocol %s dnat to %s\n", meshIP, proto, guestIP)
	}
	cmd := exec.Command("ip", "netns", "exec", nsName, "nft", "-f", "-")
	cmd.Stdin = strings.NewReader(rule)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("add mesh DNAT %s→%s in ns %s: %w: %s", meshIP, guestIP, nsName, err, out)
	}
	return nil
}

// RemoveGuestRoute removes the mesh DNAT table from the VM's network namespace.
func RemoveGuestRoute(nsName string, _ string, _ string) error {
	cmd := exec.Command("ip", "netns", "exec", nsName, "nft", "delete", "table", "ip", "vmsan_mesh")
	cmd.Run() // best-effort
	return nil
}
