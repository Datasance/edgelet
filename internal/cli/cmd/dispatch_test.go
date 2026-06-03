package cmd

import (
	"strings"
	"testing"
)

func TestShouldRunCLI(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{"bare binary", []string{"edgelet"}, true},
		{"daemon alias", []string{"edgelet", "daemon"}, false},
		{"runtime-bootstrap", []string{"edgelet", "runtime-bootstrap"}, false},
		{"cli subcommand", []string{"edgelet", "provision"}, true},
		{"shutdown subcommand", []string{"edgelet", "shutdown"}, true},
		{"cgroup-preflight subcommand", []string{"edgelet", "cgroup-preflight"}, true},
		{"version subcommand", []string{"edgelet", "version"}, true},
		{"help flag", []string{"edgelet", "--help"}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ShouldRunCLI(tc.args); got != tc.want {
				t.Fatalf("ShouldRunCLI(%v) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}

func TestVersionLocalWithoutDaemon(t *testing.T) {
	stdout, stderr, code := runCLI(t, &fakeClient{running: false}, "version")
	if code != 0 {
		t.Fatalf("version exit=%d stderr=%q", code, stderr)
	}
	for _, part := range []string{"edgelet", "embedded engine:", "allowed containerEngine:"} {
		if !strings.Contains(stdout, part) {
			t.Fatalf("expected %q in stdout=%q", part, stdout)
		}
	}
}
