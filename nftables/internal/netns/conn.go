package netns

import (
	"fmt"
	"os"

	"github.com/google/nftables"
)

// NewConn returns an nftables.Conn bound to the given network namespace.
// If netnsName is empty, returns a connection to the default (host) namespace.
// The caller must call cleanup() when done to release the namespace fd.
//
// IMPORTANT: The caller must hold runtime.LockOSThread() for the duration
// of all operations on the returned connection to prevent goroutine migration.
func NewConn(netnsName string) (c *nftables.Conn, cleanup func(), err error) {
	if netnsName == "" {
		conn, err := nftables.New()
		return conn, func() {}, err
	}

	nsPath := fmt.Sprintf("/var/run/netns/%s", netnsName)
	fd, err := os.Open(nsPath)
	if err != nil {
		return nil, nil, fmt.Errorf("open netns %s: %w", netnsName, err)
	}

	conn, err := nftables.New(nftables.WithNetNSFd(int(fd.Fd())))
	if err != nil {
		fd.Close()
		return nil, nil, fmt.Errorf("nftables conn in netns %s: %w", netnsName, err)
	}

	return conn, func() { fd.Close() }, nil
}
