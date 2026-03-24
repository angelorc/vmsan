package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/angelorc/vmsan/hostd/internal/agentclient"
	"github.com/spf13/cobra"
)

var uploadCmd = &cobra.Command{
	Use:   "upload <vmId> <files...>",
	Short: "Upload local files to a running VM",
	Args:  cobra.MinimumNArgs(2),
	RunE:  runUpload,
}

func init() {
	uploadCmd.Flags().StringP("dest", "d", "/root", "Destination directory inside the VM")

	rootCmd.AddCommand(uploadCmd)
}

func runUpload(cmd *cobra.Command, args []string) error {
	vmID := args[0]
	filePaths := args[1:]
	dest, _ := cmd.Flags().GetString("dest")

	vm, err := resolveVM(vmID)
	if err != nil {
		return err
	}

	ctx := context.Background()
	if err := waitForAgent(ctx, vm.State.Network.GuestIP, vm.State.AgentPort); err != nil {
		return err
	}

	// Read local files.
	var files []agentclient.WriteFileEntry
	for _, p := range filePaths {
		content, err := os.ReadFile(p)
		if err != nil {
			return fmt.Errorf("read %s: %w", p, err)
		}
		files = append(files, agentclient.WriteFileEntry{
			Path:    filepath.Base(p),
			Content: content,
			Mode:    0644,
		})
	}

	fmt.Printf("Uploading %d file(s) to %s...\n", len(files), dest)

	if err := vm.Client.WriteFiles(ctx, files, dest); err != nil {
		return err
	}

	fmt.Printf("Uploaded %d file(s) to %s\n", len(files), dest)
	return nil
}
