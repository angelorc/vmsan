package netsetup

import (
	"testing"
)

func TestVMHostIP(t *testing.T) {
	tests := []struct {
		slot int
		want string
	}{
		{0, "198.19.0.1"},
		{1, "198.19.1.1"},
		{127, "198.19.127.1"},
		{254, "198.19.254.1"},
	}
	for _, tt := range tests {
		if got := VMHostIP(tt.slot); got != tt.want {
			t.Errorf("VMHostIP(%d) = %q, want %q", tt.slot, got, tt.want)
		}
	}
}

func TestVMGuestIP(t *testing.T) {
	tests := []struct {
		slot int
		want string
	}{
		{0, "198.19.0.2"},
		{1, "198.19.1.2"},
		{127, "198.19.127.2"},
		{254, "198.19.254.2"},
	}
	for _, tt := range tests {
		if got := VMGuestIP(tt.slot); got != tt.want {
			t.Errorf("VMGuestIP(%d) = %q, want %q", tt.slot, got, tt.want)
		}
	}
}

func TestVethHostDev(t *testing.T) {
	tests := []struct {
		slot int
		want string
	}{
		{0, "veth-h-0"},
		{1, "veth-h-1"},
		{127, "veth-h-127"},
		{254, "veth-h-254"},
	}
	for _, tt := range tests {
		if got := VethHostDev(tt.slot); got != tt.want {
			t.Errorf("VethHostDev(%d) = %q, want %q", tt.slot, got, tt.want)
		}
	}
}

func TestVethGuestDev(t *testing.T) {
	tests := []struct {
		slot int
		want string
	}{
		{0, "veth-g-0"},
		{1, "veth-g-1"},
		{127, "veth-g-127"},
		{254, "veth-g-254"},
	}
	for _, tt := range tests {
		if got := VethGuestDev(tt.slot); got != tt.want {
			t.Errorf("VethGuestDev(%d) = %q, want %q", tt.slot, got, tt.want)
		}
	}
}

func TestTransitHostIP(t *testing.T) {
	tests := []struct {
		slot int
		want string
	}{
		{0, "10.200.0.1"},
		{1, "10.200.1.1"},
		{127, "10.200.127.1"},
		{254, "10.200.254.1"},
	}
	for _, tt := range tests {
		if got := TransitHostIP(tt.slot); got != tt.want {
			t.Errorf("TransitHostIP(%d) = %q, want %q", tt.slot, got, tt.want)
		}
	}
}

func TestTransitGuestIP(t *testing.T) {
	tests := []struct {
		slot int
		want string
	}{
		{0, "10.200.0.2"},
		{1, "10.200.1.2"},
		{127, "10.200.127.2"},
		{254, "10.200.254.2"},
	}
	for _, tt := range tests {
		if got := TransitGuestIP(tt.slot); got != tt.want {
			t.Errorf("TransitGuestIP(%d) = %q, want %q", tt.slot, got, tt.want)
		}
	}
}

func TestTAPDevice(t *testing.T) {
	tests := []struct {
		slot int
		want string
	}{
		{0, "fhvm0"},
		{1, "fhvm1"},
		{127, "fhvm127"},
		{254, "fhvm254"},
	}
	for _, tt := range tests {
		if got := TAPDevice(tt.slot); got != tt.want {
			t.Errorf("TAPDevice(%d) = %q, want %q", tt.slot, got, tt.want)
		}
	}
}

func TestDNSPort(t *testing.T) {
	tests := []struct {
		slot int
		want int
	}{
		{0, 10053},
		{1, 10054},
		{127, 10180},
		{254, 10307},
	}
	for _, tt := range tests {
		if got := DNSPort(tt.slot); got != tt.want {
			t.Errorf("DNSPort(%d) = %d, want %d", tt.slot, got, tt.want)
		}
	}
}

func TestSNIPort(t *testing.T) {
	tests := []struct {
		slot int
		want int
	}{
		{0, 10443},
		{1, 10444},
		{127, 10570},
		{254, 10697},
	}
	for _, tt := range tests {
		if got := SNIPort(tt.slot); got != tt.want {
			t.Errorf("SNIPort(%d) = %d, want %d", tt.slot, got, tt.want)
		}
	}
}

func TestHTTPPort(t *testing.T) {
	tests := []struct {
		slot int
		want int
	}{
		{0, 10698},
		{1, 10699},
		{127, 10825},
		{254, 10952},
	}
	for _, tt := range tests {
		if got := HTTPPort(tt.slot); got != tt.want {
			t.Errorf("HTTPPort(%d) = %d, want %d", tt.slot, got, tt.want)
		}
	}
}

func TestNetNSName(t *testing.T) {
	tests := []struct {
		vmId string
		want string
	}{
		{"abc123", "vmsan-abc123"},
		{"vm-test-1", "vmsan-vm-test-1"},
		{"", "vmsan-"},
	}
	for _, tt := range tests {
		if got := NetNSName(tt.vmId); got != tt.want {
			t.Errorf("NetNSName(%q) = %q, want %q", tt.vmId, got, tt.want)
		}
	}
}

func TestMACAddress(t *testing.T) {
	tests := []struct {
		slot int
		want string
	}{
		{0, "AA:FC:00:00:00:01"},
		{1, "AA:FC:00:00:00:02"},
		{127, "AA:FC:00:00:00:80"},
		{254, "AA:FC:00:00:00:FF"},
	}
	for _, tt := range tests {
		if got := MACAddress(tt.slot); got != tt.want {
			t.Errorf("MACAddress(%d) = %q, want %q", tt.slot, got, tt.want)
		}
	}
}

func TestVMLinkCIDR(t *testing.T) {
	tests := []struct {
		slot int
		want string
	}{
		{0, "198.19.0.0/30"},
		{1, "198.19.1.0/30"},
		{127, "198.19.127.0/30"},
		{254, "198.19.254.0/30"},
	}
	for _, tt := range tests {
		if got := VMLinkCIDR(tt.slot); got != tt.want {
			t.Errorf("VMLinkCIDR(%d) = %q, want %q", tt.slot, got, tt.want)
		}
	}
}

func TestIsReservedPort(t *testing.T) {
	tests := []struct {
		port int
		want bool
	}{
		// Below DNS range
		{10052, false},
		// DNS range boundaries
		{10053, true},
		{10307, true},
		{10308, false},
		// Between DNS and SNI ranges
		{10400, false},
		// SNI range boundaries
		{10443, true},
		{10697, true},
		{10698, true}, // Start of HTTP range
		// HTTP range boundaries
		{10952, true},
		{10953, false},
		// Well outside
		{80, false},
		{443, false},
		{8080, false},
		{65535, false},
	}
	for _, tt := range tests {
		if got := IsReservedPort(tt.port); got != tt.want {
			t.Errorf("IsReservedPort(%d) = %v, want %v", tt.port, got, tt.want)
		}
	}
}

func TestBootArgs(t *testing.T) {
	tests := []struct {
		guestIP    string
		hostIP     string
		subnetMask string
		want       string
	}{
		{
			"198.19.0.2", "198.19.0.1", "255.255.255.252",
			"console=ttyS0 reboot=k panic=1 pci=off ip=198.19.0.2::198.19.0.1:255.255.255.252::eth0:off:8.8.8.8",
		},
		{
			"198.19.127.2", "198.19.127.1", "255.255.255.252",
			"console=ttyS0 reboot=k panic=1 pci=off ip=198.19.127.2::198.19.127.1:255.255.255.252::eth0:off:8.8.8.8",
		},
	}
	for _, tt := range tests {
		if got := BootArgs(tt.guestIP, tt.hostIP, tt.subnetMask); got != tt.want {
			t.Errorf("BootArgs(%q, %q, %q) = %q, want %q", tt.guestIP, tt.hostIP, tt.subnetMask, got, tt.want)
		}
	}
}
