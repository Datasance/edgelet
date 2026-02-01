package utils

import (
	"errors"
	"testing"
)

func TestEither(t *testing.T) {
	// Test Left
	left := NewLeft[string](errors.New("error"))
	if !left.IsLeft() {
		t.Error("Expected Left to be left")
	}
	if left.IsRight() {
		t.Error("Expected Left not to be right")
	}
	if left.Left() == nil {
		t.Error("Expected Left to have an error")
	}

	// Test Right
	right := NewRight("success")
	if right.IsLeft() {
		t.Error("Expected Right not to be left")
	}
	if !right.IsRight() {
		t.Error("Expected Right to be right")
	}
	if right.Right() != "success" {
		t.Errorf("Expected Right value to be 'success', got %q", right.Right())
	}
}

func TestPair(t *testing.T) {
	pair := NewPair("first", 42)
	if pair.First != "first" {
		t.Errorf("Expected First to be 'first', got %q", pair.First)
	}
	if pair.Second != 42 {
		t.Errorf("Expected Second to be 42, got %d", pair.Second)
	}
}

func TestUnit(t *testing.T) {
	unit := NewUnit()
	if unit != (Unit{}) {
		t.Error("Expected Unit to be empty struct")
	}
}
