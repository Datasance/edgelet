package containerexec

import (
	"strings"
	"testing"
)

func TestIsInteractiveShellCommand(t *testing.T) {
	if !IsInteractiveShellCommand(nil) {
		t.Fatal("empty command should be interactive")
	}
	if !IsInteractiveShellCommand(ShellCommandInteractive()) {
		t.Fatal("default interactive argv should match")
	}
	if IsInteractiveShellCommand([]string{"nslookup", "edgelet.local-dns-b"}) {
		t.Fatal("one-shot argv should not be interactive")
	}
	if IsInteractiveShellCommand([]string{"/bin/sh", "-lc", "echo hi"}) {
		t.Fatal("custom shell script should not be interactive")
	}
}

func TestShellCommandInteractive(t *testing.T) {
	cmd := ShellCommandInteractive()
	if len(cmd) != 3 || cmd[0] != "/bin/sh" || cmd[1] != "-lc" {
		t.Fatalf("unexpected argv prefix: %v", cmd)
	}
	body := cmd[2]
	for _, want := range []string{"/bin/bash", "/bin/sh", "/busybox/sh"} {
		if !strings.Contains(body, want) {
			t.Fatalf("interactive shell probe missing %q: %q", want, body)
		}
	}
}

func TestShellCommandForScript(t *testing.T) {
	script := "curl -f http://localhost/ || exit 1"
	cmd := ShellCommandForScript(script)
	if len(cmd) != 3 || cmd[0] != "/bin/sh" || cmd[1] != "-lc" {
		t.Fatalf("unexpected argv prefix: %v", cmd)
	}
	body := cmd[2]
	for _, want := range []string{
		"exec /bin/bash -c",
		"exec /bin/sh -c",
		"exec /busybox/sh -c",
		script,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("script wrapper missing %q: %q", want, body)
		}
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", "''"},
		{"hello", "'hello'"},
		{"it's fine", "'it'\\''s fine'"},
		{"$HOME \"quoted\"", "'$HOME \"quoted\"'"},
		{"a'b'c", "'a'\\''b'\\''c'"},
	}
	for _, tc := range tests {
		if got := shellQuote(tc.in); got != tc.want {
			t.Errorf("shellQuote(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
