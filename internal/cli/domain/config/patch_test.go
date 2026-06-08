package config

import "testing"

func TestHasRejections(t *testing.T) {
	if HasRejections(nil) {
		t.Fatal("expected false for nil data")
	}
	if HasRejections(map[string]any{}) {
		t.Fatal("expected false for empty data")
	}
	data := map[string]any{
		"errorMap": map[string]any{"k": "v"},
	}
	if !HasRejections(data) {
		t.Fatal("expected true when errorMap is non-empty")
	}
}
