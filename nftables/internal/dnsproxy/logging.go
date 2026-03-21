// Package dnsproxy provides DNS query logging for per-VM DNS proxies.
package dnsproxy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	// MaxLogSize is the maximum size of a DNS log file before rotation (10 MB).
	MaxLogSize = 10 * 1024 * 1024
	// MaxLogBackups is the maximum number of rotated log files to keep.
	MaxLogBackups = 2
)

// DNSLogEntry represents a single DNS query log entry.
type DNSLogEntry struct {
	Event     string `json:"event"`
	Domain    string `json:"domain"`
	Result    string `json:"result"`
	Policy    string `json:"policy"`
	LatencyMs int64  `json:"latency_ms"`
	VMId      string `json:"vmId"`
	Timestamp string `json:"ts"`
}

// DNSLogger writes structured DNS query logs to a per-VM log file.
// It is safe for concurrent use.
type DNSLogger struct {
	vmId     string
	logDir   string
	mu       sync.Mutex
	file     *os.File
	size     int64
	maxSize  int64 // configurable for testing; 0 means MaxLogSize
}

// NewDNSLogger creates a DNSLogger that writes to logDir/vmsan-dns-<vmId>.log.
// The log directory is created if it does not exist.
func NewDNSLogger(vmId string, logDir string) *DNSLogger {
	return &DNSLogger{
		vmId:   vmId,
		logDir: logDir,
	}
}

// LogPath returns the path to the current log file.
func (l *DNSLogger) LogPath() string {
	return filepath.Join(l.logDir, fmt.Sprintf("vmsan-dns-%s.log", l.vmId))
}

// effectiveMaxSize returns the rotation threshold.
func (l *DNSLogger) effectiveMaxSize() int64 {
	if l.maxSize > 0 {
		return l.maxSize
	}
	return MaxLogSize
}

// Log writes a single DNS log entry as a JSON line, rotating the file
// if it exceeds the size limit.
func (l *DNSLogger) Log(entry DNSLogEntry) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if err := l.ensureOpen(); err != nil {
		return err
	}

	if entry.Timestamp == "" {
		entry.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal dns log entry: %w", err)
	}
	data = append(data, '\n')

	n, err := l.file.Write(data)
	if err != nil {
		return fmt.Errorf("write dns log: %w", err)
	}
	l.size += int64(n)

	if l.size >= l.effectiveMaxSize() {
		if err := l.rotate(); err != nil {
			return fmt.Errorf("rotate dns log: %w", err)
		}
	}

	return nil
}

// Close closes the underlying log file.
func (l *DNSLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file != nil {
		err := l.file.Close()
		l.file = nil
		l.size = 0
		return err
	}
	return nil
}

// ensureOpen opens the log file if it is not already open.
// Must be called with l.mu held.
func (l *DNSLogger) ensureOpen() error {
	if l.file != nil {
		return nil
	}

	if err := os.MkdirAll(l.logDir, 0o755); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}

	f, err := os.OpenFile(l.LogPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open dns log: %w", err)
	}

	info, err := f.Stat()
	if err != nil {
		f.Close()
		return fmt.Errorf("stat dns log: %w", err)
	}

	l.file = f
	l.size = info.Size()
	return nil
}

// rotate closes the current log file and renames it with a numeric suffix,
// removing old backups beyond MaxLogBackups.
// Must be called with l.mu held.
func (l *DNSLogger) rotate() error {
	if l.file != nil {
		l.file.Close()
		l.file = nil
		l.size = 0
	}

	base := l.LogPath()

	// Shift existing backups: .{n-1} → .{n}, removing the oldest.
	for i := MaxLogBackups; i >= 2; i-- {
		src := fmt.Sprintf("%s.%d", base, i-1)
		dst := fmt.Sprintf("%s.%d", base, i)
		os.Remove(dst)
		if _, err := os.Stat(src); err == nil {
			os.Rename(src, dst)
		}
	}

	// Rename current log to .1
	if _, err := os.Stat(base); err == nil {
		os.Rename(base, fmt.Sprintf("%s.%d", base, 1))
	}

	return nil
}
