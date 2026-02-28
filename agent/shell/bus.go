package shell

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

const (
	DefaultMaxSubscribers    = 8
	DefaultInactivityTimeout = 60 * time.Second
	subscriberChCap          = 100
	DefaultMaxSessions       = 4
	maxWSReadSize            = 64 * 1024 // 64 KB max incoming WebSocket message
)

// SessionInfo is the exported struct for JSON serialization.
type SessionInfo struct {
	SessionID       string    `json:"sessionId"`
	Shell           string    `json:"shell"`
	CreatedAt       time.Time `json:"createdAt"`
	SubscriberCount int       `json:"subscriberCount"`
}

// subscriber represents one WebSocket connection attached to a session.
type subscriber struct {
	id       string
	writer   *WSWriter
	outCh    chan []byte
	cancel   context.CancelFunc
	doneCh   chan struct{} // closed when subscriber is removed
	doneOnce sync.Once    // ensures doneCh is closed exactly once
}

// Session represents one PTY process with multiple subscribers.
type Session struct {
	ID        string
	Shell     string
	CreatedAt time.Time

	ptmx   *os.File
	cmd    *exec.Cmd
	ctx    context.Context
	cancel context.CancelFunc

	subscribers   map[string]*subscriber
	subscribersMu sync.RWMutex

	buffer    *BufferedOutput
	onDestroy func(id string)

	inactivityTimer *time.Timer
	inactivityMu    sync.Mutex

	destroyed sync.Once
	logger    *slog.Logger
}

// NewSession creates a PTY session, starts the producer and wait loops,
// and arms the inactivity timer.
func NewSession(id, shell string, onDestroy func(string), logger *slog.Logger) (*Session, error) {
	cmd := exec.Command(shell)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, fmt.Errorf("pty start: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	s := &Session{
		ID:          id,
		Shell:       shell,
		CreatedAt:   time.Now(),
		ptmx:        ptmx,
		cmd:         cmd,
		ctx:         ctx,
		cancel:      cancel,
		subscribers: make(map[string]*subscriber),
		buffer:      NewBufferedOutput(),
		onDestroy:   onDestroy,
		logger:      logger.With("sessionId", id),
	}

	s.armInactivityTimer()
	go s.producerLoop()
	go s.waitLoop()

	return s, nil
}

// producerLoop reads PTY stdout in 32KB chunks and fans out to subscribers.
func (s *Session) producerLoop() {
	buf := make([]byte, 32*1024)
	for {
		n, err := s.ptmx.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])

			passthrough, isDirect := s.buffer.Append(chunk)
			if isDirect {
				frame := SerializeData(passthrough)
				s.fanOut(frame)
			}
		}
		if err != nil {
			return
		}
	}
}

// fanOut sends a pre-serialized frame to all subscribers' outCh (non-blocking).
func (s *Session) fanOut(frame []byte) {
	s.subscribersMu.RLock()
	defer s.subscribersMu.RUnlock()
	for _, sub := range s.subscribers {
		select {
		case sub.outCh <- frame:
		default:
			// backpressure: drop frame for slow subscriber
		}
	}
}

// AddSubscriber attaches a WebSocket connection to this session.
// Returns the subscriber ID, a done channel (closed when the subscriber is
// removed), or an error if at max capacity.
func (s *Session) AddSubscriber(conn *websocket.Conn) (string, <-chan struct{}, error) {
	s.subscribersMu.Lock()
	defer s.subscribersMu.Unlock()

	if len(s.subscribers) >= DefaultMaxSubscribers {
		return "", nil, errors.New("max subscribers reached")
	}

	id, err := generateID()
	if err != nil {
		return "", nil, fmt.Errorf("generate subscriber id: %w", err)
	}

	subCtx, subCancel := context.WithCancel(s.ctx)

	sub := &subscriber{
		id:     id,
		writer: NewWSWriter(conn),
		outCh:  make(chan []byte, subscriberChCap),
		cancel: subCancel,
		doneCh: make(chan struct{}),
	}
	s.subscribers[id] = sub

	s.cancelInactivityTimer()

	go s.subscriberWritePump(sub, subCtx)
	go s.subscriberReadPump(sub)

	s.logger.Info("subscriber added", "subscriberId", id, "total", len(s.subscribers))
	return id, sub.doneCh, nil
}

// RemoveSubscriber detaches a subscriber. Safe to call multiple times for the
// same ID (the second call is a no-op). If it was the last subscriber, the
// inactivity timer is armed.
func (s *Session) RemoveSubscriber(id string) {
	s.subscribersMu.Lock()
	sub, ok := s.subscribers[id]
	if !ok {
		s.subscribersMu.Unlock()
		return
	}
	delete(s.subscribers, id)
	remaining := len(s.subscribers)
	s.subscribersMu.Unlock()

	sub.cancel()
	sub.doneOnce.Do(func() {
		close(sub.outCh)
		close(sub.doneCh)
	})

	s.logger.Info("subscriber removed", "subscriberId", id, "remaining", remaining)

	if remaining == 0 {
		s.armInactivityTimer()
	}
}

// subscriberWritePump drains the subscriber's outCh to its WebSocket writer.
func (s *Session) subscriberWritePump(sub *subscriber, ctx context.Context) {
	defer s.RemoveSubscriber(sub.id)
	for {
		select {
		case <-ctx.Done():
			return
		case frame, ok := <-sub.outCh:
			if !ok {
				return
			}
			if err := sub.writer.WriteRaw(frame); err != nil {
				s.logger.Debug("write pump error", "subscriberId", sub.id, "error", err)
				return
			}
		}
	}
}

// subscriberReadPump reads WebSocket messages from a subscriber and processes them.
func (s *Session) subscriberReadPump(sub *subscriber) {
	defer s.RemoveSubscriber(sub.id)
	sub.writer.conn.SetReadLimit(maxWSReadSize)
	for {
		_, data, err := sub.writer.conn.ReadMessage()
		if err != nil {
			return
		}
		msg := ParseMessage(data)
		if msg == nil {
			continue
		}
		switch msg.Type {
		case MsgData:
			if _, err := s.ptmx.Write(msg.Data); err != nil {
				s.logger.Debug("pty write error", "subscriberId", sub.id, "error", err)
				return
			}
		case MsgResize:
			pty.Setsize(s.ptmx, &pty.Winsize{
				Cols: msg.Cols,
				Rows: msg.Rows,
			})
		case MsgReady:
			flushed := s.buffer.MarkReady()
			if len(flushed) > 0 {
				frame := SerializeData(flushed)
				s.fanOut(frame)
			}
		}
	}
}

// waitLoop waits for the shell process to exit and then destroys the session.
func (s *Session) waitLoop() {
	s.cmd.Wait()
	s.destroy()
}

// destroy kills the PTY, cancels all contexts, sends close frames, and
// invokes the onDestroy callback.
func (s *Session) destroy() {
	s.destroyed.Do(func() {
		s.logger.Info("destroying session")

		s.cancelInactivityTimer()

		// Snapshot subscribers BEFORE cancelling the context so the
		// write-pump goroutines haven't exited and removed themselves yet.
		s.subscribersMu.RLock()
		subs := make([]*subscriber, 0, len(s.subscribers))
		for _, sub := range s.subscribers {
			subs = append(subs, sub)
		}
		s.subscribersMu.RUnlock()

		// Send close frames while connections are still live.
		for _, sub := range subs {
			sub.writer.conn.SetWriteDeadline(time.Now().Add(200 * time.Millisecond))
			sub.writer.WriteClose(websocket.CloseNormalClosure, "session destroyed")
		}
		for _, sub := range subs {
			sub.writer.conn.Close()
		}

		// Now cancel the context and tear down the PTY.
		s.cancel()
		s.cmd.Process.Kill()
		s.ptmx.Close()

		if s.onDestroy != nil {
			s.onDestroy(s.ID)
		}
	})
}

// armInactivityTimer starts (or resets) the inactivity timer.
func (s *Session) armInactivityTimer() {
	s.inactivityMu.Lock()
	defer s.inactivityMu.Unlock()
	if s.inactivityTimer != nil {
		s.inactivityTimer.Stop()
	}
	s.inactivityTimer = time.AfterFunc(DefaultInactivityTimeout, func() {
		s.logger.Info("inactivity timeout, destroying session")
		s.destroy()
	})
}

// cancelInactivityTimer stops the inactivity timer if active.
func (s *Session) cancelInactivityTimer() {
	s.inactivityMu.Lock()
	defer s.inactivityMu.Unlock()
	if s.inactivityTimer != nil {
		s.inactivityTimer.Stop()
		s.inactivityTimer = nil
	}
}

// SubscriberCount returns the current number of subscribers.
func (s *Session) SubscriberCount() int {
	s.subscribersMu.RLock()
	defer s.subscribersMu.RUnlock()
	return len(s.subscribers)
}

// Info returns an exported SessionInfo for JSON serialization.
func (s *Session) Info() SessionInfo {
	return SessionInfo{
		SessionID:       s.ID,
		Shell:           s.Shell,
		CreatedAt:       s.CreatedAt,
		SubscriberCount: s.SubscriberCount(),
	}
}

// --- SessionManager ---

// SessionManager maps session IDs to Sessions.
type SessionManager struct {
	sessions map[string]*Session
	mu       sync.RWMutex
	logger   *slog.Logger
}

// NewSessionManager creates a new SessionManager.
func NewSessionManager(logger *slog.Logger) *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*Session),
		logger:   logger,
	}
}

// CreateSession creates a new PTY session with the given shell.
// Enforces DefaultMaxSessions limit.
func (m *SessionManager) CreateSession(shell string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.sessions) >= DefaultMaxSessions {
		return nil, errors.New("max sessions reached")
	}

	id, err := generateID()
	if err != nil {
		return nil, fmt.Errorf("generate session id: %w", err)
	}

	onDestroy := func(sid string) {
		m.mu.Lock()
		delete(m.sessions, sid)
		m.mu.Unlock()
		m.logger.Info("session removed from manager", "sessionId", sid)
	}

	s, err := NewSession(id, shell, onDestroy, m.logger)
	if err != nil {
		return nil, err
	}

	m.sessions[id] = s
	m.logger.Info("session created", "sessionId", id, "shell", shell)
	return s, nil
}

// GetSession returns a session by ID or nil if not found.
func (m *SessionManager) GetSession(id string) *Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[id]
}

// ListSessions returns info for all active sessions.
func (m *SessionManager) ListSessions() []SessionInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	infos := make([]SessionInfo, 0, len(m.sessions))
	for _, s := range m.sessions {
		infos = append(infos, s.Info())
	}
	return infos
}

// KillSession destroys a session by ID.
func (m *SessionManager) KillSession(id string) error {
	m.mu.RLock()
	s, ok := m.sessions[id]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("session %s not found", id)
	}
	s.destroy()
	return nil
}

// generateID creates a 32-character hex string from 16 random bytes.
func generateID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
