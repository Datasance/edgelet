package store

import (
	"testing"

	"github.com/datasance/edgelet/internal/models"
)

func TestClearControllerMicroserviceRuntimeFieldsKeepsSpec(t *testing.T) {
	db := openFreshStoreDB(t)
	ms := &models.Microservice{
		MicroserviceUUID: "ms-1",
		ImageName:        "alpine:3.19",
		ContainerID:      "cid-old",
		MicroserviceName: "test-ms",
	}
	if err := db.SaveControllerMicroservices([]*models.Microservice{ms}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := db.ClearControllerMicroserviceRuntimeFields(); err != nil {
		t.Fatalf("clear runtime: %v", err)
	}
	loaded, err := db.LoadControllerMicroservices()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 spec row, got %d", len(loaded))
	}
	if loaded[0].ContainerID != "" {
		t.Fatalf("container_id should be cleared, got %q", loaded[0].ContainerID)
	}
	if loaded[0].ImageName != "alpine:3.19" {
		t.Fatal("spec image should remain")
	}
}
