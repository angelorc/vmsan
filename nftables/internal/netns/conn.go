package netns

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/angelorc/vmsan/nftables"
	nft "github.com/google/nftables"
)

// NetNSConn wraps an nftables connection with its namespace file descriptor
// for proper cleanup. It implements io.Closer.
type NetNSConn struct {
	*nft.Conn
	nsFD *os.File
}

// Close closes the namespace file descriptor if one was opened.
func (c *NetNSConn) Close() error {
	if c.nsFD != nil {
		return c.nsFD.Close()
	}
	return nil
}

// Compile-time check: ensure NetNSConn implements io.Closer.
var _ io.Closer = (*NetNSConn)(nil)

// NewConn returns a NetNSConn bound to the given network namespace.
// If netnsName is empty, returns a connection to the default (host) namespace.
// The caller must call Close() when done to release the namespace fd.
//
// IMPORTANT: The caller must hold runtime.LockOSThread() for the duration
// of all operations on the returned connection to prevent goroutine migration.
func NewConn(ctx context.Context, netnsName string) (*NetNSConn, error) {
	slog.DebugContext(ctx, "opening netns connection", "netns", netnsName)

	if netnsName == "" {
		conn, err := nft.New()
		if err != nil {
			slog.ErrorContext(ctx, "failed to create nftables connection", "error", err)
			return nil, &nftables.NetNSError{
				Op:    "create",
				NetNS: "default",
				Err:   err,
			}
		}
		return &NetNSConn{Conn: conn, nsFD: nil}, nil
	}

	nsPath := fmt.Sprintf("/var/run/netns/%s", netnsName)
	fd, err := os.Open(nsPath)
	if err != nil {
		slog.ErrorContext(ctx, "failed to open netns", "netns", netnsName, "path", nsPath, "error", err)
		return nil, &nftables.NetNSError{
			Op:    "enter",
			NetNS: netnsName,
			Err:   fmt.Errorf("open netns %s: %w", netnsName, err),
		}
	}

	conn, err := nft.New(nft.WithNetNSFd(int(fd.Fd())))
	if err != nil {
		fd.Close()
		slog.ErrorContext(ctx, "failed to create nftables connection in netns", "netns", netnsName, "error", err)
		return nil, &nftables.NetNSError{
			Op:    "create",
			NetNS: netnsName,
			Err:   fmt.Errorf("nftables conn in netns %s: %w", netnsName, err),
		}
	}

	slog.DebugContext(ctx, "netns connection established", "netns", netnsName)
	return &NetNSConn{Conn: conn, nsFD: fd}, nil
}
