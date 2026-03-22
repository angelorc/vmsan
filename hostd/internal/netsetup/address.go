// Package netsetup provides network namespace, veth, TAP, and routing
// operations for VM network isolation.
package netsetup

import "fmt"

const (
	// VMNetworkPrefix is the first two octets of VM IP addresses.
	VMNetworkPrefix = "198.19"
	// VMSubnetMask is the /30 subnet mask for each VM link.
	VMSubnetMask = "255.255.255.252"
	// DNSPortBase is the starting port for per-VM DNS proxies.
	DNSPortBase = 10053
	// SNIPortBase is the starting port for per-VM SNI proxies.
	SNIPortBase = 10443
	// HTTPPortBase is the starting port for per-VM HTTP proxies.
	HTTPPortBase = 10698
	// MaxSlot is the maximum VM network slot (0-254).
	MaxSlot = 254
)

// VMHostIP returns the host-side IP for a VM slot: 198.19.<slot>.1
func VMHostIP(slot int) string {
	return fmt.Sprintf("%s.%d.1", VMNetworkPrefix, slot)
}

// VMGuestIP returns the guest-side IP for a VM slot: 198.19.<slot>.2
func VMGuestIP(slot int) string {
	return fmt.Sprintf("%s.%d.2", VMNetworkPrefix, slot)
}

// VethHostDev returns the host-side veth device name: veth-h-<slot>
func VethHostDev(slot int) string {
	return fmt.Sprintf("veth-h-%d", slot)
}

// VethGuestDev returns the guest-side veth device name: veth-g-<slot>
func VethGuestDev(slot int) string {
	return fmt.Sprintf("veth-g-%d", slot)
}

// TransitHostIP returns the host-side transit IP: 10.200.<slot>.1
func TransitHostIP(slot int) string {
	return fmt.Sprintf("10.200.%d.1", slot)
}

// TransitGuestIP returns the guest-side transit IP: 10.200.<slot>.2
func TransitGuestIP(slot int) string {
	return fmt.Sprintf("10.200.%d.2", slot)
}

// TAPDevice returns the TAP device name: fhvm<slot>
func TAPDevice(slot int) string {
	return fmt.Sprintf("fhvm%d", slot)
}

// DNSPort returns the DNS proxy port for a slot: 10053 + slot
func DNSPort(slot int) int {
	return DNSPortBase + slot
}

// SNIPort returns the SNI proxy port for a slot: 10443 + slot
func SNIPort(slot int) int {
	return SNIPortBase + slot
}

// HTTPPort returns the HTTP proxy port for a slot: 10698 + slot
func HTTPPort(slot int) int {
	return HTTPPortBase + slot
}

// NetNSName returns the network namespace name for a VM: vmsan-<vmId>
func NetNSName(vmId string) string {
	return "vmsan-" + vmId
}

// MACAddress returns a deterministic MAC address for a slot.
// Format: AA:FC:00:00:00:XX where XX = slot + 1
func MACAddress(slot int) string {
	return fmt.Sprintf("AA:FC:00:00:00:%02X", slot+1)
}

// VMLinkCIDR returns the /30 CIDR for a VM's link: 198.19.<slot>.0/30
func VMLinkCIDR(slot int) string {
	return fmt.Sprintf("%s.%d.0/30", VMNetworkPrefix, slot)
}

// IsReservedPort checks if a port falls within any reserved proxy range.
func IsReservedPort(port int) bool {
	return (port >= DNSPortBase && port < DNSPortBase+255) ||
		(port >= SNIPortBase && port < SNIPortBase+255) ||
		(port >= HTTPPortBase && port < HTTPPortBase+255)
}

// BootArgs returns the kernel boot arguments for a VM's network config.
// Format: console=ttyS0 reboot=k panic=1 pci=off ip=<guest>::<host>:<mask>::eth0:off:<dns>
func BootArgs(guestIP, hostIP, subnetMask string) string {
	return fmt.Sprintf(
		"console=ttyS0 reboot=k panic=1 pci=off ip=%s::%s:%s::eth0:off:8.8.8.8",
		guestIP, hostIP, subnetMask,
	)
}
