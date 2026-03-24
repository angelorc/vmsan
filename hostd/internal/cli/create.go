package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	vmsanv1 "github.com/angelorc/vmsan/hostd/gen/vmsan/v1"
	"github.com/angelorc/vmsan/hostd/internal/gwclient"
	"github.com/angelorc/vmsan/hostd/internal/paths"
	"github.com/angelorc/vmsan/hostd/internal/vmstate"
	"github.com/spf13/cobra"
)

const currentStateVersion = 2

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create and start a Firecracker microVM",
	RunE:  runCreate,
}

func init() {
	f := createCmd.Flags()
	f.Int("vcpus", 1, "Number of vCPUs")
	f.Int("memory", 128, "Memory in MiB")
	f.String("kernel", "", "Path to kernel image (auto-detected if not specified)")
	f.String("rootfs", "", "Path to rootfs image (auto-detected if not specified)")
	f.String("runtime", "base", "Runtime environment (base, node22, node24, python3.13)")
	f.String("project", "", "Project label for grouping VMs")
	f.String("disk", "10gb", "Root disk size (e.g. 10gb)")
	f.String("timeout", "", "Auto-shutdown timeout (e.g. 30s, 5m, 1h, 2h30m)")
	f.String("publish-port", "", "Ports to forward (comma-separated, e.g. 8080,3000)")
	f.String("snapshot", "", "Snapshot ID to restore from")
	f.String("network-policy", "allow-all", "Network mode: allow-all, deny-all, or custom")
	f.String("allowed-domain", "", "Domains to allow (comma-separated, wildcard * for subdomains)")
	f.String("allowed-cidr", "", "CIDR ranges to allow (comma-separated)")
	f.String("denied-cidr", "", "CIDR ranges to deny (comma-separated)")
	f.Bool("no-seccomp", false, "Disable seccomp-bpf filter")
	f.Bool("no-pid-ns", false, "Disable PID namespace isolation")
	f.Bool("no-cgroup", false, "Disable cgroup resource limits")
	f.String("bandwidth", "", "Max bandwidth per VM (e.g. 50mbit)")
	f.Bool("allow-icmp", false, "Allow ICMP traffic (ping)")
	f.String("connect-to", "", "Mesh connections (comma-separated service:port pairs)")
	f.String("service", "", "Register as mesh service (e.g. --service web)")

	rootCmd.AddCommand(createCmd)
}

func runCreate(cmd *cobra.Command, _ []string) error {
	f := cmd.Flags()

	vcpus, _ := f.GetInt("vcpus")
	memMib, _ := f.GetInt("memory")
	runtime, _ := f.GetString("runtime")
	project, _ := f.GetString("project")
	diskStr, _ := f.GetString("disk")
	timeoutStr, _ := f.GetString("timeout")
	snapshot, _ := f.GetString("snapshot")
	kernelPath, _ := f.GetString("kernel")
	rootfsPath, _ := f.GetString("rootfs")
	networkPolicy, _ := f.GetString("network-policy")
	allowedDomain, _ := f.GetString("allowed-domain")
	allowedCidr, _ := f.GetString("allowed-cidr")
	deniedCidr, _ := f.GetString("denied-cidr")
	publishPort, _ := f.GetString("publish-port")
	noSeccomp, _ := f.GetBool("no-seccomp")
	noPidNs, _ := f.GetBool("no-pid-ns")
	noCgroup, _ := f.GetBool("no-cgroup")
	bandwidthStr, _ := f.GetString("bandwidth")
	allowIcmp, _ := f.GetBool("allow-icmp")
	connectToStr, _ := f.GetString("connect-to")
	service, _ := f.GetString("service")

	// Parse disk size
	diskSizeGb, err := parseDiskSize(diskStr)
	if err != nil {
		return err
	}

	// Parse timeout
	var timeoutMs int64
	var timeoutAt string
	if timeoutStr != "" {
		d, err := parseDuration(timeoutStr)
		if err != nil {
			return fmt.Errorf("invalid timeout: %w", err)
		}
		timeoutMs = d.Milliseconds()
		timeoutAt = time.Now().Add(d).Format(time.RFC3339)
	}

	// Parse comma-separated lists
	domains := splitCSV(allowedDomain)
	allowedCidrs := splitCSV(allowedCidr)
	deniedCidrs := splitCSV(deniedCidr)
	ports := parsePortList(publishPort)
	connectTo := splitCSV(connectToStr)

	// Parse bandwidth
	var bandwidthMbit int32
	if bandwidthStr != "" {
		bw, err := parseBandwidth(bandwidthStr)
		if err != nil {
			return err
		}
		bandwidthMbit = int32(bw)
	}

	// Auto-promote to custom policy when domains/cidrs provided
	if networkPolicy == "allow-all" && (len(domains) > 0 || len(allowedCidrs) > 0 || len(deniedCidrs) > 0) {
		networkPolicy = "custom"
	}

	// Resolve paths
	p := paths.Resolve()
	if kernelPath == "" {
		kernelPath = autoDetectKernel(p.KernelsDir)
	}
	if rootfsPath == "" {
		rootfsPath = autoDetectRootfs(p.RootfsDir, runtime)
	}

	// Generate agent token
	agentToken := generateToken()

	// Connect to gateway
	gw, err := gwclient.New()
	if err != nil {
		return fmt.Errorf("gateway connection failed: %w", err)
	}
	defer gw.Close()

	// Build gRPC request
	req := &vmsanv1.CreateVMRequest{
		Vcpus:           int32(vcpus),
		MemMib:          int32(memMib),
		Runtime:         runtime,
		DiskSizeGb:      float64(diskSizeGb),
		NetworkPolicy:   networkPolicy,
		Domains:         domains,
		AllowedCidrs:    allowedCidrs,
		DeniedCidrs:     deniedCidrs,
		Ports:           toInt32Slice(ports),
		BandwidthMbit:   bandwidthMbit,
		AllowIcmp:       allowIcmp,
		Project:         project,
		Service:         service,
		ConnectTo:       connectTo,
		KernelPath:      kernelPath,
		RootfsPath:      rootfsPath,
		SnapshotId:      snapshot,
		AgentBinary:     p.AgentBin,
		AgentToken:      agentToken,
		DisableSeccomp:  noSeccomp,
		DisablePidNs:    noPidNs,
		DisableCgroup:   noCgroup,
		SeccompFilter:   p.SeccompFilter,
		JailerBaseDir:   p.JailerBaseDir,
		OwnerUid:        int32(os.Getuid()),
		OwnerGid:        int32(os.Getgid()),
		TimeoutMs:       timeoutMs,
		TimeoutAt:       timeoutAt,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	resp, err := gw.CreateVM(ctx, req)
	if err != nil {
		return fmt.Errorf("create VM: %w", err)
	}

	// Build and save local state
	pid := int(resp.Pid)
	state := &vmstate.VmState{
		ID:         resp.VmId,
		Project:    project,
		Runtime:    runtime,
		DiskSizeGb: float64(diskSizeGb),
		Status:     "running",
		PID:        &pid,
		APISocket:  resp.SocketPath,
		ChrootDir:  resp.ChrootDir,
		Kernel:     kernelPath,
		Rootfs:     rootfsPath,
		VcpuCount:  vcpus,
		MemSizeMib: memMib,
		Network: vmstate.VmNetwork{
			TapDevice:       resp.TapDevice,
			HostIP:          resp.HostIp,
			GuestIP:         resp.GuestIp,
			SubnetMask:      resp.SubnetMask,
			MACAddress:      resp.MacAddress,
			NetworkPolicy:   networkPolicy,
			AllowedDomains:  domains,
			AllowedCidrs:    allowedCidrs,
			DeniedCidrs:     deniedCidrs,
			PublishedPorts:  ports,
			BandwidthMbit:   int(bandwidthMbit),
			NetNSName:       resp.NetNsName,
			SkipDnat:        false,
			AllowIcmp:       allowIcmp,
			FirewallBackend: "nftables",
			ConnectTo:       connectTo,
			MeshIP:          resp.MeshIp,
			Service:         service,
		},
		Snapshot:       strPtr(snapshot),
		TimeoutMs:      int64Ptr(timeoutMs),
		TimeoutAt:      strPtr(timeoutAt),
		CreatedAt:      time.Now().UTC().Format(time.RFC3339),
		AgentToken:     &agentToken,
		AgentPort:      p.AgentPort,
		StateVersion:   currentStateVersion,
		DisableSeccomp: noSeccomp,
		DisablePidNs:   noPidNs,
		DisableCgroup:  noCgroup,
	}

	store := vmstate.NewStore(p.VmsDir)
	if err := store.Save(state); err != nil {
		return fmt.Errorf("save state: %w", err)
	}

	// Output
	if jsonOutput {
		data, _ := json.MarshalIndent(map[string]any{
			"vmId":    resp.VmId,
			"slot":    resp.Slot,
			"hostIp":  resp.HostIp,
			"guestIp": resp.GuestIp,
			"meshIp":  resp.MeshIp,
			"pid":     resp.Pid,
		}, "", "  ")
		fmt.Println(string(data))
	} else {
		printCreateSummary(resp, state, p)
	}

	return nil
}

func printCreateSummary(resp *vmsanv1.CreateVMResponse, state *vmstate.VmState, p paths.Paths) {
	fmt.Println()
	fmt.Printf("  VM Created: %s\n", resp.VmId)
	fmt.Println()
	fmt.Println("  Status:   running")
	fmt.Printf("  PID:      %d\n", resp.Pid)
	fmt.Printf("  vCPUs:    %d\n", state.VcpuCount)
	fmt.Printf("  Memory:   %d MiB\n", state.MemSizeMib)
	fmt.Printf("  Runtime:  %s\n", state.Runtime)
	fmt.Printf("  Disk:     %.0f GB\n", state.DiskSizeGb)
	if state.Project != "" {
		fmt.Printf("  Project:  %s\n", state.Project)
	}
	fmt.Println()
	fmt.Println("  Network:")
	fmt.Printf("    TAP:    %s\n", resp.TapDevice)
	fmt.Printf("    Host:   %s\n", resp.HostIp)
	fmt.Printf("    Guest:  %s\n", resp.GuestIp)
	fmt.Printf("    MAC:    %s\n", resp.MacAddress)
	fmt.Printf("    Policy: %s\n", state.Network.NetworkPolicy)
	if len(state.Network.AllowedDomains) > 0 {
		fmt.Printf("    Domains: %s\n", strings.Join(state.Network.AllowedDomains, ", "))
	}
	if len(state.Network.AllowedCidrs) > 0 {
		fmt.Printf("    Allowed CIDRs: %s\n", strings.Join(state.Network.AllowedCidrs, ", "))
	}
	if len(state.Network.DeniedCidrs) > 0 {
		fmt.Printf("    Denied CIDRs:  %s\n", strings.Join(state.Network.DeniedCidrs, ", "))
	}
	if len(state.Network.PublishedPorts) > 0 {
		portStrs := make([]string, len(state.Network.PublishedPorts))
		for i, port := range state.Network.PublishedPorts {
			portStrs[i] = strconv.Itoa(port)
		}
		fmt.Printf("    Ports:  %s\n", strings.Join(portStrs, ", "))
	}
	if state.Network.Service != "" {
		fmt.Printf("    Service: %s\n", state.Network.Service)
	}
	if len(state.Network.ConnectTo) > 0 {
		fmt.Printf("    Connect-to: %s\n", strings.Join(state.Network.ConnectTo, ", "))
	}
	fmt.Println()
	fmt.Printf("  Kernel:   %s\n", state.Kernel)
	fmt.Printf("  Rootfs:   %s\n", state.Rootfs)
	if state.Snapshot != nil && *state.Snapshot != "" {
		fmt.Printf("  Snapshot: %s\n", *state.Snapshot)
	}
	if state.TimeoutAt != nil && *state.TimeoutAt != "" {
		fmt.Printf("  Timeout:  %s\n", *state.TimeoutAt)
	}
	fmt.Println()
	fmt.Printf("  Socket:   %s\n", state.APISocket)
	fmt.Printf("  Chroot:   %s\n", state.ChrootDir)
	fmt.Printf("  State:    %s\n", filepath.Join(p.VmsDir, resp.VmId+".json"))
	fmt.Println()
}

// --- helpers ---

func parseDiskSize(value string) (int, error) {
	raw := strings.ToLower(strings.TrimSpace(value))
	re := regexp.MustCompile(`^(\d+)(gb|g|gib)?$`)
	m := re.FindStringSubmatch(raw)
	if m == nil {
		return 0, fmt.Errorf("invalid disk size format: %s (expected e.g. 10gb)", value)
	}
	size, _ := strconv.Atoi(m[1])
	if size < 1 || size > 1024 {
		return 0, fmt.Errorf("disk size must be 1-1024 GB, got %d", size)
	}
	return size, nil
}

func parseDuration(s string) (time.Duration, error) {
	// Try Go's built-in parser first (handles "5m", "1h", "30s")
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}
	// Try compound format like "2h30m"
	re := regexp.MustCompile(`^(?:(\d+)h)?(?:(\d+)m)?(?:(\d+)s)?$`)
	m := re.FindStringSubmatch(s)
	if m == nil {
		return 0, fmt.Errorf("cannot parse %q as duration", s)
	}
	var d time.Duration
	if m[1] != "" {
		h, _ := strconv.Atoi(m[1])
		d += time.Duration(h) * time.Hour
	}
	if m[2] != "" {
		min, _ := strconv.Atoi(m[2])
		d += time.Duration(min) * time.Minute
	}
	if m[3] != "" {
		sec, _ := strconv.Atoi(m[3])
		d += time.Duration(sec) * time.Second
	}
	if d == 0 {
		return 0, fmt.Errorf("cannot parse %q as duration", s)
	}
	return d, nil
}

func parseBandwidth(value string) (int, error) {
	raw := strings.ToLower(strings.TrimSpace(value))
	re := regexp.MustCompile(`^(\d+)(mbit|m)?$`)
	m := re.FindStringSubmatch(raw)
	if m == nil {
		return 0, fmt.Errorf("invalid bandwidth format: %s (expected e.g. 50mbit)", value)
	}
	bw, _ := strconv.Atoi(m[1])
	if bw < 1 || bw > 1000 {
		return 0, fmt.Errorf("bandwidth must be 1-1000 mbit, got %d", bw)
	}
	return bw, nil
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func parsePortList(s string) []int {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var ports []int
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		port, err := strconv.Atoi(p)
		if err != nil || port < 1 || port > 65535 {
			continue
		}
		ports = append(ports, port)
	}
	return ports
}

func toInt32Slice(ints []int) []int32 {
	out := make([]int32, len(ints))
	for i, v := range ints {
		out[i] = int32(v)
	}
	return out
}

func autoDetectKernel(kernelsDir string) string {
	entries, err := os.ReadDir(kernelsDir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "vmlinux") {
			return filepath.Join(kernelsDir, e.Name())
		}
	}
	return ""
}

func autoDetectRootfs(rootfsDir, runtime string) string {
	// Try runtime-specific first, then generic
	candidates := []string{
		filepath.Join(rootfsDir, runtime+".ext4"),
		filepath.Join(rootfsDir, runtime+".squashfs"),
		filepath.Join(rootfsDir, "base.ext4"),
		filepath.Join(rootfsDir, "base.squashfs"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

func generateToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func int64Ptr(v int64) *int64 {
	if v == 0 {
		return nil
	}
	return &v
}
