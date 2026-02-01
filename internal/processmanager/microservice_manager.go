package processmanager

import (
	"github.com/eclipse-iofog/agent-go/internal/models"
)

// MicroserviceManagerInterface defines the interface for microservice management
// This will be implemented by FieldAgent or a dedicated MicroserviceManager
type MicroserviceManagerInterface interface {
	GetLatestMicroservices() []*models.Microservice
	GetCurrentMicroservices() []*models.Microservice
	FindLatestMicroserviceByUuid(uuid string) *models.Microservice
	GetRegistry(id int) *models.Registry
	SetCurrentMicroservices(microservices []*models.Microservice)
}
