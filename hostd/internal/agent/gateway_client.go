package agent

import (
	"encoding/json"
	"fmt"
	"net"
	"time"
)

// GatewayClient communicates with the local vmsan-gateway over Unix socket.
type GatewayClient struct {
	socketPath string
}

// NewGatewayClient creates a gateway client.
func NewGatewayClient(socketPath string) *GatewayClient {
	return &GatewayClient{socketPath: socketPath}
}

// gatewayRequest is the JSON-RPC request envelope.
type gatewayRequest struct {
	Method string `json:"method"`
	Params any    `json:"params,omitempty"`
}

// gatewayResponse is the JSON-RPC response envelope.
type gatewayResponse struct {
	OK    bool            `json:"ok"`
	Error string          `json:"error,omitempty"`
	Code  string          `json:"code,omitempty"`
	VM    json.RawMessage `json:"vm,omitempty"`
}

// VMCreateParams are the params for vm.create.
type VMCreateParams struct {
	VMId          string   `json:"vmId,omitempty"`
	VCPUs         int      `json:"vcpus,omitempty"`
	MemMiB        int      `json:"memMib,omitempty"`
	Runtime       string   `json:"runtime,omitempty"`
	DiskSizeGb    float64  `json:"diskSizeGb,omitempty"`
	NetworkPolicy string   `json:"networkPolicy,omitempty"`
	Domains       []string `json:"domains,omitempty"`
	AllowedCIDRs  []string `json:"allowedCidrs,omitempty"`
	DeniedCIDRs   []string `json:"deniedCidrs,omitempty"`
	Ports         []int    `json:"ports,omitempty"`
	BandwidthMbit int      `json:"bandwidthMbit,omitempty"`
	AllowICMP     bool     `json:"allowIcmp,omitempty"`
	Project       string   `json:"project,omitempty"`
	Service       string   `json:"service,omitempty"`
	ConnectTo     []string `json:"connectTo,omitempty"`
}

// VMCreateResult is the result of vm.create.
type VMCreateResult struct {
	VMId    string `json:"vmId"`
	PID     int    `json:"pid,omitempty"`
	Slot    int    `json:"slot"`
	HostIP  string `json:"hostIp"`
	GuestIP string `json:"guestIp"`
	MeshIP  string `json:"meshIp,omitempty"`
}

// VMCreate creates a new VM via the local gateway.
func (c *GatewayClient) VMCreate(params VMCreateParams) (*VMCreateResult, error) {
	resp, err := c.send("vm.create", params, 60*time.Second)
	if err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, fmt.Errorf("vm.create failed: %s (code: %s)", resp.Error, resp.Code)
	}
	var result VMCreateResult
	if err := json.Unmarshal(resp.VM, &result); err != nil {
		return nil, fmt.Errorf("parse vm.create result: %w", err)
	}
	return &result, nil
}

// VMStop stops a VM via the local gateway.
func (c *GatewayClient) VMStop(vmId string) error {
	resp, err := c.send("vm.stop", map[string]string{"vmId": vmId}, 10*time.Second)
	if err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("vm.stop failed: %s", resp.Error)
	}
	return nil
}

// VMDelete deletes a VM via the local gateway.
func (c *GatewayClient) VMDelete(vmId string) error {
	resp, err := c.send("vm.delete", map[string]string{"vmId": vmId}, 30*time.Second)
	if err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("vm.delete failed: %s", resp.Error)
	}
	return nil
}

// VMUpdatePolicy updates VM network policy via the local gateway.
func (c *GatewayClient) VMUpdatePolicy(vmId, policy string) error {
	resp, err := c.send("vm.updatePolicy", map[string]string{
		"vmId":   vmId,
		"policy": policy,
	}, 10*time.Second)
	if err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("vm.updatePolicy failed: %s", resp.Error)
	}
	return nil
}

// Ping checks if the gateway is running.
func (c *GatewayClient) Ping() error {
	resp, err := c.send("ping", nil, 5*time.Second)
	if err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("ping failed: %s", resp.Error)
	}
	return nil
}

// send sends a JSON-RPC request and returns the response.
func (c *GatewayClient) send(method string, params any, timeout time.Duration) (*gatewayResponse, error) {
	conn, err := net.DialTimeout("unix", c.socketPath, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("connect to gateway: %w", err)
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(timeout))

	req := gatewayRequest{Method: method, Params: params}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	var resp gatewayResponse
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	return &resp, nil
}
