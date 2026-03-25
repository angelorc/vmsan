package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
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

	// DNS logs are written to a host-side file by the gateway's dnsproxy,
	// not inside the VM. Read them directly from the host filesystem.
	if service == "dns" {
		return runLogsDNS(vmID, lines, follow)
	}

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

	// When no service is specified, read the app log file (/var/log/app.log)
	// which is where the deploy engine's StartApp writes stdout/stderr.
	// When a service is specified, use journalctl to read its systemd unit logs.
	var params agentclient.RunParams
	if service == "" {
		tailArgs := []string{"-n", strconv.Itoa(lines), "/var/log/app.log"}
		if follow {
			tailArgs = []string{"-n", strconv.Itoa(lines), "-f", "/var/log/app.log"}
		}
		params = agentclient.RunParams{
			Cmd:  "tail",
			Args: tailArgs,
			User: "root",
		}
	} else {
		journalArgs := []string{"-n", strconv.Itoa(lines), "--no-pager", "-u", service}
		if follow {
			journalArgs = append(journalArgs, "-f")
		}
		if timestamps {
			journalArgs = append(journalArgs, "-o", "short-iso")
		}
		params = agentclient.RunParams{
			Cmd:  "journalctl",
			Args: journalArgs,
			User: "root",
		}
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

// runLogsDNS reads DNS query logs from the host-side log file written by
// the gateway's dnsproxy at /tmp/vmsan-dns-<vmId>.log.
func runLogsDNS(vmID string, lines int, follow bool) error {
	logPath := fmt.Sprintf("/tmp/vmsan-dns-%s.log", vmID)

	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "No DNS log found for VM %s\nExpected: %s\n", vmID, logPath)
		os.Exit(1)
	}

	if follow {
		// tail -f the log file
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigs
			cancel()
		}()
		defer signal.Stop(sigs)

		cmd := exec.Command("tail", "-n", strconv.Itoa(lines), "-f", logPath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		go func() {
			<-ctx.Done()
			if cmd.Process != nil {
				cmd.Process.Kill()
			}
		}()
		return cmd.Run()
	}

	// Read last N lines
	cmd := exec.Command("tail", "-n", strconv.Itoa(lines), logPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
