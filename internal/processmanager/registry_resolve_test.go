package processmanager

import (
	"testing"

	"github.com/eclipse-iofog/edgelet/internal/models"
)

type resolveRegistryMSM struct {
	registry *models.Registry
}

func (m *resolveRegistryMSM) GetLatestMicroservices() []*models.Microservice  { return nil }
func (m *resolveRegistryMSM) GetCurrentMicroservices() []*models.Microservice { return nil }
func (m *resolveRegistryMSM) FindLatestMicroserviceByUUID(string) *models.Microservice {
	return nil
}
func (m *resolveRegistryMSM) GetRegistry(_ int) *models.Registry             { return m.registry }
func (m *resolveRegistryMSM) SetCurrentMicroservices([]*models.Microservice) {}

func TestResolveRegistryForMicroservice_ControllerSnapshot(t *testing.T) {
	msm := &resolveRegistryMSM{registry: models.NewRegistry(3, "quay.io", true, "", "", "")}
	ms := models.NewMicroservice("uuid-1", "org/app:v1")
	ms.RegistryID = 3

	reg, err := resolveRegistryForMicroservice(msm, ms)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reg.URL != "quay.io" {
		t.Fatalf("registry URL=%q", reg.URL)
	}
}

func TestResolveRegistryForMicroservice_InvalidID(t *testing.T) {
	ms := models.NewMicroservice("uuid-1", "org/app:v1")
	ms.RegistryID = 0

	if _, err := resolveRegistryForMicroservice(nil, ms); err == nil {
		t.Fatal("expected error for invalid registry id")
	}
}
