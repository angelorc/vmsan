package shell

import (
	"testing"
)

func TestBufferedOutput_BuffersBeforeReady(t *testing.T) {
	b := NewBufferedOutput()

	passthrough, isDirect := b.Append([]byte("hello"))
	if passthrough != nil {
		t.Fatal("expected nil passthrough before ready")
	}
	if isDirect {
		t.Fatal("expected isDirect=false before ready")
	}

	passthrough, isDirect = b.Append([]byte(" world"))
	if passthrough != nil {
		t.Fatal("expected nil passthrough before ready on second append")
	}
	if isDirect {
		t.Fatal("expected isDirect=false before ready on second append")
	}
}

func TestBufferedOutput_FlushOnReady(t *testing.T) {
	b := NewBufferedOutput()

	b.Append([]byte("hello"))
	b.Append([]byte(" world"))

	flushed := b.MarkReady()
	if string(flushed) != "hello world" {
		t.Fatalf("expected 'hello world', got %q", flushed)
	}
}

func TestBufferedOutput_DirectAfterReady(t *testing.T) {
	b := NewBufferedOutput()
	b.MarkReady()

	data := []byte("direct data")
	passthrough, isDirect := b.Append(data)
	if !isDirect {
		t.Fatal("expected isDirect=true after ready")
	}
	if string(passthrough) != "direct data" {
		t.Fatalf("expected 'direct data', got %q", passthrough)
	}
}

func TestBufferedOutput_IdempotentReady(t *testing.T) {
	b := NewBufferedOutput()

	b.Append([]byte("data"))
	first := b.MarkReady()
	if string(first) != "data" {
		t.Fatalf("expected 'data', got %q", first)
	}

	second := b.MarkReady()
	if second != nil {
		t.Fatalf("expected nil on second MarkReady, got %q", second)
	}
}

func TestBufferedOutput_EmptyFlush(t *testing.T) {
	b := NewBufferedOutput()

	flushed := b.MarkReady()
	if flushed != nil {
		t.Fatalf("expected nil flush with no data, got %q", flushed)
	}
}

func TestBufferedOutput_MarkedReadyAtTimestamp(t *testing.T) {
	b := NewBufferedOutput()

	if !b.MarkedReadyAt.IsZero() {
		t.Fatal("expected zero timestamp before MarkReady")
	}

	b.MarkReady()

	if b.MarkedReadyAt.IsZero() {
		t.Fatal("expected non-zero timestamp after MarkReady")
	}
}
