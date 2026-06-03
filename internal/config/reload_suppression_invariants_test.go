package config

import "testing"

func TestReloadSuppressionToggle(t *testing.T) {
	RestoreReloadAfterDeprovision()
	RestoreReloadAfterInProcessMutation()
	if IsReloadSuppressedForDeprovision() {
		t.Fatal("expected deprovision suppression flag to be false after restore")
	}
	if IsReloadSuppressedForInProcessMutation() {
		t.Fatal("expected in-process suppression flag to be false after restore")
	}

	SuppressReloadForDeprovision()
	if !IsReloadSuppressedForDeprovision() {
		t.Fatal("expected deprovision suppression flag to be true after suppress")
	}

	SuppressReloadForInProcessMutation()
	if !IsReloadSuppressedForInProcessMutation() {
		t.Fatal("expected in-process suppression flag to be true after suppress")
	}

	RestoreReloadAfterInProcessMutation()
	if IsReloadSuppressedForInProcessMutation() {
		t.Fatal("expected in-process suppression flag to be false after restore")
	}

	RestoreReloadAfterDeprovision()
	if IsReloadSuppressedForDeprovision() {
		t.Fatal("expected deprovision suppression flag to be false after restore")
	}
}
