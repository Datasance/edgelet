package client

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/eclipse-iofog/edgelet/internal/cli/ui"
)

type pollFakeAPI struct {
	responses []map[string]any
	calls     int
}

func (f *pollFakeAPI) Request(string, string, any) (map[string]any, error) {
	if f.calls >= len(f.responses) {
		return map[string]any{"status": "running"}, nil
	}
	resp := f.responses[f.calls]
	f.calls++
	return resp, nil
}

func (f *pollFakeAPI) RequestMultipartFile(string, string, string, string, map[string]string) (map[string]any, error) {
	return nil, nil
}

func (f *pollFakeAPI) IsDaemonRunning() bool { return true }

func TestPollAsyncOperation_StageTerminalSuccess(t *testing.T) {
	api := &pollFakeAPI{
		responses: []map[string]any{
			{"status": "running", "stage": "persisting"},
			{"status": "running", "stage": "pulling"},
			{"status": "succeeded", "deploymentId": "dep-1"},
		},
	}
	var stderr strings.Builder
	u := ui.NewWithWriters(nil, &stderr, ui.Options{ForceTTY: true})
	spin := u.StartSpinner("Applying microservice manifest...")
	defer spin.Stop()

	final, stages, err := PollAsyncOperation(context.Background(), PollConfig{Interval: time.Millisecond}, func() (map[string]any, error) {
		return api.Request("GET", "/status", nil)
	}, PollProgress{
		UI:             u,
		Spinner:        spin,
		StageFormatter: ui.FormatDeployStageLine,
	})
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if final["deploymentId"] != "dep-1" {
		t.Fatalf("unexpected final payload: %#v", final)
	}
	if len(stages) != 2 || stages[0] != "persisting" || stages[1] != "pulling" {
		t.Fatalf("unexpected stages: %#v", stages)
	}
	if strings.Contains(stderr.String(), "(pulling)ng)") {
		t.Fatalf("progress corruption detected: %q", stderr.String())
	}
}

func TestPollAsyncOperation_SpinnerUsesStageSuffix(t *testing.T) {
	clearInteractiveEnv(t)
	api := &pollFakeAPI{
		responses: []map[string]any{
			{"status": "running", "stage": "pulling"},
			{"status": "succeeded", "deploymentId": "dep-2"},
		},
	}
	var stderr strings.Builder
	u := ui.NewWithWriters(nil, &stderr, ui.Options{ForceTTY: true})
	spin := u.StartSpinner("Applying microservice manifest...")

	final, stages, err := PollAsyncOperation(context.Background(), PollConfig{Interval: time.Millisecond}, func() (map[string]any, error) {
		return api.Request("GET", "/status", nil)
	}, PollProgress{
		UI:             u,
		Spinner:        spin,
		StageFormatter: ui.FormatDeployStageLine,
	})
	spin.Stop()
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if final["deploymentId"] != "dep-2" {
		t.Fatalf("unexpected final payload: %#v", final)
	}
	if len(stages) != 1 || stages[0] != "pulling" {
		t.Fatalf("expected stage pulling recorded, got: %#v", stages)
	}
	// Interactive spinner Stop() clears the line; only the clear sequence may remain.
	if msg := stderr.String(); msg != "" && !strings.Contains(msg, "\r") {
		t.Fatalf("expected empty or cleared stderr after spinner stop, got: %q", msg)
	}
}

func TestPollAsyncOperation_StageSuffixNonInteractive(t *testing.T) {
	t.Setenv("CI", "true")
	api := &pollFakeAPI{
		responses: []map[string]any{
			{"status": "running", "stage": "pulling"},
			{"status": "succeeded", "deploymentId": "dep-3"},
		},
	}
	var stderr strings.Builder
	u := ui.NewWithWriters(nil, &stderr, ui.Options{})

	_, stages, err := PollAsyncOperation(context.Background(), PollConfig{Interval: time.Millisecond}, func() (map[string]any, error) {
		return api.Request("GET", "/status", nil)
	}, PollProgress{
		UI:             u,
		StageFormatter: ui.FormatDeployStageLine,
	})
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(stages) != 1 || stages[0] != "pulling" {
		t.Fatalf("expected stage pulling recorded, got: %#v", stages)
	}
	line := ui.FormatDeployStageLine("pulling")
	if !strings.Contains(stderr.String(), line) {
		t.Fatalf("expected stage line %q in stderr, got: %q", line, stderr.String())
	}
}

func TestPollAsyncOperation_PercentDone(t *testing.T) {
	api := &pollFakeAPI{
		responses: []map[string]any{
			{"status": "running", "progress": 10},
			{"status": "running", "progress": 20},
			{"status": "succeeded", "progress": 100},
		},
	}
	var stderr strings.Builder
	u := ui.NewWithWriters(nil, &stderr, ui.Options{ForceTTY: true})

	_, _, err := PollAsyncOperation(context.Background(), PollConfig{Interval: time.Millisecond, PercentStep: 5}, func() (map[string]any, error) {
		return api.Request("GET", "/status", nil)
	}, PollProgress{UI: u, PercentLabel: "pulling image"})
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if !strings.Contains(stderr.String(), "pulling image") {
		t.Fatalf("expected percent output, got: %q", stderr.String())
	}
}

func clearInteractiveEnv(t *testing.T) {
	t.Helper()
	t.Setenv("CI", "")
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
}
