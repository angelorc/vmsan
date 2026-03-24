// Package gateway implements the vmsan-gateway gRPC server over a Unix socket.
// It manages per-VM proxy resources and provides a control plane for the CLI.
package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/user"
	"strconv"
	"time"

	vmsanv1 "github.com/angelorc/vmsan/hostd/gen/vmsan/v1"
	"google.golang.org/grpc"
)

// Version is reported by the ping handler. Set at build time via ldflags or
// bump manually with each release.
var Version = "0.4.0"

// Config holds the server configuration.
type Config struct {
	SocketPath string
	PIDFile    string
}

// Server is the vmsan-gateway gRPC server.
type Server struct {
	vmsanv1.UnimplementedGatewayServiceServer

	config         Config
	manager        *Manager
	meshManager    *MeshManager
	dnsSupervisor  *DNSSupervisor
	slots          *SlotAllocator
	timeoutManager *TimeoutManager
	startTime      time.Time
	cancelFunc     context.CancelFunc // set by Run(), used by shutdown handler
	grpcServer     *grpc.Server
}

// Response is the internal response envelope used by handler methods.
// It bridges between the existing handlers and the gRPC layer.
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
	VethHost       string   `json:"vethHost,omitempty"`
	NetNS          string   `json:"netns,omitempty"`
	GuestDev       string   `json:"guestDev,omitempty"`
}

// vmStopParams holds the parameters for vm.stop.
type vmStopParams struct {
	VMId     string `json:"vmId"`
	VethHost string `json:"vethHost,omitempty"`
	NetNS    string `json:"netns,omitempty"`
	GuestDev string `json:"guestDev,omitempty"`
}

// vmUpdatePolicyParams holds the parameters for vm.updatePolicy.
type vmUpdatePolicyParams struct {
	VMId   string `json:"vmId"`
	Policy string `json:"policy"`
}

// NewServer creates a new gateway server. The meshManager may be nil if
// mesh networking is not enabled.
func NewServer(cfg Config, meshManager *MeshManager, slots *SlotAllocator) (*Server, error) {
	if cfg.SocketPath == "" {
		return nil, errors.New("socket path is required")
	}
	if cfg.PIDFile == "" {
		return nil, errors.New("PID file path is required")
	}
	dnsSup := NewDNSSupervisor(slog.Default())
	s := &Server{
		config:        cfg,
		manager:       NewManager(dnsSup),
		meshManager:   meshManager,
		dnsSupervisor: dnsSup,
		slots:         slots,
	}
	s.timeoutManager = NewTimeoutManager(s.onVMTimeout)
	return s, nil
}

// Run starts the gRPC server and blocks until the context is cancelled.
// It removes stale socket and PID files on startup, writes the PID file,
// and cleans up on shutdown.
func (s *Server) Run(ctx context.Context) error {
	// Record startup time for health reporting.
	s.startTime = time.Now()

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

	// Try to set socket group to "vmsan" for non-root access.
	if grp, err := user.LookupGroup("vmsan"); err == nil {
		if gid, err := strconv.Atoi(grp.Gid); err == nil {
			os.Chown(s.config.SocketPath, -1, gid)
		}
	}

	// Backward compat: symlink old path → new path
	oldSocket := "/run/vmsan-gateway.sock"
	os.Remove(oldSocket) // ignore error
	os.Symlink(s.config.SocketPath, oldSocket)

	if err := writePIDFile(s.config.PIDFile); err != nil {
		listener.Close()
		return fmt.Errorf("write PID file: %w", err)
	}

	// Create and register gRPC server.
	s.grpcServer = grpc.NewServer()
	vmsanv1.RegisterGatewayServiceServer(s.grpcServer, s)

	slog.Info("gateway started",
		"socket", s.config.SocketPath,
		"pid", os.Getpid(),
		"protocol", "grpc",
	)

	// Graceful shutdown on context cancellation.
	go func() {
		<-ctx.Done()
		slog.Info("shutting down gateway")
		s.grpcServer.GracefulStop()
	}()

	// Serve blocks until the server is stopped.
	if err := s.grpcServer.Serve(listener); err != nil {
		// grpc.ErrServerStopped is expected on graceful shutdown.
		if ctx.Err() == nil {
			return fmt.Errorf("grpc serve: %w", err)
		}
	}

	// Graceful shutdown: stop VMs and supervised DNS processes, remove socket, symlink, and PID file.
	s.manager.StopAll()
	removeIfExists(s.config.SocketPath)
	removeIfExists("/run/vmsan-gateway.sock") // backward compat symlink
	removeIfExists(s.config.PIDFile)

	slog.Info("gateway stopped")
	return nil
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

	state, err := s.manager.StartVM(p.VMId, p.Slot, p.Policy, p.AllowedDomains)
	if err != nil {
		return Response{OK: false, Error: err.Error(), Code: "START_ERROR"}
	}

	// Wire into mesh networking if the VM belongs to a project.
	var meshResult *VMStartResult
	if p.Project != "" && s.meshManager != nil {
		meshResult, err = s.meshManager.OnVMStart(VMStartParams{
			VMId:      p.VMId,
			Slot:      p.Slot,
			Policy:    p.Policy,
			Project:   p.Project,
			Service:   p.Service,
			ConnectTo: p.ConnectTo,
			VethHost:  p.VethHost,
			NetNS:     p.NetNS,
			GuestDev:  p.GuestDev,
		})
		if err != nil {
			slog.Warn("mesh setup failed", "vmId", p.VMId, "error", err)
		}
	}

	// Build combined response with mesh info when available.
	type vmStartResponse struct {
		*VMState
		MeshIP  string `json:"meshIp,omitempty"`
		Service string `json:"meshService,omitempty"`
	}
	resp := vmStartResponse{VMState: state}
	if meshResult != nil {
		resp.MeshIP = meshResult.MeshIP
		resp.Service = meshResult.Service
	}

	return Response{OK: true, VM: resp}
}

func (s *Server) handleVMStop(params json.RawMessage) Response {
	var p vmStopParams
	if err := json.Unmarshal(params, &p); err != nil {
		return Response{OK: false, Error: "invalid params: " + err.Error(), Code: "PARSE_ERROR"}
	}
	if p.VMId == "" {
		return Response{OK: false, Error: "vmId is required", Code: "VALIDATION_ERROR"}
	}

	// Tear down mesh networking before stopping the VM.
	if s.meshManager != nil {
		if err := s.meshManager.OnVMStop(p.VMId, p.VethHost, p.NetNS, p.GuestDev); err != nil {
			slog.Warn("mesh teardown failed", "vmId", p.VMId, "error", err)
		}
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
	metas, err := listVMMetadata()
	if err != nil {
		// Fall back to manager state if metadata is unavailable.
		vms := s.manager.ListVMs()
		return Response{OK: true, VMs: len(vms), List: vms}
	}
	return Response{OK: true, VMs: len(metas), List: metas}
}

// handleVMGet returns the full metadata for a single VM.
func (s *Server) handleVMGet(params json.RawMessage) Response {
	var p struct {
		VMId string `json:"vmId"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return Response{OK: false, Error: "invalid params", Code: "PARSE_ERROR"}
	}
	if p.VMId == "" {
		return Response{OK: false, Error: "vmId is required", Code: "VALIDATION_ERROR"}
	}
	meta, err := readVMMetadata(p.VMId)
	if err != nil {
		return Response{OK: false, Error: "VM not found: " + p.VMId, Code: "NOT_FOUND"}
	}
	return Response{OK: true, VM: meta}
}

// extendTimeoutParams holds the parameters for vm.extendTimeout.
type extendTimeoutParams struct {
	VMId      string `json:"vmId"`
	TimeoutAt string `json:"timeoutAt"` // ISO 8601
}

// handleExtendTimeout updates the timeout for a running VM.
func (s *Server) handleExtendTimeout(params json.RawMessage) Response {
	var p extendTimeoutParams
	if err := json.Unmarshal(params, &p); err != nil {
		return Response{OK: false, Error: "invalid params: " + err.Error(), Code: "PARSE_ERROR"}
	}
	if p.VMId == "" {
		return Response{OK: false, Error: "vmId is required", Code: "VALIDATION_ERROR"}
	}
	if p.TimeoutAt == "" {
		return Response{OK: false, Error: "timeoutAt is required", Code: "VALIDATION_ERROR"}
	}

	timeoutAt, err := time.Parse(time.RFC3339, p.TimeoutAt)
	if err != nil {
		return Response{OK: false, Error: "invalid timeoutAt format (expected RFC3339): " + err.Error(), Code: "VALIDATION_ERROR"}
	}

	if err := s.timeoutManager.Extend(p.VMId, timeoutAt); err != nil {
		return Response{OK: false, Error: err.Error(), Code: "TIMEOUT_ERROR"}
	}

	// Update metadata.
	if err := updateVMMetadataFields(p.VMId, func(m *VMMetadata) {
		m.TimeoutAt = p.TimeoutAt
	}); err != nil {
		slog.Warn("failed to update timeout metadata", "vmId", p.VMId, "error", err)
	}

	return Response{OK: true}
}

// onVMTimeout is called when a VM's timeout expires.
func (s *Server) onVMTimeout(vmId string) {
	slog.Info("VM timeout expired, stopping", "vmId", vmId)
	meta, err := readVMMetadata(vmId)
	if err != nil {
		slog.Error("timeout: failed to read metadata", "vmId", vmId, "error", err)
		return
	}
	params := vmFullStopParams{
		VMId:       vmId,
		Slot:       meta.Slot,
		PID:        meta.PID,
		NetNSName:  meta.NetNSName,
		SocketPath: meta.SocketPath,
	}
	raw, _ := json.Marshal(params)
	resp := s.handleVMFullStop(context.Background(), raw)
	if !resp.OK {
		slog.Error("timeout: fullStop failed", "vmId", vmId, "error", resp.Error)
	}
	updateVMMetadataFields(vmId, func(m *VMMetadata) {
		m.Status = "stopped"
		m.PID = 0
	})
}

func (s *Server) handleShutdown(_ context.Context) Response {
	s.timeoutManager.CancelAll()
	s.manager.StopAll()
	if s.cancelFunc != nil {
		s.cancelFunc()
	}
	return Response{OK: true}
}


func (s *Server) handleHealth() Response {
	vms := s.manager.ListVMs()

	type healthResponse struct {
		Version    string `json:"version"`
		VMs        int    `json:"vms"`
		Uptime     string `json:"uptime"`
		DNSProxies int    `json:"dnsProxies,omitempty"`
		SNIProxies int    `json:"sniProxies,omitempty"`
	}

	return Response{
		OK: true,
		VM: healthResponse{
			Version:    Version,
			VMs:        len(vms),
			Uptime:     time.Since(s.startTime).Truncate(time.Second).String(),
			DNSProxies: s.dnsSupervisor.Count(),
			SNIProxies: len(vms), // one per VM
		},
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
