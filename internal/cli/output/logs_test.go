package output

import (
	"bytes"
	"testing"
)

func TestWriteStreamLogLine_PreservesIncomingNewline(t *testing.T) {
	var b bytes.Buffer
	WriteStreamLogLine(&b, "", "line1\n\nline3\n", false)
	if b.String() != "line1\n\nline3\n" {
		t.Fatalf("unexpected output: %q", b.String())
	}
}

func TestWriteStreamLogLine_AppendsMissingTrailingNewline(t *testing.T) {
	var b bytes.Buffer
	WriteStreamLogLine(&b, "", "line-without-newline", false)
	if b.String() != "line-without-newline\n" {
		t.Fatalf("unexpected output: %q", b.String())
	}
}
