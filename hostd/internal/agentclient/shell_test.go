package agentclient

import (
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

var testUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func TestShellProtocol_MsgReady(t *testing.T) {
	// Verify that a msgReady binary frame [0x02, ...sessionID] can be received
	// and decoded correctly.
	sessionID := "session-abc-123"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close()

		// Send a msgReady message.
		msg := make([]byte, 1+len(sessionID))
		msg[0] = msgReady
		copy(msg[1:], []byte(sessionID))
		if err := conn.WriteMessage(websocket.BinaryMessage, msg); err != nil {
			t.Errorf("write: %v", err)
		}

		// Close cleanly.
		conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	msgType, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if msgType != websocket.BinaryMessage {
		t.Errorf("msgType = %d, want BinaryMessage (%d)", msgType, websocket.BinaryMessage)
	}
	if len(data) < 1 {
		t.Fatal("empty message")
	}
	if data[0] != msgReady {
		t.Errorf("tag = 0x%02x, want 0x%02x (msgReady)", data[0], msgReady)
	}
	gotSession := string(data[1:])
	if gotSession != sessionID {
		t.Errorf("sessionID = %q, want %q", gotSession, sessionID)
	}
}

func TestShellProtocol_MsgData(t *testing.T) {
	payload := "hello from shell"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close()

		// Send a msgData message.
		msg := make([]byte, 1+len(payload))
		msg[0] = msgData
		copy(msg[1:], []byte(payload))
		if err := conn.WriteMessage(websocket.BinaryMessage, msg); err != nil {
			t.Errorf("write: %v", err)
		}

		conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	msgType, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if msgType != websocket.BinaryMessage {
		t.Errorf("msgType = %d, want BinaryMessage (%d)", msgType, websocket.BinaryMessage)
	}
	if len(data) < 1 {
		t.Fatal("empty message")
	}
	if data[0] != msgData {
		t.Errorf("tag = 0x%02x, want 0x%02x (msgData)", data[0], msgData)
	}
	gotPayload := string(data[1:])
	if gotPayload != payload {
		t.Errorf("payload = %q, want %q", gotPayload, payload)
	}
}

func TestShellProtocol_MsgResize(t *testing.T) {
	// Verify resize message format: [0x01, cols_hi, cols_lo, rows_hi, rows_lo]
	var wantCols uint16 = 120
	var wantRows uint16 = 40

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close()

		// Read a binary message from the client (the resize message).
		_, data, err := conn.ReadMessage()
		if err != nil {
			t.Errorf("read: %v", err)
			return
		}
		if len(data) != 5 {
			t.Errorf("resize message len = %d, want 5", len(data))
			return
		}
		if data[0] != msgResize {
			t.Errorf("tag = 0x%02x, want 0x%02x (msgResize)", data[0], msgResize)
		}
		gotCols := binary.BigEndian.Uint16(data[1:3])
		gotRows := binary.BigEndian.Uint16(data[3:5])
		if gotCols != wantCols {
			t.Errorf("cols = %d, want %d", gotCols, wantCols)
		}
		if gotRows != wantRows {
			t.Errorf("rows = %d, want %d", gotRows, wantRows)
		}

		conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Build resize message.
	msg := make([]byte, 5)
	msg[0] = msgResize
	binary.BigEndian.PutUint16(msg[1:3], wantCols)
	binary.BigEndian.PutUint16(msg[3:5], wantRows)

	if err := conn.WriteMessage(websocket.BinaryMessage, msg); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Read close.
	conn.ReadMessage()
}

func TestShellProtocol_SendData(t *testing.T) {
	// Verify that sendData builds the correct [0x00, payload...] frame.
	inputText := "user input text"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close()

		// Read client data message.
		_, data, err := conn.ReadMessage()
		if err != nil {
			t.Errorf("read: %v", err)
			return
		}
		if len(data) < 1 {
			t.Error("empty message")
			return
		}
		if data[0] != msgData {
			t.Errorf("tag = 0x%02x, want 0x%02x (msgData)", data[0], msgData)
		}
		got := string(data[1:])
		if got != inputText {
			t.Errorf("payload = %q, want %q", got, inputText)
		}

		conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Build data message the same way sendData does.
	msg := make([]byte, 1+len(inputText))
	msg[0] = msgData
	copy(msg[1:], []byte(inputText))

	if err := conn.WriteMessage(websocket.BinaryMessage, msg); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Read close.
	conn.ReadMessage()
}

func TestBuildURL(t *testing.T) {
	tests := []struct {
		name string
		opts ShellOptions
		want string
	}{
		{
			name: "basic",
			opts: ShellOptions{Host: "192.168.0.1", Port: 9119, Token: "abc"},
			want: "ws://192.168.0.1:9119/ws/shell?token=abc",
		},
		{
			name: "with session ID",
			opts: ShellOptions{Host: "192.168.0.1", Port: 9119, Token: "abc", SessionID: "sess-1"},
			want: "ws://192.168.0.1:9119/ws/shell/sess-1?token=abc",
		},
		{
			name: "with user",
			opts: ShellOptions{Host: "192.168.0.1", Port: 9119, Token: "abc", User: "root"},
			want: "ws://192.168.0.1:9119/ws/shell?token=abc&user=root",
		},
		{
			name: "with session and user",
			opts: ShellOptions{Host: "10.0.0.1", Port: 8080, Token: "t", SessionID: "s1", User: "admin"},
			want: "ws://10.0.0.1:8080/ws/shell/s1?token=t&user=admin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &ShellSession{opts: tt.opts, sessionID: tt.opts.SessionID}
			got := s.buildURL()
			if got != tt.want {
				t.Errorf("buildURL() = %q, want %q", got, tt.want)
			}
		})
	}
}
