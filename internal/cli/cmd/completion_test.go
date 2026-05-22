package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestCompletionBashGeneratesValidScript(t *testing.T) {
	root := newRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"completion", "bash"})
	if err := root.Execute(); err != nil {
		t.Fatalf("completion bash failed: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "bash completion") && !strings.Contains(out, "complete -F") && !strings.Contains(out, "complete -o") {
		t.Fatalf("expected bash completion script markers, got: %q", truncate(out, 200))
	}
	if !strings.Contains(out, "iofog-agent") {
		t.Fatalf("expected program name in completion script")
	}
}

func TestCompletionZshGeneratesScript(t *testing.T) {
	root := newRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"completion", "zsh"})
	if err := root.Execute(); err != nil {
		t.Fatalf("completion zsh failed: %v", err)
	}
	if !strings.Contains(buf.String(), "#compdef") && !strings.Contains(buf.String(), "compdef") {
		t.Fatalf("expected zsh compdef header, got: %q", truncate(buf.String(), 200))
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
