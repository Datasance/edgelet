package models

import "testing"

func TestControlPlaneDeploymentNormalizeDefaults(t *testing.T) {
	item := &ControlPlaneDeployment{
		ControllerUUID: "cp-uuid-1",
		Name:           "pot",
		State:          "running",
		Generation:     0,
		RuntimeState:   "",
		DesiredState:   "",
		Namespace:      "",
	}

	item.NormalizeDefaults()

	if item.Namespace != ControlPlaneDefaultNamespace {
		t.Fatalf("expected namespace %q, got %q", ControlPlaneDefaultNamespace, item.Namespace)
	}
	if item.DesiredState != "running" {
		t.Fatalf("expected desired_state running, got %q", item.DesiredState)
	}
	if item.RuntimeState != "running" {
		t.Fatalf("expected runtime_state inferred from state, got %q", item.RuntimeState)
	}
	if item.Generation != 1 {
		t.Fatalf("expected generation 1, got %d", item.Generation)
	}
	if item.LastTransitionAt <= 0 {
		t.Fatalf("expected last_transition_at > 0, got %d", item.LastTransitionAt)
	}
	if item.LastReconcileAt != 0 {
		t.Fatalf("expected last_reconcile_at default 0, got %d", item.LastReconcileAt)
	}
	if item.LastStartAttemptAt != 0 {
		t.Fatalf("expected last_start_attempt_at default 0, got %d", item.LastStartAttemptAt)
	}
	if item.FailureCount != 0 {
		t.Fatalf("expected failure_count default 0, got %d", item.FailureCount)
	}
}
