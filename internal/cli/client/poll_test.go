package client

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/eclipse-iofog/agent/internal/cli/ui"
)

type pollFakeAPI struct {
	responses []map[string]interface{}
	calls     int
}

func (f *pollFakeAPI) RequestV3(string, string, interface{}) (map[string]interface{}, error) {
	if f.calls >= len(f.responses) {
		return map[string]interface{}{"status": "running"}, nil
	}
	resp := f.responses[f.calls]
	f.calls++
	return resp, nil
}

func (f *pollFakeAPI) RequestV3MultipartFile(string, string, string, string, map[string]string) (map[string]interface{}, error) {
	return nil, nil
}

func (f *pollFakeAPI) IsDaemonRunning() bool { return true }

func TestPollAsyncOperation_StageTerminalSuccess(t *testing.T) {
	api := &pollFakeAPI{
		responses: []map[string]interface{}{
			{"status": "running", "stage": "persisting"},
			{"status": "running", "stage": "pulling"},
			{"status": "succeeded", "deploymentId": "dep-1"},
		},
	}
	var stderr strings.Builder
	u := ui.NewWithWriters(nil, &stderr, ui.Options{ForceTTY: true})
	spin := u.StartSpinner("Applying microservice manifest...")
	defer spin.Stop()

	final, stages, err := PollAsyncOperation(context.Background(), PollConfig{Interval: time.Millisecond}, func() (map[string]interface{}, error) {
		return api.RequestV3("GET", "/status", nil)
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
	api := &pollFakeAPI{
		responses: []map[string]interface{}{
			{"status": "running", "stage": "pulling"},
			{"status": "succeeded", "deploymentId": "dep-2"},
		},
	}
	var stderr strings.Builder
	u := ui.NewWithWriters(nil, &stderr, ui.Options{ForceTTY: true})
	spin := u.StartSpinner("Applying microservice manifest...")

	_, _, err := PollAsyncOperation(context.Background(), PollConfig{Interval: time.Millisecond}, func() (map[string]interface{}, error) {
		return api.RequestV3("GET", "/status", nil)
	}, PollProgress{
		UI:             u,
		Spinner:        spin,
		StageFormatter: ui.FormatDeployStageLine,
	})
	spin.Stop()
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if !strings.Contains(stderr.String(), "applying microservice manifest") {
		t.Fatalf("expected spinner output, got: %q", stderr.String())
	}
}

func TestPollAsyncOperation_PercentDone(t *testing.T) {
	api := &pollFakeAPI{
		responses: []map[string]interface{}{
			{"status": "running", "progress": 10},
			{"status": "running", "progress": 20},
			{"status": "succeeded", "progress": 100},
		},
	}
	var stderr strings.Builder
	u := ui.NewWithWriters(nil, &stderr, ui.Options{ForceTTY: true})

	_, _, err := PollAsyncOperation(context.Background(), PollConfig{Interval: time.Millisecond, PercentStep: 5}, func() (map[string]interface{}, error) {
		return api.RequestV3("GET", "/status", nil)
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
