package gateway

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/angelorc/vmsan/hostd/internal/firewall"
)

// DoctorCheck represents a single health check result.
type DoctorCheck struct {
	Category string `json:"category"`
	Name     string `json:"name"`
	Status   string `json:"status"` // "pass", "fail", "warn"
	Detail   string `json:"detail"`
	Fix      string `json:"fix,omitempty"`
}

// handleDoctor runs system health checks and returns results.
func (s *Server) handleDoctor() Response {
	checks := runDoctorChecks()
	return Response{OK: true, List: checks}
}

// runDoctorChecks executes all diagnostic checks.
func runDoctorChecks() []DoctorCheck {
	var checks []DoctorCheck
	checks = append(checks, checkKVMAccess())
	checks = append(checks, checkTUNDevice())
	checks = append(checks, checkDiskSpace())
	checks = append(checks, checkDefaultInterface())
	checks = append(checks, checkNftablesKernel())
	checks = append(checks, checkHostFirewall())
	checks = append(checks, checkJailerFilesystem())
	checks = append(checks, checkIptables())
	checks = append(checks, checkBinaries()...)
	checks = append(checks, checkKernelImage())
	checks = append(checks, checkRootfsImage())
	checks = append(checks, checkGatewayDaemon())
	return checks
}

func checkKVMAccess() DoctorCheck {
	check := DoctorCheck{Category: "virtualization", Name: "KVM access"}
	err := syscall.Access("/dev/kvm", syscall.O_RDWR)
	if err != nil {
		check.Status = "fail"
		check.Detail = fmt.Sprintf("/dev/kvm not accessible: %s", err)
		check.Fix = "Ensure KVM is enabled and current user has access: sudo chmod 666 /dev/kvm"
		return check
	}
	check.Status = "pass"
	check.Detail = "/dev/kvm is accessible"
	return check
}

func checkTUNDevice() DoctorCheck {
	check := DoctorCheck{Category: "networking", Name: "TUN device"}
	err := syscall.Access("/dev/net/tun", syscall.O_RDWR)
	if err != nil {
		check.Status = "fail"
		check.Detail = fmt.Sprintf("/dev/net/tun not accessible: %s", err)
		check.Fix = "Ensure TUN module is loaded: sudo modprobe tun"
		return check
	}
	check.Status = "pass"
	check.Detail = "/dev/net/tun is accessible"
	return check
}

func checkDiskSpace() DoctorCheck {
	check := DoctorCheck{Category: "storage", Name: "disk space"}
	var stat syscall.Statfs_t
	dir := jailerBaseDir
	if err := syscall.Statfs(dir, &stat); err != nil {
		// Try parent dir if jailer dir doesn't exist yet.
		dir = filepath.Dir(dir)
		if err := syscall.Statfs(dir, &stat); err != nil {
			check.Status = "warn"
			check.Detail = fmt.Sprintf("cannot stat %s: %s", jailerBaseDir, err)
			return check
		}
	}
	freeBytes := stat.Bavail * uint64(stat.Bsize)
	freeGB := float64(freeBytes) / (1024 * 1024 * 1024)
	minFreeGB := 5.0
	if freeGB < minFreeGB {
		check.Status = "fail"
		check.Detail = fmt.Sprintf("%.1f GB free on %s (minimum %.0f GB)", freeGB, dir, minFreeGB)
		check.Fix = "Free up disk space or expand the volume"
		return check
	}
	check.Status = "pass"
	check.Detail = fmt.Sprintf("%.1f GB free on %s", freeGB, dir)
	return check
}

func checkDefaultInterface() DoctorCheck {
	check := DoctorCheck{Category: "networking", Name: "default interface"}
	out, err := exec.Command("ip", "route", "show", "default").Output()
	if err != nil {
		check.Status = "fail"
		check.Detail = fmt.Sprintf("failed to run ip route: %s", err)
		check.Fix = "Ensure network is configured with a default route"
		return check
	}
	fields := strings.Fields(string(out))
	iface := ""
	for i, f := range fields {
		if f == "dev" && i+1 < len(fields) {
			iface = fields[i+1]
			break
		}
	}
	if iface == "" {
		check.Status = "fail"
		check.Detail = "no default route found"
		check.Fix = "Configure a default network route"
		return check
	}
	check.Status = "pass"
	check.Detail = fmt.Sprintf("default interface: %s", iface)
	return check
}

func checkNftablesKernel() DoctorCheck {
	check := DoctorCheck{Category: "firewall", Name: "nftables kernel module"}
	if _, err := os.Stat("/sys/module/nf_tables"); err == nil {
		check.Status = "pass"
		check.Detail = "nf_tables kernel module loaded"
		return check
	}
	// Try verifying via vmsan-nftables binary.
	nftBin := findBinary("vmsan-nftables")
	if nftBin != "" {
		if out, err := exec.Command(nftBin, "verify").CombinedOutput(); err == nil {
			check.Status = "pass"
			check.Detail = fmt.Sprintf("vmsan-nftables verify passed: %s", strings.TrimSpace(string(out)))
			return check
		}
	}
	check.Status = "fail"
	check.Detail = "nf_tables kernel module not loaded"
	check.Fix = "Load the module: sudo modprobe nf_tables"
	return check
}

func checkHostFirewall() DoctorCheck {
	check := DoctorCheck{Category: "firewall", Name: "host firewall"}
	// Check ufw.
	if out, err := exec.Command("ufw", "status").CombinedOutput(); err == nil {
		status := strings.TrimSpace(string(out))
		if strings.Contains(status, "Status: active") {
			check.Status = "warn"
			check.Detail = "ufw is active — may conflict with vmsan nftables rules"
			check.Fix = "Consider disabling ufw: sudo ufw disable"
			return check
		}
	}
	// Check firewalld.
	if out, err := exec.Command("systemctl", "is-active", "firewalld").CombinedOutput(); err == nil {
		status := strings.TrimSpace(string(out))
		if status == "active" {
			check.Status = "warn"
			check.Detail = "firewalld is active — may conflict with vmsan nftables rules"
			check.Fix = "Consider disabling firewalld: sudo systemctl disable --now firewalld"
			return check
		}
	}
	check.Status = "pass"
	check.Detail = "no conflicting host firewall detected"
	return check
}

func checkJailerFilesystem() DoctorCheck {
	check := DoctorCheck{Category: "storage", Name: "jailer filesystem"}
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		check.Status = "warn"
		check.Detail = fmt.Sprintf("cannot read /proc/mounts: %s", err)
		return check
	}
	// Find the mount that contains jailerBaseDir.
	lines := strings.Split(string(data), "\n")
	var bestMount string
	var bestOpts string
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		mountPoint := fields[1]
		opts := fields[3]
		if strings.HasPrefix(jailerBaseDir, mountPoint) && len(mountPoint) > len(bestMount) {
			bestMount = mountPoint
			bestOpts = opts
		}
	}
	if bestMount == "" {
		check.Status = "warn"
		check.Detail = fmt.Sprintf("could not find mount for %s", jailerBaseDir)
		return check
	}
	if strings.Contains(bestOpts, "nodev") {
		check.Status = "warn"
		check.Detail = fmt.Sprintf("%s is mounted with nodev on %s — Firecracker may fail", jailerBaseDir, bestMount)
		check.Fix = "Remount without nodev: sudo mount -o remount,dev " + bestMount
		return check
	}
	check.Status = "pass"
	check.Detail = fmt.Sprintf("%s mount (%s) does not have nodev", jailerBaseDir, bestMount)
	return check
}

func checkIptables() DoctorCheck {
	check := DoctorCheck{Category: "firewall", Name: "iptables"}
	if !firewall.IptablesAvailable() {
		// iptables is only needed for Docker coexistence.
		if _, dockerErr := exec.LookPath("docker"); dockerErr == nil {
			check.Status = "warn"
			check.Detail = "iptables not found but Docker is installed — Docker's FORWARD DROP policy may block VM traffic"
			check.Fix = "Install iptables: apt install iptables"
			return check
		}
		check.Status = "pass"
		check.Detail = "iptables not installed (not needed — no Docker detected)"
		return check
	}
	check.Status = "pass"
	check.Detail = "iptables available"
	return check
}

func checkBinaries() []DoctorCheck {
	binaries := []struct {
		name     string
		required bool
	}{
		{"firecracker", true},
		{"jailer", true},
		{"vmsan", true},
		{"vmsan-agent", false},
		{"dnsproxy", false},
	}
	var checks []DoctorCheck
	for _, bin := range binaries {
		check := DoctorCheck{Category: "binaries", Name: bin.name}
		path := findBinary(bin.name)
		if path != "" {
			check.Status = "pass"
			check.Detail = path
		} else if bin.required {
			check.Status = "fail"
			check.Detail = fmt.Sprintf("%s not found in standard paths", bin.name)
			check.Fix = fmt.Sprintf("Install %s or add it to PATH", bin.name)
		} else {
			check.Status = "warn"
			check.Detail = fmt.Sprintf("%s not found (optional)", bin.name)
		}
		checks = append(checks, check)
	}
	return checks
}

// vmsanSearchDirs returns ~/.vmsan/<subdir> for the current user and all /home/* users.
func vmsanSearchDirs(subdir string) []string {
	var dirs []string
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".vmsan", subdir))
	}
	if entries, err := os.ReadDir("/home"); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				dirs = append(dirs, filepath.Join("/home", e.Name(), ".vmsan", subdir))
			}
		}
	}
	return dirs
}

func checkKernelImage() DoctorCheck {
	check := DoctorCheck{Category: "images", Name: "kernel image"}
	for _, dir := range vmsanSearchDirs("kernels") {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), "vmlinux") {
				check.Status = "pass"
				check.Detail = filepath.Join(dir, e.Name())
				return check
			}
		}
	}
	check.Status = "fail"
	check.Detail = "no vmlinux kernel image found"
	check.Fix = "Download a Firecracker-compatible kernel image"
	return check
}

func checkRootfsImage() DoctorCheck {
	check := DoctorCheck{Category: "images", Name: "rootfs image"}
	for _, dir := range vmsanSearchDirs("rootfs") {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".ext4") {
				check.Status = "pass"
				check.Detail = filepath.Join(dir, e.Name())
				return check
			}
		}
	}
	check.Status = "fail"
	check.Detail = "no rootfs .ext4 image found"
	check.Fix = "Build or download a rootfs image (e.g., ubuntu-24.04.ext4)"
	return check
}

func checkGatewayDaemon() DoctorCheck {
	check := DoctorCheck{Category: "daemon", Name: "gateway process"}
	pidFile := "/run/vmsan/gateway.pid"
	data, err := os.ReadFile(pidFile)
	if err != nil {
		check.Status = "warn"
		check.Detail = fmt.Sprintf("PID file not found: %s", pidFile)
		return check
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		check.Status = "warn"
		check.Detail = fmt.Sprintf("invalid PID in %s", pidFile)
		return check
	}
	// Check if the process is alive.
	proc, err := os.FindProcess(pid)
	if err != nil {
		check.Status = "fail"
		check.Detail = fmt.Sprintf("process %d not found", pid)
		return check
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		check.Status = "fail"
		check.Detail = fmt.Sprintf("process %d not running", pid)
		return check
	}
	check.Status = "pass"
	check.Detail = fmt.Sprintf("gateway running (PID %d)", pid)
	return check
}

// findBinary searches for a binary in standard paths and /home/*/.vmsan/bin/.
func findBinary(name string) string {
	candidates := []string{
		"/usr/local/bin/" + name,
		"/usr/bin/" + name,
	}
	if entries, err := os.ReadDir("/home"); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				candidates = append(candidates, filepath.Join("/home", e.Name(), ".vmsan", "bin", name))
			}
		}
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}
