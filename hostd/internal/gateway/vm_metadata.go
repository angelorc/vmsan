package gateway

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const metadataDir = "/run/vmsan/vms"

// VMMetadata is the authoritative VM state written to /run/vmsan/vms/{vmId}.json.
type VMMetadata struct {
	VMId       string  `json:"vmId"`
	Slot       int     `json:"slot"`
	Status     string  `json:"status"`              // "running", "stopped", "creating"
	HostIP     string  `json:"hostIp"`
	GuestIP    string  `json:"guestIp"`
	MeshIP     string  `json:"meshIp,omitempty"`
	PID        int     `json:"pid"`
	CreatedAt  string  `json:"createdAt"`            // ISO 8601
	TimeoutAt  string  `json:"timeoutAt,omitempty"`  // ISO 8601, empty if no timeout
	AgentToken string  `json:"agentToken,omitempty"`
	Runtime    string  `json:"runtime"`
	VCPUs      int     `json:"vcpus"`
	MemMiB     int     `json:"memMib"`
	DiskSizeGb float64 `json:"diskSizeGb"`
	Project    string  `json:"project,omitempty"`
	Service    string  `json:"service,omitempty"`
	Network    VMNetworkMeta `json:"network"`
	ChrootDir  string  `json:"chrootDir"`
	SocketPath string  `json:"socketPath"`
	TAPDevice  string  `json:"tapDevice"`
	MACAddress string  `json:"macAddress"`
	NetNSName  string  `json:"netnsName"`
	VethHost   string  `json:"vethHost"`
	VethGuest  string  `json:"vethGuest"`
	SubnetMask string  `json:"subnetMask"`
	DNSPort    int     `json:"dnsPort"`
	SNIPort    int     `json:"sniPort"`
	HTTPPort   int     `json:"httpPort"`
}

// VMNetworkMeta holds the per-VM network policy metadata.
type VMNetworkMeta struct {
	Policy        string   `json:"policy"`
	Domains       []string `json:"domains,omitempty"`
	AllowedCIDRs  []string `json:"allowedCidrs,omitempty"`
	DeniedCIDRs   []string `json:"deniedCidrs,omitempty"`
	Ports         []int    `json:"ports,omitempty"`
	BandwidthMbit int      `json:"bandwidthMbit,omitempty"`
	AllowICMP     bool     `json:"allowIcmp,omitempty"`
}

// writeVMMetadata writes VM metadata JSON to /run/vmsan/vms/{vmId}.json.
func writeVMMetadata(meta *VMMetadata) error {
	if err := os.MkdirAll(metadataDir, 0755); err != nil {
		return fmt.Errorf("create metadata dir: %w", err)
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	path := filepath.Join(metadataDir, meta.VMId+".json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write metadata file: %w", err)
	}
	return nil
}

// readVMMetadata reads a single VM's metadata from /run/vmsan/vms/{vmId}.json.
func readVMMetadata(vmId string) (*VMMetadata, error) {
	path := filepath.Join(metadataDir, vmId+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read metadata: %w", err)
	}
	var meta VMMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("unmarshal metadata: %w", err)
	}
	return &meta, nil
}

// listVMMetadata reads all VM metadata files from the metadata directory.
func listVMMetadata() ([]*VMMetadata, error) {
	entries, err := os.ReadDir(metadataDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read metadata dir: %w", err)
	}
	var metas []*VMMetadata
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(metadataDir, entry.Name()))
		if err != nil {
			continue
		}
		var meta VMMetadata
		if err := json.Unmarshal(data, &meta); err != nil {
			continue
		}
		metas = append(metas, &meta)
	}
	return metas, nil
}

// deleteVMMetadata removes the metadata file for a VM.
func deleteVMMetadata(vmId string) error {
	path := filepath.Join(metadataDir, vmId+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove metadata: %w", err)
	}
	return nil
}

// updateVMMetadataFields performs a read-modify-write on VM metadata.
func updateVMMetadataFields(vmId string, updates func(*VMMetadata)) error {
	meta, err := readVMMetadata(vmId)
	if err != nil {
		return err
	}
	updates(meta)
	return writeVMMetadata(meta)
}
