package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	vmsanv1 "github.com/angelorc/vmsan/hostd/gen/vmsan/v1"
	"github.com/angelorc/vmsan/hostd/internal/config"
	"github.com/angelorc/vmsan/hostd/internal/gwclient"
	"github.com/angelorc/vmsan/hostd/internal/output"
	"github.com/angelorc/vmsan/hostd/internal/paths"
	"github.com/angelorc/vmsan/hostd/internal/vmstate"
	"github.com/spf13/cobra"
)

var networkCmd = &cobra.Command{
	Use:   "network",
	Short: "Manage VM network settings",
}

var networkUpdateCmd = &cobra.Command{
	Use:   "update <vmId>",
	Short: "Update network policy for a VM",
	Args:  cobra.ExactArgs(1),
	RunE:  runNetworkUpdate,
}

var networkConnectionsCmd = &cobra.Command{
	Use:   "connections",
	Short: "Show mesh network connections for current project",
	RunE:  runNetworkConnections,
}

func init() {
	f := networkUpdateCmd.Flags()
	f.String("policy", "", "Network policy (allow-all, deny-all, custom)")
	f.String("domains", "", "Comma-separated allowed domains")
	f.String("cidrs", "", "Comma-separated allowed CIDRs")
	f.Bool("allow-icmp", false, "Allow ICMP traffic")

	networkConnectionsCmd.Flags().String("config", "vmsan.toml", "Path to vmsan.toml")

	networkCmd.AddCommand(networkUpdateCmd)
	networkCmd.AddCommand(networkConnectionsCmd)
	rootCmd.AddCommand(networkCmd)
}

func runNetworkUpdate(cmd *cobra.Command, args []string) error {
	vmID := args[0]

	policy, _ := cmd.Flags().GetString("policy")
	domainsStr, _ := cmd.Flags().GetString("domains")
	cidrsStr, _ := cmd.Flags().GetString("cidrs")
	allowIcmp, _ := cmd.Flags().GetBool("allow-icmp")

	domains := splitCSV(domainsStr)
	cidrs := splitCSV(cidrsStr)

	gw, err := gwclient.New()
	if err != nil {
		return fmt.Errorf("gateway connection failed: %w", err)
	}
	defer gw.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Use full update policy — replaces the entire policy
	_, err = gw.FullUpdatePolicy(ctx, &vmsanv1.FullUpdatePolicyRequest{
		VmId:         vmID,
		Policy:       policy,
		Domains:      domains,
		AllowedCidrs: cidrs,
		AllowIcmp:    allowIcmp,
	})
	if err != nil {
		return fmt.Errorf("update network policy: %w", err)
	}

	// Update local state
	p := paths.Resolve()
	store := vmstate.NewStore(p.VmsDir)
	_ = store.Update(vmID, func(s *vmstate.VmState) {
		if policy != "" {
			s.Network.NetworkPolicy = policy
		}
		s.Network.AllowedDomains = domains
		s.Network.AllowedCidrs = cidrs
		s.Network.AllowIcmp = allowIcmp
	})

	fmt.Printf("  Network policy updated for %s\n", vmID)
	return nil
}

func runNetworkConnections(cmd *cobra.Command, _ []string) error {
	cfgPath, _ := cmd.Flags().GetString("config")

	cfg, err := config.LoadVmsanToml(cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	p := paths.Resolve()
	store := vmstate.NewStore(p.VmsDir)
	allVMs, err := store.List()
	if err != nil {
		return fmt.Errorf("list VMs: %w", err)
	}

	services := config.NormalizeToml(cfg)

	type connection struct {
		From   string `json:"from"`
		To     string `json:"to"`
		Port   string `json:"port"`
		MeshIP string `json:"meshIp"`
		Status string `json:"status"`
	}
	var connections []connection

	for svcName, svcCfg := range services {
		for _, ct := range svcCfg.ConnectTo {
			parts := strings.SplitN(ct, ":", 2)
			target := parts[0]
			port := ""
			if len(parts) == 2 {
				port = parts[1]
			}

			meshIP := ""
			status := "not deployed"
			vm := findProjectVM(allVMs, cfg.Project, target)
			if vm != nil {
				meshIP = vm.Network.MeshIP
				status = vm.Status
			}

			connections = append(connections, connection{
				From:   svcName,
				To:     target,
				Port:   port,
				MeshIP: meshIP,
				Status: status,
			})
		}
	}

	if jsonOutput {
		data, _ := json.MarshalIndent(connections, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	if len(connections) == 0 {
		fmt.Println("  No mesh connections configured.")
		return nil
	}

	headers := []string{"FROM", "TO", "PORT", "MESH IP", "STATUS"}
	var rows [][]string
	for _, c := range connections {
		meshIP := c.MeshIP
		if meshIP == "" {
			meshIP = "-"
		}
		port := c.Port
		if port == "" {
			port = "-"
		}
		rows = append(rows, []string{c.From, c.To, port, meshIP, output.StatusColor(c.Status)})
	}
	fmt.Println()
	output.PrintTable(os.Stdout, headers, rows)
	return nil
}

func appendUnique(existing, additions []string) []string {
	set := make(map[string]bool, len(existing))
	for _, v := range existing {
		set[v] = true
	}
	for _, v := range additions {
		if !set[v] {
			existing = append(existing, v)
			set[v] = true
		}
	}
	return existing
}
