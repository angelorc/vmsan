package agentclient

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealth_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	if err := c.Health(context.Background()); err != nil {
		t.Errorf("Health() error: %v", err)
	}
}

func TestHealth_Failure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "internal error")
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	err := c.Health(context.Background())
	if err == nil {
		t.Fatal("Health() expected error for 500 response")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("Health() error = %v, want error containing 500", err)
	}
}

func TestExec_StreamEvents(t *testing.T) {
	events := []RunEvent{
		{Type: "started", ID: "cmd-1", PID: intPtr(42)},
		{Type: "stdout", Data: "hello\n"},
		{Type: "exit", ExitCode: intPtr(0)},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/exec" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}

		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(http.StatusOK)

		for _, ev := range events {
			data, _ := json.Marshal(ev)
			fmt.Fprintf(w, "%s\n", data)
		}
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	ch, err := c.Exec(context.Background(), RunParams{Cmd: "echo", Args: []string{"hello"}})
	if err != nil {
		t.Fatalf("Exec() error: %v", err)
	}

	var received []RunEvent
	for ev := range ch {
		received = append(received, ev)
	}

	if len(received) != len(events) {
		t.Fatalf("received %d events, want %d", len(received), len(events))
	}

	// Verify event types in order.
	wantTypes := []string{"started", "stdout", "exit"}
	for i, ev := range received {
		if ev.Type != wantTypes[i] {
			t.Errorf("event[%d].Type = %q, want %q", i, ev.Type, wantTypes[i])
		}
	}

	// Verify started event.
	if received[0].ID != "cmd-1" {
		t.Errorf("event[0].ID = %q, want %q", received[0].ID, "cmd-1")
	}
	if received[0].PID == nil || *received[0].PID != 42 {
		t.Errorf("event[0].PID = %v, want 42", received[0].PID)
	}

	// Verify stdout event.
	if received[1].Data != "hello\n" {
		t.Errorf("event[1].Data = %q, want %q", received[1].Data, "hello\n")
	}

	// Verify exit event.
	if received[2].ExitCode == nil || *received[2].ExitCode != 0 {
		t.Errorf("event[2].ExitCode = %v, want 0", received[2].ExitCode)
	}
}

func TestExec_AuthHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			t.Errorf("Authorization = %q, want %q", auth, "Bearer test-token")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	ch, err := c.Exec(context.Background(), RunParams{Cmd: "ls"})
	if err != nil {
		t.Fatalf("Exec() error: %v", err)
	}
	// Drain channel.
	for range ch {
	}
}

func TestExec_ErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "bad request")
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	_, err := c.Exec(context.Background(), RunParams{Cmd: "bad"})
	if err == nil {
		t.Fatal("Exec() expected error for 400 response")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("Exec() error = %v, want error containing 400", err)
	}
}

func TestReadFile_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/files/read" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}

		// Verify auth header.
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			t.Errorf("Authorization = %q, want %q", auth, "Bearer test-token")
		}

		// Verify request body contains the path.
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["path"] != "/etc/hostname" {
			t.Errorf("body path = %q, want %q", body["path"], "/etc/hostname")
		}

		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "file content")
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	data, err := c.ReadFile(context.Background(), "/etc/hostname")
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}
	if string(data) != "file content" {
		t.Errorf("ReadFile() = %q, want %q", string(data), "file content")
	}
}

func TestReadFile_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	data, err := c.ReadFile(context.Background(), "/nonexistent")
	if err != nil {
		t.Fatalf("ReadFile() error: %v, want nil", err)
	}
	if data != nil {
		t.Errorf("ReadFile() = %v, want nil for 404", data)
	}
}

func TestReadFile_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "server error")
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	_, err := c.ReadFile(context.Background(), "/etc/hostname")
	if err == nil {
		t.Fatal("ReadFile() expected error for 500 response")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("ReadFile() error = %v, want error containing 500", err)
	}
}

func TestWriteFiles_TarGzip(t *testing.T) {
	files := []WriteFileEntry{
		{Path: "hello.txt", Content: []byte("hello world"), Mode: 0644},
		{Path: "script.sh", Content: []byte("#!/bin/bash\necho hi"), Mode: 0755},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/files/write" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}

		// Verify Content-Type.
		ct := r.Header.Get("Content-Type")
		if ct != "application/gzip" {
			t.Errorf("Content-Type = %q, want %q", ct, "application/gzip")
		}

		// Verify X-Extract-Dir header.
		extractDir := r.Header.Get("X-Extract-Dir")
		if extractDir != "/tmp/dest" {
			t.Errorf("X-Extract-Dir = %q, want %q", extractDir, "/tmp/dest")
		}

		// Verify auth header.
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			t.Errorf("Authorization = %q, want %q", auth, "Bearer test-token")
		}

		// Decompress and verify tar contents.
		gr, err := gzip.NewReader(r.Body)
		if err != nil {
			t.Fatalf("gzip reader: %v", err)
		}
		defer gr.Close()

		tr := tar.NewReader(gr)
		var found []string
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("tar next: %v", err)
			}
			content, _ := io.ReadAll(tr)
			found = append(found, hdr.Name)

			switch hdr.Name {
			case "hello.txt":
				if string(content) != "hello world" {
					t.Errorf("hello.txt content = %q, want %q", string(content), "hello world")
				}
				if hdr.Mode != 0644 {
					t.Errorf("hello.txt mode = %o, want %o", hdr.Mode, 0644)
				}
			case "script.sh":
				if string(content) != "#!/bin/bash\necho hi" {
					t.Errorf("script.sh content = %q, want %q", string(content), "#!/bin/bash\necho hi")
				}
				if hdr.Mode != 0755 {
					t.Errorf("script.sh mode = %o, want %o", hdr.Mode, 0755)
				}
			default:
				t.Errorf("unexpected file in tar: %s", hdr.Name)
			}
		}

		if len(found) != 2 {
			t.Errorf("tar contained %d files, want 2: %v", len(found), found)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	err := c.WriteFiles(context.Background(), files, "/tmp/dest")
	if err != nil {
		t.Fatalf("WriteFiles() error: %v", err)
	}
}

func TestWriteFiles_DefaultMode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gr, err := gzip.NewReader(r.Body)
		if err != nil {
			t.Fatalf("gzip reader: %v", err)
		}
		defer gr.Close()

		tr := tar.NewReader(gr)
		hdr, err := tr.Next()
		if err != nil {
			t.Fatalf("tar next: %v", err)
		}
		if hdr.Mode != 0644 {
			t.Errorf("default mode = %o, want %o", hdr.Mode, 0644)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	err := c.WriteFiles(context.Background(), []WriteFileEntry{
		{Path: "file.txt", Content: []byte("data"), Mode: 0}, // mode 0 should default to 0644
	}, "")
	if err != nil {
		t.Fatalf("WriteFiles() error: %v", err)
	}
}

func TestKillCommand(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/exec/cmd-1/kill" {
			t.Errorf("unexpected path: %s, want /exec/cmd-1/kill", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}

		// Verify auth header.
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			t.Errorf("Authorization = %q, want %q", auth, "Bearer test-token")
		}

		// Verify signal in body.
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["signal"] != "SIGTERM" {
			t.Errorf("signal = %q, want %q", body["signal"], "SIGTERM")
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	err := c.KillCommand(context.Background(), "cmd-1", "SIGTERM")
	if err != nil {
		t.Errorf("KillCommand() error: %v", err)
	}
}

func TestKillCommand_NoSignal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/exec/cmd-2/kill" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	err := c.KillCommand(context.Background(), "cmd-2", "")
	if err != nil {
		t.Errorf("KillCommand() error: %v", err)
	}
}

func TestKillCommand_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "command not found")
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	err := c.KillCommand(context.Background(), "cmd-missing", "SIGKILL")
	if err == nil {
		t.Fatal("KillCommand() expected error for 404 response")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("KillCommand() error = %v, want error containing 404", err)
	}
}

// intPtr is a test helper to create *int values.
func intPtr(v int) *int {
	return &v
}
