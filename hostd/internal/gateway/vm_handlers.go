package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/angelorc/vmsan/hostd/internal/firecracker"
	"github.com/angelorc/vmsan/hostd/internal/jailer"
	"github.com/angelorc/vmsan/hostd/internal/netsetup"
)

// vmRestartParams holds the parameters for vm.restart (full lifecycle restart).
type vmRestartParams struct {
	VMId          string   `json:"vmId"`
	Slot          int      `json:"slot"`
	ChrootDir     string   `json:"chrootDir"`
	SocketPath    string   `json:"socketPath"`
	NetworkPolicy string   `json:"networkPolicy"`
	Domains       []string `json:"domains,omitempty"`
	AllowedCIDRs  []string `json:"allowedCidrs,omitempty"`
	DeniedCIDRs   []string `json:"deniedCidrs,omitempty"`
	Ports         []int    `json:"ports,omitempty"`
	BandwidthMbit int      `json:"bandwidthMbit,omitempty"`
	AllowICMP     bool     `json:"allowIcmp,omitempty"`
	SkipDNAT      bool     `json:"skipDnat,omitempty"`
	Project       string   `json:"project,omitempty"`
	Service       string   `json:"service,omitempty"`
	ConnectTo     []string `json:"connectTo,omitempty"`
	// Isolation
	DisableSeccomp bool   `json:"disableSeccomp,omitempty"`
	DisablePidNs   bool   `json:"disablePidNs,omitempty"`
	DisableCgroup  bool   `json:"disableCgroup,omitempty"`
	SeccompFilter  string `json:"seccompFilter,omitempty"`
	// Runtime info
	VCPUs       int    `json:"vcpus"`
	MemMiB      int    `json:"memMib"`
	KernelPath  string `json:"kernelPath"`
	RootfsPath  string `json:"rootfsPath"`
	AgentBinary   string `json:"agentBinary,omitempty"`
	AgentToken    string `json:"agentToken,omitempty"`
	NetNSName     string `json:"netnsName,omitempty"`
	JailerBaseDir string `json:"jailerBaseDir,omitempty"`
}

// vmFullStopParams holds the parameters for vm.fullStop (full lifecycle stop).
type vmFullStopParams struct {
	VMId          string `json:"vmId"`
	Slot          int    `json:"slot"`
	PID           int    `json:"pid,omitempty"`
	NetNSName     string `json:"netnsName,omitempty"`
	SocketPath    string `json:"socketPath,omitempty"`
	JailerBaseDir string `json:"jailerBaseDir,omitempty"`
}

// vmFullUpdatePolicyParams holds the parameters for vm.fullUpdatePolicy.
type vmFullUpdatePolicyParams struct {
	VMId         string   `json:"vmId"`
	Policy       string   `json:"policy"`
	Slot         int      `json:"slot,omitempty"`
	Domains      []string `json:"domains,omitempty"`
	AllowedCIDRs []string `json:"allowedCidrs,omitempty"`
	DeniedCIDRs  []string `json:"deniedCidrs,omitempty"`
	Ports        []int    `json:"ports,omitempty"`
	AllowICMP    bool     `json:"allowIcmp,omitempty"`
	SkipDNAT     bool     `json:"skipDnat,omitempty"`
	NetNSName    string   `json:"netnsName,omitempty"`
}

// vmSnapshotCreateParams holds the parameters for vm.snapshot.create.
type vmSnapshotCreateParams struct {
	VMId          string `json:"vmId"`
	SnapshotID    string `json:"snapshotId"`
	SocketPath    string `json:"socketPath"`
	DestDir       string `json:"destDir"`
	ChrootDir     string `json:"chrootDir"`
	JailerBaseDir string `json:"jailerBaseDir,omitempty"`
	OwnerUID      int    `json:"ownerUid"`
	OwnerGID   int    `json:"ownerGid"`
}

// rootfsBuildParams holds the parameters for rootfs.build.
type rootfsBuildParams struct {
	ImageRef  string `json:"imageRef"`
	OutputDir string `json:"outputDir"`
	OwnerUID  int    `json:"ownerUid"`
	OwnerGID  int    `json:"ownerGid"`
}

// handleVMRestart re-creates a VM from existing state (restart after host reboot).
// Flow: re-register slot → clean stale files → setupVMNetwork → spawnFirecracker → enriched response.
func (s *Server) handleVMRestart(ctx context.Context, params json.RawMessage) Response {
	var p vmRestartParams
	if err := json.Unmarshal(params, &p); err != nil {
		return Response{OK: false, Error: "invalid params: " + err.Error(), Code: "PARSE_ERROR"}
	}
	if p.VMId == "" {
		return Response{OK: false, Error: "vmId is required", Code: "VALIDATION_ERROR"}
	}
	if p.Slot < 0 {
		return Response{OK: false, Error: "slot must be >= 0", Code: "VALIDATION_ERROR"}
	}

	// Defaults.
	if p.VCPUs <= 0 {
		p.VCPUs = defaultVCPUs
	}
	if p.MemMiB <= 0 {
		p.MemMiB = defaultMemMiB
	}

	// Resolve kernel and rootfs paths.
	kernelPath := p.KernelPath
	rootfsPath := p.RootfsPath
	if kernelPath == "" || rootfsPath == "" {
		k, r, err := resolveRuntimePaths(defaultRuntime, p.JailerBaseDir)
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

	slog.Info("vm restart: starting", "vmId", p.VMId, "slot", p.Slot, "kernel", kernelPath, "rootfs", rootfsPath)

	// Rollback cleanup stack.
	var retErr error
	var cleanup []func()
	defer func() {
		if retErr != nil {
			slog.Error("vm restart: rolling back", "vmId", p.VMId, "error", retErr)
			for i := len(cleanup) - 1; i >= 0; i-- {
				cleanup[i]()
			}
		}
	}()

	// 1. Re-register slot.
	if err := s.slots.AllocateAt(p.Slot, p.VMId); err != nil {
		retErr = err
		slog.Error("vm restart: slot allocation failed", "vmId", p.VMId, "slot", p.Slot, "error", err)
		return Response{OK: false, Error: err.Error(), Code: "SLOT_ERROR"}
	}
	cleanup = append(cleanup, func() {
		s.slots.Release(p.VMId)
	})

	// 2. Clean stale chroot and files from previous lifecycle.
	paths := jailer.NewPaths(p.VMId, resolveJailerBaseDir(p.JailerBaseDir))
	if err := jailer.Cleanup(paths.ChrootDir); err != nil {
		slog.Debug("stale chroot cleanup failed", "vmId", p.VMId, "error", err)
	}
	if p.SocketPath != "" {
		os.Remove(p.SocketPath)
	}
	os.Remove(paths.SocketPath)

	// 3. Build a vmCreateParams from restart params for the helpers.
	createP := vmCreateParams{
		VCPUs:          p.VCPUs,
		MemMiB:         p.MemMiB,
		NetworkPolicy:  p.NetworkPolicy,
		Domains:        p.Domains,
		AllowedCIDRs:   p.AllowedCIDRs,
		DeniedCIDRs:    p.DeniedCIDRs,
		Ports:          p.Ports,
		BandwidthMbit:  p.BandwidthMbit,
		AllowICMP:      p.AllowICMP,
		SkipDNAT:       p.SkipDNAT,
		Project:        p.Project,
		Service:        p.Service,
		ConnectTo:      p.ConnectTo,
		KernelPath:     kernelPath,
		RootfsPath:     rootfsPath,
		AgentBinary:    p.AgentBinary,
		AgentToken:     p.AgentToken,
		VMId:           p.VMId,
		DisableSeccomp: p.DisableSeccomp,
		DisablePidNs:   p.DisablePidNs,
		DisableCgroup:  p.DisableCgroup,
		SeccompFilter:  p.SeccompFilter,
		JailerBaseDir:  p.JailerBaseDir,
	}

	// 4. Setup network.
	netResult, netCleanup, err := s.setupVMNetwork(ctx, p.VMId, p.Slot, createP)
	cleanup = append(cleanup, netCleanup...)
	if err != nil {
		slog.Error("vm restart: network setup failed", "vmId", p.VMId, "error", err)
		retErr = err
		return Response{OK: false, Error: err.Error(), Code: "NETWORK_ERROR"}
	}

	// 5. Spawn Firecracker.
	spawnRes, spawnCleanup, err := s.spawnFirecracker(ctx, p.VMId, p.Slot, createP, netResult.NetCfg, kernelPath, rootfsPath)
	cleanup = append(cleanup, spawnCleanup...)
	if err != nil {
		slog.Error("vm restart: spawn failed", "vmId", p.VMId, "error", err)
		retErr = err
		return Response{OK: false, Error: err.Error(), Code: "SPAWN_ERROR"}
	}

	netCfg := netResult.NetCfg
	slog.Info("vm restarted",
		"vmId", p.VMId,
		"slot", p.Slot,
		"hostIp", netCfg.HostIP,
		"guestIp", netCfg.GuestIP,
	)

	// Write authoritative VM metadata.
	policy := effectivePolicy(p.NetworkPolicy, p.Domains, p.AllowedCIDRs, p.DeniedCIDRs)
	meta := &VMMetadata{
		VMId:       p.VMId,
		Slot:       p.Slot,
		Status:     "running",
		HostIP:     netCfg.HostIP,
		GuestIP:    netCfg.GuestIP,
		PID:        spawnRes.PID,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
		AgentToken: spawnRes.AgentToken,
		Runtime:    defaultRuntime,
		VCPUs:      p.VCPUs,
		MemMiB:     p.MemMiB,
		Project:    p.Project,
		Service:    p.Service,
		Network: VMNetworkMeta{
			Policy:        policy,
			Domains:       p.Domains,
			AllowedCIDRs:  p.AllowedCIDRs,
			DeniedCIDRs:   p.DeniedCIDRs,
			Ports:         p.Ports,
			BandwidthMbit: p.BandwidthMbit,
			AllowICMP:     p.AllowICMP,
		},
		ChrootDir:  spawnRes.Paths.ChrootDir,
		SocketPath: spawnRes.Paths.SocketPath,
		TAPDevice:  netCfg.TAPDevice,
		MACAddress: netCfg.MACAddress,
		NetNSName:  netCfg.NetNSName,
		VethHost:   netCfg.VethHost,
		VethGuest:  netCfg.VethGuest,
		SubnetMask: netsetup.VMSubnetMask,
		DNSPort:    netsetup.DNSPort(p.Slot),
		SNIPort:    netsetup.SNIPort(p.Slot),
		HTTPPort:   netsetup.HTTPPort(p.Slot),
	}
	if err := writeVMMetadata(meta); err != nil {
		slog.Warn("failed to write VM metadata on restart", "vmId", p.VMId, "error", err)
	}

	return Response{
		OK: true,
		VM: vmCreateResponse{
			VMId:       p.VMId,
			Slot:       p.Slot,
			HostIP:     netCfg.HostIP,
			GuestIP:    netCfg.GuestIP,
			TAPDevice:  netCfg.TAPDevice,
			MACAddress: netCfg.MACAddress,
			NetNSName:  netCfg.NetNSName,
			VethHost:   netCfg.VethHost,
			VethGuest:  netCfg.VethGuest,
			SubnetMask: netsetup.VMSubnetMask,
			ChrootDir:  spawnRes.Paths.ChrootDir,
			SocketPath: spawnRes.Paths.SocketPath,
			PID:        spawnRes.PID,
			AgentToken: spawnRes.AgentToken,
			DNSPort:    netsetup.DNSPort(p.Slot),
			SNIPort:    netsetup.SNIPort(p.Slot),
			HTTPPort:   netsetup.HTTPPort(p.Slot),
		},
	}
}

// handleVMFullStop stops a running VM: kill Firecracker, teardown network, stop proxies.
// Unlike vm.delete, it preserves the chroot and slot for restart.
func (s *Server) handleVMFullStop(ctx context.Context, params json.RawMessage) Response {
	var p vmFullStopParams
	if err := json.Unmarshal(params, &p); err != nil {
		return Response{OK: false, Error: "invalid params: " + err.Error(), Code: "PARSE_ERROR"}
	}
	if p.VMId == "" {
		return Response{OK: false, Error: "vmId is required", Code: "VALIDATION_ERROR"}
	}

	// Resolve slot from allocator if not provided.
	slot := p.Slot
	if slot <= 0 {
		slot = s.slots.GetSlot(p.VMId)
	}

	// 1. Kill Firecracker process.
	paths := jailer.NewPaths(p.VMId, resolveJailerBaseDir(p.JailerBaseDir))
	socketPath := p.SocketPath
	if socketPath == "" {
		socketPath = paths.SocketPath
	}

	// Try graceful stop via API first.
	fcClient := firecracker.NewClient(socketPath)
	stopped := false
	if err := fcClient.Stop(); err != nil {
		slog.Debug("graceful FC stop failed", "vmId", p.VMId, "error", err)
	} else {
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
		// Fall back to PID-based kill.
		if p.PID > 0 {
			proc, err := os.FindProcess(p.PID)
			if err == nil {
				proc.Kill()
				proc.Wait()
			}
		}
		killFirecracker(paths)
	}

	// 2. Teardown network.
	if slot > 0 {
		netnsName := p.NetNSName
		if netnsName == "" {
			netnsName = netsetup.NetNSName(p.VMId)
		}

		defaultIface, _ := netsetup.DetectDefaultInterface()
		netCfg := netsetup.SetupConfig{
			VMId:         p.VMId,
			Slot:         slot,
			TAPDevice:    netsetup.TAPDevice(slot),
			HostIP:       netsetup.VMHostIP(slot),
			GuestIP:      netsetup.VMGuestIP(slot),
			SubnetMask:   netsetup.VMSubnetMask,
			MACAddress:   netsetup.MACAddress(slot),
			NetNSName:    netnsName,
			VethHost:     netsetup.VethHostDev(slot),
			VethGuest:    netsetup.VethGuestDev(slot),
			DefaultIface: defaultIface,
		}

		if err := netsetup.TeardownFirewall(ctx, netCfg, nil); err != nil {
			slog.Debug("firewall teardown failed", "vmId", p.VMId, "error", err)
		}
		netsetup.TeardownThrottle(netCfg.TAPDevice, netCfg.NetNSName)
		if err := netsetup.TeardownNamespace(netCfg); err != nil {
			slog.Debug("namespace teardown failed", "vmId", p.VMId, "error", err)
		}
	}

	// 3. Stop proxies.
	if err := s.manager.StopVM(p.VMId); err != nil {
		slog.Debug("proxy stop failed", "vmId", p.VMId, "error", err)
	}

	// 4. Stop mesh networking.
	if s.meshManager != nil {
		netnsName := p.NetNSName
		if netnsName == "" {
			netnsName = netsetup.NetNSName(p.VMId)
		}
		vethHost := ""
		vethGuest := ""
		if slot > 0 {
			vethHost = netsetup.VethHostDev(slot)
			vethGuest = netsetup.VethGuestDev(slot)
		}
		if err := s.meshManager.OnVMStop(p.VMId, vethHost, netnsName, vethGuest); err != nil {
			slog.Debug("mesh stop failed", "vmId", p.VMId, "error", err)
		}
	}

	// 5. Cancel timeout and update metadata.
	s.timeoutManager.Cancel(p.VMId)
	if err := updateVMMetadataFields(p.VMId, func(m *VMMetadata) {
		m.Status = "stopped"
		m.PID = 0
	}); err != nil {
		slog.Debug("metadata update failed", "vmId", p.VMId, "error", err)
	}

	slog.Info("vm stopped (full)", "vmId", p.VMId, "slot", slot)
	return Response{OK: true}
}

// handleVMFullUpdatePolicy tears down old nftables rules and re-sets them with
// the new policy. It also updates the SNI proxy domains.
func (s *Server) handleVMFullUpdatePolicy(ctx context.Context, params json.RawMessage) Response {
	var p vmFullUpdatePolicyParams
	if err := json.Unmarshal(params, &p); err != nil {
		return Response{OK: false, Error: "invalid params: " + err.Error(), Code: "PARSE_ERROR"}
	}
	if p.VMId == "" {
		return Response{OK: false, Error: "vmId is required", Code: "VALIDATION_ERROR"}
	}
	if p.Policy == "" {
		return Response{OK: false, Error: "policy is required", Code: "VALIDATION_ERROR"}
	}

	// Update proxy-level policy first.
	if err := s.manager.UpdatePolicy(p.VMId, p.Policy); err != nil {
		return Response{OK: false, Error: err.Error(), Code: "UPDATE_ERROR"}
	}

	// Update DNS handler policy for this VM.
	slot := p.Slot
	if slot <= 0 {
		slot = s.slots.GetSlot(p.VMId)
	}
	if s.meshManager != nil && slot >= 0 {
		guestIP := netsetup.VMGuestIP(slot)
		s.meshManager.RegisterDNSVM(p.VMId, guestIP, p.Policy)
	}

	// Rebuild nftables rules with new policy.
	if slot > 0 {
		netnsName := p.NetNSName
		if netnsName == "" {
			netnsName = netsetup.NetNSName(p.VMId)
		}

		defaultIface, _ := netsetup.DetectDefaultInterface()
		guestIP := netsetup.VMGuestIP(slot)

		netCfg := netsetup.SetupConfig{
			VMId:         p.VMId,
			Slot:         slot,
			TAPDevice:    netsetup.TAPDevice(slot),
			HostIP:       netsetup.VMHostIP(slot),
			GuestIP:      guestIP,
			SubnetMask:   netsetup.VMSubnetMask,
			MACAddress:   netsetup.MACAddress(slot),
			NetNSName:    netnsName,
			VethHost:     netsetup.VethHostDev(slot),
			VethGuest:    netsetup.VethGuestDev(slot),
			DefaultIface: defaultIface,
		}

		// Teardown old rules.
		if err := netsetup.TeardownFirewall(ctx, netCfg, nil); err != nil {
			slog.Debug("firewall teardown for policy update failed", "vmId", p.VMId, "error", err)
		}

		// Setup with new policy.
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
			return Response{OK: false, Error: fmt.Sprintf("setup firewall with new policy: %s", err), Code: "FIREWALL_ERROR"}
		}
	}

	// Update metadata with new policy.
	if err := updateVMMetadataFields(p.VMId, func(m *VMMetadata) {
		m.Network.Policy = p.Policy
		m.Network.Domains = p.Domains
		m.Network.AllowedCIDRs = p.AllowedCIDRs
		m.Network.DeniedCIDRs = p.DeniedCIDRs
		m.Network.Ports = p.Ports
		m.Network.AllowICMP = p.AllowICMP
	}); err != nil {
		slog.Debug("metadata update for policy failed", "vmId", p.VMId, "error", err)
	}

	slog.Info("vm policy updated (full)", "vmId", p.VMId, "policy", p.Policy)
	return Response{OK: true}
}

// handleVMSnapshotCreate pauses a VM, creates a snapshot, copies files out,
// chowns them, and resumes the VM.
func (s *Server) handleVMSnapshotCreate(ctx context.Context, params json.RawMessage) Response {
	var p vmSnapshotCreateParams
	if err := json.Unmarshal(params, &p); err != nil {
		return Response{OK: false, Error: "invalid params: " + err.Error(), Code: "PARSE_ERROR"}
	}
	if p.VMId == "" {
		return Response{OK: false, Error: "vmId is required", Code: "VALIDATION_ERROR"}
	}
	if p.SnapshotID == "" {
		return Response{OK: false, Error: "snapshotId is required", Code: "VALIDATION_ERROR"}
	}
	if p.SocketPath == "" {
		return Response{OK: false, Error: "socketPath is required", Code: "VALIDATION_ERROR"}
	}
	if p.DestDir == "" {
		return Response{OK: false, Error: "destDir is required", Code: "VALIDATION_ERROR"}
	}

	fcClient := firecracker.NewClient(p.SocketPath)

	// 1. Pause VM.
	if err := fcClient.Pause(); err != nil {
		return Response{OK: false, Error: fmt.Sprintf("pause VM: %s", err), Code: "FIRECRACKER_ERROR"}
	}

	// Always resume on exit, even if snapshot fails.
	defer func() {
		if err := fcClient.Resume(); err != nil {
			slog.Warn("resume after snapshot failed", "vmId", p.VMId, "error", err)
		}
	}()

	// 2. Compute chroot paths (needed for both snapshot creation and file copying).
	chrootDir := p.ChrootDir
	if chrootDir == "" {
		chrootDir = jailer.NewPaths(p.VMId, resolveJailerBaseDir(p.JailerBaseDir)).ChrootDir
	}
	rootDir := filepath.Join(chrootDir, "root")

	// 3. Create snapshot directory inside chroot before Firecracker writes to it.
	snapshotDir := filepath.Join(rootDir, "snapshot")
	if err := os.MkdirAll(snapshotDir, 0755); err != nil {
		return Response{OK: false, Error: fmt.Sprintf("create snapshot dir: %s", err), Code: "INTERNAL_ERROR"}
	}

	// Chown to jailer UID so the Firecracker process can write.
	if err := os.Chown(snapshotDir, jailer.JailerUID, jailer.JailerGID); err != nil {
		slog.Warn("chown snapshot dir failed", "error", err)
	}

	// 4. Create snapshot via Firecracker API.
	// Snapshot paths are relative to the chroot root.
	if err := fcClient.Snapshot("snapshot/snapshot_file", "snapshot/mem_file"); err != nil {
		return Response{OK: false, Error: fmt.Sprintf("create snapshot: %s", err), Code: "FIRECRACKER_ERROR"}
	}

	// 5. Ensure destination directory exists.
	if err := os.MkdirAll(p.DestDir, 0755); err != nil {
		return Response{OK: false, Error: fmt.Sprintf("create dest dir: %s", err), Code: "INTERNAL_ERROR"}
	}

	// 6. Copy snapshot files from chroot to destination.
	srcSnapshot := filepath.Join(snapshotDir, "snapshot_file")
	srcMem := filepath.Join(snapshotDir, "mem_file")
	dstSnapshot := filepath.Join(p.DestDir, "snapshot_file")
	dstMem := filepath.Join(p.DestDir, "mem_file")

	if err := copyFileForSnapshot(srcSnapshot, dstSnapshot); err != nil {
		return Response{OK: false, Error: fmt.Sprintf("copy snapshot file: %s", err), Code: "INTERNAL_ERROR"}
	}
	if err := copyFileForSnapshot(srcMem, dstMem); err != nil {
		return Response{OK: false, Error: fmt.Sprintf("copy mem file: %s", err), Code: "INTERNAL_ERROR"}
	}

	// 7. Copy rootfs to snapshot dir for complete restore.
	srcRootfs := filepath.Join(rootDir, "rootfs", "rootfs.ext4")
	dstRootfs := filepath.Join(p.DestDir, "rootfs.ext4")
	if err := copyFileForSnapshot(srcRootfs, dstRootfs); err != nil {
		slog.Warn("copy rootfs for snapshot failed", "vmId", p.VMId, "error", err)
		// Non-fatal: snapshot_file and mem_file are the critical ones.
	}

	// 8. Chown to owner if specified.
	if p.OwnerUID > 0 || p.OwnerGID > 0 {
		uid := p.OwnerUID
		gid := p.OwnerGID
		if gid <= 0 {
			gid = uid
		}
		cmd := exec.Command("chown", "-R", fmt.Sprintf("%d:%d", uid, gid), p.DestDir)
		if output, err := cmd.CombinedOutput(); err != nil {
			slog.Warn("chown snapshot dir failed", "error", err, "output", string(output))
		}
	}

	slog.Info("snapshot created",
		"vmId", p.VMId,
		"snapshotId", p.SnapshotID,
		"destDir", p.DestDir,
	)

	return Response{
		OK: true,
		VM: map[string]any{
			"vmId":       p.VMId,
			"snapshotId": p.SnapshotID,
			"destDir":    p.DestDir,
		},
	}
}

// handleRootfsBuild builds a rootfs image from a Docker/OCI image reference.
// Flow: docker build/export → tar extract → mkfs.ext4 → tune2fs → chown.
func (s *Server) handleRootfsBuild(_ context.Context, params json.RawMessage) Response {
	var p rootfsBuildParams
	if err := json.Unmarshal(params, &p); err != nil {
		return Response{OK: false, Error: "invalid params: " + err.Error(), Code: "PARSE_ERROR"}
	}
	if p.ImageRef == "" {
		return Response{OK: false, Error: "imageRef is required", Code: "VALIDATION_ERROR"}
	}
	if p.OutputDir == "" {
		return Response{OK: false, Error: "outputDir is required", Code: "VALIDATION_ERROR"}
	}

	// 1. Ensure output directory exists.
	if err := os.MkdirAll(p.OutputDir, 0755); err != nil {
		return Response{OK: false, Error: fmt.Sprintf("create output dir: %s", err), Code: "INTERNAL_ERROR"}
	}

	tarPath := filepath.Join(p.OutputDir, "rootfs.tar")
	rootfsPath := filepath.Join(p.OutputDir, "rootfs.ext4")
	extractDir := filepath.Join(p.OutputDir, "rootfs-extract")

	// 2. Create a temporary container and export its filesystem.
	containerName := fmt.Sprintf("vmsan-build-%d", time.Now().UnixNano())
	createCmd := exec.Command("docker", "create", "--name", containerName, p.ImageRef)
	if output, err := createCmd.CombinedOutput(); err != nil {
		return Response{OK: false, Error: fmt.Sprintf("docker create: %s: %s", err, string(output)), Code: "BUILD_ERROR"}
	}
	defer exec.Command("docker", "rm", "-f", containerName).Run()

	exportCmd := exec.Command("docker", "export", "-o", tarPath, containerName)
	if output, err := exportCmd.CombinedOutput(); err != nil {
		return Response{OK: false, Error: fmt.Sprintf("docker export: %s: %s", err, string(output)), Code: "BUILD_ERROR"}
	}

	// 3. Extract tar.
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		return Response{OK: false, Error: fmt.Sprintf("create extract dir: %s", err), Code: "INTERNAL_ERROR"}
	}
	defer os.RemoveAll(extractDir)

	extractCmd := exec.Command("tar", "xf", tarPath, "-C", extractDir)
	if output, err := extractCmd.CombinedOutput(); err != nil {
		return Response{OK: false, Error: fmt.Sprintf("tar extract: %s: %s", err, string(output)), Code: "BUILD_ERROR"}
	}

	// 4. Create ext4 filesystem image.
	// Estimate size: 1.5x the extracted size, minimum 256MB.
	var totalSize int64
	filepath.Walk(extractDir, func(_ string, info os.FileInfo, _ error) error {
		if info != nil {
			totalSize += info.Size()
		}
		return nil
	})
	imgSizeBytes := int64(float64(totalSize) * 1.5)
	if imgSizeBytes < 256*1024*1024 {
		imgSizeBytes = 256 * 1024 * 1024
	}

	// Create empty file of target size.
	truncCmd := exec.Command("truncate", "-s", fmt.Sprintf("%d", imgSizeBytes), rootfsPath)
	if output, err := truncCmd.CombinedOutput(); err != nil {
		return Response{OK: false, Error: fmt.Sprintf("truncate: %s: %s", err, string(output)), Code: "BUILD_ERROR"}
	}

	// Create ext4 filesystem.
	mkfsCmd := exec.Command("mkfs.ext4", "-F", rootfsPath)
	if output, err := mkfsCmd.CombinedOutput(); err != nil {
		return Response{OK: false, Error: fmt.Sprintf("mkfs.ext4: %s: %s", err, string(output)), Code: "BUILD_ERROR"}
	}

	// Set reserved blocks to 0.
	tuneCmd := exec.Command("tune2fs", "-m", "0", rootfsPath)
	if output, err := tuneCmd.CombinedOutput(); err != nil {
		slog.Warn("tune2fs failed", "error", err, "output", string(output))
	}

	// 5. Mount and copy extracted files.
	mountDir := filepath.Join(p.OutputDir, "rootfs-mount")
	if err := os.MkdirAll(mountDir, 0755); err != nil {
		return Response{OK: false, Error: fmt.Sprintf("create mount dir: %s", err), Code: "INTERNAL_ERROR"}
	}
	defer os.RemoveAll(mountDir)

	mountCmd := exec.Command("sudo", "mount", "-o", "loop", rootfsPath, mountDir)
	if output, err := mountCmd.CombinedOutput(); err != nil {
		return Response{OK: false, Error: fmt.Sprintf("mount: %s: %s", err, string(output)), Code: "BUILD_ERROR"}
	}
	defer func() {
		exec.Command("sudo", "umount", mountDir).Run()
	}()

	copyCmd := exec.Command("sudo", "cp", "-a", extractDir+"/.", mountDir)
	if output, err := copyCmd.CombinedOutput(); err != nil {
		return Response{OK: false, Error: fmt.Sprintf("copy to rootfs: %s: %s", err, string(output)), Code: "BUILD_ERROR"}
	}

	// Unmount before chown.
	exec.Command("sudo", "umount", mountDir).Run()

	// Clean up tar file.
	os.Remove(tarPath)

	// 6. Chown to owner if specified.
	if p.OwnerUID > 0 || p.OwnerGID > 0 {
		uid := p.OwnerUID
		gid := p.OwnerGID
		if gid <= 0 {
			gid = uid
		}
		cmd := exec.Command("chown", fmt.Sprintf("%d:%d", uid, gid), rootfsPath)
		if output, err := cmd.CombinedOutput(); err != nil {
			slog.Warn("chown rootfs failed", "error", err, "output", string(output))
		}
	}

	slog.Info("rootfs built", "imageRef", p.ImageRef, "output", rootfsPath)

	return Response{
		OK: true,
		VM: map[string]any{
			"rootfsPath": rootfsPath,
			"imageRef":   p.ImageRef,
		},
	}
}

// copyFileForSnapshot copies a file from src to dst using io.Copy.
func copyFileForSnapshot(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create %s: %w", dst, err)
	}
	defer out.Close()

	if _, err := out.ReadFrom(in); err != nil {
		return fmt.Errorf("copy %s -> %s: %w", src, dst, err)
	}
	return out.Close()
}
