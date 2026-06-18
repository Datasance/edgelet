package logging

import "testing"

func TestDaemonLogBudgetMB_ControlPlaneFullWhenNotSplit(t *testing.T) {
	got := DaemonLogBudgetMB(10, SeriesControlPlane, false)
	if got != 10240 {
		t.Fatalf("expected 10240 MiB, got %d", got)
	}
}

func TestDaemonLogBudgetMB_ControlPlaneSplit(t *testing.T) {
	got := DaemonLogBudgetMB(10, SeriesControlPlane, true)
	want := int(10240 * controlPlaneLogShare)
	if got != want {
		t.Fatalf("expected %d MiB, got %d", want, got)
	}
}

func TestDaemonLogBudgetMB_DataPlaneSplit(t *testing.T) {
	got := DaemonLogBudgetMB(10, SeriesDataPlane, true)
	want := int(10240 * dataPlaneLogShare)
	if got != want {
		t.Fatalf("expected %d MiB, got %d", want, got)
	}
}
