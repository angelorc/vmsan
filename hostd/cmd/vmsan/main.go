package main

import (
	"os"

	"github.com/angelorc/vmsan/hostd/internal/cli"
)

var version = "dev"

func main() {
	// For now, just run the CLI. Gateway/server dispatch will be added in Phase 4.
	cli.SetVersion(version)
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
