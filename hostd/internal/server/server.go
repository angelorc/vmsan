package server

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

// Version is the server version string.
const Version = "0.4.0-dev"

// Server is the vmsan control plane HTTP server.
type Server struct {
	addr   string
	db     *Store
	logger *slog.Logger
	srv    *http.Server
}

// New creates a new Server, opening the SQLite database and configuring routes.
func New(addr, dbPath string, logger *slog.Logger) (*Server, error) {
	store, err := NewStore(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}

	s := &Server{
		addr:   addr,
		db:     store,
		logger: logger,
	}

	mux := http.NewServeMux()

	// Host endpoints
	mux.HandleFunc("POST /api/v1/hosts/join", s.handleJoin)
	mux.HandleFunc("GET /api/v1/hosts", s.handleListHosts)
	mux.HandleFunc("GET /api/v1/hosts/{id}", s.handleGetHost)
	mux.HandleFunc("DELETE /api/v1/hosts/{id}", s.handleDeleteHost)
	mux.HandleFunc("PUT /api/v1/hosts/{id}/heartbeat", s.handleHeartbeat)

	// VM endpoints
	mux.HandleFunc("GET /api/v1/vms", s.handleListVMs)
	mux.HandleFunc("POST /api/v1/vms", s.handleCreateVM)
	mux.HandleFunc("GET /api/v1/vms/{id}", s.handleGetVM)
	mux.HandleFunc("PUT /api/v1/vms/{id}/start", s.handleStartVM)
	mux.HandleFunc("PUT /api/v1/vms/{id}/stop", s.handleStopVM)
	mux.HandleFunc("DELETE /api/v1/vms/{id}", s.handleDeleteVM)

	// Sync, tokens, status
	mux.HandleFunc("GET /api/v1/sync", s.handleSync)
	mux.HandleFunc("POST /api/v1/tokens", s.handleCreateToken)
	mux.HandleFunc("GET /api/v1/status", s.handleStatus)

	s.srv = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return s, nil
}

// ListenAndServe starts the HTTP server. It blocks until the server is shut down.
func (s *Server) ListenAndServe() error {
	err := s.srv.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// Close shuts down the server and closes the database.
func (s *Server) Close() {
	s.srv.Close()
	s.db.Close()
}

// --- Host handlers ---

func (s *Server) handleJoin(w http.ResponseWriter, r *http.Request) {
	var req JoinRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Token == "" {
		s.writeError(w, http.StatusBadRequest, "token is required")
		return
	}
	if req.Name == "" {
		s.writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Address == "" {
		s.writeError(w, http.StatusBadRequest, "address is required")
		return
	}

	// Validate and consume the join token
	hostID := generateID()
	if err := s.db.ValidateAndConsumeToken(req.Token, hostID); err != nil {
		s.logger.Warn("join token validation failed", "error", err, "name", req.Name)
		s.writeError(w, http.StatusUnauthorized, "invalid or expired token")
		return
	}

	// Create the host
	if err := s.db.CreateHost(hostID, req.Name, req.Address, req.PublicKey, "active"); err != nil {
		s.logger.Error("failed to create host", "error", err, "name", req.Name)
		s.writeError(w, http.StatusInternalServerError, "failed to register host")
		return
	}

	// Log to sync
	payloadBytes, _ := json.Marshal(map[string]string{"name": req.Name, "address": req.Address})
	payload := string(payloadBytes)
	if err := s.db.AppendSyncLog("host", hostID, "create", &payload); err != nil {
		s.logger.Error("failed to append sync log", "error", err)
	}

	s.logger.Info("host joined", "host_id", hostID, "name", req.Name, "address", req.Address)
	s.writeJSON(w, http.StatusOK, JoinResponse{HostID: hostID})
}

func (s *Server) handleListHosts(w http.ResponseWriter, r *http.Request) {
	hosts, err := s.db.ListHosts()
	if err != nil {
		s.logger.Error("failed to list hosts", "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to list hosts")
		return
	}

	infos := make([]HostInfo, 0, len(hosts))
	for _, h := range hosts {
		info, err := s.hostRowToInfo(&h)
		if err != nil {
			s.logger.Error("failed to convert host", "error", err, "host_id", h.ID)
			continue
		}
		infos = append(infos, *info)
	}

	s.writeJSON(w, http.StatusOK, infos)
}

func (s *Server) handleGetHost(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	host, err := s.db.GetHost(id)
	if err == sql.ErrNoRows {
		s.writeError(w, http.StatusNotFound, "host not found")
		return
	}
	if err != nil {
		s.logger.Error("failed to get host", "error", err, "host_id", id)
		s.writeError(w, http.StatusInternalServerError, "failed to get host")
		return
	}

	info, err := s.hostRowToInfo(host)
	if err != nil {
		s.logger.Error("failed to convert host", "error", err, "host_id", id)
		s.writeError(w, http.StatusInternalServerError, "failed to get host")
		return
	}

	s.writeJSON(w, http.StatusOK, info)
}

func (s *Server) handleDeleteHost(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.db.DeleteHost(id); err == sql.ErrNoRows {
		s.writeError(w, http.StatusNotFound, "host not found")
		return
	} else if err != nil {
		s.logger.Error("failed to delete host", "error", err, "host_id", id)
		s.writeError(w, http.StatusInternalServerError, "failed to delete host")
		return
	}

	if err := s.db.AppendSyncLog("host", id, "delete", nil); err != nil {
		s.logger.Error("failed to append sync log", "error", err)
	}

	s.logger.Info("host removed", "host_id", id)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req HeartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var resourcesJSON *string
	if len(req.Resources) > 0 {
		rs := string(req.Resources)
		resourcesJSON = &rs
	}

	if err := s.db.UpdateHeartbeat(id, resourcesJSON); err == sql.ErrNoRows {
		s.writeError(w, http.StatusNotFound, "host not found")
		return
	} else if err != nil {
		s.logger.Error("failed to update heartbeat", "error", err, "host_id", id)
		s.writeError(w, http.StatusInternalServerError, "failed to update heartbeat")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// --- VM handlers ---

func (s *Server) handleListVMs(w http.ResponseWriter, r *http.Request) {
	project := r.URL.Query().Get("project")
	vms, err := s.db.ListVMs(project)
	if err != nil {
		s.logger.Error("failed to list vms", "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to list vms")
		return
	}

	infos := make([]VMInfo, 0, len(vms))
	for _, v := range vms {
		infos = append(infos, vmRowToInfo(&v))
	}

	s.writeJSON(w, http.StatusOK, infos)
}

func (s *Server) handleCreateVM(w http.ResponseWriter, r *http.Request) {
	var req CreateVMRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.State) == 0 {
		s.writeError(w, http.StatusBadRequest, "state is required")
		return
	}

	vmID := generateID()
	stateJSON := string(req.State)

	status := "stopped"
	if err := s.db.CreateVM(vmID, req.Name, req.Project, req.Service, req.HostID, stateJSON, status); err != nil {
		s.logger.Error("failed to create vm", "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to create vm")
		return
	}

	payload := stateJSON
	if err := s.db.AppendSyncLog("vm", vmID, "create", &payload); err != nil {
		s.logger.Error("failed to append sync log", "error", err)
	}

	s.logger.Info("vm created", "vm_id", vmID, "host_id", req.HostID)

	vm, err := s.db.GetVM(vmID)
	if err != nil {
		s.writeJSON(w, http.StatusCreated, map[string]string{"id": vmID})
		return
	}
	s.writeJSON(w, http.StatusCreated, vmRowToInfo(vm))
}

func (s *Server) handleGetVM(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	vm, err := s.db.GetVM(id)
	if err == sql.ErrNoRows {
		s.writeError(w, http.StatusNotFound, "vm not found")
		return
	}
	if err != nil {
		s.logger.Error("failed to get vm", "error", err, "vm_id", id)
		s.writeError(w, http.StatusInternalServerError, "failed to get vm")
		return
	}

	s.writeJSON(w, http.StatusOK, vmRowToInfo(vm))
}

func (s *Server) handleStartVM(w http.ResponseWriter, r *http.Request) {
	s.handleVMStatusChange(w, r, "running")
}

func (s *Server) handleStopVM(w http.ResponseWriter, r *http.Request) {
	s.handleVMStatusChange(w, r, "stopped")
}

func (s *Server) handleVMStatusChange(w http.ResponseWriter, r *http.Request, newStatus string) {
	id := r.PathValue("id")
	vm, err := s.db.GetVM(id)
	if err == sql.ErrNoRows {
		s.writeError(w, http.StatusNotFound, "vm not found")
		return
	}
	if err != nil {
		s.logger.Error("failed to get vm", "error", err, "vm_id", id)
		s.writeError(w, http.StatusInternalServerError, "failed to get vm")
		return
	}

	if err := s.db.UpdateVM(id, vm.StateJSON, newStatus); err != nil {
		s.logger.Error("failed to update vm status", "error", err, "vm_id", id, "status", newStatus)
		s.writeError(w, http.StatusInternalServerError, "failed to update vm status")
		return
	}

	payload, _ := json.Marshal(map[string]string{"status": newStatus})
	payloadStr := string(payload)
	if err := s.db.AppendSyncLog("vm", id, "update", &payloadStr); err != nil {
		s.logger.Error("failed to append sync log", "error", err)
	}

	s.logger.Info("vm status changed", "vm_id", id, "status", newStatus)
	s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleDeleteVM(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.db.DeleteVM(id); err == sql.ErrNoRows {
		s.writeError(w, http.StatusNotFound, "vm not found")
		return
	} else if err != nil {
		s.logger.Error("failed to delete vm", "error", err, "vm_id", id)
		s.writeError(w, http.StatusInternalServerError, "failed to delete vm")
		return
	}

	if err := s.db.AppendSyncLog("vm", id, "delete", nil); err != nil {
		s.logger.Error("failed to append sync log", "error", err)
	}

	s.logger.Info("vm deleted", "vm_id", id)
	w.WriteHeader(http.StatusNoContent)
}

// --- Sync handler ---

func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	sinceStr := r.URL.Query().Get("since")
	var since int64
	if sinceStr != "" {
		var err error
		since, err = strconv.ParseInt(sinceStr, 10, 64)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid since parameter")
			return
		}
	}

	entries, err := s.db.ReadSyncLogSince(since)
	if err != nil {
		s.logger.Error("failed to read sync log", "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to read sync log")
		return
	}
	if entries == nil {
		entries = []SyncLogEntry{}
	}

	s.writeJSON(w, http.StatusOK, SyncResponse{Entries: entries})
}

// --- Token handler ---

func (s *Server) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	var req CreateTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Allow empty body — use defaults
		req = CreateTokenRequest{}
	}

	expiry := DefaultTokenExpiry
	if req.ExpiryHours > 0 {
		expiry = time.Duration(req.ExpiryHours) * time.Hour
	}

	rawToken, expiresAt, err := s.db.GenerateToken(expiry)
	if err != nil {
		s.logger.Error("failed to generate token", "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	s.logger.Info("token created", "expires_at", expiresAt)
	s.writeJSON(w, http.StatusCreated, CreateTokenResponse{
		Token:     rawToken,
		ExpiresAt: expiresAt,
	})
}

// --- Status handler ---

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	hostCount, err := s.db.HostCount()
	if err != nil {
		s.logger.Error("failed to count hosts", "error", err)
		hostCount = 0
	}
	vmCount, err := s.db.VMCount()
	if err != nil {
		s.logger.Error("failed to count vms", "error", err)
		vmCount = 0
	}

	s.writeJSON(w, http.StatusOK, StatusResponse{
		OK:      true,
		Version: Version,
		Hosts:   hostCount,
		VMs:     vmCount,
	})
}

// --- Helpers ---

func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		s.logger.Error("failed to encode response", "error", err)
	}
}

func (s *Server) writeError(w http.ResponseWriter, status int, msg string) {
	s.writeJSON(w, status, ErrorResponse{Error: msg})
}

func (s *Server) hostRowToInfo(h *HostRow) (*HostInfo, error) {
	info := &HostInfo{
		ID:        h.ID,
		Name:      h.Name,
		Address:   h.Address,
		Status:    h.Status,
		CreatedAt: h.CreatedAt,
	}
	if h.LastHeartbeat.Valid {
		t := h.LastHeartbeat.Time
		info.LastHeartbeat = &t
	}
	if h.ResourcesJSON.Valid {
		info.Resources = json.RawMessage(h.ResourcesJSON.String)
	}

	vmCount, err := s.db.VMCountForHost(h.ID)
	if err != nil {
		return nil, fmt.Errorf("count vms for host %s: %w", h.ID, err)
	}
	info.VMCount = vmCount

	return info, nil
}

func vmRowToInfo(v *VMRow) VMInfo {
	info := VMInfo{
		ID:        v.ID,
		State:     json.RawMessage(v.StateJSON),
		Status:    v.Status,
		CreatedAt: v.CreatedAt,
		UpdatedAt: v.UpdatedAt,
	}
	if v.Name.Valid {
		info.Name = v.Name.String
	}
	if v.Project.Valid {
		info.Project = v.Project.String
	}
	if v.Service.Valid {
		info.Service = v.Service.String
	}
	if v.HostID.Valid {
		info.HostID = v.HostID.String
	}
	return info
}

// generateID creates a random 16-byte hex string (32 chars) for use as an ID.
func generateID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}
