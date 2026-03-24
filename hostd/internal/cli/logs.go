package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/angelorc/vmsan/hostd/internal/agentclient"
	"github.com/spf13/cobra"
)

var logsCmd = &cobra.Command{
	Use:   "logs <vmId> [service]",
	Short: "Stream logs from a running VM",
	Long: `Stream logs from a running VM via the in-VM agent.

By default, reads the last 100 lines of journal output. Use -f to follow.
Optionally specify a systemd service unit to filter.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runLogs,
}

func init() {
	f := logsCmd.Flags()
	f.IntP("lines", "n", 100, "Number of historical lines to show")
	f.BoolP("follow", "f", false, "Follow log output")
	f.BoolP("timestamps", "t", false, "Show timestamps")

	rootCmd.AddCommand(logsCmd)
}

func runLogs(cmd *cobra.Command, args []string) error {
	vmID := args[0]
	service := ""
	if len(args) > 1 {
		service = args[1]
	}

	lines, _ := cmd.Flags().GetInt("lines")
	follow, _ := cmd.Flags().GetBool("follow")
	timestamps, _ := cmd.Flags().GetBool("timestamps")

	vm, err := resolveVM(vmID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := waitForAgent(ctx, vm.State.Network.GuestIP, vm.State.AgentPort); err != nil {
		return err
	}

	// Handle signals.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		cancel()
	}()
	defer signal.Stop(sigs)

	// Build journalctl command.
	journalArgs := []string{"-n", strconv.Itoa(lines), "--no-pager"}
	if follow {
		journalArgs = append(journalArgs, "-f")
	}
	if timestamps {
		journalArgs = append(journalArgs, "-o", "short-iso")
	}
	if service != "" {
		journalArgs = append(journalArgs, "-u", service)
	}

	params := agentclient.RunParams{
		Cmd:  "journalctl",
		Args: journalArgs,
		User: "root",
	}

	events, err := vm.Client.Exec(ctx, params)
	if err != nil {
		return err
	}

	for event := range events {
		switch event.Type {
		case "stdout":
			fmt.Fprint(os.Stdout, event.Data)
		case "stderr":
			fmt.Fprint(os.Stderr, event.Data)
		case "error":
			fmt.Fprintf(os.Stderr, "Error: %s\n", event.Error)
		}
	}

	return nil
}
