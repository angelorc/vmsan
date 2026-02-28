package shell

import (
	"sync"

	"github.com/gorilla/websocket"
)

// WSWriter wraps a WebSocket connection with a mutex for safe concurrent writes.
type WSWriter struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

// NewWSWriter creates a new WSWriter for the given WebSocket connection.
func NewWSWriter(conn *websocket.Conn) *WSWriter {
	return &WSWriter{conn: conn}
}

// Write serializes p as a MsgData frame and sends it as a binary WebSocket message.
// Implements io.Writer.
func (w *WSWriter) Write(p []byte) (int, error) {
	frame := SerializeData(p)
	w.mu.Lock()
	defer w.mu.Unlock()
	err := w.conn.WriteMessage(websocket.BinaryMessage, frame)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

// WriteRaw sends a pre-serialized binary frame over WebSocket.
func (w *WSWriter) WriteRaw(frame []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.conn.WriteMessage(websocket.BinaryMessage, frame)
}

// WriteClose sends a WebSocket close frame with the given code and text.
func (w *WSWriter) WriteClose(code int, text string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.conn.WriteMessage(websocket.CloseMessage,
		websocket.FormatCloseMessage(code, text))
}
