package models

import (
	"testing"
)

func TestMicroserviceStateFromText(t *testing.T) {
	tests := []struct {
		input    string
		expected MicroserviceState
	}{
		{"QUEUED", MicroserviceStateQueued},
		{"RUNNING", MicroserviceStateRunning},
		{"STOPPED", MicroserviceStateStopped},
		{"unknown", MicroserviceStateUnknown},
		{"invalid", MicroserviceStateUnknown},
	}

	for _, tt := range tests {
		result := MicroserviceStateFromText(tt.input)
		if result != tt.expected {
			t.Errorf("MicroserviceStateFromText(%q) = %v, want %v", tt.input, result, tt.expected)
		}
	}
}

func TestPortMapping(t *testing.T) {
	pm := NewPortMapping(8080, 80, false)
	if pm.Outside != 8080 || pm.Inside != 80 || pm.UDP {
		t.Errorf("PortMapping creation failed")
	}

	pm2 := NewPortMapping(8080, 80, false)
	if !pm.Equals(pm2) {
		t.Errorf("PortMapping equality check failed")
	}
}

func TestEnvVar(t *testing.T) {
	ev := NewEnvVar("KEY", "VALUE")
	if ev.Key != "KEY" || ev.Value != "VALUE" {
		t.Errorf("EnvVar creation failed")
	}

	ev2 := NewEnvVar("KEY", "VALUE")
	if !ev.Equals(ev2) {
		t.Errorf("EnvVar equality check failed")
	}
}

func TestRoute(t *testing.T) {
	r := NewRoute()
	if len(r.Receivers) != 0 {
		t.Errorf("Route should start with empty receivers")
	}

	r.SetReceivers([]string{"receiver1", "receiver2"})
	if len(r.Receivers) != 2 {
		t.Errorf("Route receivers not set correctly")
	}
}

func TestRegistry(t *testing.T) {
	reg := NewRegistry(1, "https://registry.example.com", true, "user", "pass", "user@example.com")
	if reg.ID != 1 || reg.URL != "https://registry.example.com" || !reg.IsPublic {
		t.Errorf("Registry creation failed")
	}

	// Test builder
	builder := NewRegistryBuilder()
	reg2 := builder.SetID(2).SetURL("https://registry2.example.com").SetIsPublic(false).Build()
	if reg2.ID != 2 || reg2.URL != "https://registry2.example.com" || reg2.IsPublic {
		t.Errorf("RegistryBuilder failed")
	}
}

func TestMicroservice(t *testing.T) {
	ms := NewMicroservice("uuid-123", "image:tag")
	if ms.MicroserviceUUID != "uuid-123" || ms.ImageName != "image:tag" {
		t.Errorf("Microservice creation failed")
	}

	if err := ms.Validate(); err != nil {
		t.Errorf("Valid microservice should not fail validation: %v", err)
	}

	ms2 := NewMicroservice("", "image:tag")
	if err := ms2.Validate(); err == nil {
		t.Errorf("Microservice with empty UUID should fail validation")
	}
}

func TestMicroserviceStatus(t *testing.T) {
	ms := NewMicroserviceStatus()
	if ms.Status != MicroserviceStateUnknown {
		t.Errorf("MicroserviceStatus should start with UNKNOWN state")
	}

	ms.AddExecSessionID("exec-1")
	if len(ms.GetExecSessionIDs()) != 1 {
		t.Errorf("Exec session ID not added correctly")
	}

	ms.RemoveExecSessionID("exec-1")
	if len(ms.GetExecSessionIDs()) != 0 {
		t.Errorf("Exec session ID not removed correctly")
	}
}

func TestFieldAgentStatus(t *testing.T) {
	fa := NewFieldAgentStatus()
	if fa.ControllerStatus != ControllerStatusNotConnected {
		t.Errorf("FieldAgentStatus should start with NOT_CONNECTED")
	}
}

func TestStatusReporterStatus(t *testing.T) {
	sr := NewStatusReporterStatus()
	if sr.SystemTime == 0 || sr.LastUpdate == 0 {
		t.Errorf("StatusReporterStatus should have non-zero timestamps")
	}
}

func TestExecMessage(t *testing.T) {
	em := NewExecMessage(ExecMessageTypeStdout, []byte("test"), "uuid-123", "exec-1")
	if em.Type != ExecMessageTypeStdout || em.MicroserviceUUID != "uuid-123" {
		t.Errorf("ExecMessage creation failed")
	}
}

func TestLogMessage(t *testing.T) {
	lm := NewLogMessage(LogMessageTypeLogLine, []byte("log line"), "session-1", "uuid-123", "iofog-123")
	if lm.Type != LogMessageTypeLogLine || lm.SessionID != "session-1" {
		t.Errorf("LogMessage creation failed")
	}
}

func TestYamlConfig(t *testing.T) {
	yc := NewYamlConfig()
	if yc.Profiles == nil {
		t.Errorf("YamlConfig profiles should be initialized")
	}

	profile := NewProfileConfig()
	profile.SetProperty("key1", "value1")
	if profile.GetProperty("key1") != "value1" {
		t.Errorf("ProfileConfig property not set correctly")
	}
}

// Helper function
func stringPtr(s string) *string {
	return &s
}
