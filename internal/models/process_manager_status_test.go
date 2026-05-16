package models

import "testing"

func TestProcessManagerStatusPruneMicroserviceStatus(t *testing.T) {
	pm := NewProcessManagerStatus()
	pm.SetMicroservicesState("ms-a", MicroserviceStateDeleted)
	pm.SetMicroservicesState("ms-b", MicroserviceStateRunning)

	pm.PruneMicroserviceStatus(func(_ string, status *MicroserviceStatus) bool {
		return status != nil && status.Status == MicroserviceStateDeleted
	})

	if st := pm.GetMicroserviceStatus("ms-a"); st == nil || st.Status != MicroserviceStateUnknown {
		t.Fatalf("expected ms-a to be pruned from map")
	}
	if st := pm.GetMicroserviceStatus("ms-b"); st == nil || st.Status != MicroserviceStateRunning {
		t.Fatalf("expected ms-b to remain running after prune")
	}
}

func TestProcessManagerStatusClearMicroserviceStatuses(t *testing.T) {
	pm := NewProcessManagerStatus()
	pm.SetMicroservicesState("ms-a", MicroserviceStateRunning)
	pm.SetRunningMicroservicesCount(1)

	pm.ClearMicroserviceStatuses()

	if got := len(pm.MicroservicesStatus); got != 0 {
		t.Fatalf("expected empty microservice status map, got len=%d", got)
	}
	if pm.RunningMicroservicesCount != 0 {
		t.Fatalf("expected running count reset to 0, got=%d", pm.RunningMicroservicesCount)
	}
}
