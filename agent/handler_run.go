package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/angelorc/vmsan/agent/internal/cmdstore"
)

const maxConcurrentCommands = 16

var activeCommands atomic.Int32

type runRequest struct {
	Cmd       string            `json:"cmd"`
	Args      []string          `json:"args,omitempty"`
	Cwd       string            `json:"cwd,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	TimeoutMs int               `json:"timeoutMs,omitempty"`
	Detached  bool              `json:"detached,omitempty"`
}

type ndjsonEvent struct {
	Type      string `json:"type"`
	Data      string `json:"data,omitempty"`
	ID        string `json:"id,omitempty"`
	PID       int    `json:"pid,omitempty"`
	ExitCode  *int   `json:"exitCode,omitempty"`
	Timestamp string `json:"ts"`
	Error     string `json:"error,omitempty"`
}

func now() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func writeEvent(w io.Writer, mu *sync.Mutex, evt ndjsonEvent) {
	mu.Lock()
	defer mu.Unlock()
	data, _ := json.Marshal(evt)
	fmt.Fprintf(w, "%s\n", data)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func makeRunHandler(logger *slog.Logger) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		handleRun(w, r, logger)
	}
}

func handleRun(w http.ResponseWriter, r *http.Request, logger *slog.Logger) {
	var req runRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if req.Cmd == "" {
		http.Error(w, `{"error":"cmd is required"}`, http.StatusBadRequest)
		return
	}

	logger.Info("exec",
		"cmd", req.Cmd,
		"args", req.Args,
		"cwd", req.Cwd,
		"detached", req.Detached,
		"timeout_ms", req.TimeoutMs,
	)

	if int(activeCommands.Load()) >= maxConcurrentCommands {
		http.Error(w, `{"error":"too many concurrent commands"}`, http.StatusTooManyRequests)
		return
	}
	activeCommands.Add(1)

	start := time.Now()

	cmd := exec.Command(req.Cmd, req.Args...)
	if req.Cwd != "" {
		cmd.Dir = req.Cwd
	}
	if len(req.Env) > 0 {
		env := cmd.Environ()
		for k, v := range req.Env {
			env = append(env, k+"="+v)
		}
		cmd.Env = env
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		activeCommands.Add(-1)
		http.Error(w, fmt.Sprintf(`{"error":"stdout pipe: %s"}`, err), http.StatusInternalServerError)
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		activeCommands.Add(-1)
		http.Error(w, fmt.Sprintf(`{"error":"stderr pipe: %s"}`, err), http.StatusInternalServerError)
		return
	}

	if err := cmd.Start(); err != nil {
		activeCommands.Add(-1)
		http.Error(w, fmt.Sprintf(`{"error":"start: %s"}`, err), http.StatusInternalServerError)
		return
	}

	cmdID := cmdstore.Store(cmd)

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)

	var mu sync.Mutex
	writeEvent(w, &mu, ndjsonEvent{
		Type:      "started",
		ID:        cmdID,
		PID:       cmd.Process.Pid,
		Timestamp: now(),
	})

	// In detached mode, return after the started event; the process continues in background.
	if req.Detached {
		go func() {
			cmd.Wait()
			activeCommands.Add(-1)
			cmdstore.Remove(cmdID)
			logger.Info("exec.done",
				"cmd_id", cmdID,
				"exit_code", -1,
				"duration_ms", time.Since(start).Milliseconds(),
				"detached", true,
			)
		}()
		return
	}

	// Stream stdout and stderr concurrently.
	var wg sync.WaitGroup
	wg.Add(2)

	streamPipe := func(pipe io.ReadCloser, streamType string) {
		defer wg.Done()
		scanner := bufio.NewScanner(pipe)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			writeEvent(w, &mu, ndjsonEvent{
				Type:      streamType,
				Data:      scanner.Text(),
				Timestamp: now(),
			})
		}
	}

	go streamPipe(stdout, "stdout")
	go streamPipe(stderr, "stderr")

	// Handle timeout.
	var timedOut atomic.Bool
	if req.TimeoutMs > 0 {
		timer := time.AfterFunc(time.Duration(req.TimeoutMs)*time.Millisecond, func() {
			timedOut.Store(true)
			cmd.Process.Kill()
		})
		defer timer.Stop()
	}

	wg.Wait()
	err = cmd.Wait()
	activeCommands.Add(-1)
	cmdstore.Remove(cmdID)

	if timedOut.Load() {
		logger.Info("exec.done",
			"cmd_id", cmdID,
			"exit_code", -1,
			"duration_ms", time.Since(start).Milliseconds(),
			"timeout", true,
		)
		writeEvent(w, &mu, ndjsonEvent{
			Type:      "timeout",
			Timestamp: now(),
		})
		return
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code := exitErr.ExitCode()
			logger.Info("exec.done",
				"cmd_id", cmdID,
				"exit_code", code,
				"duration_ms", time.Since(start).Milliseconds(),
			)
			writeEvent(w, &mu, ndjsonEvent{
				Type:      "exit",
				ExitCode:  &code,
				Timestamp: now(),
			})
		} else {
			logger.Error("exec.done",
				"cmd_id", cmdID,
				"error", err.Error(),
				"duration_ms", time.Since(start).Milliseconds(),
			)
			writeEvent(w, &mu, ndjsonEvent{
				Type:      "error",
				Error:     err.Error(),
				Timestamp: now(),
			})
		}
		return
	}

	code := 0
	logger.Info("exec.done",
		"cmd_id", cmdID,
		"exit_code", code,
		"duration_ms", time.Since(start).Milliseconds(),
	)
	writeEvent(w, &mu, ndjsonEvent{
		Type:      "exit",
		ExitCode:  &code,
		Timestamp: now(),
	})
}

func handleKill(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, `{"error":"id is required"}`, http.StatusBadRequest)
		return
	}

	cmd := cmdstore.Get(id)
	if cmd == nil {
		http.Error(w, `{"error":"command not found"}`, http.StatusNotFound)
		return
	}

	if err := cmd.Process.Kill(); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"kill: %s"}`, err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "killed"})
}
