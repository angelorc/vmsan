package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS vms (
    id TEXT PRIMARY KEY,
    name TEXT,
    project TEXT,
    service TEXT,
    host_id TEXT,
    state_json TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'stopped',
    deploy_hash TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS hosts (
    id TEXT PRIMARY KEY,
    name TEXT UNIQUE NOT NULL,
    address TEXT NOT NULL,
    public_key TEXT,
    status TEXT NOT NULL DEFAULT 'pending',
    resources_json TEXT,
    last_heartbeat DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS projects (
    id TEXT PRIMARY KEY,
    name TEXT UNIQUE NOT NULL,
    config_json TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS mesh_allocations (
    vm_id TEXT PRIMARY KEY,
    mesh_ip TEXT UNIQUE NOT NULL,
    project TEXT NOT NULL,
    service TEXT
);

CREATE TABLE IF NOT EXISTS sync_log (
    version INTEGER PRIMARY KEY AUTOINCREMENT,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    operation TEXT NOT NULL,
    payload_json TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS tokens (
    token_hash TEXT PRIMARY KEY,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    expires_at DATETIME NOT NULL,
    used_at DATETIME,
    used_by TEXT
);

CREATE INDEX IF NOT EXISTS idx_vms_project ON vms(project);
CREATE INDEX IF NOT EXISTS idx_vms_host ON vms(host_id);
CREATE INDEX IF NOT EXISTS idx_vms_status ON vms(status);
CREATE INDEX IF NOT EXISTS idx_sync_log_version ON sync_log(version);
CREATE INDEX IF NOT EXISTS idx_mesh_project ON mesh_allocations(project);
`

// Store wraps a SQLite database for server state.
type Store struct {
	db *sql.DB
}

// NewStore opens or creates a SQLite database at dbPath and initializes the schema.
func NewStore(dbPath string) (*Store, error) {
	// Ensure parent directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize schema: %w", err)
	}

	return &Store{db: db}, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// --- Host operations ---

// HostRow represents a host record from the database.
type HostRow struct {
	ID            string
	Name          string
	Address       string
	PublicKey     sql.NullString
	Status        string
	ResourcesJSON sql.NullString
	LastHeartbeat sql.NullTime
	CreatedAt     time.Time
}

// CreateHost inserts a new host record.
func (s *Store) CreateHost(id, name, address, publicKey, status string) error {
	var pk sql.NullString
	if publicKey != "" {
		pk = sql.NullString{String: publicKey, Valid: true}
	}
	_, err := s.db.Exec(
		`INSERT INTO hosts (id, name, address, public_key, status, created_at)
		 VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		id, name, address, pk, status,
	)
	if err != nil {
		return fmt.Errorf("create host: %w", err)
	}
	return nil
}

// GetHost retrieves a host by ID.
func (s *Store) GetHost(id string) (*HostRow, error) {
	row := s.db.QueryRow("SELECT id, name, address, public_key, status, resources_json, last_heartbeat, created_at FROM hosts WHERE id = ?", id)
	return scanHost(row)
}

// ListHosts returns all host records.
func (s *Store) ListHosts() ([]HostRow, error) {
	rows, err := s.db.Query("SELECT id, name, address, public_key, status, resources_json, last_heartbeat, created_at FROM hosts ORDER BY created_at ASC")
	if err != nil {
		return nil, fmt.Errorf("list hosts: %w", err)
	}
	defer rows.Close()

	var hosts []HostRow
	for rows.Next() {
		h, err := scanHostRows(rows)
		if err != nil {
			return nil, err
		}
		hosts = append(hosts, *h)
	}
	return hosts, rows.Err()
}

// DeleteHost removes a host by ID.
func (s *Store) DeleteHost(id string) error {
	result, err := s.db.Exec("DELETE FROM hosts WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete host: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// UpdateHeartbeat updates a host's last heartbeat time and optionally its resources.
func (s *Store) UpdateHeartbeat(id string, resourcesJSON *string) error {
	var result sql.Result
	var err error
	if resourcesJSON != nil {
		result, err = s.db.Exec(
			"UPDATE hosts SET last_heartbeat = CURRENT_TIMESTAMP, resources_json = ?, status = 'active' WHERE id = ?",
			*resourcesJSON, id,
		)
	} else {
		result, err = s.db.Exec(
			"UPDATE hosts SET last_heartbeat = CURRENT_TIMESTAMP, status = 'active' WHERE id = ?",
			id,
		)
	}
	if err != nil {
		return fmt.Errorf("update heartbeat: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// --- VM operations ---

// VMRow represents a VM record from the database.
type VMRow struct {
	ID         string
	Name       sql.NullString
	Project    sql.NullString
	Service    sql.NullString
	HostID     sql.NullString
	StateJSON  string
	Status     string
	DeployHash sql.NullString
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// CreateVM inserts a new VM record.
func (s *Store) CreateVM(id, name, project, service, hostID, stateJSON, status string) error {
	_, err := s.db.Exec(
		`INSERT INTO vms (id, name, project, service, host_id, state_json, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		id, nullStr(name), nullStr(project), nullStr(service), nullStr(hostID), stateJSON, status,
	)
	if err != nil {
		return fmt.Errorf("create vm: %w", err)
	}
	return nil
}

// GetVM retrieves a VM by ID.
func (s *Store) GetVM(id string) (*VMRow, error) {
	row := s.db.QueryRow(
		"SELECT id, name, project, service, host_id, state_json, status, deploy_hash, created_at, updated_at FROM vms WHERE id = ?",
		id,
	)
	return scanVM(row)
}

// ListVMs returns VMs, optionally filtered by project.
func (s *Store) ListVMs(project string) ([]VMRow, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if project != "" {
		rows, err = s.db.Query(
			"SELECT id, name, project, service, host_id, state_json, status, deploy_hash, created_at, updated_at FROM vms WHERE project = ? ORDER BY created_at ASC",
			project,
		)
	} else {
		rows, err = s.db.Query(
			"SELECT id, name, project, service, host_id, state_json, status, deploy_hash, created_at, updated_at FROM vms ORDER BY created_at ASC",
		)
	}
	if err != nil {
		return nil, fmt.Errorf("list vms: %w", err)
	}
	defer rows.Close()

	var vms []VMRow
	for rows.Next() {
		v, err := scanVMRows(rows)
		if err != nil {
			return nil, err
		}
		vms = append(vms, *v)
	}
	return vms, rows.Err()
}

// UpdateVM updates a VM's state, status, and timestamp.
func (s *Store) UpdateVM(id, stateJSON, status string) error {
	result, err := s.db.Exec(
		"UPDATE vms SET state_json = ?, status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		stateJSON, status, id,
	)
	if err != nil {
		return fmt.Errorf("update vm: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteVM removes a VM by ID and its mesh allocation.
func (s *Store) DeleteVM(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM mesh_allocations WHERE vm_id = ?", id); err != nil {
		return fmt.Errorf("delete mesh allocation: %w", err)
	}
	result, err := tx.Exec("DELETE FROM vms WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete vm: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return tx.Commit()
}

// --- Sync log ---

// SyncLogEntry represents a sync log record.
type SyncLogEntry struct {
	Version     int64           `json:"version"`
	EntityType  string          `json:"entity_type"`
	EntityID    string          `json:"entity_id"`
	Operation   string          `json:"operation"`
	PayloadJSON json.RawMessage `json:"payload,omitempty"`
	CreatedAt   string          `json:"created_at,omitempty"`
}

// AppendSyncLog adds an entry to the sync log.
func (s *Store) AppendSyncLog(entityType, entityID, operation string, payloadJSON *string) error {
	_, err := s.db.Exec(
		"INSERT INTO sync_log (entity_type, entity_id, operation, payload_json) VALUES (?, ?, ?, ?)",
		entityType, entityID, operation, payloadJSON,
	)
	if err != nil {
		return fmt.Errorf("append sync log: %w", err)
	}
	return nil
}

// ReadSyncLogSince returns all sync log entries with version > since.
func (s *Store) ReadSyncLogSince(since int64) ([]SyncLogEntry, error) {
	rows, err := s.db.Query(
		"SELECT version, entity_type, entity_id, operation, payload_json, created_at FROM sync_log WHERE version > ? ORDER BY version ASC",
		since,
	)
	if err != nil {
		return nil, fmt.Errorf("read sync log: %w", err)
	}
	defer rows.Close()

	var entries []SyncLogEntry
	for rows.Next() {
		var e SyncLogEntry
		var payload sql.NullString
		var createdAt sql.NullString
		if err := rows.Scan(&e.Version, &e.EntityType, &e.EntityID, &e.Operation, &payload, &createdAt); err != nil {
			return nil, fmt.Errorf("scan sync log: %w", err)
		}
		if payload.Valid {
			e.PayloadJSON = json.RawMessage(payload.String)
		}
		if createdAt.Valid {
			e.CreatedAt = createdAt.String
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// --- Counts ---

// HostCount returns the number of hosts.
func (s *Store) HostCount() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM hosts").Scan(&count)
	return count, err
}

// VMCount returns the number of VMs.
func (s *Store) VMCount() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM vms").Scan(&count)
	return count, err
}

// VMCountForHost returns the number of VMs on a given host.
func (s *Store) VMCountForHost(hostID string) (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM vms WHERE host_id = ?", hostID).Scan(&count)
	return count, err
}

// --- Scan helpers ---

type scanner interface {
	Scan(dest ...any) error
}

func scanHost(row scanner) (*HostRow, error) {
	var h HostRow
	if err := row.Scan(&h.ID, &h.Name, &h.Address, &h.PublicKey, &h.Status, &h.ResourcesJSON, &h.LastHeartbeat, &h.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, err
		}
		return nil, fmt.Errorf("scan host: %w", err)
	}
	return &h, nil
}

func scanHostRows(rows *sql.Rows) (*HostRow, error) {
	return scanHost(rows)
}

func scanVM(row scanner) (*VMRow, error) {
	var v VMRow
	if err := row.Scan(&v.ID, &v.Name, &v.Project, &v.Service, &v.HostID, &v.StateJSON, &v.Status, &v.DeployHash, &v.CreatedAt, &v.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, err
		}
		return nil, fmt.Errorf("scan vm: %w", err)
	}
	return &v, nil
}

func scanVMRows(rows *sql.Rows) (*VMRow, error) {
	return scanVM(rows)
}

func nullStr(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}
