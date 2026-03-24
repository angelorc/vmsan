package agentclient

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/gorilla/websocket"
	"golang.org/x/term"
)

// Shell protocol message types (binary prefix byte).
const (
	msgData   = 0x00
	msgResize = 0x01
	msgReady  = 0x02
)

// ShellOptions configures a PTY shell session.
type ShellOptions struct {
	Host           string
	Port           int
	Token          string
	SessionID      string // resume an existing session
	User           string // run as this user
	InitialCommand string // inject on first connect
}

// ShellCloseInfo describes how the shell session ended.
type ShellCloseInfo struct {
	SessionDestroyed bool
	SessionID        string
}

// ShellSession manages an interactive WebSocket PTY session.
type ShellSession struct {
	opts      ShellOptions
	conn      *websocket.Conn
	sessionID string
	done      chan struct{}
	mu        sync.Mutex
}

// RunShell connects to the agent shell and runs an interactive PTY session.
// It blocks until the session ends (user exits or connection drops).
func RunShell(opts ShellOptions) (*ShellCloseInfo, error) {
	s := &ShellSession{
		opts:      opts,
		sessionID: opts.SessionID,
		done:      make(chan struct{}),
	}
	return s.run()
}

func (s *ShellSession) run() (*ShellCloseInfo, error) {
	wsURL := s.buildURL()

	header := http.Header{}
	dialer := websocket.Dialer{}

	conn, _, err := dialer.Dial(wsURL, header)
	if err != nil {
		return nil, fmt.Errorf("shell connection failed: %w", err)
	}
	s.conn = conn

	// Set terminal to raw mode.
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		s.conn.Close()
		return nil, fmt.Errorf("stdin is not a terminal")
	}
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		s.conn.Close()
		return nil, fmt.Errorf("failed to set raw mode: %w", err)
	}
	defer term.Restore(fd, oldState)

	// Send ready signal.
	s.sendReady()

	// Send initial terminal size.
	s.sendCurrentSize()

	// Send initial command if provided.
	if s.opts.InitialCommand != "" {
		s.sendData([]byte(s.opts.InitialCommand))
	}

	closeInfo := &ShellCloseInfo{}

	var wg sync.WaitGroup

	// Handle SIGWINCH for terminal resize.
	sigwinch := make(chan os.Signal, 1)
	signal.Notify(sigwinch, syscall.SIGWINCH)
	defer signal.Stop(sigwinch)

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-sigwinch:
				s.sendCurrentSize()
			case <-s.done:
				return
			}
		}
	}()

	// Read from stdin -> send to websocket.
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 {
				s.sendData(buf[:n])
			}
			if err != nil {
				return
			}
			select {
			case <-s.done:
				return
			default:
			}
		}
	}()

	// Read from websocket -> write to stdout.
	// This runs in the main goroutine effectively (via the wg).
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(s.done)
		for {
			msgType, data, err := s.conn.ReadMessage()
			if err != nil {
				// Check close code.
				if ce, ok := err.(*websocket.CloseError); ok {
					if ce.Code == websocket.CloseNormalClosure || ce.Code == websocket.CloseGoingAway {
						closeInfo.SessionDestroyed = (ce.Text == "session destroyed")
					}
				}
				return
			}

			switch msgType {
			case websocket.TextMessage:
				// Text messages carry session metadata JSON.
				var meta struct {
					SessionID string `json:"sessionId"`
				}
				if json.Unmarshal(data, &meta) == nil && meta.SessionID != "" {
					s.sessionID = meta.SessionID
				}
			case websocket.BinaryMessage:
				if len(data) == 0 {
					continue
				}
				tag := data[0]
				switch tag {
				case msgData:
					os.Stdout.Write(data[1:])
				case msgReady:
					// Server ready — nothing to do.
				}
			}
		}
	}()

	wg.Wait()

	closeInfo.SessionID = s.sessionID
	return closeInfo, nil
}

func (s *ShellSession) sendData(payload []byte) {
	msg := make([]byte, 1+len(payload))
	msg[0] = msgData
	copy(msg[1:], payload)
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.conn.WriteMessage(websocket.BinaryMessage, msg)
}

func (s *ShellSession) sendReady() {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.conn.WriteMessage(websocket.BinaryMessage, []byte{msgReady})
}

func (s *ShellSession) sendResize(cols, rows uint16) {
	msg := make([]byte, 5)
	msg[0] = msgResize
	binary.BigEndian.PutUint16(msg[1:3], cols)
	binary.BigEndian.PutUint16(msg[3:5], rows)
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.conn.WriteMessage(websocket.BinaryMessage, msg)
}

func (s *ShellSession) sendCurrentSize() {
	w, h, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return
	}
	s.sendResize(uint16(w), uint16(h))
}

func (s *ShellSession) buildURL() string {
	var path string
	if s.sessionID != "" {
		path = fmt.Sprintf("/ws/shell/%s", url.PathEscape(s.sessionID))
	} else {
		path = "/ws/shell"
	}

	u := url.URL{
		Scheme: "ws",
		Host:   fmt.Sprintf("%s:%d", s.opts.Host, s.opts.Port),
		Path:   path,
	}

	q := u.Query()
	q.Set("token", s.opts.Token)
	if s.opts.User != "" {
		q.Set("user", s.opts.User)
	}
	u.RawQuery = q.Encode()

	return u.String()
}

// Ensure io.Reader is available (used implicitly by stdin read).
var _ = io.EOF
