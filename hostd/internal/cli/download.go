package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var downloadCmd = &cobra.Command{
	Use:   "download <vmId> <remotePath>",
	Short: "Download a file from a running VM",
	Args:  cobra.ExactArgs(2),
	RunE:  runDownload,
}

func init() {
	downloadCmd.Flags().StringP("dest", "d", "", "Local destination path (default: basename of remote path in cwd)")

	rootCmd.AddCommand(downloadCmd)
}

func runDownload(cmd *cobra.Command, args []string) error {
	vmID := args[0]
	remotePath := args[1]
	dest, _ := cmd.Flags().GetString("dest")

	vm, err := resolveVM(vmID)
	if err != nil {
		return err
	}

	ctx := context.Background()
	if err := waitForAgent(ctx, vm.State.Network.GuestIP, vm.State.AgentPort); err != nil {
		return err
	}

	fmt.Printf("Downloading %s...\n", remotePath)

	data, err := vm.Client.ReadFile(ctx, remotePath)
	if err != nil {
		return err
	}
	if data == nil {
		return fmt.Errorf("file not found on VM: %s", remotePath)
	}

	// Determine local destination path.
	localPath := dest
	if localPath == "" {
		localPath = filepath.Base(remotePath)
	} else {
		// If dest is a directory (or ends with /), put file inside it.
		info, err := os.Stat(localPath)
		if (err == nil && info.IsDir()) || localPath[len(localPath)-1] == '/' {
			if err := os.MkdirAll(localPath, 0755); err != nil {
				return fmt.Errorf("create directory: %w", err)
			}
			localPath = filepath.Join(localPath, filepath.Base(remotePath))
		}
	}

	if err := os.WriteFile(localPath, data, 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	fmt.Printf("Downloaded to %s (%d bytes)\n", localPath, len(data))
	return nil
}
