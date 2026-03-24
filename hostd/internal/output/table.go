package output

import (
	"fmt"
	"io"
	"strings"
)

// PrintTable renders a simple aligned table to w.
func PrintTable(w io.Writer, headers []string, rows [][]string) {
	// Calculate column widths
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && len(stripAnsi(cell)) > widths[i] {
				widths[i] = len(stripAnsi(cell))
			}
		}
	}

	// Print header
	for i, h := range headers {
		fmt.Fprintf(w, "%-*s  ", widths[i], h)
	}
	fmt.Fprintln(w)

	// Print separator
	for i, width := range widths {
		fmt.Fprint(w, strings.Repeat("\u2500", width))
		if i < len(widths)-1 {
			fmt.Fprint(w, "  ")
		}
	}
	fmt.Fprintln(w)

	// Print rows
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) {
				// Account for ANSI escape codes in width calculation
				padding := widths[i] - len(stripAnsi(cell))
				if padding < 0 {
					padding = 0
				}
				fmt.Fprintf(w, "%s%s  ", cell, strings.Repeat(" ", padding))
			}
		}
		fmt.Fprintln(w)
	}
}

// stripAnsi removes ANSI escape sequences for width calculation.
func stripAnsi(s string) string {
	var result strings.Builder
	inEscape := false
	for _, r := range s {
		if r == '\033' {
			inEscape = true
			continue
		}
		if inEscape {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}
		result.WriteRune(r)
	}
	return result.String()
}
