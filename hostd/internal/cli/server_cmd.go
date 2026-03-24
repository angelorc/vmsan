package cli

import (
	"github.com/angelorc/vmsan/hostd/internal/server"
	"github.com/spf13/cobra"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the vmsan control plane server",
	// Flags are parsed inside RunServer from os.Args[2:] via flag.FlagSet,
	// so we disable Cobra's flag parsing for this command.
	DisableFlagParsing: true,
	Run: func(cmd *cobra.Command, args []string) {
		server.RunServer()
	},
}

func init() {
	rootCmd.AddCommand(serverCmd)
}
