package cli

import (
	"github.com/angelorc/vmsan/hostd/internal/gateway"
	"github.com/spf13/cobra"
)

var gatewayCmd = &cobra.Command{
	Use:   "gateway",
	Short: "Start the vmsan gateway daemon",
	Run: func(cmd *cobra.Command, args []string) {
		gateway.RunGateway()
	},
}

func init() {
	rootCmd.AddCommand(gatewayCmd)
}
