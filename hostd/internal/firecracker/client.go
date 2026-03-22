// Package firecracker provides an HTTP client for the Firecracker VMM API
// over a Unix socket.
package firecracker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"
)

// Client communicates with a Firecracker instance over its Unix socket API.
type Client struct {
	socketPath string
	http       *http.Client
}

// NewClient creates a Firecracker API client.
func NewClient(socketPath string) *Client {
	return &Client{
		socketPath: socketPath,
		http: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return net.Dial("unix", socketPath)
				},
			},
			Timeout: 30 * time.Second,
		},
	}
}

// MachineConfig holds the Firecracker machine configuration.
type MachineConfig struct {
	VCPUs  int `json:"vcpu_count"`
	MemMiB int `json:"mem_size_mib"`
}

// Configure sends the machine configuration.
func (c *Client) Configure(cfg MachineConfig) error {
	return c.put("/machine-config", map[string]any{
		"vcpu_count":        cfg.VCPUs,
		"mem_size_mib":      cfg.MemMiB,
		"smt":               false,
		"track_dirty_pages": false,
	})
}

// Boot sets the boot source (kernel + boot args).
func (c *Client) Boot(kernelPath, bootArgs string) error {
	return c.put("/boot-source", map[string]any{
		"kernel_image_path": kernelPath,
		"boot_args":         bootArgs,
	})
}

// AddDrive adds a block device (rootfs).
func (c *Client) AddDrive(driveId, pathOnHost string, isRoot, isReadOnly bool) error {
	return c.put("/drives/"+driveId, map[string]any{
		"drive_id":       driveId,
		"path_on_host":   pathOnHost,
		"is_root_device": isRoot,
		"is_read_only":   isReadOnly,
		"cache_type":     "Unsafe",
		"io_engine":      "Sync",
	})
}

// AddNetwork adds a network interface.
func (c *Client) AddNetwork(ifaceId, hostDevName, guestMAC string) error {
	return c.put("/network-interfaces/"+ifaceId, map[string]any{
		"iface_id":      ifaceId,
		"host_dev_name": hostDevName,
		"guest_mac":     guestMAC,
	})
}

// Start issues the InstanceStart action.
func (c *Client) Start() error {
	return c.put("/actions", map[string]any{
		"action_type": "InstanceStart",
	})
}

// Stop issues a graceful shutdown (SendCtrlAltDel).
func (c *Client) Stop() error {
	return c.put("/actions", map[string]any{
		"action_type": "SendCtrlAltDel",
	})
}

// Snapshot creates a VM snapshot.
func (c *Client) Snapshot(snapshotPath, memPath string) error {
	return c.put("/snapshot/create", map[string]any{
		"snapshot_path": snapshotPath,
		"mem_file_path": memPath,
		"snapshot_type": "Full",
	})
}

// LoadSnapshot loads a snapshot.
func (c *Client) LoadSnapshot(snapshotPath, memPath string) error {
	return c.put("/snapshot/load", map[string]any{
		"snapshot_path": snapshotPath,
		"mem_file_path": memPath,
	})
}

// Pause pauses the VM.
func (c *Client) Pause() error {
	return c.patch("/vm", map[string]any{"state": "Paused"})
}

// Resume resumes a paused VM.
func (c *Client) Resume() error {
	return c.patch("/vm", map[string]any{"state": "Resumed"})
}

// WaitForSocket polls until the Firecracker socket is available.
func (c *Client) WaitForSocket(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if _, err := os.Stat(c.socketPath); err == nil {
			conn, err := net.DialTimeout("unix", c.socketPath, time.Second)
			if err == nil {
				conn.Close()
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("firecracker socket %s not available after %s", c.socketPath, timeout)
}

// put sends a PUT request.
func (c *Client) put(path string, body any) error {
	return c.do("PUT", path, body)
}

// patch sends a PATCH request.
func (c *Client) patch(path string, body any) error {
	return c.do("PATCH", path, body)
}

// do sends an HTTP request to the Firecracker API.
func (c *Client) do(method, path string, body any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request body: %w", err)
	}

	req, err := http.NewRequest(method, "http://localhost"+path, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("firecracker API %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("firecracker API %s %s: status %d: %s", method, path, resp.StatusCode, string(respBody))
	}

	return nil
}
