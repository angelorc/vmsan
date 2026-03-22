// Package sync implements a pull-based sync engine that polls the vmsan
// server for state changes and applies them locally on the agent host.
package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// Change represents a single state change from the server.
type Change struct {
	Version    int64           `json:"version"`
	EntityType string          `json:"entity_type"`
	EntityID   string          `json:"entity_id"`
	Operation  string          `json:"operation"`
	Payload    json.RawMessage `json:"payload"`
}

// SyncResponse is the server's response to a sync poll.
type SyncResponse struct {
	Changes []Change `json:"changes"`
	Latest  int64    `json:"latest"`
}

// Handlers contains callbacks for processing sync changes.
type Handlers struct {
	OnVMCreate func(payload json.RawMessage) error
	OnVMUpdate func(payload json.RawMessage) error
	OnVMDelete func(id string) error
}

// Engine polls the server for state changes and dispatches them to handlers.
type Engine struct {
	serverURL    string
	hostID       string
	lastVersion  int64
	pollInterval time.Duration
	maxInterval  time.Duration
	logger       *slog.Logger
	client       *http.Client
	handlers     Handlers
}

// New creates a sync engine that polls the given server for changes.
func New(serverURL, hostID string, handlers Handlers, logger *slog.Logger) *Engine {
	return &Engine{
		serverURL:    serverURL,
		hostID:       hostID,
		pollInterval: 10 * time.Second,
		maxInterval:  5 * time.Minute,
		logger:       logger,
		client:       &http.Client{Timeout: 30 * time.Second},
		handlers:     handlers,
	}
}

// Run starts the polling loop. It blocks until ctx is cancelled.
func (e *Engine) Run(ctx context.Context) error {
	// Do an immediate poll on startup before waiting for the first tick.
	if err := e.poll(ctx); err != nil {
		e.logger.Warn("initial sync poll failed", "error", err)
	}

	currentInterval := e.pollInterval
	timer := time.NewTimer(currentInterval)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
			if err := e.poll(ctx); err != nil {
				e.logger.Warn("sync poll failed", "error", err, "next_retry", currentInterval*2)
				// Exponential backoff on failure, capped at maxInterval.
				currentInterval *= 2
				if currentInterval > e.maxInterval {
					currentInterval = e.maxInterval
				}
			} else {
				// Reset to base interval on success.
				currentInterval = e.pollInterval
			}
			timer.Reset(currentInterval)
		}
	}
}

// poll fetches changes from the server since lastVersion and applies them.
func (e *Engine) poll(ctx context.Context) error {
	url := fmt.Sprintf("%s/api/v1/sync?host_id=%s&since=%d", e.serverURL, e.hostID, e.lastVersion)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}

	var syncResp SyncResponse
	if err := json.NewDecoder(resp.Body).Decode(&syncResp); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	// Apply changes in order.
	for _, change := range syncResp.Changes {
		if err := e.applyChange(change); err != nil {
			e.logger.Error("failed to apply change",
				"version", change.Version,
				"entity_type", change.EntityType,
				"entity_id", change.EntityID,
				"operation", change.Operation,
				"error", err,
			)
			// Continue processing remaining changes; a single failure
			// should not block the entire batch.
		}
	}

	if syncResp.Latest > e.lastVersion {
		e.logger.Debug("sync updated", "previous_version", e.lastVersion, "latest_version", syncResp.Latest)
		e.lastVersion = syncResp.Latest
	}

	return nil
}

// applyChange dispatches a single change to the appropriate handler.
func (e *Engine) applyChange(c Change) error {
	switch c.EntityType {
	case "vm":
		return e.applyVMChange(c)
	default:
		e.logger.Debug("ignoring unknown entity type", "entity_type", c.EntityType)
		return nil
	}
}

// applyVMChange routes VM operations to the registered handlers.
func (e *Engine) applyVMChange(c Change) error {
	switch c.Operation {
	case "create":
		if e.handlers.OnVMCreate != nil {
			return e.handlers.OnVMCreate(c.Payload)
		}
	case "update":
		if e.handlers.OnVMUpdate != nil {
			return e.handlers.OnVMUpdate(c.Payload)
		}
	case "delete":
		if e.handlers.OnVMDelete != nil {
			return e.handlers.OnVMDelete(c.EntityID)
		}
	default:
		e.logger.Debug("ignoring unknown vm operation", "operation", c.Operation)
	}
	return nil
}
