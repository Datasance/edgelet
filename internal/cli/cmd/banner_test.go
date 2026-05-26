package cmd

import (
	"strings"
	"testing"
)

const bannerMarker = "Edgelet"

func TestBanner_BareCommandOnce(t *testing.T) {
	client := &fakeClient{running: true}
	_, stderr, code := runCLI(t, client)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	if count := strings.Count(stderr, bannerMarker); count != 1 {
		t.Fatalf("expected banner once, got %d in stderr=%q", count, stderr)
	}
}

func TestBanner_HelpFlagOnce(t *testing.T) {
	client := &fakeClient{running: true}
	_, stderr, code := runCLI(t, client, "--help")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	if count := strings.Count(stderr, bannerMarker); count != 1 {
		t.Fatalf("expected banner once for --help, got %d in stderr=%q", count, stderr)
	}
}

func TestBanner_ShortHelpFlagOnce(t *testing.T) {
	client := &fakeClient{running: true}
	_, stderr, code := runCLI(t, client, "-h")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	if count := strings.Count(stderr, bannerMarker); count != 1 {
		t.Fatalf("expected banner once for -h, got %d in stderr=%q", count, stderr)
	}
}

func TestBanner_HelpSubcommandOnce(t *testing.T) {
	client := &fakeClient{running: true}
	_, stderr, code := runCLI(t, client, "help")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	if count := strings.Count(stderr, bannerMarker); count != 1 {
		t.Fatalf("expected banner once for help subcommand, got %d in stderr=%q", count, stderr)
	}
}

func TestBanner_SubcommandHelpNone(t *testing.T) {
	client := &fakeClient{running: true}
	_, stderr, code := runCLI(t, client, "deploy", "--help")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	if strings.Contains(stderr, bannerMarker) {
		t.Fatalf("expected no banner for deploy --help, got stderr=%q", stderr)
	}
}

func TestBanner_CommandNoBanner(t *testing.T) {
	client := &fakeClient{
		running: true,
		gets: map[string]map[string]interface{}{
			"GET /v1/system/status": {"iofogDaemon": "running"},
		},
	}
	_, stderr, code := runCLI(t, client, "system", "status")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
	if strings.Contains(stderr, bannerMarker) {
		t.Fatalf("expected no banner for system status, got stderr=%q", stderr)
	}
}

func TestFilterHelpArgs(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want int
	}{
		{name: "empty", in: nil, want: 0},
		{name: "help flag", in: []string{"--help"}, want: 0},
		{name: "short help", in: []string{"-h"}, want: 0},
		{name: "subcommand", in: []string{"deploy"}, want: 1},
		{name: "mixed", in: []string{"--help", "deploy"}, want: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := len(filterHelpArgs(tc.in)); got != tc.want {
				t.Fatalf("filterHelpArgs(%v) len=%d want=%d", tc.in, got, tc.want)
			}
		})
	}
}
