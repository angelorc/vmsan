package server

import (
	"encoding/json"
	"time"
)

// --- Request types ---

// JoinRequest is the body for POST /api/v1/hosts/join.
type JoinRequest struct {
	Token     string `json:"token"`
	Name      string `json:"name"`
	Address   string `json:"address"`
	PublicKey string `json:"public_key,omitempty"`
}

// HeartbeatRequest is the body for PUT /api/v1/hosts/:id/heartbeat.
type HeartbeatRequest struct {
	Resources json.RawMessage `json:"resources,omitempty"`
}

// CreateVMRequest is the body for POST /api/v1/vms.
type CreateVMRequest struct {
	Name    string          `json:"name"`
	Project string          `json:"project,omitempty"`
	Service string          `json:"service,omitempty"`
	HostID  string          `json:"host_id"`
	State   json.RawMessage `json:"state"`
}

// --- Response types ---

// JoinResponse is returned by POST /api/v1/hosts/join.
type JoinResponse struct {
	HostID string `json:"host_id"`
}

// HostInfo is the JSON representation of a host.
type HostInfo struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	Address       string          `json:"address"`
	Status        string          `json:"status"`
	VMCount       int             `json:"vm_count"`
	Resources     json.RawMessage `json:"resources,omitempty"`
	LastHeartbeat *time.Time      `json:"last_heartbeat,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
}

// VMInfo is the JSON representation of a VM.
type VMInfo struct {
	ID        string          `json:"id"`
	Name      string          `json:"name,omitempty"`
	Project   string          `json:"project,omitempty"`
	Service   string          `json:"service,omitempty"`
	HostID    string          `json:"host_id,omitempty"`
	State     json.RawMessage `json:"state"`
	Status    string          `json:"status"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// StatusResponse is returned by GET /api/v1/status.
type StatusResponse struct {
	OK      bool   `json:"ok"`
	Version string `json:"version"`
	Hosts   int    `json:"hosts"`
	VMs     int    `json:"vms"`
}

// CreateTokenRequest is the body for POST /api/v1/tokens.
type CreateTokenRequest struct {
	ExpiryHours int `json:"expiry_hours,omitempty"`
}

// CreateTokenResponse is returned by POST /api/v1/tokens.
type CreateTokenResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// SyncResponse is returned by GET /api/v1/sync.
type SyncResponse struct {
	Changes []SyncLogEntry `json:"changes"`
	Latest  int64          `json:"latest"`
}

// ErrorResponse is the standard error envelope.
type ErrorResponse struct {
	Error string `json:"error"`
}
