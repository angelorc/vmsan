package server

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	srv, err := New("127.0.0.1:0", dbPath, logger)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	t.Cleanup(func() { srv.Close() })
	return srv
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

// --- Store tests ---

func TestStoreHostCRUD(t *testing.T) {
	store := newTestStore(t)

	// Create
	err := store.CreateHost("h1", "host-1", "10.0.0.1", "pk1", "active")
	if err != nil {
		t.Fatalf("CreateHost: %v", err)
	}

	// Get
	h, err := store.GetHost("h1")
	if err != nil {
		t.Fatalf("GetHost: %v", err)
	}
	if h.Name != "host-1" || h.Address != "10.0.0.1" || h.Status != "active" {
		t.Errorf("unexpected host: %+v", h)
	}

	// List
	hosts, err := store.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts: %v", err)
	}
	if len(hosts) != 1 {
		t.Errorf("expected 1 host, got %d", len(hosts))
	}

	// Heartbeat
	resources := `{"cpus":4,"memoryMB":8192}`
	err = store.UpdateHeartbeat("h1", &resources)
	if err != nil {
		t.Fatalf("UpdateHeartbeat: %v", err)
	}
	h, _ = store.GetHost("h1")
	if !h.LastHeartbeat.Valid {
		t.Error("expected last_heartbeat to be set")
	}
	if !h.ResourcesJSON.Valid || h.ResourcesJSON.String != resources {
		t.Errorf("unexpected resources: %v", h.ResourcesJSON)
	}

	// Delete
	err = store.DeleteHost("h1")
	if err != nil {
		t.Fatalf("DeleteHost: %v", err)
	}
	_, err = store.GetHost("h1")
	if err != sql.ErrNoRows {
		t.Errorf("expected ErrNoRows, got: %v", err)
	}
}

func TestStoreHostDeleteNotFound(t *testing.T) {
	store := newTestStore(t)
	err := store.DeleteHost("nonexistent")
	if err != sql.ErrNoRows {
		t.Errorf("expected ErrNoRows, got: %v", err)
	}
}

func TestStoreVMCRUD(t *testing.T) {
	store := newTestStore(t)

	stateJSON := `{"kernel":"vmlinux","rootfs":"rootfs.ext4"}`

	// Create
	err := store.CreateVM("vm1", "my-vm", "proj1", "web", "h1", stateJSON, "stopped")
	if err != nil {
		t.Fatalf("CreateVM: %v", err)
	}

	// Get
	vm, err := store.GetVM("vm1")
	if err != nil {
		t.Fatalf("GetVM: %v", err)
	}
	if vm.Status != "stopped" || vm.StateJSON != stateJSON {
		t.Errorf("unexpected vm: %+v", vm)
	}

	// List all
	vms, err := store.ListVMs("")
	if err != nil {
		t.Fatalf("ListVMs: %v", err)
	}
	if len(vms) != 1 {
		t.Errorf("expected 1 vm, got %d", len(vms))
	}

	// List by project
	vms, err = store.ListVMs("proj1")
	if err != nil {
		t.Fatalf("ListVMs(proj1): %v", err)
	}
	if len(vms) != 1 {
		t.Errorf("expected 1 vm, got %d", len(vms))
	}
	vms, err = store.ListVMs("nonexistent")
	if err != nil {
		t.Fatalf("ListVMs(nonexistent): %v", err)
	}
	if len(vms) != 0 {
		t.Errorf("expected 0 vms, got %d", len(vms))
	}

	// Update
	err = store.UpdateVM("vm1", stateJSON, "running")
	if err != nil {
		t.Fatalf("UpdateVM: %v", err)
	}
	vm, _ = store.GetVM("vm1")
	if vm.Status != "running" {
		t.Errorf("expected running, got %s", vm.Status)
	}

	// Delete
	err = store.DeleteVM("vm1")
	if err != nil {
		t.Fatalf("DeleteVM: %v", err)
	}
	_, err = store.GetVM("vm1")
	if err != sql.ErrNoRows {
		t.Errorf("expected ErrNoRows, got: %v", err)
	}
}

func TestStoreSyncLog(t *testing.T) {
	store := newTestStore(t)

	payload := `{"name":"host-1"}`
	err := store.AppendSyncLog("host", "h1", "create", &payload)
	if err != nil {
		t.Fatalf("AppendSyncLog: %v", err)
	}

	err = store.AppendSyncLog("vm", "vm1", "create", nil)
	if err != nil {
		t.Fatalf("AppendSyncLog: %v", err)
	}

	// Read all since 0
	entries, err := store.ReadSyncLogSince(0)
	if err != nil {
		t.Fatalf("ReadSyncLogSince: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].EntityType != "host" || entries[0].Operation != "create" {
		t.Errorf("unexpected entry[0]: %+v", entries[0])
	}
	if entries[1].EntityType != "vm" {
		t.Errorf("unexpected entry[1]: %+v", entries[1])
	}

	// Read since version 1
	entries, err = store.ReadSyncLogSince(entries[0].Version)
	if err != nil {
		t.Fatalf("ReadSyncLogSince: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(entries))
	}
}

func TestStoreCounts(t *testing.T) {
	store := newTestStore(t)

	hc, _ := store.HostCount()
	vc, _ := store.VMCount()
	if hc != 0 || vc != 0 {
		t.Errorf("expected 0 counts, got hosts=%d vms=%d", hc, vc)
	}

	store.CreateHost("h1", "host-1", "10.0.0.1", "", "active")
	store.CreateVM("vm1", "vm1", "p", "", "h1", "{}", "stopped")

	hc, _ = store.HostCount()
	vc, _ = store.VMCount()
	if hc != 1 || vc != 1 {
		t.Errorf("expected 1 each, got hosts=%d vms=%d", hc, vc)
	}

	vch, _ := store.VMCountForHost("h1")
	if vch != 1 {
		t.Errorf("expected 1 vm for host, got %d", vch)
	}
}

// --- Token tests ---

func TestTokenGenerateAndConsume(t *testing.T) {
	store := newTestStore(t)

	rawToken, expiresAt, err := store.GenerateToken(1 * time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	if len(rawToken) != 64 { // 32 bytes = 64 hex chars
		t.Errorf("expected 64 char token, got %d", len(rawToken))
	}
	if expiresAt.Before(time.Now()) {
		t.Error("token already expired")
	}

	// Consume
	err = store.ValidateAndConsumeToken(rawToken, "host-1")
	if err != nil {
		t.Fatalf("ValidateAndConsumeToken: %v", err)
	}

	// Second use should fail
	err = store.ValidateAndConsumeToken(rawToken, "host-2")
	if err == nil {
		t.Error("expected error on second use")
	}
}

func TestTokenInvalid(t *testing.T) {
	store := newTestStore(t)

	err := store.ValidateAndConsumeToken("nonexistent", "host-1")
	if err == nil {
		t.Error("expected error for invalid token")
	}
}

func TestTokenExpired(t *testing.T) {
	store := newTestStore(t)

	// Generate with very short expiry, then manually expire it
	rawToken, _, err := store.GenerateToken(1 * time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	// Manually set expires_at to the past
	hash := hashToken(rawToken)
	_, err = store.db.Exec("UPDATE tokens SET expires_at = ? WHERE token_hash = ?",
		time.Now().UTC().Add(-1*time.Hour), hash)
	if err != nil {
		t.Fatalf("manual expire: %v", err)
	}

	err = store.ValidateAndConsumeToken(rawToken, "host-1")
	if err == nil {
		t.Error("expected error for expired token")
	}
}

func TestTokenList(t *testing.T) {
	store := newTestStore(t)

	store.GenerateToken(1 * time.Hour)
	store.GenerateToken(2 * time.Hour)

	tokens, err := store.ListTokens()
	if err != nil {
		t.Fatalf("ListTokens: %v", err)
	}
	if len(tokens) != 2 {
		t.Errorf("expected 2 tokens, got %d", len(tokens))
	}
}

func TestTokenRevoke(t *testing.T) {
	store := newTestStore(t)

	rawToken, _, _ := store.GenerateToken(1 * time.Hour)
	hash := hashToken(rawToken)

	err := store.RevokeToken(hash)
	if err != nil {
		t.Fatalf("RevokeToken: %v", err)
	}

	err = store.ValidateAndConsumeToken(rawToken, "host-1")
	if err == nil {
		t.Error("expected error after revoke")
	}
}

// --- HTTP handler tests ---

func TestHandlerStatus(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest("GET", "/api/v1/status", nil)
	w := httptest.NewRecorder()
	srv.srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp StatusResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if !resp.OK || resp.Version != Version {
		t.Errorf("unexpected status: %+v", resp)
	}
}

func TestHandlerCreateToken(t *testing.T) {
	srv := newTestServer(t)

	body := bytes.NewBufferString(`{"expiry_hours": 2}`)
	req := httptest.NewRequest("POST", "/api/v1/tokens", body)
	w := httptest.NewRecorder()
	srv.srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp CreateTokenResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Token) != 64 {
		t.Errorf("expected 64-char token, got %d", len(resp.Token))
	}
}

func TestHandlerJoinFlow(t *testing.T) {
	srv := newTestServer(t)

	// Create token
	req := httptest.NewRequest("POST", "/api/v1/tokens", bytes.NewBufferString("{}"))
	w := httptest.NewRecorder()
	srv.srv.Handler.ServeHTTP(w, req)
	var tokenResp CreateTokenResponse
	json.NewDecoder(w.Body).Decode(&tokenResp)

	// Join with token
	joinBody, _ := json.Marshal(JoinRequest{
		Token:   tokenResp.Token,
		Name:    "node-1",
		Address: "10.0.0.1:6444",
	})
	req = httptest.NewRequest("POST", "/api/v1/hosts/join", bytes.NewBuffer(joinBody))
	w = httptest.NewRecorder()
	srv.srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("join expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var joinResp JoinResponse
	json.NewDecoder(w.Body).Decode(&joinResp)
	if joinResp.HostID == "" {
		t.Error("expected host_id in join response")
	}

	// List hosts should show 1
	req = httptest.NewRequest("GET", "/api/v1/hosts", nil)
	w = httptest.NewRecorder()
	srv.srv.Handler.ServeHTTP(w, req)

	var hosts []HostInfo
	json.NewDecoder(w.Body).Decode(&hosts)
	if len(hosts) != 1 {
		t.Fatalf("expected 1 host, got %d", len(hosts))
	}
	if hosts[0].Name != "node-1" {
		t.Errorf("expected node-1, got %s", hosts[0].Name)
	}

	// Second join with same token should fail
	req = httptest.NewRequest("POST", "/api/v1/hosts/join", bytes.NewBuffer(joinBody))
	w = httptest.NewRecorder()
	srv.srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for reused token, got %d", w.Code)
	}
}

func TestHandlerJoinInvalidToken(t *testing.T) {
	srv := newTestServer(t)

	joinBody, _ := json.Marshal(JoinRequest{
		Token:   "invalid-token",
		Name:    "node-1",
		Address: "10.0.0.1:6444",
	})
	req := httptest.NewRequest("POST", "/api/v1/hosts/join", bytes.NewBuffer(joinBody))
	w := httptest.NewRecorder()
	srv.srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandlerJoinMissingFields(t *testing.T) {
	srv := newTestServer(t)

	// Missing token
	joinBody, _ := json.Marshal(JoinRequest{Name: "n", Address: "a"})
	req := httptest.NewRequest("POST", "/api/v1/hosts/join", bytes.NewBuffer(joinBody))
	w := httptest.NewRecorder()
	srv.srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing token, got %d", w.Code)
	}

	// Missing name
	joinBody, _ = json.Marshal(JoinRequest{Token: "t", Address: "a"})
	req = httptest.NewRequest("POST", "/api/v1/hosts/join", bytes.NewBuffer(joinBody))
	w = httptest.NewRecorder()
	srv.srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing name, got %d", w.Code)
	}
}

func TestHandlerHostDetail(t *testing.T) {
	srv := newTestServer(t)

	// Create a host directly in the store
	srv.db.CreateHost("h1", "host-1", "10.0.0.1", "", "active")

	req := httptest.NewRequest("GET", "/api/v1/hosts/h1", nil)
	w := httptest.NewRecorder()
	srv.srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var info HostInfo
	json.NewDecoder(w.Body).Decode(&info)
	if info.Name != "host-1" {
		t.Errorf("expected host-1, got %s", info.Name)
	}
}

func TestHandlerHostNotFound(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest("GET", "/api/v1/hosts/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandlerDeleteHost(t *testing.T) {
	srv := newTestServer(t)

	srv.db.CreateHost("h1", "host-1", "10.0.0.1", "", "active")

	req := httptest.NewRequest("DELETE", "/api/v1/hosts/h1", nil)
	w := httptest.NewRecorder()
	srv.srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}

	// Should be gone
	req = httptest.NewRequest("GET", "/api/v1/hosts/h1", nil)
	w = httptest.NewRecorder()
	srv.srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", w.Code)
	}
}

func TestHandlerHeartbeat(t *testing.T) {
	srv := newTestServer(t)

	srv.db.CreateHost("h1", "host-1", "10.0.0.1", "", "pending")

	body := bytes.NewBufferString(`{"resources":{"cpus":8,"memoryMB":16384}}`)
	req := httptest.NewRequest("PUT", "/api/v1/hosts/h1/heartbeat", body)
	w := httptest.NewRecorder()
	srv.srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify host is now active with resources
	h, _ := srv.db.GetHost("h1")
	if h.Status != "active" {
		t.Errorf("expected active, got %s", h.Status)
	}
	if !h.ResourcesJSON.Valid {
		t.Error("expected resources to be set")
	}
}

func TestHandlerVMFlow(t *testing.T) {
	srv := newTestServer(t)

	// Create VM
	createBody, _ := json.Marshal(CreateVMRequest{
		Name:    "web-1",
		Project: "myproj",
		HostID:  "h1",
		State:   json.RawMessage(`{"kernel":"vmlinux"}`),
	})
	req := httptest.NewRequest("POST", "/api/v1/vms", bytes.NewBuffer(createBody))
	w := httptest.NewRecorder()
	srv.srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("create vm expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var vmInfo VMInfo
	json.NewDecoder(w.Body).Decode(&vmInfo)
	vmID := vmInfo.ID
	if vmID == "" {
		t.Fatal("expected vm ID")
	}
	if vmInfo.Status != "stopped" {
		t.Errorf("expected stopped, got %s", vmInfo.Status)
	}

	// Get VM
	req = httptest.NewRequest("GET", "/api/v1/vms/"+vmID, nil)
	w = httptest.NewRecorder()
	srv.srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("get vm expected 200, got %d", w.Code)
	}

	// List VMs
	req = httptest.NewRequest("GET", "/api/v1/vms", nil)
	w = httptest.NewRecorder()
	srv.srv.Handler.ServeHTTP(w, req)
	var vms []VMInfo
	json.NewDecoder(w.Body).Decode(&vms)
	if len(vms) != 1 {
		t.Errorf("expected 1 vm, got %d", len(vms))
	}

	// List VMs by project
	req = httptest.NewRequest("GET", "/api/v1/vms?project=myproj", nil)
	w = httptest.NewRecorder()
	srv.srv.Handler.ServeHTTP(w, req)
	json.NewDecoder(w.Body).Decode(&vms)
	if len(vms) != 1 {
		t.Errorf("expected 1 vm for project, got %d", len(vms))
	}

	// Start VM
	req = httptest.NewRequest("PUT", "/api/v1/vms/"+vmID+"/start", nil)
	w = httptest.NewRecorder()
	srv.srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("start vm expected 200, got %d", w.Code)
	}
	vm, _ := srv.db.GetVM(vmID)
	if vm.Status != "running" {
		t.Errorf("expected running, got %s", vm.Status)
	}

	// Stop VM
	req = httptest.NewRequest("PUT", "/api/v1/vms/"+vmID+"/stop", nil)
	w = httptest.NewRecorder()
	srv.srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("stop vm expected 200, got %d", w.Code)
	}
	vm, _ = srv.db.GetVM(vmID)
	if vm.Status != "stopped" {
		t.Errorf("expected stopped, got %s", vm.Status)
	}

	// Delete VM
	req = httptest.NewRequest("DELETE", "/api/v1/vms/"+vmID, nil)
	w = httptest.NewRecorder()
	srv.srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("delete vm expected 204, got %d", w.Code)
	}

	// Should be gone
	req = httptest.NewRequest("GET", "/api/v1/vms/"+vmID, nil)
	w = httptest.NewRecorder()
	srv.srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandlerSync(t *testing.T) {
	srv := newTestServer(t)

	payload := `{"key":"value"}`
	srv.db.AppendSyncLog("host", "h1", "create", &payload)
	srv.db.AppendSyncLog("vm", "vm1", "create", nil)

	// All entries
	req := httptest.NewRequest("GET", "/api/v1/sync?since=0", nil)
	w := httptest.NewRecorder()
	srv.srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp SyncResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(resp.Entries))
	}

	// Since version 1 should return 1
	req = httptest.NewRequest("GET", "/api/v1/sync?since=1", nil)
	w = httptest.NewRecorder()
	srv.srv.Handler.ServeHTTP(w, req)
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Entries) != 1 {
		t.Errorf("expected 1 entry since v1, got %d", len(resp.Entries))
	}
}

func TestHandlerSyncEmpty(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest("GET", "/api/v1/sync", nil)
	w := httptest.NewRecorder()
	srv.srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp SyncResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(resp.Entries))
	}
}

func TestGenerateID(t *testing.T) {
	id1 := generateID()
	id2 := generateID()
	if len(id1) != 32 {
		t.Errorf("expected 32 chars, got %d", len(id1))
	}
	if id1 == id2 {
		t.Error("expected unique IDs")
	}
}

func TestNewStoreCreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "dir")
	dbPath := filepath.Join(dir, "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	store.Close()

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("expected directory to be created")
	}
}
