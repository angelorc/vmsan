package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrintTable(t *testing.T) {
	var buf bytes.Buffer
	headers := []string{"NAME", "AGE"}
	rows := [][]string{
		{"alice", "30"},
		{"bob", "25"},
	}

	PrintTable(&buf, headers, rows)
	output := buf.String()

	// Verify header is present.
	if !strings.Contains(output, "NAME") {
		t.Error("output should contain header NAME")
	}
	if !strings.Contains(output, "AGE") {
		t.Error("output should contain header AGE")
	}

	// Verify rows are present.
	if !strings.Contains(output, "alice") {
		t.Error("output should contain alice")
	}
	if !strings.Contains(output, "bob") {
		t.Error("output should contain bob")
	}
	if !strings.Contains(output, "30") {
		t.Error("output should contain 30")
	}
	if !strings.Contains(output, "25") {
		t.Error("output should contain 25")
	}

	// Verify separator line exists (uses unicode box-drawing character).
	lines := strings.Split(output, "\n")
	if len(lines) < 4 {
		t.Fatalf("expected at least 4 lines (header, separator, 2 rows), got %d", len(lines))
	}
	separator := lines[1]
	if !strings.Contains(separator, "\u2500") {
		t.Errorf("separator line should contain box-drawing character, got %q", separator)
	}
}

func TestPrintTable_WideColumn(t *testing.T) {
	var buf bytes.Buffer
	headers := []string{"ID", "DESCRIPTION"}
	rows := [][]string{
		{"1", "short"},
		{"2", "a much longer description value"},
	}

	PrintTable(&buf, headers, rows)
	output := buf.String()

	// The longer row value should be present and aligned.
	if !strings.Contains(output, "a much longer description value") {
		t.Error("output should contain long description")
	}
}

func TestPrintTable_EmptyRows(t *testing.T) {
	var buf bytes.Buffer
	headers := []string{"NAME", "VALUE"}

	PrintTable(&buf, headers, nil)
	output := buf.String()

	// Should still have header and separator.
	if !strings.Contains(output, "NAME") {
		t.Error("output should contain header NAME")
	}
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 lines (header + separator), got %d", len(lines))
	}
}

func TestPrintTable_AnsiColors(t *testing.T) {
	var buf bytes.Buffer
	headers := []string{"STATUS", "NAME"}
	rows := [][]string{
		{"\033[32mrunning\033[0m", "vm-1"},
		{"stopped", "vm-2"},
	}

	PrintTable(&buf, headers, rows)
	output := buf.String()

	// The ANSI-colored value should be present.
	if !strings.Contains(output, "running") {
		t.Error("output should contain running")
	}
	// Columns should be aligned properly despite ANSI codes.
	// Both vm-1 and vm-2 should start at the same column offset.
	lines := strings.Split(output, "\n")
	if len(lines) < 4 {
		t.Fatalf("expected at least 4 lines, got %d", len(lines))
	}
	// The name column positions in the stripped version should be consistent.
	stripped2 := stripAnsi(lines[2]) // first data row
	stripped3 := stripAnsi(lines[3]) // second data row
	idx1 := strings.Index(stripped2, "vm-1")
	idx2 := strings.Index(stripped3, "vm-2")
	if idx1 != idx2 {
		t.Errorf("columns not aligned: vm-1 at %d, vm-2 at %d", idx1, idx2)
	}
}

func TestStripAnsi(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"red text", "\033[31mred\033[0m", "red"},
		{"plain text", "plain", "plain"},
		{"empty string", "", ""},
		{"bold green", "\033[1m\033[32mbold green\033[0m", "bold green"},
		{"multiple colors", "\033[31mred\033[0m and \033[32mgreen\033[0m", "red and green"},
		{"gray text", "\033[90mgray\033[0m", "gray"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripAnsi(tt.input)
			if got != tt.want {
				t.Errorf("stripAnsi(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
