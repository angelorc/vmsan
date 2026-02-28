package shell

import (
	"log/slog"
	"os"
	"testing"
	"time"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestSessionManager_MaxSessions(t *testing.T) {
	logger := testLogger()
	m := NewSessionManager(logger)

	created := make([]*Session, 0, DefaultMaxSessions)
	for i := 0; i < DefaultMaxSessions; i++ {
		s, err := m.CreateSession("/bin/sh")
		if err != nil {
			t.Fatalf("failed to create session %d: %v", i, err)
		}
		created = append(created, s)
	}

	_, err := m.CreateSession("/bin/sh")
	if err == nil {
		t.Fatal("expected error when exceeding max sessions")
	}

	// Clean up
	for _, s := range created {
		s.destroy()
	}
}

func TestSessionManager_CreateAndGet(t *testing.T) {
	logger := testLogger()
	m := NewSessionManager(logger)

	s, err := m.CreateSession("/bin/sh")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	defer s.destroy()

	got := m.GetSession(s.ID)
	if got == nil {
		t.Fatal("expected to get session by ID")
	}
	if got.ID != s.ID {
		t.Fatalf("expected session ID %s, got %s", s.ID, got.ID)
	}
	if got.Shell != "/bin/sh" {
		t.Fatalf("expected shell /bin/sh, got %s", got.Shell)
	}
}

func TestSessionManager_ListSessions(t *testing.T) {
	logger := testLogger()
	m := NewSessionManager(logger)

	s1, err := m.CreateSession("/bin/sh")
	if err != nil {
		t.Fatalf("create session 1: %v", err)
	}
	defer s1.destroy()

	s2, err := m.CreateSession("/bin/sh")
	if err != nil {
		t.Fatalf("create session 2: %v", err)
	}
	defer s2.destroy()

	infos := m.ListSessions()
	if len(infos) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(infos))
	}

	ids := make(map[string]bool)
	for _, info := range infos {
		ids[info.SessionID] = true
	}
	if !ids[s1.ID] {
		t.Fatalf("session 1 ID %s not in list", s1.ID)
	}
	if !ids[s2.ID] {
		t.Fatalf("session 2 ID %s not in list", s2.ID)
	}
}

func TestSessionManager_KillSession(t *testing.T) {
	logger := testLogger()
	m := NewSessionManager(logger)

	s, err := m.CreateSession("/bin/sh")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	err = m.KillSession(s.ID)
	if err != nil {
		t.Fatalf("kill session: %v", err)
	}

	// Give onDestroy callback time to run
	time.Sleep(50 * time.Millisecond)

	got := m.GetSession(s.ID)
	if got != nil {
		t.Fatal("expected session to be removed after kill")
	}
}

func TestSessionManager_KillSession_NotFound(t *testing.T) {
	logger := testLogger()
	m := NewSessionManager(logger)

	err := m.KillSession("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent session")
	}
}

func TestSession_MaxSubscribers(t *testing.T) {
	logger := testLogger()
	m := NewSessionManager(logger)

	s, err := m.CreateSession("/bin/sh")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	defer s.destroy()

	// Directly verify the limit constant
	if DefaultMaxSubscribers != 8 {
		t.Fatalf("expected DefaultMaxSubscribers=8, got %d", DefaultMaxSubscribers)
	}

	// Verify subscriber count starts at 0
	if s.SubscriberCount() != 0 {
		t.Fatalf("expected 0 subscribers, got %d", s.SubscriberCount())
	}
}

func TestSession_InactivityTimeout(t *testing.T) {
	logger := testLogger()

	onDestroyCalled := make(chan string, 1)
	onDestroy := func(id string) {
		onDestroyCalled <- id
	}

	id, err := generateID()
	if err != nil {
		t.Fatalf("generate id: %v", err)
	}

	s, err := NewSession(id, "/bin/sh", onDestroy, logger)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Override with a short timeout to avoid slow tests
	s.inactivityMu.Lock()
	if s.inactivityTimer != nil {
		s.inactivityTimer.Stop()
	}
	s.inactivityTimer = time.AfterFunc(100*time.Millisecond, func() {
		s.destroy()
	})
	s.inactivityMu.Unlock()

	select {
	case gotID := <-onDestroyCalled:
		if gotID != id {
			t.Fatalf("expected session ID %s, got %s", id, gotID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("inactivity timeout did not fire within 2 seconds")
		s.destroy()
	}
}

func TestSession_Info(t *testing.T) {
	logger := testLogger()
	m := NewSessionManager(logger)

	s, err := m.CreateSession("/bin/sh")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	defer s.destroy()

	info := s.Info()
	if info.SessionID != s.ID {
		t.Fatalf("expected session ID %s, got %s", s.ID, info.SessionID)
	}
	if info.Shell != "/bin/sh" {
		t.Fatalf("expected shell /bin/sh, got %s", info.Shell)
	}
	if info.SubscriberCount != 0 {
		t.Fatalf("expected 0 subscribers, got %d", info.SubscriberCount)
	}
	if info.CreatedAt.IsZero() {
		t.Fatal("expected non-zero CreatedAt")
	}
}

func TestGenerateID(t *testing.T) {
	id, err := generateID()
	if err != nil {
		t.Fatalf("generate id: %v", err)
	}
	if len(id) != 32 {
		t.Fatalf("expected 32 char hex ID, got %d chars: %s", len(id), id)
	}

	// Verify uniqueness
	id2, err := generateID()
	if err != nil {
		t.Fatalf("generate id2: %v", err)
	}
	if id == id2 {
		t.Fatal("expected unique IDs")
	}
}
