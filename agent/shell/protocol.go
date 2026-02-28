package shell

import "encoding/binary"

// Shell WebSocket binary protocol.
// Each message is prefixed with a 1-byte type tag.
const (
	MsgData   byte = 0x00 // [0x00][payload...]              terminal I/O
	MsgResize byte = 0x01 // [0x01][cols u16BE][rows u16BE]  resize
	MsgReady  byte = 0x02 // [0x02]                          client ready
)

// Message represents a parsed WebSocket shell message.
type Message struct {
	Type byte
	Data []byte // MsgData payload
	Cols uint16 // MsgResize
	Rows uint16 // MsgResize
}

// ParseMessage parses a binary WebSocket message into a Message.
// Returns nil for empty, malformed, or unknown-type messages.
func ParseMessage(buf []byte) *Message {
	if len(buf) == 0 {
		return nil
	}
	msg := &Message{Type: buf[0]}
	switch buf[0] {
	case MsgData:
		msg.Data = buf[1:]
	case MsgResize:
		if len(buf) < 5 {
			return nil
		}
		msg.Cols = binary.BigEndian.Uint16(buf[1:3])
		msg.Rows = binary.BigEndian.Uint16(buf[3:5])
	case MsgReady:
		// no payload
	default:
		return nil
	}
	return msg
}

// SerializeData prepends the MsgData type byte to payload.
func SerializeData(data []byte) []byte {
	out := make([]byte, 1+len(data))
	out[0] = MsgData
	copy(out[1:], data)
	return out
}

// SerializeResize builds a MsgResize frame with the given dimensions.
func SerializeResize(cols, rows uint16) []byte {
	out := make([]byte, 5)
	out[0] = MsgResize
	binary.BigEndian.PutUint16(out[1:3], cols)
	binary.BigEndian.PutUint16(out[3:5], rows)
	return out
}
