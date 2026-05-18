package fieldagent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/eclipse-iofog/agent/internal/config"
	"github.com/eclipse-iofog/agent/internal/models"
	"github.com/eclipse-iofog/agent/internal/statusreporter"
)

func TestPostStatusHelper_SuccessfulPostSendsAndClearsTerminalStates(t *testing.T) {
	sr := statusreporter.GetInstance()
	sr.ResetProcessManagerStatus()
	t.Cleanup(func() { sr.ResetProcessManagerStatus() })
	sr.UpdateProcessManagerStatus(func(pm *models.ProcessManagerStatus) {
		pm.SetMicroservicesState("ms-deleted", models.MicroserviceStateDeleted)
		pm.SetMicroservicesState("ms-running", models.MicroserviceStateRunning)
	})

	var sentStatus map[string]interface{}
	fa := &FieldAgent{
		config: config.GetInstance(),
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
