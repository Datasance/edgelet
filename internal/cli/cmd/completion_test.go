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
	if !strings.Contains(out, "edgelet") {
		t.Fatal("expected program name in completion script")
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

func TestCompletionHelpShowsLongAndExamples(t *testing.T) {
	root := newRootCommand()
	var outBuf, errBuf bytes.Buffer
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)
	root.SetArgs([]string{"completion", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("completion --help failed: %v", err)
	}
	stdout := outBuf.String()
	if !strings.Contains(stdout, "Generate shell completion scripts") {
		t.Fatalf("expected completion Long in help, got stdout=%q", stdout)
	}
	if !strings.Contains(stdout, "Examples:") || !strings.Contains(stdout, "completion bash") {
		t.Fatalf("expected completion Examples in help, got stdout=%q", stdout)
	}
}

func TestCompletionListedInRootHelp(t *testing.T) {
	root := newRootCommand()
	var outBuf, errBuf bytes.Buffer
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)
	root.SetArgs([]string{"--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("root --help failed: %v", err)
	}
	if !strings.Contains(outBuf.String(), "completion") {
		t.Fatalf("expected completion in root Available Commands, got stdout=%q", outBuf.String())
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
