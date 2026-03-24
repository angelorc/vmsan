package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/angelorc/vmsan/hostd/internal/config"
	"github.com/spf13/cobra"
)

var validateCmd = &cobra.Command{
	Use:   "validate [path]",
	Short: "Validate a vmsan.toml configuration file",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runValidate,
}

func init() {
	rootCmd.AddCommand(validateCmd)
}

func runValidate(_ *cobra.Command, args []string) error {
	path := "vmsan.toml"
	if len(args) > 0 {
		path = args[0]
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	tomlText := string(data)

	// Parse
	cfg, parseErrors := config.ParseTomlSafe(tomlText)
	if len(parseErrors) > 0 {
		return printValidationResult(path, parseErrors)
	}

	// Validate
	errs := config.ValidateToml(cfg)
	return printValidationResult(path, errs)
}

func printValidationResult(path string, errs []config.ValidationError) error {
	if jsonOutput {
		result := map[string]any{
			"file":   path,
			"valid":  len(errs) == 0,
			"errors": errs,
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(data))
		if len(errs) > 0 {
			os.Exit(1)
		}
		return nil
	}

	if len(errs) == 0 {
		fmt.Printf("  %s is valid\n", path)
		return nil
	}

	fmt.Printf("  %s has %d error(s):\n\n", path, len(errs))
	for _, e := range errs {
		prefix := "  X"
		if e.Line > 0 {
			fmt.Printf("  %s [line %d] %s: %s\n", prefix, e.Line, e.Field, e.Message)
		} else {
			fmt.Printf("  %s %s: %s\n", prefix, e.Field, e.Message)
		}
		if e.Suggestion != "" {
			fmt.Printf("       Suggestion: %s\n", e.Suggestion)
		}
	}
	fmt.Println()
	os.Exit(1)
	return nil
}
