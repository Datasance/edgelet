package microservice

import (
	"reflect"
	"testing"
)

func TestParseExecCommand_ExplicitSeparator(t *testing.T) {
	got, err := ParseExecCommand([]string{"--", "nslookup", "edgelet.local-dns-b"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"nslookup", "edgelet.local-dns-b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestParseExecCommand_CobraStrippedSeparator(t *testing.T) {
	got, err := ParseExecCommand([]string{"nslookup", "edgelet.local-dns-b"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"nslookup", "edgelet.local-dns-b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestParseExecCommand_RemoteFlags(t *testing.T) {
	got, err := ParseExecCommand([]string{"ls", "-la"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"ls", "-la"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestParseExecArgs(t *testing.T) {
	id, command, err := ParseExecArgs([]string{"639e64f5-d609-4209-89e2-7eee63b6c6f0", "nslookup", "edgelet.local-dns-b"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "639e64f5-d609-4209-89e2-7eee63b6c6f0" {
		t.Fatalf("id=%q", id)
	}
	want := []string{"nslookup", "edgelet.local-dns-b"}
	if !reflect.DeepEqual(command, want) {
		t.Fatalf("command=%v, want %v", command, want)
	}
}
