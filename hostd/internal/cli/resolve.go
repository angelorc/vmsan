package cli

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/angelorc/vmsan/hostd/internal/agentclient"
	"github.com/angelorc/vmsan/hostd/internal/paths"
	"github.com/angelorc/vmsan/hostd/internal/vmstate"
)

// resolvedVM bundles the state and a ready-to-use agent client.
type resolvedVM struct {
	State  *vmstate.VmState
	Client *agentclient.Client
}

// resolveVM loads VM state and creates an agent client.
// It returns an error if the VM is not found or not running.
func resolveVM(vmID string) (*resolvedVM, error) {
	p := paths.Resolve()
	store := vmstate.NewStore(p.VmsDir)

	state, err := store.Load(vmID)
	if err != nil {
		return nil, fmt.Errorf("VM %s not found", vmID)
	}
	if state.Status != "running" {
		return nil, fmt.Errorf("VM %s is not running (status: %s)", vmID, state.Status)
	}

	token := ""
	if state.AgentToken != nil {
		token = *state.AgentToken
	}

	client := agentclient.New(
		fmt.Sprintf("http://%s:%d", state.Network.GuestIP, state.AgentPort),
		token,
	)

	return &resolvedVM{State: state, Client: client}, nil
}

// waitForAgent polls the agent health endpoint until it responds or the
// context is cancelled. Retries with 500ms backoff, up to ~30s by default.
func waitForAgent(ctx context.Context, guestIP string, port int) error {
	deadline := time.After(30 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	addr := net.JoinHostPort(guestIP, fmt.Sprintf("%d", port))

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("agent on %s did not become ready within 30s", addr)
		case <-ticker.C:
			conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
			if err == nil {
				conn.Close()
				return nil
			}
		}
	}
}
