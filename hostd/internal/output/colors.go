package output

import (
	"os"

	"golang.org/x/term"
)

// ANSI color codes.
const (
	Reset  = "\033[0m"
	Red    = "\033[31m"
	Green  = "\033[32m"
	Yellow = "\033[33m"
	Cyan   = "\033[36m"
	Gray   = "\033[90m"
	Bold   = "\033[1m"
)

// IsTerminal returns true when stdout is a terminal (not piped).
func IsTerminal() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// StatusColor wraps a VM status string with ANSI color codes.
// Returns the plain string when stdout is not a terminal.
func StatusColor(status string) string {
	if !IsTerminal() {
		return status
	}
	switch status {
	case "running":
		return Green + status + Reset
	case "stopped":
		return Gray + status + Reset
	case "error", "creating":
		return Red + status + Reset
	default:
		return status
	}
}
