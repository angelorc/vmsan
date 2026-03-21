package dnsproxy

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestLogWritesJSONLines(t *testing.T) {
	dir := t.TempDir()
	logger := NewDNSLogger("test-vm", dir)
	defer logger.Close()

	entry := DNSLogEntry{
		Event:     "dns_query",
		Domain:    "example.com",
		Result:    "93.184.216.34",
		Policy:    "allow",
		LatencyMs: 12,
		VMId:      "test-vm",
		Timestamp: "2026-03-21T10:00:00Z",
	}

	if err := logger.Log(entry); err != nil {
		t.Fatalf("Log() error: %v", err)
	}

	data, err := os.ReadFile(logger.LogPath())
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}

	var got DNSLogEntry
	if err := json.Unmarshal([]byte(lines[0]), &got); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}

	if got.Domain != "example.com" {
		t.Errorf("Domain = %q, want %q", got.Domain, "example.com")
	}
	if got.Policy != "allow" {
		t.Errorf("Policy = %q, want %q", got.Policy, "allow")
	}
	if got.LatencyMs != 12 {
		t.Errorf("LatencyMs = %d, want 12", got.LatencyMs)
	}
	if got.Timestamp != "2026-03-21T10:00:00Z" {
		t.Errorf("Timestamp = %q, want %q", got.Timestamp, "2026-03-21T10:00:00Z")
	}
}

func TestLogPathFormat(t *testing.T) {
	logger := NewDNSLogger("abc123", "/tmp")
	want := "/tmp/vmsan-dns-abc123.log"
	if got := logger.LogPath(); got != want {
		t.Errorf("LogPath() = %q, want %q", got, want)
	}
}

func TestLogAutoTimestamp(t *testing.T) {
	dir := t.TempDir()
	logger := NewDNSLogger("ts-vm", dir)
	defer logger.Close()

	entry := DNSLogEntry{
		Event:  "dns_query",
		Domain: "auto-ts.com",
		VMId:   "ts-vm",
	}

	if err := logger.Log(entry); err != nil {
		t.Fatalf("Log() error: %v", err)
	}

	data, err := os.ReadFile(logger.LogPath())
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}

	var got DNSLogEntry
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(data))), &got); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}

	if got.Timestamp == "" {
		t.Error("expected auto-generated timestamp, got empty string")
	}
}

func TestLogMultipleEntries(t *testing.T) {
	dir := t.TempDir()
	logger := NewDNSLogger("multi-vm", dir)
	defer logger.Close()

	for i := 0; i < 5; i++ {
		entry := DNSLogEntry{
			Event:     "dns_query",
			Domain:    fmt.Sprintf("test%d.com", i),
			VMId:      "multi-vm",
			Timestamp: "2026-03-21T10:00:00Z",
		}
		if err := logger.Log(entry); err != nil {
			t.Fatalf("Log() entry %d error: %v", i, err)
		}
	}

	f, err := os.Open(logger.LogPath())
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	count := 0
	for scanner.Scan() {
		var entry DNSLogEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			t.Fatalf("line %d: Unmarshal() error: %v", count, err)
		}
		count++
	}
	if count != 5 {
		t.Errorf("expected 5 lines, got %d", count)
	}
}

func TestLogRotation(t *testing.T) {
	dir := t.TempDir()
	logger := NewDNSLogger("rot-vm", dir)
	// Use a small max size to trigger rotation quickly in tests.
	logger.maxSize = 1024
	defer logger.Close()

	entry := DNSLogEntry{
		Event:     "dns_query",
		Domain:    "rotation-test.com",
		Result:    "1.2.3.4",
		Policy:    "allow",
		LatencyMs: 1,
		VMId:      "rot-vm",
		Timestamp: "2026-03-21T10:00:00Z",
	}

	// Write enough entries to exceed 1 KB.
	for i := 0; i < 100; i++ {
		if err := logger.Log(entry); err != nil {
			t.Fatalf("Log() error at entry %d: %v", i, err)
		}
	}

	// Verify the backup file exists.
	backup := logger.LogPath() + ".1"
	info, err := os.Stat(backup)
	if err != nil {
		t.Fatalf("backup file not found: %v", err)
	}
	if info.Size() == 0 {
		t.Error("backup file is empty")
	}
}

func TestLogRotationMaxBackups(t *testing.T) {
	dir := t.TempDir()
	logger := NewDNSLogger("maxb-vm", dir)
	// Use a very small max size so rotation happens quickly.
	logger.maxSize = 512
	defer logger.Close()

	entry := DNSLogEntry{
		Event:     "dns_query",
		Domain:    "maxbackups-test.com",
		VMId:      "maxb-vm",
		Timestamp: "2026-03-21T10:00:00Z",
	}

	// Write enough to trigger multiple rotations (> 3 * 512 bytes).
	for i := 0; i < 200; i++ {
		if err := logger.Log(entry); err != nil {
			t.Fatalf("Log() error at entry %d: %v", i, err)
		}
	}

	// .1 and .2 should exist (MaxLogBackups=2).
	for i := 1; i <= MaxLogBackups; i++ {
		backup := fmt.Sprintf("%s.%d", logger.LogPath(), i)
		if _, err := os.Stat(backup); err != nil {
			t.Errorf("backup .%d not found: %v", i, err)
		}
	}

	// .3 should NOT exist.
	extra := fmt.Sprintf("%s.%d", logger.LogPath(), MaxLogBackups+1)
	if _, err := os.Stat(extra); err == nil {
		t.Errorf("backup .%d should not exist", MaxLogBackups+1)
	}
}

func TestConcurrentWrites(t *testing.T) {
	dir := t.TempDir()
	logger := NewDNSLogger("conc-vm", dir)
	defer logger.Close()

	const goroutines = 10
	const entriesPerGoroutine = 50

	var wg sync.WaitGroup
	errCh := make(chan error, goroutines*entriesPerGoroutine)

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < entriesPerGoroutine; i++ {
				entry := DNSLogEntry{
					Event:     "dns_query",
					Domain:    fmt.Sprintf("g%d-q%d.com", id, i),
					VMId:      "conc-vm",
					Timestamp: "2026-03-21T10:00:00Z",
				}
				if err := logger.Log(entry); err != nil {
					errCh <- err
				}
			}
		}(g)
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent Log() error: %v", err)
	}

	// Verify all entries were written.
	data, err := os.ReadFile(logger.LogPath())
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	want := goroutines * entriesPerGoroutine
	if len(lines) != want {
		t.Errorf("expected %d lines, got %d", want, len(lines))
	}

	// Each line should be valid JSON.
	for i, line := range lines {
		var entry DNSLogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Errorf("line %d: invalid JSON: %v", i, err)
		}
	}
}

func TestCloseIdempotent(t *testing.T) {
	dir := t.TempDir()
	logger := NewDNSLogger("close-vm", dir)

	entry := DNSLogEntry{
		Event:     "dns_query",
		Domain:    "close.com",
		VMId:      "close-vm",
		Timestamp: "2026-03-21T10:00:00Z",
	}
	if err := logger.Log(entry); err != nil {
		t.Fatalf("Log() error: %v", err)
	}

	if err := logger.Close(); err != nil {
		t.Fatalf("first Close() error: %v", err)
	}
	if err := logger.Close(); err != nil {
		t.Fatalf("second Close() error: %v", err)
	}
}

func TestLogCreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "dir")
	logger := NewDNSLogger("dir-vm", dir)
	defer logger.Close()

	entry := DNSLogEntry{
		Event:     "dns_query",
		Domain:    "dirtest.com",
		VMId:      "dir-vm",
		Timestamp: "2026-03-21T10:00:00Z",
	}
	if err := logger.Log(entry); err != nil {
		t.Fatalf("Log() error: %v", err)
	}

	if _, err := os.Stat(logger.LogPath()); err != nil {
		t.Fatalf("log file not created: %v", err)
	}
}
