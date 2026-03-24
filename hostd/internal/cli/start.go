package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	vmsanv1 "github.com/angelorc/vmsan/hostd/gen/vmsan/v1"
	"github.com/angelorc/vmsan/hostd/internal/gwclient"
	"github.com/angelorc/vmsan/hostd/internal/paths"
	"github.com/angelorc/vmsan/hostd/internal/vmstate"
	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start <vmId>",
	Short: "Start a previously stopped VM",
	Args:  cobra.ExactArgs(1),
	RunE:  runStart,
}

func init() {
	rootCmd.AddCommand(startCmd)
}

func runStart(_ *cobra.Command, args []string) error {
	vmID := args[0]
	p := paths.Resolve()
	store := vmstate.NewStore(p.VmsDir)

	state, err := store.Load(vmID)
	if err != nil {
		return fmt.Errorf("VM %s not found: %w", vmID, err)
	}
	if state.Status == "running" {
		return fmt.Errorf("VM %s is already running", vmID)
	}

	gw, err := gwclient.New()
	if err != nil {
		return fmt.Errorf("gateway connection failed: %w", err)
	}
	defer gw.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	req := &vmsanv1.RestartVMRequest{
		VmId:           vmID,
		Slot:           int32(slotFromState(state)),
		ChrootDir:      state.ChrootDir,
		SocketPath:     state.APISocket,
		NetworkPolicy:  state.Network.NetworkPolicy,
		Domains:        state.Network.AllowedDomains,
		AllowedCidrs:   state.Network.AllowedCidrs,
		DeniedCidrs:    state.Network.DeniedCidrs,
		Ports:          toInt32Slice(state.Network.PublishedPorts),
		BandwidthMbit:  int32(state.Network.BandwidthMbit),
		AllowIcmp:      state.Network.AllowIcmp,
		SkipDnat:       state.Network.SkipDnat,
		Project:        state.Project,
		Service:        state.Network.Service,
		ConnectTo:      state.Network.ConnectTo,
		DisableSeccomp: state.DisableSeccomp,
		DisablePidNs:   state.DisablePidNs,
		DisableCgroup:  state.DisableCgroup,
		SeccompFilter:  p.SeccompFilter,
		Vcpus:          int32(state.VcpuCount),
		MemMib:         int32(state.MemSizeMib),
		KernelPath:     state.Kernel,
		RootfsPath:     state.Rootfs,
		AgentBinary:    p.AgentBin,
		AgentToken:     derefStr(state.AgentToken),
		NetNsName:      state.Network.NetNSName,
		JailerBaseDir:  p.JailerBaseDir,
	}

	resp, err := gw.RestartVM(ctx, req)
	if err != nil {
		return fmt.Errorf("start VM: %w", err)
	}

	// Update local state
	pid := int(resp.Pid)
	if err := store.Update(vmID, func(s *vmstate.VmState) {
		s.Status = "running"
		s.PID = &pid
		s.APISocket = resp.SocketPath
		s.ChrootDir = resp.ChrootDir
		s.Network.TapDevice = resp.TapDevice
		s.Network.HostIP = resp.HostIp
		s.Network.GuestIP = resp.GuestIp
		s.Network.MACAddress = resp.MacAddress
		s.Network.SubnetMask = resp.SubnetMask
		s.Network.NetNSName = resp.NetNsName
		s.Network.MeshIP = resp.MeshIp
	}); err != nil {
		return fmt.Errorf("update state: %w", err)
	}

	if jsonOutput {
		data, _ := json.MarshalIndent(map[string]any{
			"vmId":    vmID,
			"status":  "running",
			"pid":     resp.Pid,
			"guestIp": resp.GuestIp,
		}, "", "  ")
		fmt.Println(string(data))
	} else {
		fmt.Printf("Started %s (PID %d, guest %s)\n", vmID, resp.Pid, resp.GuestIp)
	}

	return nil
}

func slotFromState(state *vmstate.VmState) int {
	// The slot is encoded in the host IP: 10.0.<slot>.1
	// Parse the third octet from hostIP
	parts := splitDot(state.Network.HostIP)
	if len(parts) >= 3 {
		slot := 0
		for _, c := range parts[2] {
			if c >= '0' && c <= '9' {
				slot = slot*10 + int(c-'0')
			}
		}
		return slot
	}
	return 0
}

func splitDot(s string) []string {
	var parts []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == '.' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	return parts
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
