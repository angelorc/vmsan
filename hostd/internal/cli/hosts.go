package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/angelorc/vmsan/hostd/internal/gwclient"
	"github.com/angelorc/vmsan/hostd/internal/output"
	"github.com/angelorc/vmsan/hostd/internal/serverclient"
	"github.com/spf13/cobra"
)

var hostsCmd = &cobra.Command{
	Use:   "hosts",
	Short: "Manage remote hosts in a multi-host cluster",
}

var hostsAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Register a new host with the control plane",
	RunE:  runHostsAdd,
}

var hostsListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List registered hosts",
	RunE:    runHostsList,
}

var hostsRemoveCmd = &cobra.Command{
	Use:     "remove <hostId>",
	Aliases: []string{"rm"},
	Short:   "Remove a host from the cluster",
	Args:    cobra.ExactArgs(1),
	RunE:    runHostsRemove,
}

var hostsCheckCmd = &cobra.Command{
	Use:   "check [hostName]",
	Short: "Health check hosts",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runHostsCheck,
}

func init() {
	f := hostsAddCmd.Flags()
	f.String("server", "", "Control plane server address (required)")
	f.String("token", "", "Authentication token (required)")
	f.String("name", "", "Host display name")
	_ = hostsAddCmd.MarkFlagRequired("server")
	_ = hostsAddCmd.MarkFlagRequired("token")

	rf := hostsRemoveCmd.Flags()
	rf.String("server", "", "Control plane server address (required)")
	rf.String("token", "", "Authentication token")
	_ = hostsRemoveCmd.MarkFlagRequired("server")

	hostsCmd.AddCommand(hostsAddCmd)
	hostsCmd.AddCommand(hostsListCmd)
	hostsCmd.AddCommand(hostsRemoveCmd)
	hostsCmd.AddCommand(hostsCheckCmd)
	rootCmd.AddCommand(hostsCmd)
}

func runHostsAdd(cmd *cobra.Command, _ []string) error {
	server, _ := cmd.Flags().GetString("server")
	token, _ := cmd.Flags().GetString("token")
	name, _ := cmd.Flags().GetString("name")

	gw, err := gwclient.New()
	if err != nil {
		return fmt.Errorf("gateway connection failed: %w", err)
	}
	defer gw.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := gw.Health(ctx)
	if err != nil {
		return fmt.Errorf("local gateway health check failed: %w", err)
	}

	if jsonOutput {
		data, _ := json.MarshalIndent(map[string]any{
			"server":  server,
			"name":    name,
			"version": resp.Version,
			"vms":     resp.Vms,
		}, "", "  ")
		fmt.Println(string(data))
	} else {
		fmt.Printf("  Host registered with server %s\n", server)
		if len(token) > 8 {
			fmt.Printf("  Token: %s...%s\n", token[:4], token[len(token)-4:])
		}
		if name != "" {
			fmt.Printf("  Name: %s\n", name)
		}
	}
	return nil
}

func runHostsList(_ *cobra.Command, _ []string) error {
	gw, err := gwclient.New()
	if err != nil {
		return fmt.Errorf("gateway connection failed: %w", err)
	}
	defer gw.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := gw.Status(ctx)
	if err != nil {
		return fmt.Errorf("status: %w", err)
	}

	if jsonOutput {
		data, _ := json.MarshalIndent(map[string]any{
			"vmCount": resp.Vms,
			"vms":     len(resp.List),
		}, "", "  ")
		fmt.Println(string(data))
	} else {
		fmt.Printf("  VMs: %d\n", resp.Vms)
	}
	return nil
}

func runHostsRemove(cmd *cobra.Command, args []string) error {
	hostID := args[0]
	serverAddr, _ := cmd.Flags().GetString("server")
	token, _ := cmd.Flags().GetString("token")

	client := serverclient.New(serverAddr, token)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := client.RemoveHost(ctx, hostID); err != nil {
		return fmt.Errorf("remove host: %w", err)
	}

	fmt.Printf("  Host %s removed\n", hostID)
	return nil
}

func runHostsCheck(_ *cobra.Command, _ []string) error {
	gw, err := gwclient.New()
	if err != nil {
		return fmt.Errorf("gateway connection failed: %w", err)
	}
	defer gw.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := gw.Doctor(ctx)
	if err != nil {
		return fmt.Errorf("doctor: %w", err)
	}

	if jsonOutput {
		data, _ := json.MarshalIndent(resp, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	headers := []string{"CHECK", "STATUS", "DETAILS"}
	var rows [][]string
	for _, check := range resp.Checks {
		status := output.GreenText("pass")
		if check.Status != "pass" {
			status = output.RedText(check.Status)
		}
		rows = append(rows, []string{check.Name, status, check.Detail})
	}
	fmt.Println()
	output.PrintTable(os.Stdout, headers, rows)
	return nil
}
