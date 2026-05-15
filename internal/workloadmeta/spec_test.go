package workloadmeta

import "testing"

func TestRoleFromMicroservicePrecedence(t *testing.T) {
	if got := RoleFromMicroservice(true, true); got != RoleRouter {
		t.Fatalf("expected router precedence, got %q", got)
	}
	if got := RoleFromMicroservice(false, true); got != RoleNats {
		t.Fatalf("expected nats role, got %q", got)
	}
	if got := RoleFromMicroservice(false, false); got != RoleWorkload {
		t.Fatalf("expected workload role, got %q", got)
	}
}

func TestScopeFromMicroservice(t *testing.T) {
	tests := []struct {
		name        string
		application string
		hostNetwork bool
		want        string
	}{
		{name: "local non-host network", application: "local", hostNetwork: false, want: ScopeLocal},
		{name: "local host network", application: "local", hostNetwork: true, want: ScopeManaged},
		{name: "managed app", application: "app", hostNetwork: false, want: ScopeManaged},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ScopeFromMicroservice(tt.application, tt.hostNetwork); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestMicroserviceUIDFromLabels(t *testing.T) {
	labels := map[string]string{
		LabelMicroserviceUID: "  ms-123  ",
	}
	if got := MicroserviceUIDFromLabels(labels); got != "ms-123" {
		t.Fatalf("expected trimmed UID, got %q", got)
	}
}

func TestIsManagedByIofog(t *testing.T) {
	labels := map[string]string{
		LabelAppManagedBy:    ManagedByValue,
		LabelMicroserviceUID: "ms-1",
	}
	if !IsManagedByIofog(labels) {
		t.Fatal("expected labels to be managed by iofog")
	}

	labels[LabelMicroserviceUID] = ""
	if IsManagedByIofog(labels) {
		t.Fatal("expected missing uid to not be managed")
	}
}

func TestIsSystemWorkload(t *testing.T) {
	if !IsSystemWorkload(map[string]string{LabelSystem: "true"}) {
		t.Fatal("expected true for system workload")
	}
	if IsSystemWorkload(map[string]string{LabelSystem: "false"}) {
		t.Fatal("expected false for non-system workload")
	}
}

