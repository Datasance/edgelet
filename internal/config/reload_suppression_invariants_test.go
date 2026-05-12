package config

import "testing"

func TestReloadSuppressionToggle(t *testing.T) {
	RestoreReloadAfterDeprovision()
	if IsReloadSuppressedForDeprovision() {
		t.Fatal("expected suppression flag to be false after restore")
	}

	SuppressReloadForDeprovision()
	if !IsReloadSuppressedForDeprovision() {
		t.Fatal("expected suppression flag to be true after suppress")
	}

	RestoreReloadAfterDeprovision()
	if IsReloadSuppressedForDeprovision() {
		t.Fatal("expected suppression flag to be false after restore")
	}
}
