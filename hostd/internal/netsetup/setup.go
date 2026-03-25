package netsetup

import (
	"fmt"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"strings"
	"time"
)

// SetupConfig holds the configuration for network setup.
type SetupConfig struct {
	VMId          string
	Slot          int
	TAPDevice     string // e.g. "fhvm0"
	HostIP        string // e.g. "198.19.0.1"
	GuestIP       string // e.g. "198.19.0.2"
	SubnetMask    string // "255.255.255.252"
	MACAddress    string
	NetNSName     string // e.g. "vmsan-vm-abc123" (empty = no namespace)
	VethHost      string // e.g. "veth-h-0"
	VethGuest     string // e.g. "veth-g-0"
	DefaultIface  string // e.g. "eth0" (host default interface)
	BandwidthMbit int    // 0 = no throttle
}

// SetupNamespace creates the network namespace and veth pair.
// If NetNSName is empty, this is a no-op.
func SetupNamespace(cfg SetupConfig) error {
	if cfg.NetNSName == "" {
		return nil
	}

	transitHost := TransitHostIP(cfg.Slot)
	transitGuest := TransitGuestIP(cfg.Slot)

	// Clean up stale veth pair if it exists
	if fileExists(fmt.Sprintf("/sys/class/net/%s", cfg.VethHost)) {
		_ = run("ip", "link", "delete", cfg.VethHost)
	}

	// Clean up stale namespace if it exists from a previous lifecycle
	if fileExists("/var/run/netns/" + cfg.NetNSName) {
		_ = run("ip", "netns", "delete", cfg.NetNSName)
	}

	// 1. Create namespace
	if err := run("ip", "netns", "add", cfg.NetNSName); err != nil {
		return fmt.Errorf("create namespace %s: %w", cfg.NetNSName, err)
	}

	// 2. Create veth pair
	if err := run("ip", "link", "add", cfg.VethHost, "type", "veth", "peer", "name", cfg.VethGuest); err != nil {
		return fmt.Errorf("create veth pair: %w", err)
	}

	// 3. Move guest end into namespace
	if err := run("ip", "link", "set", cfg.VethGuest, "netns", cfg.NetNSName); err != nil {
		return fmt.Errorf("move veth to namespace: %w", err)
	}

	// 4. Configure host side
	if err := run("ip", "addr", "add", transitHost+"/30", "dev", cfg.VethHost); err != nil {
		return fmt.Errorf("assign host veth IP: %w", err)
	}
	if err := run("ip", "link", "set", cfg.VethHost, "up"); err != nil {
		return fmt.Errorf("bring up host veth: %w", err)
	}

	// 5. Configure guest side inside namespace
	if err := nsRun(cfg.NetNSName, "ip", "addr", "add", transitGuest+"/30", "dev", cfg.VethGuest); err != nil {
		return fmt.Errorf("assign guest veth IP: %w", err)
	}
	if err := nsRun(cfg.NetNSName, "ip", "link", "set", cfg.VethGuest, "up"); err != nil {
		return fmt.Errorf("bring up guest veth: %w", err)
	}
	if err := nsRun(cfg.NetNSName, "ip", "link", "set", "lo", "up"); err != nil {
		return fmt.Errorf("bring up loopback: %w", err)
	}

	// 6. Default route inside namespace via host veth
	if err := nsRun(cfg.NetNSName, "ip", "route", "add", "default", "via", transitHost); err != nil {
		return fmt.Errorf("add default route in namespace: %w", err)
	}

	// 7. Enable IP forwarding inside namespace
	if err := nsRun(cfg.NetNSName, "sysctl", "-w", "net.ipv4.ip_forward=1"); err != nil {
		return fmt.Errorf("enable forwarding in namespace: %w", err)
	}

	// 8. Host: route VM subnet via netns veth
	linkCIDR := VMLinkCIDR(cfg.Slot)
	if err := run("ip", "route", "add", linkCIDR, "via", transitGuest); err != nil {
		return fmt.Errorf("add host route to VM subnet: %w", err)
	}

	// 9. Host: enable IP forwarding
	if err := run("sysctl", "-w", "net.ipv4.ip_forward=1"); err != nil {
		return fmt.Errorf("enable host forwarding: %w", err)
	}

	return nil
}

// SetupTAP creates the TAP device inside the namespace (or host if no namespace).
func SetupTAP(cfg SetupConfig) error {
	if cfg.NetNSName == "" {
		// Non-namespaced: create TAP in host namespace
		if fileExists(fmt.Sprintf("/sys/class/net/%s", cfg.TAPDevice)) {
			_ = run("ip", "link", "delete", cfg.TAPDevice)
		}

		if err := run("ip", "tuntap", "add", "dev", cfg.TAPDevice, "mode", "tap"); err != nil {
			return fmt.Errorf("create TAP device: %w", err)
		}
		if err := run("ip", "addr", "add", cfg.HostIP+"/30", "dev", cfg.TAPDevice); err != nil {
			return fmt.Errorf("assign TAP IP: %w", err)
		}
		if err := run("ip", "link", "set", cfg.TAPDevice, "up"); err != nil {
			return fmt.Errorf("bring up TAP: %w", err)
		}
		if err := run("sysctl", "-w", "net.ipv4.ip_forward=1"); err != nil {
			return fmt.Errorf("enable host forwarding: %w", err)
		}
	} else {
		// Namespaced: create TAP inside the network namespace
		if err := nsRun(cfg.NetNSName, "ip", "tuntap", "add", "dev", cfg.TAPDevice, "mode", "tap"); err != nil {
			return fmt.Errorf("create TAP in namespace: %w", err)
		}
		if err := nsRun(cfg.NetNSName, "ip", "addr", "add", cfg.HostIP+"/30", "dev", cfg.TAPDevice); err != nil {
			return fmt.Errorf("assign TAP IP in namespace: %w", err)
		}
		if err := nsRun(cfg.NetNSName, "ip", "link", "set", cfg.TAPDevice, "up"); err != nil {
			return fmt.Errorf("bring up TAP in namespace: %w", err)
		}
	}
	return nil
}

// SetupRoutes configures host-side routing for the VM.
// This is a no-op when using namespaces (routing is done by SetupNamespace).
func SetupRoutes(cfg SetupConfig) error {
	// For namespaced VMs, routing is handled by SetupNamespace.
	// For non-namespaced VMs, TAP is directly on host — no extra routes needed.
	return nil
}

// SetupThrottle applies bandwidth limiting via tc tbf qdisc.
func SetupThrottle(cfg SetupConfig) error {
	if cfg.BandwidthMbit <= 0 {
		return nil
	}

	rateKbit := cfg.BandwidthMbit * 1000
	burstKb := int(math.Max(32, float64(rateKbit/8)))
	rateMbit := fmt.Sprintf("%dmbit", cfg.BandwidthMbit)
	burst := fmt.Sprintf("%dkb", burstKb)

	args := []string{"tc", "qdisc", "add", "dev", cfg.TAPDevice, "root", "tbf",
		"rate", rateMbit, "burst", burst, "latency", "400ms"}

	if cfg.NetNSName != "" {
		return nsRun(cfg.NetNSName, args...)
	}
	return run(args...)
}

// TeardownNamespace removes the namespace and host route.
// Kernel auto-cleans veth pair when namespace is deleted.
func TeardownNamespace(cfg SetupConfig) error {
	if cfg.NetNSName == "" {
		return nil
	}

	// Remove host route (best-effort)
	linkCIDR := VMLinkCIDR(cfg.Slot)
	if err := run("ip", "route", "del", linkCIDR); err != nil {
		slog.Debug("namespace teardown: route del failed", "cidr", linkCIDR, "error", err)
	}

	// Delete namespace — auto-cleans veth pair, TAP, rules inside
	nsPath := "/var/run/netns/" + cfg.NetNSName
	if !fileExists(nsPath) {
		return nil // already gone
	}
	if err := run("ip", "netns", "delete", cfg.NetNSName); err != nil {
		slog.Debug("namespace teardown: netns delete failed, retrying", "netns", cfg.NetNSName, "error", err)
		time.Sleep(500 * time.Millisecond)
		if fileExists(nsPath) {
			if err := run("ip", "netns", "delete", cfg.NetNSName); err != nil {
				slog.Debug("namespace teardown: netns delete retry failed", "netns", cfg.NetNSName, "error", err)
			}
		}
	}

	return nil
}

// TeardownTAP removes the TAP device (only for non-namespaced VMs).
func TeardownTAP(tapDevice, netnsName string) error {
	if netnsName != "" {
		return nil // TAP auto-cleaned by namespace deletion
	}
	return run("ip", "link", "delete", tapDevice)
}

// TeardownThrottle removes tc qdisc from TAP device (best-effort, silent if none set).
func TeardownThrottle(tapDevice, netnsName string) error {
	var err error
	if netnsName != "" {
		err = nsRun(netnsName, "tc", "qdisc", "del", "dev", tapDevice, "root")
	} else {
		err = run("tc", "qdisc", "del", "dev", tapDevice, "root")
	}
	// "Cannot delete qdisc with handle of zero" means no qdisc was set — not an error
	if err != nil && strings.Contains(err.Error(), "handle of zero") {
		return nil
	}
	return err
}

// DetectDefaultInterface finds the host's default network interface.
func DetectDefaultInterface() (string, error) {
	out, err := exec.Command("ip", "route", "show", "default").Output()
	if err != nil {
		return "", fmt.Errorf("ip route show default: %w", err)
	}
	// Parse "default via X.X.X.X dev <iface> ..."
	fields := strings.Fields(string(out))
	for i, f := range fields {
		if f == "dev" && i+1 < len(fields) {
			return fields[i+1], nil
		}
	}
	return "", fmt.Errorf("default interface not found in: %s", strings.TrimSpace(string(out)))
}

// run executes a command with explicit argument list (no shell injection).
// Stderr is captured (not dumped to gateway output) to avoid noisy false positives.
func run(args ...string) error {
	cmd := exec.Command(args[0], args[1:]...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

// nsRun executes a command inside a network namespace.
func nsRun(netns string, args ...string) error {
	fullArgs := append([]string{"ip", "netns", "exec", netns}, args...)
	return run(fullArgs...)
}

// fileExists checks if a path exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
