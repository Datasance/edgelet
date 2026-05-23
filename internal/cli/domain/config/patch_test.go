package config

import "testing"

func TestHasRejections(t *testing.T) {
	if HasRejections(nil) {
		t.Fatal("expected false for nil data")
	}
	if HasRejections(map[string]interface{}{}) {
		t.Fatal("expected false for empty data")
	}
	data := map[string]interface{}{
		"errorMap": map[string]interface{}{"k": "v"},
	}
	if !HasRejections(data) {
		t.Fatal("expected true when errorMap is non-empty")
	}
}
