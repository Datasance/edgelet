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
		t.Fatal("expected ms-a to be pruned from map")
	}
	if st := pm.GetMicroserviceStatus("ms-b"); st == nil || st.Status != MicroserviceStateRunning {
		t.Fatal("expected ms-b to remain running after prune")
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

func TestProcessManagerStatusPruneMicroserviceStatus_NoOpForNilPredicate(t *testing.T) {
	pm := NewProcessManagerStatus()
	pm.SetMicroservicesState("ms-a", MicroserviceStateRunning)

	pm.PruneMicroserviceStatus(nil)

	if got := len(pm.MicroservicesStatus); got != 1 {
		t.Fatalf("expected prune with nil predicate to keep entries, got len=%d", got)
	}
}

func TestProcessManagerStatusPruneMicroserviceStatus_RemovesInvalidEntries(t *testing.T) {
	pm := NewProcessManagerStatus()
	pm.SetMicroservicesState("valid-ms", MicroserviceStateRunning)
	pm.MicroservicesStatus[""] = NewMicroserviceStatusWithState(MicroserviceStateRunning)
	pm.MicroservicesStatus["nil-ms"] = nil

	pm.PruneMicroserviceStatus(func(uuid string, status *MicroserviceStatus) bool {
		return uuid == "" || status == nil
	})

	if _, ok := pm.MicroservicesStatus[""]; ok {
		t.Fatal("expected empty uuid entry to be pruned")
	}
	if _, ok := pm.MicroservicesStatus["nil-ms"]; ok {
		t.Fatal("expected nil status entry to be pruned")
	}
	if st, ok := pm.MicroservicesStatus["valid-ms"]; !ok || st == nil || st.Status != MicroserviceStateRunning {
		t.Fatal("expected valid status entry to remain")
	}
}
