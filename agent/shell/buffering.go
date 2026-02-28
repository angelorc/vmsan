package shell

import (
	"sync"
	"time"
)

// BufferedOutput accumulates PTY output until the first Ready signal,
// then switches to direct passthrough mode.
type BufferedOutput struct {
	mu            sync.Mutex
	buf           []byte
	direct        bool
	readyOnce     sync.Once
	MarkedReadyAt time.Time
}

// NewBufferedOutput creates a new BufferedOutput in buffering mode.
func NewBufferedOutput() *BufferedOutput {
	return &BufferedOutput{}
}

// Append adds data to the buffer. If not yet ready, data is buffered internally
// and (nil, false) is returned. If ready, data is returned as passthrough.
func (b *BufferedOutput) Append(data []byte) (passthrough []byte, isDirect bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.direct {
		b.buf = append(b.buf, data...)
		return nil, false
	}
	return data, true
}

// MarkReady switches to direct mode idempotently. Returns the accumulated
// buffer for flushing (nil on subsequent calls).
func (b *BufferedOutput) MarkReady() (flushed []byte) {
	b.readyOnce.Do(func() {
		b.mu.Lock()
		flushed = b.buf
		b.buf = nil
		b.direct = true
		b.MarkedReadyAt = time.Now()
		b.mu.Unlock()
	})
	return flushed
}
