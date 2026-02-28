package shell

import (
	"encoding/binary"
	"testing"
)

func TestParseMessage_Data(t *testing.T) {
	payload := []byte("hello world")
	frame := SerializeData(payload)

	msg := ParseMessage(frame)
	if msg == nil {
		t.Fatal("expected non-nil message")
	}
	if msg.Type != MsgData {
		t.Fatalf("expected type %d, got %d", MsgData, msg.Type)
	}
	if string(msg.Data) != "hello world" {
		t.Fatalf("expected 'hello world', got %q", msg.Data)
	}
}

func TestParseMessage_Resize(t *testing.T) {
	var cols, rows uint16 = 120, 40
	frame := SerializeResize(cols, rows)

	msg := ParseMessage(frame)
	if msg == nil {
		t.Fatal("expected non-nil message")
	}
	if msg.Type != MsgResize {
		t.Fatalf("expected type %d, got %d", MsgResize, msg.Type)
	}
	if msg.Cols != cols {
		t.Fatalf("expected cols %d, got %d", cols, msg.Cols)
	}
	if msg.Rows != rows {
		t.Fatalf("expected rows %d, got %d", rows, msg.Rows)
	}
}

func TestParseMessage_Ready(t *testing.T) {
	frame := []byte{MsgReady}
	msg := ParseMessage(frame)
	if msg == nil {
		t.Fatal("expected non-nil message")
	}
	if msg.Type != MsgReady {
		t.Fatalf("expected type %d, got %d", MsgReady, msg.Type)
	}
}

func TestParseMessage_Empty(t *testing.T) {
	msg := ParseMessage([]byte{})
	if msg != nil {
		t.Fatal("expected nil for empty input")
	}

	msg = ParseMessage(nil)
	if msg != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestParseMessage_Unknown(t *testing.T) {
	frame := []byte{0xFF, 0x01, 0x02}
	msg := ParseMessage(frame)
	if msg != nil {
		t.Fatal("expected nil for unknown message type")
	}
}

func TestParseMessage_MalformedResize(t *testing.T) {
	// Resize needs at least 5 bytes: type + 2 cols + 2 rows
	frame := []byte{MsgResize, 0x00, 0x50}
	msg := ParseMessage(frame)
	if msg != nil {
		t.Fatal("expected nil for malformed resize (3 bytes)")
	}

	frame = []byte{MsgResize, 0x00, 0x50, 0x00}
	msg = ParseMessage(frame)
	if msg != nil {
		t.Fatal("expected nil for malformed resize (4 bytes)")
	}
}

func TestSerializeData(t *testing.T) {
	payload := []byte("test data")
	frame := SerializeData(payload)

	if len(frame) != 1+len(payload) {
		t.Fatalf("expected frame length %d, got %d", 1+len(payload), len(frame))
	}
	if frame[0] != MsgData {
		t.Fatalf("expected type byte %d, got %d", MsgData, frame[0])
	}
	if string(frame[1:]) != "test data" {
		t.Fatalf("expected payload 'test data', got %q", frame[1:])
	}
}

func TestSerializeResize(t *testing.T) {
	var cols, rows uint16 = 200, 50
	frame := SerializeResize(cols, rows)

	if len(frame) != 5 {
		t.Fatalf("expected frame length 5, got %d", len(frame))
	}
	if frame[0] != MsgResize {
		t.Fatalf("expected type byte %d, got %d", MsgResize, frame[0])
	}
	gotCols := binary.BigEndian.Uint16(frame[1:3])
	gotRows := binary.BigEndian.Uint16(frame[3:5])
	if gotCols != cols {
		t.Fatalf("expected cols %d, got %d", cols, gotCols)
	}
	if gotRows != rows {
		t.Fatalf("expected rows %d, got %d", rows, gotRows)
	}
}
