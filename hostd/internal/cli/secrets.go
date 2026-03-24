package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/angelorc/vmsan/hostd/internal/config"
	"github.com/angelorc/vmsan/hostd/internal/output"
	"github.com/angelorc/vmsan/hostd/internal/paths"
	"github.com/angelorc/vmsan/hostd/internal/secrets"
	"github.com/spf13/cobra"
)

var secretsCmd = &cobra.Command{
	Use:   "secrets",
	Short: "Manage encrypted project secrets",
}

var secretsSetCmd = &cobra.Command{
	Use:   "set <KEY=VALUE...>",
	Short: "Set one or more secrets",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runSecretsSet,
}

var secretsListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List secret names",
	RunE:    runSecretsList,
}

var secretsUnsetCmd = &cobra.Command{
	Use:   "unset <KEY...>",
	Short: "Remove one or more secrets",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runSecretsUnset,
}

func init() {
	for _, cmd := range []*cobra.Command{secretsSetCmd, secretsListCmd, secretsUnsetCmd} {
		cmd.Flags().String("config", "vmsan.toml", "Path to vmsan.toml")
	}
	secretsCmd.AddCommand(secretsSetCmd)
	secretsCmd.AddCommand(secretsListCmd)
	secretsCmd.AddCommand(secretsUnsetCmd)
	rootCmd.AddCommand(secretsCmd)
}

func resolveProject(cmd *cobra.Command) (string, error) {
	cfgPath, _ := cmd.Flags().GetString("config")
	cfg, err := config.LoadVmsanToml(cfgPath)
	if err != nil {
		return "", fmt.Errorf("load config: %w (needed to determine project name)", err)
	}
	if cfg.Project == "" {
		return "", fmt.Errorf("no project name in %s; add project = \"name\" to your vmsan.toml", cfgPath)
	}
	return cfg.Project, nil
}

func runSecretsSet(cmd *cobra.Command, args []string) error {
	project, err := resolveProject(cmd)
	if err != nil {
		return err
	}

	p := paths.Resolve()
	store := secrets.NewStore(p.BaseDir)

	for _, arg := range args {
		idx := strings.IndexByte(arg, '=')
		if idx < 1 {
			return fmt.Errorf("invalid secret format %q (expected KEY=VALUE)", arg)
		}
		key := arg[:idx]
		value := arg[idx+1:]

		if err := store.Set(project, key, value); err != nil {
			return fmt.Errorf("set %s: %w", key, err)
		}
		fmt.Printf("  Set %s\n", key)
	}
	return nil
}

func runSecretsList(cmd *cobra.Command, _ []string) error {
	project, err := resolveProject(cmd)
	if err != nil {
		return err
	}

	p := paths.Resolve()
	store := secrets.NewStore(p.BaseDir)

	keys, err := store.List(project)
	if err != nil {
		return fmt.Errorf("list secrets: %w", err)
	}

	if len(keys) == 0 {
		fmt.Println("  No secrets set for this project.")
		return nil
	}

	sort.Strings(keys)

	if jsonOutput {
		data, _ := json.MarshalIndent(keys, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	headers := []string{"KEY"}
	var rows [][]string
	for _, k := range keys {
		rows = append(rows, []string{k})
	}
	output.PrintTable(os.Stdout, headers, rows)
	return nil
}

func runSecretsUnset(cmd *cobra.Command, args []string) error {
	project, err := resolveProject(cmd)
	if err != nil {
		return err
	}

	p := paths.Resolve()
	store := secrets.NewStore(p.BaseDir)

	for _, key := range args {
		removed, err := store.Unset(project, key)
		if err != nil {
			return fmt.Errorf("unset %s: %w", key, err)
		}
		if removed {
			fmt.Printf("  Removed %s\n", key)
		} else {
			fmt.Printf("  %s not found\n", key)
		}
	}
	return nil
}
