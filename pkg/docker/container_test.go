package docker

import (
	"testing"

	"github.com/moby/moby/api/types/container"
)

func TestContainersFromDockerList_RunningOnly(t *testing.T) {
	input := []container.Summary{
		{ID: "running-1", State: "running", Names: []string{"/edgelet_ms-a"}},
		{ID: "exited-1", State: "exited", Names: []string{"/edgelet_ms-b"}},
		{ID: "created-1", State: "created", Names: []string{"/edgelet_ms-c"}},
	}

	got := containersFromDockerList(false, input)
	if len(got) != 1 {
		t.Fatalf("expected 1 running container, got %d: %+v", len(got), got)
	}
	if got[0].ID != "running-1" {
		t.Fatalf("expected running-1, got %q", got[0].ID)
	}
}

func TestContainersFromDockerList_AllStates(t *testing.T) {
	input := []container.Summary{
		{ID: "running-1", State: "running"},
		{ID: "exited-1", State: "exited"},
	}

	got := containersFromDockerList(true, input)
	if len(got) != 2 {
		t.Fatalf("expected 2 containers, got %d", len(got))
	}
}

func TestContainersFromDockerList_AllVsRunningNotAlias(t *testing.T) {
	input := []container.Summary{
		{ID: "running-1", State: "running"},
		{ID: "exited-1", State: "exited"},
	}

	running := containersFromDockerList(false, input)
	all := containersFromDockerList(true, input)
	if len(running) == len(all) {
		t.Fatal("running and all lists must differ when exited containers exist")
	}
}
