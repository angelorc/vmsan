package cli

import (
	"github.com/angelorc/vmsan/hostd/internal/agent"
	"github.com/spf13/cobra"
)

var agentHostCmd = &cobra.Command{
	Use:   "agent-host",
	Short: "Manage agent-host for multi-host clusters",
	// When invoked without a subcommand, run the agent in default mode.
	// Flags/subcommands are parsed inside RunAgentHost from os.Args[2:].
	DisableFlagParsing: true,
	Run: func(cmd *cobra.Command, args []string) {
		agent.RunAgentHost()
	},
}

func init() {
	rootCmd.AddCommand(agentHostCmd)
}
