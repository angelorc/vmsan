package gwclient

import (
	"context"
	"fmt"

	vmsanv1 "github.com/angelorc/vmsan/hostd/gen/vmsan/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const defaultSocket = "/run/vmsan/gateway.sock"

// Client wraps a gRPC connection to the vmsan gateway.
type Client struct {
	conn    *grpc.ClientConn
	gateway vmsanv1.GatewayServiceClient
}

// New connects to the default gateway socket.
func New() (*Client, error) {
	return NewWithSocket(defaultSocket)
}

// NewWithSocket connects to a specific gateway socket path.
func NewWithSocket(socketPath string) (*Client, error) {
	conn, err := grpc.NewClient(
		"unix://"+socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("connect to gateway: %w", err)
	}
	return &Client{
		conn:    conn,
		gateway: vmsanv1.NewGatewayServiceClient(conn),
	}, nil
}

// Close releases the underlying gRPC connection.
func (c *Client) Close() error {
	return c.conn.Close()
}

// --- System RPCs ---

func (c *Client) Ping(ctx context.Context) (*vmsanv1.PingResponse, error) {
	return c.gateway.Ping(ctx, &vmsanv1.PingRequest{})
}

func (c *Client) Health(ctx context.Context) (*vmsanv1.HealthResponse, error) {
	return c.gateway.Health(ctx, &vmsanv1.HealthRequest{})
}

func (c *Client) Status(ctx context.Context) (*vmsanv1.StatusResponse, error) {
	return c.gateway.Status(ctx, &vmsanv1.StatusRequest{})
}

func (c *Client) Shutdown(ctx context.Context) (*vmsanv1.ShutdownResponse, error) {
	return c.gateway.Shutdown(ctx, &vmsanv1.ShutdownRequest{})
}

func (c *Client) Doctor(ctx context.Context) (*vmsanv1.DoctorResponse, error) {
	return c.gateway.Doctor(ctx, &vmsanv1.DoctorRequest{})
}

// --- VM lifecycle RPCs ---

func (c *Client) CreateVM(ctx context.Context, req *vmsanv1.CreateVMRequest) (*vmsanv1.CreateVMResponse, error) {
	return c.gateway.CreateVM(ctx, req)
}

func (c *Client) DeleteVM(ctx context.Context, req *vmsanv1.DeleteVMRequest) (*vmsanv1.DeleteVMResponse, error) {
	return c.gateway.DeleteVM(ctx, req)
}

func (c *Client) RestartVM(ctx context.Context, req *vmsanv1.RestartVMRequest) (*vmsanv1.RestartVMResponse, error) {
	return c.gateway.RestartVM(ctx, req)
}

func (c *Client) StartVM(ctx context.Context, req *vmsanv1.StartVMRequest) (*vmsanv1.StartVMResponse, error) {
	return c.gateway.StartVM(ctx, req)
}

func (c *Client) StopVM(ctx context.Context, req *vmsanv1.StopVMRequest) (*vmsanv1.StopVMResponse, error) {
	return c.gateway.StopVM(ctx, req)
}

func (c *Client) FullStopVM(ctx context.Context, req *vmsanv1.FullStopVMRequest) (*vmsanv1.FullStopVMResponse, error) {
	return c.gateway.FullStopVM(ctx, req)
}

func (c *Client) GetVM(ctx context.Context, req *vmsanv1.GetVMRequest) (*vmsanv1.GetVMResponse, error) {
	return c.gateway.GetVM(ctx, req)
}

func (c *Client) ExtendTimeout(ctx context.Context, req *vmsanv1.ExtendTimeoutRequest) (*vmsanv1.ExtendTimeoutResponse, error) {
	return c.gateway.ExtendTimeout(ctx, req)
}

// --- Network policy RPCs ---

func (c *Client) UpdatePolicy(ctx context.Context, req *vmsanv1.UpdatePolicyRequest) (*vmsanv1.UpdatePolicyResponse, error) {
	return c.gateway.UpdatePolicy(ctx, req)
}

func (c *Client) FullUpdatePolicy(ctx context.Context, req *vmsanv1.FullUpdatePolicyRequest) (*vmsanv1.FullUpdatePolicyResponse, error) {
	return c.gateway.FullUpdatePolicy(ctx, req)
}

// --- Snapshot RPCs ---

func (c *Client) CreateSnapshot(ctx context.Context, req *vmsanv1.CreateSnapshotRequest) (*vmsanv1.CreateSnapshotResponse, error) {
	return c.gateway.CreateSnapshot(ctx, req)
}

// --- Network setup/teardown RPCs ---

func (c *Client) SetupNetwork(ctx context.Context, req *vmsanv1.SetupNetworkRequest) (*vmsanv1.SetupNetworkResponse, error) {
	return c.gateway.SetupNetwork(ctx, req)
}

func (c *Client) TeardownNetwork(ctx context.Context, req *vmsanv1.TeardownNetworkRequest) (*vmsanv1.TeardownNetworkResponse, error) {
	return c.gateway.TeardownNetwork(ctx, req)
}

// --- Rootfs RPCs ---

func (c *Client) BuildRootfs(ctx context.Context, req *vmsanv1.BuildRootfsRequest) (*vmsanv1.BuildRootfsResponse, error) {
	return c.gateway.BuildRootfs(ctx, req)
}

func (c *Client) DownloadRootfs(ctx context.Context, req *vmsanv1.DownloadRootfsRequest) (*vmsanv1.DownloadRootfsResponse, error) {
	return c.gateway.DownloadRootfs(ctx, req)
}

// --- Cloudflare RPCs ---

func (c *Client) CloudflareSetup(ctx context.Context, req *vmsanv1.CloudflareSetupRequest) (*vmsanv1.CloudflareSetupResponse, error) {
	return c.gateway.CloudflareSetup(ctx, req)
}

func (c *Client) CloudflareAddRoute(ctx context.Context, req *vmsanv1.CloudflareAddRouteRequest) (*vmsanv1.CloudflareAddRouteResponse, error) {
	return c.gateway.CloudflareAddRoute(ctx, req)
}

func (c *Client) CloudflareRemoveRoute(ctx context.Context, req *vmsanv1.CloudflareRemoveRouteRequest) (*vmsanv1.CloudflareRemoveRouteResponse, error) {
	return c.gateway.CloudflareRemoveRoute(ctx, req)
}

func (c *Client) CloudflareStatus(ctx context.Context) (*vmsanv1.CloudflareStatusResponse, error) {
	return c.gateway.CloudflareStatus(ctx, &vmsanv1.CloudflareStatusRequest{})
}
