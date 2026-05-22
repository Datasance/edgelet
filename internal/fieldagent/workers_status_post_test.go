package fieldagent

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/eclipse-iofog/agent/internal/config"
	"github.com/eclipse-iofog/agent/internal/constants"
	"github.com/eclipse-iofog/agent/internal/models"
	"github.com/eclipse-iofog/agent/internal/statusreporter"
)

func TestPostStatusHelper_SuccessfulPostSendsAndClearsTerminalStates(t *testing.T) {
	cfg := config.GetInstance()
	originalEngine := cfg.ContainerEngine
	cfg.ContainerEngine = constants.EngineDocker
	t.Cleanup(func() { cfg.ContainerEngine = originalEngine })

	sr := statusreporter.GetInstance()
	sr.ResetProcessManagerStatus()
	t.Cleanup(func() { sr.ResetProcessManagerStatus() })
	sr.UpdateProcessManagerStatus(func(pm *models.ProcessManagerStatus) {
		pm.SetMicroservicesState("ms-deleted", models.MicroserviceStateDeleted)
		pm.SetMicroservicesState("ms-running", models.MicroserviceStateRunning)
	})

	var sentStatus map[string]interface{}
	fa := &FieldAgent{
		config: cfg,
		state:  NewState(),
		postStatusFn: func(_ context.Context, status map[string]interface{}) error {
			sentStatus = status
			return nil
		},
	}
	fa.state.SetControllerStatus(models.ControllerStatusOK)

	fa.PostStatusHelper()

	if sentStatus == nil {
		t.Fatalf("expected status payload to be posted")
	}
	if !payloadHasMicroserviceState(t, sentStatus, "ms-deleted", "DELETED") {
		t.Fatalf("expected posted payload to include deleted microservice state")
	}
	if !reflect.DeepEqual(sentStatus["availableRuntimes"], []string{constants.EngineDocker}) {
		t.Fatalf("expected availableRuntimes=[docker], got: %#v", sentStatus["availableRuntimes"])
	}

	statuses := sr.GetProcessManagerStatus().MicroservicesStatus
	if _, ok := statuses["ms-deleted"]; ok {
		t.Fatalf("expected deleted state to be cleared after successful post")
	}
	if _, ok := statuses["ms-running"]; !ok {
		t.Fatalf("expected running state to remain after successful post")
	}
}

func TestPostStatusHelper_FailedPostRetainsTerminalStatesForRetry(t *testing.T) {
	sr := statusreporter.GetInstance()
	sr.ResetProcessManagerStatus()
	t.Cleanup(func() { sr.ResetProcessManagerStatus() })
	sr.UpdateProcessManagerStatus(func(pm *models.ProcessManagerStatus) {
		pm.SetMicroservicesState("ms-deleted", models.MicroserviceStateDeleted)
	})

	fa := &FieldAgent{
		config: config.GetInstance(),
		state:  NewState(),
		postStatusFn: func(_ context.Context, _ map[string]interface{}) error {
			return errors.New("temporary transport error")
		},
	}
	fa.state.SetControllerStatus(models.ControllerStatusOK)

	fa.PostStatusHelper()

	statuses := sr.GetProcessManagerStatus().MicroservicesStatus
	st, ok := statuses["ms-deleted"]
	if !ok {
		t.Fatalf("expected deleted state to remain when post fails")
	}
	if st == nil || st.Status != models.MicroserviceStateDeleted {
		t.Fatalf("expected deleted state to remain unchanged after failed post")
	}
}

func TestGetFogStatus_AvailableRuntimesPerEngine(t *testing.T) {
	cfg := config.GetInstance()
	originalEngine := cfg.ContainerEngine
	t.Cleanup(func() {
		cfg.ContainerEngine = originalEngine
	})

	fa := &FieldAgent{
		config: cfg,
		state:  NewState(),
	}

	cfg.ContainerEngine = constants.EngineDocker
	status := fa.getFogStatus()
	if !reflect.DeepEqual(status["availableRuntimes"], []string{constants.EngineDocker}) {
		t.Fatalf("expected docker runtime list, got: %#v", status["availableRuntimes"])
	}

	cfg.ContainerEngine = constants.EnginePodman
	status = fa.getFogStatus()
	if !reflect.DeepEqual(status["availableRuntimes"], []string{constants.EnginePodman}) {
		t.Fatalf("expected podman runtime list, got: %#v", status["availableRuntimes"])
	}

	cfg.ContainerEngine = constants.EngineIofog
	status = fa.getFogStatus()
	available, ok := status["availableRuntimes"].([]string)
	if !ok {
		t.Fatalf("expected []string availableRuntimes, got: %#v", status["availableRuntimes"])
	}
	for _, name := range available {
		if strings.HasSuffix(strings.ToLower(name), "-local") {
			t.Fatalf("expected controller payload runtimes to exclude local variants, got: %#v", available)
		}
	}
}

func TestRuntimeNamesForController_SortsAndDeduplicatesDeterministically(t *testing.T) {
	got := runtimeNamesForController(constants.EngineIofog, []string{
		"edgelet",
		"crun",
		"spin",
		"crun",
	})
	want := []string{"crun", "edgelet", "spin"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected filtered runtimes: got=%v want=%v", got, want)
	}

	got = runtimeNamesForController(constants.EngineDocker, []string{"runc", "crun"})
	want = []string{"crun", "runc"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected sorted non-iofog runtimes, got=%v want=%v", got, want)
	}
}

func payloadHasMicroserviceState(t *testing.T, payload map[string]interface{}, uuid, state string) bool {
	t.Helper()
	raw, ok := payload["microserviceStatus"]
	if !ok {
		return false
	}
	rawJSON, ok := raw.(string)
	if !ok {
		return false
	}
	var items []map[string]interface{}
	if err := json.Unmarshal([]byte(rawJSON), &items); err != nil {
		t.Fatalf("failed to parse microserviceStatus JSON: %v", err)
	}
	for _, item := range items {
		if item["id"] == uuid && item["status"] == state {
			return true
		}
	}
	return false
}
