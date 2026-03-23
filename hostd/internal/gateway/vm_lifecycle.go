package gateway

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/angelorc/vmsan/hostd/internal/firecracker"
	"github.com/angelorc/vmsan/hostd/internal/jailer"
	"github.com/angelorc/vmsan/hostd/internal/netsetup"
	nftypes "github.com/angelorc/vmsan/nftables"
)

// Default values for VM creation.
const (
	defaultVCPUs      = 1
	defaultMemMiB     = 128
	defaultRuntime    = "base"
	defaultDiskSizeGb = 0
	defaultPolicy     = "deny-all"
	agentPort         = 9119
	jailerBaseDir     = "/srv/jailer"
	jailerUID         = 123
	jailerGID         = 100
)

// Binary paths — configurable at startup via SetBinDir().
var (
	firecrackerBin = "/usr/bin/firecracker"
	jailerBin      = "/usr/bin/jailer"
)

// SetBinDir configures the directory containing firecracker and jailer binaries.
func SetBinDir(dir string) {
	if dir == "" {
		return
	}
	firecrackerBin = dir + "/firecracker"
	jailerBin = dir + "/jailer"
}

// vmCreateParams holds the parameters for vm.create.
type vmCreateParams struct {
	VCPUs          int      `json:"vcpus"`
	MemMiB         int      `json:"memMib"`
	Runtime        string   `json:"runtime"`
	DiskSizeGb     float64  `json:"diskSizeGb"`
	NetworkPolicy  string   `json:"networkPolicy"`
	Domains        []string `json:"domains,omitempty"`
	AllowedCIDRs   []string `json:"allowedCidrs,omitempty"`
	DeniedCIDRs    []string `json:"deniedCidrs,omitempty"`
	Ports          []int    `json:"ports,omitempty"`
	BandwidthMbit  int      `json:"bandwidthMbit,omitempty"`
	AllowICMP      bool     `json:"allowIcmp,omitempty"`
	Project        string   `json:"project,omitempty"`
	Service        string   `json:"service,omitempty"`
	ConnectTo      []string `json:"connectTo,omitempty"`
	SkipDNAT       bool     `json:"skipDnat,omitempty"`
	KernelPath     string   `json:"kernelPath,omitempty"`
	RootfsPath     string   `json:"rootfsPath,omitempty"`
	SnapshotID     string   `json:"snapshotId,omitempty"`
	AgentBinary    string   `json:"agentBinary,omitempty"`
	AgentToken     string   `json:"agentToken,omitempty"`
	VMId           string   `json:"vmId,omitempty"`
	DisableSeccomp bool     `json:"disableSeccomp,omitempty"`
	DisablePidNs   bool     `json:"disablePidNs,omitempty"`
	DisableCgroup  bool     `json:"disableCgroup,omitempty"`
	SeccompFilter  string   `json:"seccompFilter,omitempty"`
	OwnerUID       int      `json:"ownerUid,omitempty"`
	OwnerGID       int      `json:"ownerGid,omitempty"`
}

// vmCreateResponse is the response for vm.create.
type vmCreateResponse struct {
	VMId       string `json:"vmId"`
	Slot       int    `json:"slot"`
	HostIP     string `json:"hostIp"`
	GuestIP    string `json:"guestIp"`
	MeshIP     string `json:"meshIp,omitempty"`
	TAPDevice  string `json:"tapDevice"`
	MACAddress string `json:"macAddress"`
	NetNSName  string `json:"netnsName"`
	VethHost   string `json:"vethHost"`
	VethGuest  string `json:"vethGuest"`
	SubnetMask string `json:"subnetMask"`
	ChrootDir  string `json:"chrootDir"`
	SocketPath string `json:"socketPath"`
	PID        int    `json:"pid"`
	AgentToken string `json:"agentToken,omitempty"`
	DNSPort    int    `json:"dnsPort"`
	SNIPort    int    `json:"sniPort"`
	HTTPPort   int    `json:"httpPort"`
}

// vmDeleteParams holds the parameters for vm.delete.
type vmDeleteParams struct {
	VMId  string `json:"vmId"`
	Force bool   `json:"force,omitempty"`
}

// networkSetupParams holds the parameters for network.setup.
type networkSetupParams struct {
	VMId          string   `json:"vmId"`
	Slot          int      `json:"slot"`
	Policy        string   `json:"policy"`
	BandwidthMbit int      `json:"bandwidthMbit,omitempty"`
	AllowICMP     bool     `json:"allowIcmp,omitempty"`
	SkipDNAT      bool     `json:"skipDnat,omitempty"`
	AllowedCIDRs  []string `json:"allowedCidrs,omitempty"`
	DeniedCIDRs   []string `json:"deniedCidrs,omitempty"`
	Ports         []int    `json:"ports,omitempty"`
}

// networkTeardownParams holds the parameters for network.teardown.
type networkTeardownParams struct {
	VMId string `json:"vmId"`
	Slot int    `json:"slot"`
}

// generateVMId creates a VM ID in the form "vm-" + 8 hex chars.
func generateVMId() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate VM ID: %w", err)
	}
	return "vm-" + hex.EncodeToString(b), nil
}

// resolveRuntimePaths resolves the kernel and rootfs paths for a given runtime.
// It looks in ~/.vmsan/runtimes/<runtime>/ for vmlinux and rootfs.ext4.
func resolveRuntimePaths(runtime string) (kernelPath, rootfsPath string, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", fmt.Errorf("resolve home directory: %w", err)
	}

	runtimeDir := filepath.Join(home, ".vmsan", "runtimes", runtime)

	kernelPath = filepath.Join(runtimeDir, "vmlinux")
	if _, err := os.Stat(kernelPath); err != nil {
		return "", "", fmt.Errorf("kernel not found at %s: %w", kernelPath, err)
	}

	rootfsPath = filepath.Join(runtimeDir, "rootfs.ext4")
	if _, err := os.Stat(rootfsPath); err != nil {
		return "", "", fmt.Errorf("rootfs not found at %s: %w", rootfsPath, err)
	}

	return kernelPath, rootfsPath, nil
}

// effectivePolicy determines the firewall policy from the provided params.
func effectivePolicy(policy string, domains []string, allowedCidrs []string, deniedCidrs []string) string {
	if policy != "" {
		return policy
	}
	if len(domains) > 0 || len(allowedCidrs) > 0 || len(deniedCidrs) > 0 {
		return nftypes.PolicyCustom
	}
	return defaultPolicy
}

// buildPublishedPorts converts a list of port numbers to nftypes.PublishedPort
// entries, mapping each host port to the same guest port on TCP.
func buildPublishedPorts(ports []int, guestIP string) []nftypes.PublishedPort {
	if len(ports) == 0 {
		return nil
	}
	published := make([]nftypes.PublishedPort, 0, len(ports))
	for _, p := range ports {
		published = append(published, nftypes.PublishedPort{
			HostPort:  uint16(p),
			GuestIP:   guestIP,
			GuestPort: uint16(p),
			Protocol:  "tcp",
		})
	}
	return published
}

// waitForAgentHealth polls the agent /health endpoint until it responds 200
// or the timeout expires.
func waitForAgentHealth(guestIP string, timeout time.Duration) error {
	client := &http.Client{Timeout: 2 * time.Second}
	url := fmt.Sprintf("http://%s:%d/health", guestIP, agentPort)
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("agent at %s did not become healthy within %s", url, timeout)
}

// findFirecrackerPID searches for the Firecracker process associated with the
// given jailer chroot directory. Returns the PID or 0 if not found.
func findFirecrackerPID(paths jailer.Paths) int {
	// The jailer writes a PID file for the inner Firecracker process.
	// Try reading from /proc to find the firecracker process using socket path.
	pidFile := filepath.Join(paths.RootDir, "firecracker.pid")
	data, err := os.ReadFile(pidFile)
	if err == nil {
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err == nil && pid > 0 {
			return pid
		}
	}

	// Fallback: search /proc for a firecracker process using our socket.
	out, err := exec.Command("pgrep", "-f", paths.SocketPath).Output()
	if err != nil {
		return 0
	}
	lines := strings.Fields(strings.TrimSpace(string(out)))
	if len(lines) > 0 {
		pid, err := strconv.Atoi(lines[0])
		if err == nil {
			return pid
		}
	}
	return 0
}

// killFirecracker kills the Firecracker process for the given paths.
// It first attempts a graceful SIGTERM, then falls back to SIGKILL.
func killFirecracker(paths jailer.Paths) {
	pid := findFirecrackerPID(paths)
	if pid <= 0 {
		return
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return
	}

	// Try graceful termination first.
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		slog.Debug("SIGTERM failed", "pid", pid, "error", err)
		return
	}

	// Wait up to 5 seconds for the process to exit.
	done := make(chan struct{})
	go func() {
		proc.Wait()
		close(done)
	}()

	select {
	case <-done:
		return
	case <-time.After(5 * time.Second):
		slog.Debug("SIGTERM timeout, sending SIGKILL", "pid", pid)
		proc.Signal(syscall.SIGKILL)
		proc.Wait()
	}
}

// handleVMCreate orchestrates full VM creation: slot allocation, network setup,
// firewall, jailer, Firecracker boot, and agent health check.
func (s *Server) handleVMCreateImpl(ctx context.Context, params json.RawMessage) Response {
	var p vmCreateParams
	if err := json.Unmarshal(params, &p); err != nil {
		return Response{OK: false, Error: "invalid params: " + err.Error(), Code: "PARSE_ERROR"}
	}

	// Defaults.
	if p.VCPUs <= 0 {
		p.VCPUs = defaultVCPUs
	}
	if p.MemMiB <= 0 {
		p.MemMiB = defaultMemMiB
	}
	if p.Runtime == "" {
		p.Runtime = defaultRuntime
	}

	// Generate VM ID if not provided.
	vmId := p.VMId
	if vmId == "" {
		var err error
		vmId, err = generateVMId()
		if err != nil {
			return Response{OK: false, Error: err.Error(), Code: "INTERNAL_ERROR"}
		}
	}

	// Resolve kernel and rootfs paths.
	kernelPath := p.KernelPath
	rootfsPath := p.RootfsPath
	if kernelPath == "" || rootfsPath == "" {
		k, r, err := resolveRuntimePaths(p.Runtime)
		if err != nil {
			return Response{OK: false, Error: err.Error(), Code: "RUNTIME_ERROR"}
		}
		if kernelPath == "" {
			kernelPath = k
		}
		if rootfsPath == "" {
			rootfsPath = r
		}
	}

	// Rollback cleanup stack — runs in reverse order only if err != nil at return.
	var retErr error
	var cleanup []func()
	defer func() {
		if retErr != nil {
			for i := len(cleanup) - 1; i >= 0; i-- {
				cleanup[i]()
			}
		}
	}()

	// 1. Allocate network slot.
	slot, err := s.slots.Allocate(vmId)
	if err != nil {
		retErr = err
		return Response{OK: false, Error: err.Error(), Code: "SLOT_ERROR"}
	}
	cleanup = append(cleanup, func() {
		s.slots.Release(vmId)
	})

	// 2. Compute network addresses.
	hostIP := netsetup.VMHostIP(slot)
	guestIP := netsetup.VMGuestIP(slot)
	tapDevice := netsetup.TAPDevice(slot)
	macAddress := netsetup.MACAddress(slot)
	netnsName := netsetup.NetNSName(vmId)
	vethHost := netsetup.VethHostDev(slot)
	vethGuest := netsetup.VethGuestDev(slot)

	// 3. Detect default interface.
	defaultIface, err := netsetup.DetectDefaultInterface()
	if err != nil {
		retErr = err
		return Response{OK: false, Error: fmt.Sprintf("detect default interface: %s", err), Code: "NETWORK_ERROR"}
	}

	netCfg := netsetup.SetupConfig{
		VMId:          vmId,
		Slot:          slot,
		TAPDevice:     tapDevice,
		HostIP:        hostIP,
		GuestIP:       guestIP,
		SubnetMask:    netsetup.VMSubnetMask,
		MACAddress:    macAddress,
		NetNSName:     netnsName,
		VethHost:      vethHost,
		VethGuest:     vethGuest,
		DefaultIface:  defaultIface,
		BandwidthMbit: p.BandwidthMbit,
	}

	// 4. Setup network namespace + veth pair.
	if err := netsetup.SetupNamespace(netCfg); err != nil {
		retErr = err
		return Response{OK: false, Error: fmt.Sprintf("setup namespace: %s", err), Code: "NETWORK_ERROR"}
	}
	cleanup = append(cleanup, func() {
		netsetup.TeardownNamespace(netCfg)
	})

	// 5. Setup TAP device.
	if err := netsetup.SetupTAP(netCfg); err != nil {
		retErr = err
		return Response{OK: false, Error: fmt.Sprintf("setup TAP: %s", err), Code: "NETWORK_ERROR"}
	}

	// 6. Setup firewall.
	policy := effectivePolicy(p.NetworkPolicy, p.Domains, p.AllowedCIDRs, p.DeniedCIDRs)
	publishedPorts := buildPublishedPorts(p.Ports, guestIP)
	fwCfg := netsetup.FirewallConfig{
		Policy:         policy,
		AllowedCIDRs:   p.AllowedCIDRs,
		DeniedCIDRs:    p.DeniedCIDRs,
		PublishedPorts: publishedPorts,
		SkipDNAT:       p.SkipDNAT,
		AllowICMP:      p.AllowICMP,
	}
	if err := netsetup.SetupFirewall(ctx, netCfg, fwCfg); err != nil {
		retErr = err
		return Response{OK: false, Error: fmt.Sprintf("setup firewall: %s", err), Code: "FIREWALL_ERROR"}
	}
	cleanup = append(cleanup, func() {
		netsetup.TeardownFirewall(ctx, netCfg, publishedPorts)
	})

	// 7. Setup bandwidth throttling.
	if err := netsetup.SetupThrottle(netCfg); err != nil {
		slog.Warn("throttle setup failed, continuing", "vmId", vmId, "error", err)
	}

	// 8. Start proxy manager (DNS, SNI, HTTP proxies).
	if _, err := s.manager.StartVM(vmId, slot, policy, p.Domains); err != nil {
		retErr = err
		return Response{OK: false, Error: fmt.Sprintf("start proxies: %s", err), Code: "PROXY_ERROR"}
	}
	cleanup = append(cleanup, func() {
		s.manager.StopVM(vmId)
	})

	// 9. Start mesh networking if VM has a project.
	var meshIP string
	if p.Project != "" && s.meshManager != nil {
		meshResult, err := s.meshManager.OnVMStart(VMStartParams{
			VMId:      vmId,
			Slot:      slot,
			Policy:    policy,
			Project:   p.Project,
			Service:   p.Service,
			ConnectTo: p.ConnectTo,
			VethHost:  vethHost,
			NetNS:     netnsName,
			GuestDev:  vethGuest,
		})
		if err != nil {
			slog.Warn("mesh setup failed", "vmId", vmId, "error", err)
		} else if meshResult != nil {
			meshIP = meshResult.MeshIP
		}
	}

	// 10. Prepare jailer chroot.
	paths := jailer.NewPaths(vmId, jailerBaseDir)

	agentBin := p.AgentBinary
	if agentBin == "" {
		// Look for agent binary in standard locations.
		for _, candidate := range []string{
			"/usr/local/bin/vmsan-agent",
			"/usr/bin/vmsan-agent",
		} {
			if _, err := os.Stat(candidate); err == nil {
				agentBin = candidate
				break
			}
		}
	}

	jailCfg := jailer.Config{
		VMId:       vmId,
		KernelSrc:  kernelPath,
		RootfsSrc:  rootfsPath,
		DiskSizeGb: p.DiskSizeGb,
	}

	// Inject agent if binary is available.
	agentToken := p.AgentToken
	if agentBin != "" {
		if agentToken == "" {
			tokenBytes := make([]byte, 16)
			rand.Read(tokenBytes)
			agentToken = hex.EncodeToString(tokenBytes)
		}
		jailCfg.Agent = &jailer.AgentConfig{
			BinaryPath: agentBin,
			Token:      agentToken,
			Port:       agentPort,
			VMId:       vmId,
		}
	}

	// Handle snapshot restore.
	if p.SnapshotID != "" {
		home, _ := os.UserHomeDir()
		snapshotDir := filepath.Join(home, ".vmsan", "snapshots", p.SnapshotID)
		jailCfg.Snapshot = &jailer.SnapshotConfig{
			SnapshotFile: filepath.Join(snapshotDir, "snapshot_file"),
			MemFile:      filepath.Join(snapshotDir, "mem_file"),
		}
	}

	if err := jailer.Prepare(jailCfg, paths); err != nil {
		retErr = err
		return Response{OK: false, Error: fmt.Sprintf("prepare chroot: %s", err), Code: "JAILER_ERROR"}
	}
	cleanup = append(cleanup, func() {
		jailer.Cleanup(paths.ChrootDir)
	})

	// 11. Compute cgroup limits.
	cpuQuotaUs := p.VCPUs * 100000
	cpuPeriodUs := 100000
	memBytes := int64(p.MemMiB+jailer.CgroupVMMOverheadMiB) * 1024 * 1024

	// 12. Spawn jailer.
	spawnCfg := jailer.SpawnConfig{
		FirecrackerBin: firecrackerBin,
		JailerBin:      jailerBin,
		VMId:           vmId,
		Paths:          paths,
		UID:            jailerUID,
		GID:            jailerGID,
		NewPidNs:       !p.DisablePidNs,
		NetNS:          netnsName,
		SeccompFilter:  p.SeccompFilter,
	}
	if !p.DisableCgroup {
		spawnCfg.Cgroup = &jailer.CgroupConfig{
			CPUQuotaUs:  cpuQuotaUs,
			CPUPeriodUs: cpuPeriodUs,
			MemoryBytes: memBytes,
		}
	}
	if err := jailer.Spawn(spawnCfg); err != nil {
		retErr = err
		return Response{OK: false, Error: fmt.Sprintf("spawn jailer: %s", err), Code: "JAILER_ERROR"}
	}
	cleanup = append(cleanup, func() {
		killFirecracker(paths)
		jailer.Cleanup(paths.ChrootDir)
	})

	// 13. Wait for Firecracker socket.
	fcClient := firecracker.NewClient(paths.SocketPath)
	socketCtx, socketCancel := context.WithTimeout(ctx, 30*time.Second)
	defer socketCancel()
	if err := fcClient.WaitForSocket(socketCtx, 30*time.Second); err != nil {
		retErr = err
		return Response{OK: false, Error: fmt.Sprintf("wait for socket: %s", err), Code: "FIRECRACKER_ERROR"}
	}

	// 14. Configure Firecracker via API.
	if err := fcClient.Configure(firecracker.MachineConfig{
		VCPUs:  p.VCPUs,
		MemMiB: p.MemMiB,
	}); err != nil {
		retErr = err
		return Response{OK: false, Error: fmt.Sprintf("configure machine: %s", err), Code: "FIRECRACKER_ERROR"}
	}

	// Boot source — kernel path is relative to the chroot root.
	bootArgs := netsetup.BootArgs(guestIP, hostIP, netsetup.VMSubnetMask)
	if err := fcClient.Boot("kernel/vmlinux", bootArgs); err != nil {
		retErr = err
		return Response{OK: false, Error: fmt.Sprintf("configure boot: %s", err), Code: "FIRECRACKER_ERROR"}
	}

	// Root drive — rootfs path is relative to the chroot root.
	if err := fcClient.AddDrive("rootfs", "rootfs/rootfs.ext4", true, false); err != nil {
		retErr = err
		return Response{OK: false, Error: fmt.Sprintf("add drive: %s", err), Code: "FIRECRACKER_ERROR"}
	}

	// Network interface.
	if err := fcClient.AddNetwork("eth0", tapDevice, macAddress); err != nil {
		retErr = err
		return Response{OK: false, Error: fmt.Sprintf("add network: %s", err), Code: "FIRECRACKER_ERROR"}
	}

	// 15. Start Firecracker instance.
	if p.SnapshotID != "" {
		// Restore from snapshot instead of fresh boot.
		if err := fcClient.LoadSnapshot("snapshot/snapshot_file", "snapshot/mem_file"); err != nil {
			retErr = err
			return Response{OK: false, Error: fmt.Sprintf("load snapshot: %s", err), Code: "FIRECRACKER_ERROR"}
		}
		if err := fcClient.Resume(); err != nil {
			retErr = err
			return Response{OK: false, Error: fmt.Sprintf("resume snapshot: %s", err), Code: "FIRECRACKER_ERROR"}
		}
	} else {
		if err := fcClient.Start(); err != nil {
			retErr = err
			return Response{OK: false, Error: fmt.Sprintf("start instance: %s", err), Code: "FIRECRACKER_ERROR"}
		}
	}

	// 16. Wait for agent health.
	if agentBin != "" {
		if err := waitForAgentHealth(guestIP, 30*time.Second); err != nil {
			slog.Warn("agent health check failed", "vmId", vmId, "error", err)
			// Non-fatal: the VM is running, agent may just be slow.
		}
	}

	slog.Info("vm created",
		"vmId", vmId,
		"slot", slot,
		"hostIp", hostIP,
		"guestIp", guestIP,
		"vcpus", p.VCPUs,
		"memMib", p.MemMiB,
	)

	return Response{
		OK: true,
		VM: vmCreateResponse{
			VMId:       vmId,
			Slot:       slot,
			HostIP:     hostIP,
			GuestIP:    guestIP,
			MeshIP:     meshIP,
			TAPDevice:  tapDevice,
			MACAddress: macAddress,
			NetNSName:  netnsName,
			VethHost:   vethHost,
			VethGuest:  vethGuest,
			SubnetMask: netsetup.VMSubnetMask,
			ChrootDir:  paths.ChrootDir,
			SocketPath: paths.SocketPath,
			PID:        findFirecrackerPID(paths),
			AgentToken: agentToken,
			DNSPort:    netsetup.DNSPort(slot),
			SNIPort:    netsetup.SNIPort(slot),
			HTTPPort:   netsetup.HTTPPort(slot),
		},
	}
}

// handleVMDeleteImpl orchestrates full VM teardown: stop Firecracker, teardown
// firewall, network, cleanup chroot, release slot.
func (s *Server) handleVMDeleteImpl(ctx context.Context, params json.RawMessage) Response {
	var p vmDeleteParams
	if err := json.Unmarshal(params, &p); err != nil {
		return Response{OK: false, Error: "invalid params: " + err.Error(), Code: "PARSE_ERROR"}
	}
	if p.VMId == "" {
		return Response{OK: false, Error: "vmId is required", Code: "VALIDATION_ERROR"}
	}

	// 1. Get slot for the VM.
	slot := s.slots.GetSlot(p.VMId)
	if slot < 0 {
		return Response{OK: false, Error: fmt.Sprintf("vm %s not found in slot allocator", p.VMId), Code: "NOT_FOUND"}
	}

	// 2. Compute network config from slot.
	defaultIface, _ := netsetup.DetectDefaultInterface()
	netCfg := netsetup.SetupConfig{
		VMId:         p.VMId,
		Slot:         slot,
		TAPDevice:    netsetup.TAPDevice(slot),
		HostIP:       netsetup.VMHostIP(slot),
		GuestIP:      netsetup.VMGuestIP(slot),
		SubnetMask:   netsetup.VMSubnetMask,
		MACAddress:   netsetup.MACAddress(slot),
		NetNSName:    netsetup.NetNSName(p.VMId),
		VethHost:     netsetup.VethHostDev(slot),
		VethGuest:    netsetup.VethGuestDev(slot),
		DefaultIface: defaultIface,
	}

	// 3. Stop proxies.
	if err := s.manager.StopVM(p.VMId); err != nil {
		slog.Debug("proxy stop failed", "vmId", p.VMId, "error", err)
	}

	// 4. Stop mesh networking.
	if s.meshManager != nil {
		if err := s.meshManager.OnVMStop(p.VMId, netCfg.VethHost, netCfg.NetNSName, netCfg.VethGuest); err != nil {
			slog.Debug("mesh stop failed", "vmId", p.VMId, "error", err)
		}
	}

	// 5. Stop Firecracker: try graceful via API, then force kill.
	paths := jailer.NewPaths(p.VMId, jailerBaseDir)
	fcClient := firecracker.NewClient(paths.SocketPath)

	stopped := false
	if err := fcClient.Stop(); err != nil {
		slog.Debug("graceful FC stop failed", "vmId", p.VMId, "error", err)
	} else {
		// Wait up to 10 seconds for clean exit.
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if findFirecrackerPID(paths) <= 0 {
				stopped = true
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
	}

	if !stopped {
		killFirecracker(paths)
	}

	// 6. Teardown firewall.
	if err := netsetup.TeardownFirewall(ctx, netCfg, nil); err != nil {
		slog.Debug("firewall teardown failed", "vmId", p.VMId, "error", err)
	}

	// 7. Teardown throttle (best effort, namespace deletion usually handles this).
	netsetup.TeardownThrottle(netCfg.TAPDevice, netCfg.NetNSName)

	// 8. Teardown network namespace (also cleans up TAP and veth pair).
	if err := netsetup.TeardownNamespace(netCfg); err != nil {
		slog.Debug("namespace teardown failed", "vmId", p.VMId, "error", err)
	}

	// 9. Cleanup chroot.
	if err := jailer.Cleanup(paths.ChrootDir); err != nil {
		slog.Debug("chroot cleanup failed", "vmId", p.VMId, "error", err)
	}

	// 10. Release slot.
	s.slots.Release(p.VMId)

	slog.Info("vm deleted", "vmId", p.VMId, "slot", slot)
	return Response{OK: true}
}

// handleNetworkSetupImpl sets up network namespace, TAP, firewall, and throttle
// for a VM slot without launching Firecracker.
func (s *Server) handleNetworkSetupImpl(ctx context.Context, params json.RawMessage) Response {
	var p networkSetupParams
	if err := json.Unmarshal(params, &p); err != nil {
		return Response{OK: false, Error: "invalid params: " + err.Error(), Code: "PARSE_ERROR"}
	}
	if p.VMId == "" {
		return Response{OK: false, Error: "vmId is required", Code: "VALIDATION_ERROR"}
	}
	if p.Policy == "" {
		p.Policy = defaultPolicy
	}

	defaultIface, err := netsetup.DetectDefaultInterface()
	if err != nil {
		return Response{OK: false, Error: fmt.Sprintf("detect default interface: %s", err), Code: "NETWORK_ERROR"}
	}

	netCfg := netsetup.SetupConfig{
		VMId:          p.VMId,
		Slot:          p.Slot,
		TAPDevice:     netsetup.TAPDevice(p.Slot),
		HostIP:        netsetup.VMHostIP(p.Slot),
		GuestIP:       netsetup.VMGuestIP(p.Slot),
		SubnetMask:    netsetup.VMSubnetMask,
		MACAddress:    netsetup.MACAddress(p.Slot),
		NetNSName:     netsetup.NetNSName(p.VMId),
		VethHost:      netsetup.VethHostDev(p.Slot),
		VethGuest:     netsetup.VethGuestDev(p.Slot),
		DefaultIface:  defaultIface,
		BandwidthMbit: p.BandwidthMbit,
	}

	if err := netsetup.SetupNamespace(netCfg); err != nil {
		return Response{OK: false, Error: fmt.Sprintf("setup namespace: %s", err), Code: "NETWORK_ERROR"}
	}

	if err := netsetup.SetupTAP(netCfg); err != nil {
		netsetup.TeardownNamespace(netCfg)
		return Response{OK: false, Error: fmt.Sprintf("setup TAP: %s", err), Code: "NETWORK_ERROR"}
	}

	guestIP := netsetup.VMGuestIP(p.Slot)
	publishedPorts := buildPublishedPorts(p.Ports, guestIP)
	fwCfg := netsetup.FirewallConfig{
		Policy:         p.Policy,
		AllowedCIDRs:   p.AllowedCIDRs,
		DeniedCIDRs:    p.DeniedCIDRs,
		PublishedPorts: publishedPorts,
		SkipDNAT:       p.SkipDNAT,
		AllowICMP:      p.AllowICMP,
	}
	if err := netsetup.SetupFirewall(ctx, netCfg, fwCfg); err != nil {
		netsetup.TeardownNamespace(netCfg)
		return Response{OK: false, Error: fmt.Sprintf("setup firewall: %s", err), Code: "FIREWALL_ERROR"}
	}

	if err := netsetup.SetupThrottle(netCfg); err != nil {
		slog.Warn("throttle setup failed", "vmId", p.VMId, "error", err)
	}

	return Response{
		OK: true,
		VM: map[string]any{
			"vmId":    p.VMId,
			"slot":    p.Slot,
			"hostIp":  netCfg.HostIP,
			"guestIp": netCfg.GuestIP,
			"tapDev":  netCfg.TAPDevice,
			"netns":   netCfg.NetNSName,
		},
	}
}

// handleNetworkTeardownImpl tears down network namespace, firewall, and TAP
// for a VM slot.
func (s *Server) handleNetworkTeardownImpl(ctx context.Context, params json.RawMessage) Response {
	var p networkTeardownParams
	if err := json.Unmarshal(params, &p); err != nil {
		return Response{OK: false, Error: "invalid params: " + err.Error(), Code: "PARSE_ERROR"}
	}
	if p.VMId == "" {
		return Response{OK: false, Error: "vmId is required", Code: "VALIDATION_ERROR"}
	}

	defaultIface, _ := netsetup.DetectDefaultInterface()
	netCfg := netsetup.SetupConfig{
		VMId:         p.VMId,
		Slot:         p.Slot,
		TAPDevice:    netsetup.TAPDevice(p.Slot),
		HostIP:       netsetup.VMHostIP(p.Slot),
		GuestIP:      netsetup.VMGuestIP(p.Slot),
		SubnetMask:   netsetup.VMSubnetMask,
		MACAddress:   netsetup.MACAddress(p.Slot),
		NetNSName:    netsetup.NetNSName(p.VMId),
		VethHost:     netsetup.VethHostDev(p.Slot),
		VethGuest:    netsetup.VethGuestDev(p.Slot),
		DefaultIface: defaultIface,
	}

	if err := netsetup.TeardownFirewall(ctx, netCfg, nil); err != nil {
		slog.Debug("firewall teardown failed", "vmId", p.VMId, "error", err)
	}

	netsetup.TeardownThrottle(netCfg.TAPDevice, netCfg.NetNSName)

	if err := netsetup.TeardownNamespace(netCfg); err != nil {
		slog.Debug("namespace teardown failed", "vmId", p.VMId, "error", err)
	}

	return Response{OK: true}
}
