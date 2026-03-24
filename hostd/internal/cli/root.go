package cli

import (
	"github.com/spf13/cobra"
)

var (
	jsonOutput bool
	verbose    bool
	cliVersion string
)

// SetVersion sets the CLI version string shown by --version.
func SetVersion(v string) { cliVersion = v }

var rootCmd = &cobra.Command{
	Use:   "vmsan",
	Short: "Firecracker microVM sandbox toolkit",
	// No Run — shows help by default
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output structured JSON")
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "Show detailed debug output")
}

// Execute runs the root command.
func Execute() error {
	rootCmd.Version = cliVersion
	return rootCmd.Execute()
}
