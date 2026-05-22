package cliutil

import "testing"

func TestInputError(t *testing.T) {
	err := NewInputError("bad flag")
	if err.Error() != "bad flag" {
		t.Fatalf("unexpected message: %s", err.Error())
	}
}

func TestInternalErrorWrap(t *testing.T) {
	root := NewInputError("inner")
	err := NewInternalError("outer", root)
	if err.Error() != "outer: inner" {
		t.Fatalf("unexpected message: %s", err.Error())
	}
}
