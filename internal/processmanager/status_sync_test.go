package processmanager

import (
	"testing"

	"github.com/eclipse-iofog/edgelet/internal/models"
)

func TestSyncMicroserviceStatusToReporter_ClearsErrorOnRunning(t *testing.T) {
	pmStatus := models.NewProcessManagerStatus()
	errMsg := "Container ms-1 ADD operation failed after 5 attempts: docker unavailable"
	pmStatus.SetMicroservicesState("ms-1", models.MicroserviceStateFailed)
	pmStatus.SetMicroservicesStatusErrorMessage("ms-1", errMsg)

	runtimeStatus := models.NewMicroserviceStatusWithState(models.MicroserviceStateRunning)
	syncMicroserviceStatusToReporter(pmStatus, "ms-1", runtimeStatus)

	got := pmStatus.GetMicroserviceStatus("ms-1")
	if got == nil {
		t.Fatal("expected microservice status")
	}
	if got.Status != models.MicroserviceStateRunning {
		t.Fatalf("expected RUNNING, got %s", got.Status)
	}
	if got.ErrorMessage == nil {
		t.Fatal("expected explicit cleared errorMessage pointer")
	}
	if *got.ErrorMessage != "" {
		t.Fatalf("expected empty errorMessage, got %q", *got.ErrorMessage)
	}
}

func TestSyncMicroserviceStatusToReporter_PreservesErrorWhenNotRunning(t *testing.T) {
	pmStatus := models.NewProcessManagerStatus()
	errMsg := "still failing"
	runtimeStatus := models.NewMicroserviceStatusWithState(models.MicroserviceStateExiting)
	runtimeStatus.ErrorMessage = &errMsg

	syncMicroserviceStatusToReporter(pmStatus, "ms-1", runtimeStatus)

	got := pmStatus.GetMicroserviceStatus("ms-1")
	if got == nil || got.ErrorMessage == nil || *got.ErrorMessage != errMsg {
		t.Fatalf("expected errorMessage %q to remain, got %+v", errMsg, got)
	}
}
