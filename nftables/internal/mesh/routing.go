package mesh

import (
	"fmt"
	"os/exec"
)

// AddRoute adds a host-side route for a mesh IP through the target VM's veth.
// This makes the mesh IP reachable from the host, which enables forwarding
// between VMs via the host's routing table.
//
// Example: ip route add 10.90.0.1/32 dev veth-h-0
func AddRoute(meshIP string, vethHost string) error {
	cmd := exec.Command("ip", "route", "add", meshIP+"/32", "dev", vethHost)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("add mesh route %s via %s: %w: %s", meshIP, vethHost, err, out)
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

// AddGuestRoute configures a mesh IP on the guest interface inside the VM's
// network namespace. This allows the VM to receive traffic on its mesh IP.
//
// Example: ip netns exec <ns> ip addr add 10.90.0.1/32 dev eth0
func AddGuestRoute(nsName string, meshIP string, guestDev string) error {
	cmd := exec.Command("ip", "netns", "exec", nsName, "ip", "addr", "add", meshIP+"/32", "dev", guestDev)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("add guest mesh addr %s in ns %s on %s: %w: %s", meshIP, nsName, guestDev, err, out)
	}
	return nil
}

// RemoveGuestRoute removes a mesh IP from the guest interface inside the VM's
// network namespace.
//
// Example: ip netns exec <ns> ip addr del 10.90.0.1/32 dev eth0
func RemoveGuestRoute(nsName string, meshIP string, guestDev string) error {
	cmd := exec.Command("ip", "netns", "exec", nsName, "ip", "addr", "del", meshIP+"/32", "dev", guestDev)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("remove guest mesh addr %s in ns %s on %s: %w: %s", meshIP, nsName, guestDev, err, out)
	}
	return nil
}
