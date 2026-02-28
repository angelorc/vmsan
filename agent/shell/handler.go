package shell

import (
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"path/filepath"

	"github.com/gorilla/websocket"
)

// allowedShells is the set of shells that can be spawned via the shell query param.
var allowedShells = map[string]bool{
	"/bin/sh":       true,
	"/bin/bash":     true,
	"/bin/ash":      true,
	"/bin/zsh":      true,
	"/usr/bin/bash": true,
	"/usr/bin/zsh":  true,
	"/usr/bin/fish": true,
}

// Handler provides HTTP/WebSocket endpoints for the shell subsystem.
type Handler struct {
	manager    *SessionManager
	tokenBytes []byte
	upgrader   websocket.Upgrader
	logger     *slog.Logger
}

// NewHandler creates a new shell Handler with the given auth token and logger.
func NewHandler(token string, logger *slog.Logger) *Handler {
	return &Handler{
		manager:    NewSessionManager(logger),
		tokenBytes: []byte(token),
		upgrader:   websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }},
		logger:     logger,
	}
}

// Register adds shell routes to the given ServeMux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /ws/shell", h.handleNewSession)
	mux.HandleFunc("GET /ws/shell/{sessionId}", h.handleAttach)
	mux.Handle("GET /shell/sessions", h.authWrap(http.HandlerFunc(h.handleListSessions)))
	mux.Handle("POST /shell/sessions/{sessionId}/kill", h.authWrap(http.HandlerFunc(h.handleKillSession)))
}

// handleNewSession creates a new PTY session and attaches the caller as the
// first WebSocket subscriber.
func (h *Handler) handleNewSession(w http.ResponseWriter, r *http.Request) {
	if !h.checkQueryToken(r) {
		http.Error(w, `{"error":"invalid token"}`, http.StatusForbidden)
		return
	}

	shell := r.URL.Query().Get("shell")
	if shell == "" {
		shell = "/bin/bash"
	}

	// Validate the shell path: must be absolute, clean, and in the allowlist.
	cleaned := filepath.Clean(shell)
	if !allowedShells[cleaned] {
		http.Error(w, `{"error":"shell not allowed"}`, http.StatusBadRequest)
		return
	}
	shell = cleaned

	session, err := h.manager.CreateSession(shell)
	if err != nil {
		if err.Error() == "max sessions reached" {
			http.Error(w, `{"error":"too many concurrent sessions"}`, http.StatusTooManyRequests)
		} else {
			http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		}
		return
	}

	h.logger.Info("shell.session.created",
		"session_id", session.ID,
		"shell", shell,
		"remote_addr", r.RemoteAddr,
	)

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Error("websocket upgrade", "error", err)
		session.destroy() // clean up the just-created session
		return
	}

	// Send session metadata as text frame.
	meta, _ := json.Marshal(map[string]string{"sessionId": session.ID})
	conn.WriteMessage(websocket.TextMessage, meta)

	h.logger.Info("shell.subscriber.connected",
		"session_id", session.ID,
		"remote_addr", r.RemoteAddr,
	)

	_, doneCh, err := session.AddSubscriber(conn)
	if err != nil {
		h.logger.Error("add subscriber", "error", err)
		conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseInternalServerErr, err.Error()))
		conn.Close()
		return
	}

	// Block until this subscriber is removed (read/write pump exit).
	<-doneCh

	h.logger.Info("shell.subscriber.disconnected",
		"session_id", session.ID,
		"remote_addr", r.RemoteAddr,
	)
}

// handleAttach attaches the caller to an existing session as a new subscriber.
func (h *Handler) handleAttach(w http.ResponseWriter, r *http.Request) {
	if !h.checkQueryToken(r) {
		http.Error(w, `{"error":"invalid token"}`, http.StatusForbidden)
		return
	}

	sessionId := r.PathValue("sessionId")
	session := h.manager.GetSession(sessionId)
	if session == nil {
		http.Error(w, `{"error":"session not found"}`, http.StatusNotFound)
		return
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Error("websocket upgrade", "error", err)
		return
	}

	h.logger.Info("shell.subscriber.connected",
		"session_id", sessionId,
		"remote_addr", r.RemoteAddr,
		"attach", true,
	)

	_, doneCh, err := session.AddSubscriber(conn)
	if err != nil {
		conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseInternalServerErr, err.Error()))
		conn.Close()
		return
	}

	<-doneCh

	h.logger.Info("shell.subscriber.disconnected",
		"session_id", sessionId,
		"remote_addr", r.RemoteAddr,
	)
}

// handleListSessions returns JSON info for all active sessions.
func (h *Handler) handleListSessions(w http.ResponseWriter, r *http.Request) {
	sessions := h.manager.ListSessions()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sessions)
}

// handleKillSession destroys a session by ID.
func (h *Handler) handleKillSession(w http.ResponseWriter, r *http.Request) {
	sessionId := r.PathValue("sessionId")
	if err := h.manager.KillSession(sessionId); err != nil {
		http.Error(w, `{"error":"session not found"}`, http.StatusNotFound)
		return
	}
	h.logger.Info("shell.session.killed",
		"session_id", sessionId,
	)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok":true}`))
}

// checkQueryToken validates the token query parameter using constant-time
// comparison.
func (h *Handler) checkQueryToken(r *http.Request) bool {
	provided := []byte(r.URL.Query().Get("token"))
	return subtle.ConstantTimeCompare(provided, h.tokenBytes) == 1
}

// authWrap wraps an http.Handler with Bearer token authentication.
func (h *Handler) authWrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if len(auth) < 8 || auth[:7] != "Bearer " {
			http.Error(w, `{"error":"missing authorization"}`, http.StatusUnauthorized)
			return
		}
		provided := []byte(auth[7:])
		if subtle.ConstantTimeCompare(provided, h.tokenBytes) != 1 {
			http.Error(w, `{"error":"invalid token"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
