package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/angelorc/vmsan/hostd/internal/agentclient"
	"github.com/spf13/cobra"
)

var execCmd = &cobra.Command{
	Use:   "exec <vmId> <command> [args...]",
	Short: "Execute a command inside a running VM",
	Long: `Execute a command inside a running VM.

In non-interactive mode, stdout/stderr are streamed to the terminal and the
process exits with the command's exit code.

In interactive mode (-i), a WebSocket PTY session is opened with the command
injected into the shell.`,
	Args:               cobra.MinimumNArgs(2),
	DisableFlagParsing: false,
	RunE:               runExec,
}

func init() {
	f := execCmd.Flags()
	f.Bool("sudo", false, "Run with extended privileges (root)")
	f.BoolP("interactive", "i", false, "Interactive shell mode (PTY)")
	f.BoolP("tty", "t", false, "Allocate a pseudo-TTY (accepted for compatibility)")
	f.StringP("workdir", "w", "", "Working directory inside the VM")
	f.StringArrayP("env", "e", nil, "Environment variable KEY=VAL (repeatable)")
	f.Bool("no-extend-timeout", false, "Skip timeout extension (interactive only)")

	rootCmd.AddCommand(execCmd)
}

func runExec(cmd *cobra.Command, args []string) error {
	vmID := args[0]
	command := args[1]
	cmdArgs := args[2:]

	f := cmd.Flags()
	sudo, _ := f.GetBool("sudo")
	interactive, _ := f.GetBool("interactive")
	workdir, _ := f.GetString("workdir")
	envFlags, _ := f.GetStringArray("env")

	// Parse env flags into map.
	envMap := make(map[string]string)
	for _, e := range envFlags {
		idx := strings.IndexByte(e, '=')
		if idx > 0 {
			envMap[e[:idx]] = e[idx+1:]
		}
	}

	vm, err := resolveVM(vmID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Wait for agent to be reachable.
	if err := waitForAgent(ctx, vm.State.Network.GuestIP, vm.State.AgentPort); err != nil {
		return err
	}

	if interactive {
		return runExecInteractive(vm, vmID, command, cmdArgs, workdir, envMap, sudo)
	}

	return runExecNonInteractive(ctx, cancel, vm, command, cmdArgs, workdir, envMap, sudo)
}

func runExecInteractive(
	vm *resolvedVM,
	vmID, command string,
	cmdArgs []string,
	workdir string,
	envMap map[string]string,
	sudo bool,
) error {
	// Build injected command string.
	var parts []string

	// Custom prompt.
	shortID := vmID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	prompt := fmt.Sprintf(`\[\033[1;32m\]vmsan:%s\[\033[0m\]:\[\033[1;34m\]\w\[\033[0m\]\$ `, shortID)
	parts = append(parts, fmt.Sprintf("export PS1=%s TERM=xterm-256color &&", shellEscape(prompt)))

	if workdir != "" {
		parts = append(parts, fmt.Sprintf("cd %s &&", shellEscape(workdir)))
	}

	for key, val := range envMap {
		parts = append(parts, fmt.Sprintf("%s=%s", key, shellEscape(val)))
	}

	parts = append(parts, shellEscape(command))
	for _, a := range cmdArgs {
		parts = append(parts, shellEscape(a))
	}

	injectedCmd := "clear; " + strings.Join(parts, " ") + "; exit $?\n"

	token := ""
	if vm.State.AgentToken != nil {
		token = *vm.State.AgentToken
	}

	var user string
	if sudo {
		user = "root"
	}

	closeInfo, err := agentclient.RunShell(agentclient.ShellOptions{
		Host:           vm.State.Network.GuestIP,
		Port:           vm.State.AgentPort,
		Token:          token,
		User:           user,
		InitialCommand: injectedCmd,
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

func runExecNonInteractive(
	ctx context.Context,
	cancel context.CancelFunc,
	vm *resolvedVM,
	command string,
	cmdArgs []string,
	workdir string,
	envMap map[string]string,
	sudo bool,
) error {
	// Handle signals for clean cancellation.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		cancel()
	}()
	defer signal.Stop(sigs)

	var user string
	if sudo {
		user = "root"
	}

	params := agentclient.RunParams{
		Cmd:  command,
		Args: cmdArgs,
		Cwd:  workdir,
		User: user,
	}
	if len(envMap) > 0 {
		params.Env = envMap
	}

	events, err := vm.Client.Exec(ctx, params)
	if err != nil {
		return err
	}

	exitCode := 0
	for event := range events {
		switch event.Type {
		case "stdout":
			fmt.Fprint(os.Stdout, event.Data)
		case "stderr":
			fmt.Fprint(os.Stderr, event.Data)
		case "exit":
			if event.ExitCode != nil {
				exitCode = *event.ExitCode
			}
		case "error":
			fmt.Fprintf(os.Stderr, "Error: %s\n", event.Error)
			exitCode = 1
		case "timeout":
			fmt.Fprintln(os.Stderr, "Command timed out.")
			exitCode = 124
		}
	}

	if ctx.Err() != nil {
		os.Exit(130)
	}
	if exitCode != 0 {
		os.Exit(exitCode)
	}

	return nil
}

// shellEscape quotes a string for safe shell injection.
func shellEscape(s string) string {
	// If it only contains safe characters, return as-is.
	safe := true
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '.' || c == '_' || c == '-' || c == '/' || c == '=' || c == ':' || c == '@') {
			safe = false
			break
		}
	}
	if safe && len(s) > 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
