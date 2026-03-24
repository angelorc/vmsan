package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/angelorc/vmsan/hostd/internal/gwclient"
	"github.com/angelorc/vmsan/hostd/internal/output"
	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check system prerequisites and vmsan installation health",
	RunE:  runDoctor,
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}

func runDoctor(_ *cobra.Command, _ []string) error {
	gw, err := gwclient.New()
	if err != nil {
		return fmt.Errorf("gateway connection failed (is the gateway running?): %w", err)
	}
	defer gw.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := gw.Doctor(ctx)
	if err != nil {
		return fmt.Errorf("doctor: %w", err)
	}

	checks := resp.Checks
	passed := 0
	failed := 0
	warned := 0
	for _, c := range checks {
		switch c.Status {
		case "pass":
			passed++
		case "fail":
			failed++
		case "warn":
			warned++
		}
	}

	if jsonOutput {
		type checkJSON struct {
			Category string `json:"category"`
			Name     string `json:"name"`
			Status   string `json:"status"`
			Detail   string `json:"detail"`
		}
		jChecks := make([]checkJSON, len(checks))
		for i, c := range checks {
			jChecks[i] = checkJSON{
				Category: c.Category,
				Name:     c.Name,
				Status:   c.Status,
				Detail:   c.Detail,
			}
		}
		data, _ := json.MarshalIndent(map[string]any{
			"checks":  jChecks,
			"summary": map[string]int{"passed": passed, "failed": failed, "total": len(checks)},
		}, "", "  ")
		fmt.Println(string(data))
		if failed > 0 {
			return fmt.Errorf("%d check(s) failed", failed)
		}
		return nil
	}

	// Human-readable output
	passStr := output.Green + "ok" + output.Reset
	failStr := output.Red + "FAIL" + output.Reset
	warnStr := output.Yellow + "WARN" + output.Reset
	if !output.IsTerminal() {
		passStr = "ok"
		failStr = "FAIL"
		warnStr = "WARN"
	}

	fmt.Println()
	fmt.Println("vmsan doctor")
	fmt.Println()

	currentCategory := ""
	for _, c := range checks {
		if c.Category != currentCategory {
			if currentCategory != "" {
				fmt.Println()
			}
			fmt.Printf("  %s\n", c.Category)
			currentCategory = c.Category
		}

		dots := strings.Repeat(".", max(1, 30-len(c.Name)))
		var statusStr string
		switch c.Status {
		case "pass":
			statusStr = passStr
		case "warn":
			statusStr = warnStr
		default:
			statusStr = failStr
		}
		fmt.Printf("    %s %s %s (%s)\n", c.Name, dots, statusStr, c.Detail)

		if c.Status != "pass" && c.Fix != "" {
			if output.IsTerminal() {
				fmt.Printf("      %sFix: %s%s\n", output.Yellow, c.Fix, output.Reset)
			} else {
				fmt.Printf("      Fix: %s\n", c.Fix)
			}
		}
	}

	fmt.Println()
	summary := fmt.Sprintf("  Result: %d passed, %d failed", passed, failed)
	if warned > 0 {
		summary += fmt.Sprintf(", %d warnings", warned)
	}
	fmt.Println(summary)
	fmt.Println()

	if failed > 0 {
		return fmt.Errorf("%d check(s) failed", failed)
	}
	return nil
}
