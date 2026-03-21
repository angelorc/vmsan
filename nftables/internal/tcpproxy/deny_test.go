package tcpproxy

import (
	"net"
	"sync"
	"testing"
	"time"
)

// mockConn is a minimal net.Conn implementation for testing.
type mockConn struct {
	closed bool
	remote net.Addr
}

func (m *mockConn) Read([]byte) (int, error)         { return 0, nil }
func (m *mockConn) Write([]byte) (int, error)        { return 0, nil }
func (m *mockConn) Close() error                     { m.closed = true; return nil }
func (m *mockConn) LocalAddr() net.Addr              { return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 443} }
func (m *mockConn) RemoteAddr() net.Addr             { return m.remote }
func (m *mockConn) SetDeadline(time.Time) error      { return nil }
func (m *mockConn) SetReadDeadline(time.Time) error  { return nil }
func (m *mockConn) SetWriteDeadline(time.Time) error { return nil }

func newMockConn(srcIP string, srcPort int) *mockConn {
	return &mockConn{
		remote: &net.TCPAddr{
			IP:   net.ParseIP(srcIP),
			Port: srcPort,
		},
	}
}

func TestDenyTargetClosesConnection(t *testing.T) {
	logger := discardLogger()
	deny := NewDenyTarget("vm-test", logger)

	conn := newMockConn("198.19.0.2", 54321)
	deny.HandleConn(conn)

	if !conn.closed {
		t.Error("expected connection to be closed")
	}
}

func TestDenyTargetIncrementsDeniedCount(t *testing.T) {
	logger := discardLogger()
	deny := NewDenyTarget("vm-test", logger)

	if deny.DeniedCount() != 0 {
		t.Errorf("initial denied count = %d, want 0", deny.DeniedCount())
	}

	deny.HandleConn(newMockConn("198.19.0.2", 1))
	if deny.DeniedCount() != 1 {
		t.Errorf("denied count after 1 = %d, want 1", deny.DeniedCount())
	}

	deny.HandleConn(newMockConn("198.19.0.2", 2))
	deny.HandleConn(newMockConn("198.19.0.2", 3))
	if deny.DeniedCount() != 3 {
		t.Errorf("denied count after 3 = %d, want 3", deny.DeniedCount())
	}
}

func TestDenyTargetConcurrentAccess(t *testing.T) {
	logger := discardLogger()
	deny := NewDenyTarget("vm-concurrent", logger)

	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			deny.HandleConn(newMockConn("198.19.0.2", 10000+i))
		}(i)
	}

	wg.Wait()

	if count := deny.DeniedCount(); count != goroutines {
		t.Errorf("denied count = %d, want %d", count, goroutines)
	}
}

func TestDenyTargetWithNilRemoteAddr(t *testing.T) {
	logger := discardLogger()
	deny := NewDenyTarget("vm-nil-addr", logger)

	// A connection whose RemoteAddr returns a non-host:port string.
	conn := &mockConn{
		remote: &net.UnixAddr{Name: "/tmp/test.sock", Net: "unix"},
	}
	deny.HandleConn(conn)

	if !conn.closed {
		t.Error("expected connection to be closed")
	}
	if deny.DeniedCount() != 1 {
		t.Errorf("denied count = %d, want 1", deny.DeniedCount())
	}
}

func TestNewDenyTarget(t *testing.T) {
	logger := discardLogger()
	deny := NewDenyTarget("vm-abc123", logger)

	if deny == nil {
		t.Fatal("NewDenyTarget returned nil")
	}
	if deny.vmId != "vm-abc123" {
		t.Errorf("vmId = %q, want %q", deny.vmId, "vm-abc123")
	}
	if deny.DeniedCount() != 0 {
		t.Errorf("initial denied count = %d, want 0", deny.DeniedCount())
	}
}
