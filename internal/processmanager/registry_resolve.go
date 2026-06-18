package processmanager

import (
	"errors"
	"fmt"

	"github.com/eclipse-iofog/edgelet/internal/models"
	"github.com/eclipse-iofog/edgelet/internal/store"
)

// resolveRegistryForMicroservice returns the registry for ms.RegistryID.
// Managed (Pot) workloads resolve via MicroserviceManagerInterface (controller snapshot).
// Local workloads fall back to local_registries when the controller snapshot has no match.
func resolveRegistryForMicroservice(msm MicroserviceManagerInterface, ms *models.Microservice) (*models.Registry, error) {
	if ms == nil {
		return nil, errors.New("microservice is nil")
	}
	if ms.RegistryID <= 0 {
		return nil, fmt.Errorf("registry is not valid %d", ms.RegistryID)
	}
	if msm != nil {
		if reg := msm.GetRegistry(ms.RegistryID); reg != nil {
			return reg, nil
		}
	}
	if reg, err := store.GetInstance().GetLocalRegistry(ms.RegistryID); err == nil && reg != nil {
		return reg, nil
	}
	return nil, fmt.Errorf("registry is not valid %d", ms.RegistryID)
}

func registryURLFromRegistry(registry *models.Registry) string {
	if registry == nil {
		return ""
	}
	return registry.URL
}
