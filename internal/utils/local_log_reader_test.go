package utils

import (
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
