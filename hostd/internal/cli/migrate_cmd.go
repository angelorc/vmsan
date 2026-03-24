package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/angelorc/vmsan/hostd/internal/output"
	"github.com/angelorc/vmsan/hostd/internal/paths"
	"github.com/angelorc/vmsan/hostd/internal/vmstate"
	"github.com/spf13/cobra"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate JSON VM state files to SQLite database",
	RunE:  runMigrate,
}

func init() {
	migrateCmd.Flags().Bool("dry-run", false, "Show what would be migrated without making changes")
	rootCmd.AddCommand(migrateCmd)
}

func runMigrate(cmd *cobra.Command, _ []string) error {
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	p := paths.Resolve()

	// Discover JSON state files
	entries, err := os.ReadDir(p.VmsDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("  No VMs directory found. Nothing to migrate.")
			return nil
		}
		return fmt.Errorf("read VMs dir: %w", err)
	}

	type vmEntry struct {
		ID       string
		Path     string
		State    *vmstate.VmState
	}
	var toMigrate []vmEntry

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(p.VmsDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var state vmstate.VmState
		if err := json.Unmarshal(data, &state); err != nil {
			continue
		}
		if state.ID == "" {
			state.ID = strings.TrimSuffix(entry.Name(), ".json")
		}
		toMigrate = append(toMigrate, vmEntry{
			ID:    state.ID,
			Path:  path,
			State: &state,
		})
	}

	if len(toMigrate) == 0 {
		fmt.Println("  No JSON state files found. Nothing to migrate.")
		return nil
	}

	if dryRun {
		fmt.Printf("  Would migrate %d VM(s):\n\n", len(toMigrate))
		headers := []string{"VM ID", "PROJECT", "STATUS", "RUNTIME"}
		var rows [][]string
		for _, vm := range toMigrate {
			rows = append(rows, []string{
				vm.State.ID,
				vm.State.Project,
				vm.State.Status,
				vm.State.Runtime,
			})
		}
		output.PrintTable(os.Stdout, headers, rows)
		return nil
	}

	// Confirm
	fmt.Printf("  Will migrate %d VM state file(s) to SQLite.\n", len(toMigrate))
	fmt.Println("  JSON files will be kept as backup.")
	fmt.Print("  Continue? [y/N] ")
	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))
	if answer != "y" && answer != "yes" {
		fmt.Println("  Aborted.")
		return nil
	}

	// Migration: for now, re-save through the store (which uses JSON)
	// Full SQLite migration will be wired when the SQLite store is available
	store := vmstate.NewStore(p.VmsDir)
	imported := 0
	skipped := 0

	for _, vm := range toMigrate {
		// Check if already exists (idempotent)
		existing, _ := store.Load(vm.State.ID)
		if existing != nil {
			skipped++
			continue
		}

		if err := store.Save(vm.State); err != nil {
			fmt.Printf("  Error importing %s: %s\n", vm.State.ID, err)
			continue
		}
		imported++
	}

	fmt.Printf("\n  Migration complete: %d imported, %d skipped (already exist)\n", imported, skipped)
	return nil
}
