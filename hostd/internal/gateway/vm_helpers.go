package gateway

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/angelorc/vmsan/hostd/internal/firecracker"
	"github.com/angelorc/vmsan/hostd/internal/jailer"
	"github.com/angelorc/vmsan/hostd/internal/netsetup"
	nftypes "github.com/angelorc/vmsan/nftables"
)

// networkResult holds the results of setupVMNetwork.
type networkResult struct {
	NetCfg         netsetup.SetupConfig
	Policy         string
	PublishedPorts []nftypes.PublishedPort
}

// spawnResult holds the results of spawnFirecracker.
type spawnResult struct {
	Paths      jailer.Paths
	PID        int
	AgentToken string
}

// setupVMNetwork sets up the VM's network: namespace, veth pair, TAP,
// firewall, throttling, proxies, and mesh networking.
// It returns a networkResult, a list of cleanup functions (run in reverse on
// error), and an error.
func (s *Server) setupVMNetwork(ctx context.Context, vmId string, slot int, p vmCreateParams) (networkResult, []func(), error) {
	var cleanup []func()

	// 1. Compute network addresses.
	hostIP := netsetup.VMHostIP(slot)
	guestIP := netsetup.VMGuestIP(slot)
	tapDevice := netsetup.TAPDevice(slot)
	macAddress := netsetup.MACAddress(slot)
	netnsName := netsetup.NetNSName(vmId)
	vethHost := netsetup.VethHostDev(slot)
	vethGuest := netsetup.VethGuestDev(slot)

	// 2. Detect default interface.
	defaultIface, err := netsetup.DetectDefaultInterface()
	if err != nil {
		return networkResult{}, cleanup, fmt.Errorf("detect default interface: %w", err)
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

	// 3. Setup network namespace + veth pair.
	if err := netsetup.SetupNamespace(netCfg); err != nil {
		return networkResult{}, cleanup, fmt.Errorf("setup namespace: %w", err)
	}
	cleanup = append(cleanup, func() {
		netsetup.TeardownNamespace(netCfg)
	})

	// 4. Setup TAP device.
	if err := netsetup.SetupTAP(netCfg); err != nil {
		return networkResult{}, cleanup, fmt.Errorf("setup TAP: %w", err)
	}

	// 5. Setup firewall.
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
		return networkResult{}, cleanup, fmt.Errorf("setup firewall: %w", err)
	}
	cleanup = append(cleanup, func() {
		netsetup.TeardownFirewall(ctx, netCfg, publishedPorts)
	})

	// 6. Setup bandwidth throttling.
	if err := netsetup.SetupThrottle(netCfg); err != nil {
		slog.Warn("throttle setup failed, continuing", "vmId", vmId, "error", err)
	}

	// 7. Start proxy manager (DNS, SNI, HTTP proxies).
	if _, err := s.manager.StartVM(vmId, slot, policy, p.Domains); err != nil {
		return networkResult{}, cleanup, fmt.Errorf("start proxies: %w", err)
	}
	cleanup = append(cleanup, func() {
		s.manager.StopVM(vmId)
	})

	// 8. Start mesh networking if VM has a project.
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
			_ = meshResult.MeshIP // mesh IP is stored in the manager, not needed here
		}
	}

	return networkResult{
		NetCfg:         netCfg,
		Policy:         policy,
		PublishedPorts: publishedPorts,
	}, cleanup, nil
}

// spawnFirecracker prepares the jailer chroot, spawns the jailer process, and
// boots the Firecracker instance.
// It returns a spawnResult, a list of cleanup functions (run in reverse on
// error), and an error.
func (s *Server) spawnFirecracker(ctx context.Context, vmId string, slot int, p vmCreateParams, netCfg netsetup.SetupConfig, kernelPath, rootfsPath string) (spawnResult, []func(), error) {
	var cleanup []func()
	netnsName := netCfg.NetNSName
	tapDevice := netCfg.TAPDevice
	macAddress := netCfg.MACAddress
	guestIP := netCfg.GuestIP
	hostIP := netCfg.HostIP

	// 1. Prepare jailer chroot.
	paths := jailer.NewPaths(vmId, resolveJailerBaseDir(p.JailerBaseDir))

	agentBin := p.AgentBinary
	if agentBin == "" {
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

	if p.SnapshotID != "" {
		home, _ := os.UserHomeDir()
		snapshotDir := filepath.Join(home, ".vmsan", "snapshots", p.SnapshotID)
		jailCfg.Snapshot = &jailer.SnapshotConfig{
			SnapshotFile: filepath.Join(snapshotDir, "snapshot_file"),
			MemFile:      filepath.Join(snapshotDir, "mem_file"),
		}
	}

	if err := jailer.Prepare(jailCfg, paths); err != nil {
		return spawnResult{}, cleanup, fmt.Errorf("prepare chroot: %w", err)
	}
	cleanup = append(cleanup, func() {
		jailer.Cleanup(paths.ChrootDir)
	})

	// 2. Compute cgroup limits.
	cpuQuotaUs := p.VCPUs * 100000
	cpuPeriodUs := 100000
	memBytes := int64(p.MemMiB+jailer.CgroupVMMOverheadMiB) * 1024 * 1024

	// 3. Spawn jailer.
	spawnCfg := jailer.SpawnConfig{
		FirecrackerBin: firecrackerBin,
		JailerBin:      jailerBin,
		VMId:           vmId,
		Paths:          paths,
		UID:            jailer.JailerUID,
		GID:            jailer.JailerGID,
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
		return spawnResult{}, cleanup, fmt.Errorf("spawn jailer: %w", err)
	}
	cleanup = append(cleanup, func() {
		killFirecracker(paths)
		jailer.Cleanup(paths.ChrootDir)
	})

	// 4. Wait for Firecracker socket.
	fcClient := firecracker.NewClient(paths.SocketPath)
	socketCtx, socketCancel := context.WithTimeout(ctx, 30*time.Second)
	defer socketCancel()
	if err := fcClient.WaitForSocket(socketCtx, 30*time.Second); err != nil {
		return spawnResult{}, cleanup, fmt.Errorf("wait for socket: %w", err)
	}

	// 5. Configure Firecracker via API.
	if err := fcClient.Configure(firecracker.MachineConfig{
		VCPUs:  p.VCPUs,
		MemMiB: p.MemMiB,
	}); err != nil {
		return spawnResult{}, cleanup, fmt.Errorf("configure machine: %w", err)
	}

	bootArgs := netsetup.BootArgs(guestIP, hostIP, netsetup.VMSubnetMask)
	if err := fcClient.Boot("kernel/vmlinux", bootArgs); err != nil {
		return spawnResult{}, cleanup, fmt.Errorf("configure boot: %w", err)
	}

	if err := fcClient.AddDrive("rootfs", "rootfs/rootfs.ext4", true, false); err != nil {
		return spawnResult{}, cleanup, fmt.Errorf("add drive: %w", err)
	}

	if err := fcClient.AddNetwork("eth0", tapDevice, macAddress); err != nil {
		return spawnResult{}, cleanup, fmt.Errorf("add network: %w", err)
	}

	// 6. Start Firecracker instance.
	if p.SnapshotID != "" {
		if err := fcClient.LoadSnapshot("snapshot/snapshot_file", "snapshot/mem_file"); err != nil {
			return spawnResult{}, cleanup, fmt.Errorf("load snapshot: %w", err)
		}
		if err := fcClient.Resume(); err != nil {
			return spawnResult{}, cleanup, fmt.Errorf("resume snapshot: %w", err)
		}
	} else {
		if err := fcClient.Start(); err != nil {
			return spawnResult{}, cleanup, fmt.Errorf("start instance: %w", err)
		}
	}

	// 7. Wait for agent health.
	if agentBin != "" {
		if err := waitForAgentHealth(guestIP, 30*time.Second); err != nil {
			slog.Warn("agent health check failed", "vmId", vmId, "error", err)
		}
	}

	return spawnResult{
		Paths:      paths,
		PID:        findFirecrackerPID(paths),
		AgentToken: agentToken,
	}, cleanup, nil
}
