package fieldagent

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/eclipse-iofog/edgelet/internal/config"
	"github.com/eclipse-iofog/edgelet/internal/models"
	"github.com/eclipse-iofog/edgelet/internal/statusreporter"
)

func TestExecSessionManager_ListActiveControllerSessionIDs(t *testing.T) {
	esm := GetExecSessionManager()
	esm.mu.Lock()
	esm.activeSessions = map[string]*ExecSessionInfo{
		"sess-b": {
			Session: &ExecSession{SessionID: "sess-b", MicroserviceUUID: "ms-1"},
		},
		"sess-a": {
			Session: &ExecSession{SessionID: "sess-a", MicroserviceUUID: "ms-1"},
		},
		"sess-other": {
			Session: &ExecSession{SessionID: "sess-other", MicroserviceUUID: "ms-2"},
		},
		"0123456789ab-hc-deadbeef": {
			Session: &ExecSession{SessionID: "0123456789ab-hc-deadbeef", MicroserviceUUID: "ms-1"},
		},
	}
	esm.mu.Unlock()
	t.Cleanup(func() {
		esm.mu.Lock()
		esm.activeSessions = make(map[string]*ExecSessionInfo)
		esm.mu.Unlock()
	})

	got := esm.ListActiveControllerSessionIDs("ms-1")
	want := []string{"sess-a", "sess-b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListActiveControllerSessionIDs = %v, want %v", got, want)
	}
	if ids := esm.ListActiveControllerSessionIDs("ms-missing"); len(ids) != 0 {
		t.Fatalf("expected empty for missing MS, got %v", ids)
	}
}

func setupStatusExecTestMS(t *testing.T, msUUID, containerID string, engineExecIDs []string) {
	t.Helper()
	sr := statusreporter.GetInstance()
	sr.ResetProcessManagerStatus()
	t.Cleanup(func() { sr.ResetProcessManagerStatus() })
	sr.UpdateProcessManagerStatus(func(pm *models.ProcessManagerStatus) {
		st := models.NewMicroserviceStatusWithState(models.MicroserviceStateRunning)
		st.ContainerID = containerID
		st.ExecSessionIDs = append([]string(nil), engineExecIDs...)
		pm.SetMicroservicesStatus(msUUID, st)
	})
}

func execSessionIDsFromFogStatus(t *testing.T, fa *FieldAgent, msUUID string) []string {
	t.Helper()
	raw, ok := fa.getFogStatus()["microserviceStatus"].(string)
	if !ok {
		t.Fatal("microserviceStatus missing from fog status")
	}
	var items []map[string]any
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		t.Fatalf("parse microserviceStatus: %v", err)
	}
	for _, item := range items {
		if item["id"] != msUUID {
			continue
		}
		rawIDs, ok := item["execSessionIds"].([]any)
		if !ok {
			return nil
		}
		ids := make([]string, 0, len(rawIDs))
		for _, v := range rawIDs {
			id, ok := v.(string)
			if !ok || id == "" {
				continue
			}
			ids = append(ids, id)
		}
		return ids
	}
	return nil
}

func TestStatus_ExecSessionIDs_TwoControllerSessions(t *testing.T) {
	const msUUID = "ms-two-sessions"
	setupStatusExecTestMS(t, msUUID, "container-1", []string{"docker-exec-id-should-not-appear"})

	esm := GetExecSessionManager()
	esm.mu.Lock()
	esm.activeSessions = map[string]*ExecSessionInfo{
		"550e8400-e29b-41d4-a716-446655440000": {
			Session: &ExecSession{
				SessionID:        "550e8400-e29b-41d4-a716-446655440000",
				MicroserviceUUID: msUUID,
				Status:           "ACTIVE",
			},
		},
		"660e8400-e29b-41d4-a716-446655440001": {
			Session: &ExecSession{
				SessionID:        "660e8400-e29b-41d4-a716-446655440001",
				MicroserviceUUID: msUUID,
				Status:           "PENDING",
			},
		},
	}
	esm.mu.Unlock()
	t.Cleanup(func() {
		esm.mu.Lock()
		esm.activeSessions = make(map[string]*ExecSessionInfo)
		esm.mu.Unlock()
	})

	fa := &FieldAgent{config: config.GetInstance(), state: NewState()}
	got := execSessionIDsFromFogStatus(t, fa, msUUID)
	want := []string{
		"550e8400-e29b-41d4-a716-446655440000",
		"660e8400-e29b-41d4-a716-446655440001",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("execSessionIds = %v, want %v", got, want)
	}
}

func TestStatus_ExecSessionIDs_LocalSessionOnly(t *testing.T) {
	const msUUID = "ms-local-only"
	setupStatusExecTestMS(t, msUUID, "container-local", []string{
		"0123456789ab-local-abc12345",
		"legacy-docker-exec-id",
	})

	esm := GetExecSessionManager()
	esm.mu.Lock()
	esm.activeSessions = make(map[string]*ExecSessionInfo)
	esm.mu.Unlock()

	fa := &FieldAgent{config: config.GetInstance(), state: NewState()}
	got := execSessionIDsFromFogStatus(t, fa, msUUID)
	if got != nil {
		t.Fatalf("expected no execSessionIds for local-only exec, got %v", got)
	}
}

func TestStatus_ExecSessionIDs_DetachReconcile(t *testing.T) {
	const msUUID = "ms-detach-status"
	const sessionID = "sess-detach-status"
	setupStatusExecTestMS(t, msUUID, "container-detach", nil)

	esm := GetExecSessionManager()
	esm.mu.Lock()
	esm.activeSessions = map[string]*ExecSessionInfo{
		sessionID: {
			Session: &ExecSession{
				SessionID:        sessionID,
				MicroserviceUUID: msUUID,
				Status:           "ACTIVE",
			},
		},
	}
	esm.mu.Unlock()
	t.Cleanup(func() {
		esm.mu.Lock()
		esm.activeSessions = make(map[string]*ExecSessionInfo)
		esm.mu.Unlock()
	})

	fa := &FieldAgent{config: config.GetInstance(), state: NewState()}
	if got := execSessionIDsFromFogStatus(t, fa, msUUID); !reflect.DeepEqual(got, []string{sessionID}) {
		t.Fatalf("before detach: execSessionIds = %v, want [%s]", got, sessionID)
	}

	esm.HandleExecSessions([]*ExecSession{})

	got := execSessionIDsFromFogStatus(t, fa, msUUID)
	if got != nil {
		t.Fatalf("after detach reconcile: execSessionIds = %v, want absent/empty", got)
	}
}

func TestStatus_ExecSessionIDs_HealthcheckExcluded(t *testing.T) {
	const msUUID = "ms-hc"
	setupStatusExecTestMS(t, msUUID, "0123456789abcdef", []string{
		"0123456789ab-hc-deadbeef",
		"another-docker-exec-id",
	})

	esm := GetExecSessionManager()
	esm.mu.Lock()
	esm.activeSessions = make(map[string]*ExecSessionInfo)
	esm.mu.Unlock()

	fa := &FieldAgent{config: config.GetInstance(), state: NewState()}
	got := execSessionIDsFromFogStatus(t, fa, msUUID)
	if got != nil {
		t.Fatalf("expected healthcheck/runtime exec ids excluded, got %v", got)
	}
}

func TestEnrichMicroserviceStatusExecSessionIDs_ReplacesEngineIDs(t *testing.T) {
	raw := `[{"id":"ms-1","status":"RUNNING","containerId":"c1","execSessionIds":["docker-exec-1"]}]`
	enriched := enrichMicroserviceStatusExecSessionIDs(raw, func(msUUID string) []string {
		if msUUID == "ms-1" {
			return []string{"controller-session-id"}
		}
		return nil
	})

	var items []map[string]any
	if err := json.Unmarshal([]byte(enriched), &items); err != nil {
		t.Fatalf("parse enriched JSON: %v", err)
	}
	ids, ok := items[0]["execSessionIds"].([]any)
	if !ok || len(ids) != 1 || ids[0] != "controller-session-id" {
		t.Fatalf("unexpected execSessionIds after enrich: %#v", items[0]["execSessionIds"])
	}
}
