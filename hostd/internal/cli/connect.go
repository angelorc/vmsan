package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/angelorc/vmsan/hostd/internal/agentclient"
	"github.com/spf13/cobra"
)

var connectCmd = &cobra.Command{
	Use:   "connect <vmId>",
	Short: "Connect to a running VM with an interactive shell",
	Args:  cobra.ExactArgs(1),
	RunE:  runConnect,
}

func init() {
	connectCmd.Flags().StringP("session", "s", "", "Attach to an existing shell session ID")

	rootCmd.AddCommand(connectCmd)
}

func runConnect(cmd *cobra.Command, args []string) error {
	vmID := args[0]
	sessionID, _ := cmd.Flags().GetString("session")

	vm, err := resolveVM(vmID)
	if err != nil {
		return err
	}

	ctx := context.Background()
	if err := waitForAgent(ctx, vm.State.Network.GuestIP, vm.State.AgentPort); err != nil {
		return err
	}

	token := ""
	if vm.State.AgentToken != nil {
		token = *vm.State.AgentToken
	}

	closeInfo, err := agentclient.RunShell(agentclient.ShellOptions{
		Host:      vm.State.Network.GuestIP,
		Port:      vm.State.AgentPort,
		Token:     token,
		SessionID: sessionID,
	})
	if err != nil {
		return err
	}

	if !closeInfo.SessionDestroyed && closeInfo.SessionID != "" {
		dim := "\x1b[2m"
		reset := "\x1b[0m"
		fmt.Fprintf(os.Stderr, "\n%sResume this session with:\n  vmsan connect %s --session %s%s\n", dim, vmID, closeInfo.SessionID, reset)
	}

	return nil
}
