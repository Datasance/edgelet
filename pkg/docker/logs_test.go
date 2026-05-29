package docker

import (
	"strings"
	"testing"
)

type captureLogTailHandler struct {
	lines [][]byte
}

func (h *captureLogTailHandler) OnLogLine(_, _ string, lineBytes []byte, _ StreamType) {
	copied := append([]byte(nil), lineBytes...)
	h.lines = append(h.lines, copied)
}
func (h *captureLogTailHandler) OnComplete(_ string)       {}
func (h *captureLogTailHandler) OnError(_ string, _ error) {}

func TestForwardDemuxedLogStream_PreservesEmptyAndTrailingLines(t *testing.T) {
	handler := &captureLogTailHandler{}
	input := "line1\n\nline3\nlast-without-newline"

	err := forwardDemuxedLogStream(strings.NewReader(input), "sid", "ms-1", handler, STDOUT)
	if err != nil {
		t.Fatalf("forwardDemuxedLogStream returned error: %v", err)
	}
	if len(handler.lines) != 4 {
		t.Fatalf("expected 4 emitted lines, got %d", len(handler.lines))
	}
	if string(handler.lines[0]) != "line1\n" {
		t.Fatalf("unexpected first line: %q", string(handler.lines[0]))
	}
	if string(handler.lines[1]) != "\n" {
		t.Fatalf("expected preserved empty line, got %q", string(handler.lines[1]))
	}
	if string(handler.lines[2]) != "line3\n" {
		t.Fatalf("unexpected third line: %q", string(handler.lines[2]))
	}
	if string(handler.lines[3]) != "last-without-newline" {
		t.Fatalf("unexpected trailing line: %q", string(handler.lines[3]))
	}
}
