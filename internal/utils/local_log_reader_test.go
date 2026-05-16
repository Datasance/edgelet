package utils

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestLocalLogReader_NewLocalLogReader(t *testing.T) {
	tmpDir := t.TempDir()

	config := &TailConfig{
		Follow: true,
		Lines:  100,
	}

	handler := &testLogHandler{}

	reader := NewLocalLogReader("test-session", "test-uuid", tmpDir, config, handler)
	if reader == nil {
		t.Fatal("NewLocalLogReader returned nil")
	}
}

func TestLocalLogReader_StartStop(t *testing.T) {
	tmpDir := t.TempDir()

	config := &TailConfig{
		Follow: false,
		Lines:  10,
	}

	handler := &testLogHandler{}

	reader := NewLocalLogReader("test-session", "test-uuid", tmpDir, config, handler)

	// Start reader
	reader.Start()

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	// Stop reader
	reader.Stop()

	// Note: isRunning is a private field, tested indirectly via Start/Stop
}

type testLogHandler struct{}

func (h *testLogHandler) OnLogLine(sessionID, iofogUUID, line string) {
	// Test handler - do nothing
}

func (h *testLogHandler) OnComplete(sessionID string) {
	// Test handler - do nothing
}

func (h *testLogHandler) OnError(sessionID string, err error) {
	// Test handler - do nothing
}

func TestLocalLogReader_ReadTailLines_PreservesEmptyLines(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "iofog-agent.0.log")
	content := "line1\n\nline3\n"
	if err := os.WriteFile(logFile, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write test log file: %v", err)
	}

	reader := NewLocalLogReader("test-session", "test-uuid", tmpDir, &TailConfig{
		Follow: false,
		Lines:  10,
	}, &testLogHandler{})

	lines, err := reader.readTailLines(logFile, 10)
	if err != nil {
		t.Fatalf("readTailLines returned error: %v", err)
	}
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d (%v)", len(lines), lines)
	}
	if lines[1] != "" {
		t.Fatalf("expected preserved empty line at index 1, got %q", lines[1])
	}
}

type captureLogHandler struct {
	mu    sync.Mutex
	lines []string
}

func (h *captureLogHandler) OnLogLine(_, _, line string) {
	h.mu.Lock()
	h.lines = append(h.lines, line)
	h.mu.Unlock()
}

func (h *captureLogHandler) OnComplete(_ string)       {}
func (h *captureLogHandler) OnError(_ string, _ error) {}

func (h *captureLogHandler) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.lines)
}

func (h *captureLogHandler) snapshot() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]string, len(h.lines))
	copy(out, h.lines)
	return out
}

func waitForLineCount(t *testing.T, h *captureLogHandler, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if h.count() >= want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %d lines, got %d", want, h.count())
}

func TestLocalLogReader_FollowNoRotationDoesNotReplay(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, latestLogFile)
	if err := os.WriteFile(logFile, []byte("line1\nline2\n"), 0o600); err != nil {
		t.Fatalf("failed to write log file: %v", err)
	}

	handler := &captureLogHandler{}
	reader := NewLocalLogReader("session-1", "uuid-1", tmpDir, &TailConfig{
		Follow: true,
		Lines:  100,
	}, handler)
	reader.Start()
	t.Cleanup(reader.Stop)

	waitForLineCount(t, handler, 2, 2*time.Second)
	initial := handler.count()
	time.Sleep(250 * time.Millisecond)
	after := handler.count()
	if after != initial {
		t.Fatalf("expected no replay without file changes, before=%d after=%d lines=%v", initial, after, handler.snapshot())
	}
}

func TestLocalLogReader_FollowAppendEmitsOnlyNewLines(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, latestLogFile)
	if err := os.WriteFile(logFile, []byte("line1\nline2\n"), 0o600); err != nil {
		t.Fatalf("failed to write log file: %v", err)
	}

	handler := &captureLogHandler{}
	reader := NewLocalLogReader("session-2", "uuid-2", tmpDir, &TailConfig{
		Follow: true,
		Lines:  100,
	}, handler)
	reader.Start()
	t.Cleanup(reader.Stop)

	waitForLineCount(t, handler, 2, 2*time.Second)
	if err := os.WriteFile(logFile, []byte("line1\nline2\nline3\n"), 0o600); err != nil {
		t.Fatalf("failed to append test line: %v", err)
	}
	waitForLineCount(t, handler, 3, 2*time.Second)

	lines := handler.snapshot()
	if len(lines) < 3 || lines[2] != "line3" {
		t.Fatalf("expected only new appended line to be emitted, got %v", lines)
	}
}

func TestLocalLogReader_FollowRotationTruncateReopensAndContinues(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, latestLogFile)
	if err := os.WriteFile(logFile, []byte("line1\nline2\nline3\n"), 0o600); err != nil {
		t.Fatalf("failed to write log file: %v", err)
	}

	handler := &captureLogHandler{}
	reader := NewLocalLogReader("session-3", "uuid-3", tmpDir, &TailConfig{
		Follow: true,
		Lines:  100,
	}, handler)
	reader.Start()
	t.Cleanup(reader.Stop)

	waitForLineCount(t, handler, 3, 2*time.Second)
	if err := os.WriteFile(logFile, []byte("new-a\n"), 0o600); err != nil {
		t.Fatalf("failed to rewrite rotated log file: %v", err)
	}
	waitForLineCount(t, handler, 4, 2*time.Second)
	if err := os.WriteFile(logFile, []byte("new-a\nnew-b\n"), 0o600); err != nil {
		t.Fatalf("failed to append after rotation: %v", err)
	}
	waitForLineCount(t, handler, 5, 2*time.Second)

	lines := handler.snapshot()
	if lines[3] != "new-a" || lines[4] != "new-b" {
		t.Fatalf("expected continued streaming from truncated file, got %v", lines)
	}
}
