package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	vmsanv1 "github.com/angelorc/vmsan/hostd/gen/vmsan/v1"
	"github.com/angelorc/vmsan/hostd/internal/gwclient"
	"github.com/angelorc/vmsan/hostd/internal/output"
	"github.com/angelorc/vmsan/hostd/internal/paths"
	"github.com/angelorc/vmsan/hostd/internal/vmstate"
	"github.com/spf13/cobra"
)

var snapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Manage VM snapshots",
}

var snapshotCreateCmd = &cobra.Command{
	Use:   "create <vmId>",
	Short: "Create a snapshot of a VM",
	Args:  cobra.ExactArgs(1),
	RunE:  runSnapshotCreate,
}

var snapshotListCmd = &cobra.Command{
	Use:     "list [vmId]",
	Aliases: []string{"ls"},
	Short:   "List snapshots",
	Args:    cobra.MaximumNArgs(1),
	RunE:    runSnapshotList,
}

var snapshotDeleteCmd = &cobra.Command{
	Use:     "delete <snapshotId>",
	Aliases: []string{"rm"},
	Short:   "Delete a snapshot",
	Args:    cobra.ExactArgs(1),
	RunE:    runSnapshotDelete,
}

func init() {
	snapshotCmd.AddCommand(snapshotCreateCmd)
	snapshotCmd.AddCommand(snapshotListCmd)
	snapshotCmd.AddCommand(snapshotDeleteCmd)
	rootCmd.AddCommand(snapshotCmd)
}

func runSnapshotCreate(_ *cobra.Command, args []string) error {
	vmID := args[0]
	snapshotID := fmt.Sprintf("%s-%d", vmID, time.Now().Unix())

	// Load VM state to get socketPath, chrootDir, etc.
	p := paths.Resolve()
	store := vmstate.NewStore(p.VmsDir)
	state, err := store.Load(vmID)
	if err != nil {
		return fmt.Errorf("VM %s not found", vmID)
	}

	destDir := filepath.Join(p.SnapshotsDir, snapshotID)

	gw, err := gwclient.New()
	if err != nil {
		return fmt.Errorf("gateway connection failed: %w", err)
	}
	defer gw.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	resp, err := gw.CreateSnapshot(ctx, &vmsanv1.CreateSnapshotRequest{
		VmId:         vmID,
		SnapshotId:   snapshotID,
		SocketPath:   state.APISocket,
		DestDir:      destDir,
		ChrootDir:    state.ChrootDir,
		JailerBaseDir: p.JailerBaseDir,
		OwnerUid:     int32(os.Getuid()),
		OwnerGid:     int32(os.Getgid()),
	})
	if err != nil {
		return fmt.Errorf("create snapshot: %w", err)
	}

	// Save metadata.json alongside the snapshot files for restore
	metadata, _ := json.MarshalIndent(map[string]any{
		"vmId":       vmID,
		"snapshotId": snapshotID,
		"project":    state.Project,
		"runtime":    state.Runtime,
		"vcpuCount":  state.VcpuCount,
		"memSizeMib": state.MemSizeMib,
		"agentToken": state.AgentToken,
		"agentPort":  state.AgentPort,
		"network":    state.Network,
	}, "", "  ")
	_ = os.WriteFile(filepath.Join(destDir, "metadata.json"), metadata, 0644)

	if jsonOutput {
		data, _ := json.MarshalIndent(map[string]string{
			"snapshotId": resp.SnapshotId,
			"vmId":       vmID,
		}, "", "  ")
		fmt.Println(string(data))
	} else {
		fmt.Printf("  Snapshot created: %s\n", resp.SnapshotId)
	}
	return nil
}

func runSnapshotList(_ *cobra.Command, args []string) error {
	p := paths.Resolve()
	filterVM := ""
	if len(args) > 0 {
		filterVM = args[0]
	}

	entries, err := os.ReadDir(p.SnapshotsDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No snapshots found.")
			return nil
		}
		return fmt.Errorf("read snapshots dir: %w", err)
	}

	type snapshotInfo struct {
		ID        string `json:"id"`
		VmID      string `json:"vmId"`
		Size      string `json:"size"`
		CreatedAt string `json:"createdAt"`
	}
	var snapshots []snapshotInfo

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id := entry.Name()

		// Extract VM ID from snapshot directory contents
		vmID := extractVmID(filepath.Join(p.SnapshotsDir, id))
		if filterVM != "" && vmID != filterVM {
			continue
		}

		info, _ := entry.Info()
		created := ""
		size := ""
		if info != nil {
			created = info.ModTime().Format(time.RFC3339)
			size = dirSize(filepath.Join(p.SnapshotsDir, id))
		}

		snapshots = append(snapshots, snapshotInfo{
			ID:        id,
			VmID:      vmID,
			Size:      size,
			CreatedAt: created,
		})
	}

	if len(snapshots) == 0 {
		fmt.Println("No snapshots found.")
		return nil
	}

	if jsonOutput {
		data, _ := json.MarshalIndent(snapshots, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	headers := []string{"ID", "VM ID", "SIZE", "CREATED"}
	var rows [][]string
	for _, s := range snapshots {
		created := s.CreatedAt
		if t, err := time.Parse(time.RFC3339, s.CreatedAt); err == nil {
			created = output.TimeAgo(t)
		}
		rows = append(rows, []string{s.ID, s.VmID, s.Size, created})
	}
	output.PrintTable(os.Stdout, headers, rows)
	return nil
}

func runSnapshotDelete(_ *cobra.Command, args []string) error {
	snapshotID := args[0]
	p := paths.Resolve()
	snapshotDir := filepath.Join(p.SnapshotsDir, snapshotID)

	if _, err := os.Stat(snapshotDir); os.IsNotExist(err) {
		return fmt.Errorf("snapshot %q not found", snapshotID)
	}

	if err := os.RemoveAll(snapshotDir); err != nil {
		return fmt.Errorf("delete snapshot: %w", err)
	}

	fmt.Printf("  Snapshot deleted: %s\n", snapshotID)
	return nil
}

func extractVmID(snapshotDir string) string {
	// Convention: snapshot dir contains a state.json with vmId, or
	// the directory name encodes the VM ID as prefix
	data, err := os.ReadFile(filepath.Join(snapshotDir, "state.json"))
	if err != nil {
		// Fall back to parsing directory name
		parts := strings.SplitN(filepath.Base(snapshotDir), "-", 2)
		if len(parts) > 0 {
			return parts[0]
		}
		return ""
	}
	var state struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return ""
	}
	return state.ID
}

func dirSize(path string) string {
	var size int64
	_ = filepath.WalkDir(path, func(_ string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err == nil {
			size += info.Size()
		}
		return nil
	})
	return output.FormatBytes(size)
}
