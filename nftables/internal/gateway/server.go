// Package gateway implements the vmsan-gateway Unix socket server and
// JSON-RPC request dispatcher. It manages per-VM proxy resources and
// provides a control plane for the TypeScript CLI.
package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
	"sync"
)

// Version is reported by the ping handler. Set at build time via ldflags or
// bump manually with each release.
var Version = "0.4.0"

// Config holds the server configuration.
type Config struct {
	SocketPath string
	PIDFile    string
}

// Server is the vmsan-gateway Unix socket server.
type Server struct {
	config     Config
	manager    *Manager
	cancelFunc context.CancelFunc // set by Run(), used by shutdown handler
}

// Request is the JSON-RPC request envelope.
type Request struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// Response is the JSON-RPC response envelope.
type Response struct {
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
	Code    string `json:"code,omitempty"`
	Version string `json:"version,omitempty"`
	VMs     int    `json:"vms,omitempty"`
	VM      any    `json:"vm,omitempty"`
	List    any    `json:"list,omitempty"`
}

// vmStartParams holds the parameters for vm.start.
type vmStartParams struct {
	VMId           string   `json:"vmId"`
	Slot           int      `json:"slot"`
	Policy         string   `json:"policy"`
	AllowedDomains []string `json:"allowedDomains,omitempty"`
	Project        string   `json:"project,omitempty"`
	Service        string   `json:"service,omitempty"`
	ConnectTo      []string `json:"connectTo,omitempty"`
}

// vmStopParams holds the parameters for vm.stop.
type vmStopParams struct {
	VMId string `json:"vmId"`
}

// vmUpdatePolicyParams holds the parameters for vm.updatePolicy.
type vmUpdatePolicyParams struct {
	VMId   string `json:"vmId"`
	Policy string `json:"policy"`
}

// NewServer creates a new gateway server.
func NewServer(cfg Config) (*Server, error) {
	if cfg.SocketPath == "" {
		return nil, errors.New("socket path is required")
	}
	if cfg.PIDFile == "" {
		return nil, errors.New("PID file path is required")
	}
	return &Server{
		config:  cfg,
		manager: NewManager(),
	}, nil
}

// Run starts the server and blocks until the context is cancelled.
// It removes stale socket and PID files on startup, writes the PID file,
// and cleans up on shutdown.
func (s *Server) Run(ctx context.Context) error {
	// Wrap context so shutdown handler can cancel it.
	ctx, cancel := context.WithCancel(ctx)
	s.cancelFunc = cancel
	defer cancel()

	// Remove stale socket file.
	if err := removeIfExists(s.config.SocketPath); err != nil {
		return fmt.Errorf("remove stale socket: %w", err)
	}

	listener, err := net.Listen("unix", s.config.SocketPath)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.config.SocketPath, err)
	}

	// Ensure non-root callers can connect.
	if err := os.Chmod(s.config.SocketPath, 0660); err != nil {
		listener.Close()
		return fmt.Errorf("chmod socket: %w", err)
	}

	if err := writePIDFile(s.config.PIDFile); err != nil {
		listener.Close()
		return fmt.Errorf("write PID file: %w", err)
	}

	slog.Info("gateway started",
		"socket", s.config.SocketPath,
		"pid", os.Getpid(),
	)

	// Accept connections until context cancellation.
	var wg sync.WaitGroup
	go func() {
		<-ctx.Done()
		slog.Info("shutting down gateway")
		listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				break // graceful shutdown
			}
			slog.Error("accept error", "error", err)
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.handleConn(ctx, conn)
		}()
	}

	// Wait for in-flight connections to finish.
	wg.Wait()

	// Graceful shutdown: stop all VMs, remove socket and PID file.
	s.manager.StopAll()
	removeIfExists(s.config.SocketPath)
	removeIfExists(s.config.PIDFile)

	slog.Info("gateway stopped")
	return nil
}

// handleConn reads a single JSON request from the connection, dispatches it,
// and writes the JSON response.
func (s *Server) handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	var req Request
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		slog.Debug("failed to decode request", "error", err)
		writeJSON(conn, Response{OK: false, Error: "invalid JSON", Code: "PARSE_ERROR"})
		return
	}

	slog.Debug("handling request", "method", req.Method)
	resp := s.dispatch(ctx, &req)
	writeJSON(conn, resp)
}

// dispatch routes a request to the appropriate handler.
func (s *Server) dispatch(ctx context.Context, req *Request) Response {
	switch req.Method {
	case "ping":
		return s.handlePing()
	case "vm.start":
		return s.handleVMStart(req.Params)
	case "vm.stop":
		return s.handleVMStop(req.Params)
	case "vm.updatePolicy":
		return s.handleVMUpdatePolicy(req.Params)
	case "status":
		return s.handleStatus()
	case "shutdown":
		return s.handleShutdown(ctx)
	default:
		return Response{OK: false, Error: fmt.Sprintf("unknown method: %s", req.Method), Code: "UNKNOWN_METHOD"}
	}
}

func (s *Server) handlePing() Response {
	return Response{
		OK:      true,
		Version: Version,
		VMs:     len(s.manager.ListVMs()),
	}
}

func (s *Server) handleVMStart(params json.RawMessage) Response {
	var p vmStartParams
	if err := json.Unmarshal(params, &p); err != nil {
		return Response{OK: false, Error: "invalid params: " + err.Error(), Code: "PARSE_ERROR"}
	}
	if p.VMId == "" {
		return Response{OK: false, Error: "vmId is required", Code: "VALIDATION_ERROR"}
	}
	if p.Policy == "" {
		p.Policy = "deny-all"
	}

	state, err := s.manager.StartVM(p.VMId, p.Slot, p.Policy)
	if err != nil {
		return Response{OK: false, Error: err.Error(), Code: "START_ERROR"}
	}
	return Response{OK: true, VM: state}
}

func (s *Server) handleVMStop(params json.RawMessage) Response {
	var p vmStopParams
	if err := json.Unmarshal(params, &p); err != nil {
		return Response{OK: false, Error: "invalid params: " + err.Error(), Code: "PARSE_ERROR"}
	}
	if p.VMId == "" {
		return Response{OK: false, Error: "vmId is required", Code: "VALIDATION_ERROR"}
	}

	if err := s.manager.StopVM(p.VMId); err != nil {
		return Response{OK: false, Error: err.Error(), Code: "STOP_ERROR"}
	}
	return Response{OK: true}
}

func (s *Server) handleVMUpdatePolicy(params json.RawMessage) Response {
	var p vmUpdatePolicyParams
	if err := json.Unmarshal(params, &p); err != nil {
		return Response{OK: false, Error: "invalid params: " + err.Error(), Code: "PARSE_ERROR"}
	}
	if p.VMId == "" {
		return Response{OK: false, Error: "vmId is required", Code: "VALIDATION_ERROR"}
	}
	if p.Policy == "" {
		return Response{OK: false, Error: "policy is required", Code: "VALIDATION_ERROR"}
	}

	if err := s.manager.UpdatePolicy(p.VMId, p.Policy); err != nil {
		return Response{OK: false, Error: err.Error(), Code: "UPDATE_ERROR"}
	}
	return Response{OK: true}
}

func (s *Server) handleStatus() Response {
	vms := s.manager.ListVMs()
	return Response{OK: true, VMs: len(vms), List: vms}
}

func (s *Server) handleShutdown(_ context.Context) Response {
	s.manager.StopAll()
	if s.cancelFunc != nil {
		s.cancelFunc()
	}
	return Response{OK: true}
}

// writeJSON encodes v as JSON and writes it to the connection.
func writeJSON(conn net.Conn, v any) {
	enc := json.NewEncoder(conn)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		slog.Error("failed to write response", "error", err)
	}
}

// writePIDFile writes the current process ID to path.
func writePIDFile(path string) error {
	return os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())+"\n"), 0644)
}

// removeIfExists removes a file if it exists, ignoring "not exist" errors.
func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
