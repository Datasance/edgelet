package fieldagent

import (
	"errors"

	"github.com/datasance/edgelet/internal/models"
	"github.com/datasance/edgelet/internal/store"
)

// saveControllerMicroservicesToStore persists the given slice to SQLite.
func saveControllerMicroservicesToStore(microservices []*models.Microservice) error {
	db := store.GetInstance()
	if db.Conn() == nil {
		return errors.New("SQLite not open")
	}
	return db.SaveControllerMicroservices(microservices)
}

// loadControllerMicroservicesFromStore retrieves all controller microservices from SQLite.
func loadControllerMicroservicesFromStore() ([]*models.Microservice, error) {
	db := store.GetInstance()
	if db.Conn() == nil {
		return nil, errors.New("SQLite not open")
	}
	return db.LoadControllerMicroservices()
}

// saveControllerRegistriesToStore persists the given slice to SQLite.
func saveControllerRegistriesToStore(registries []*models.Registry) error {
	db := store.GetInstance()
	if db.Conn() == nil {
		return errors.New("SQLite not open")
	}
	return db.SaveControllerRegistries(registries)
}

// loadControllerRegistriesFromStore retrieves all controller registries from SQLite.
func loadControllerRegistriesFromStore() ([]*models.Registry, error) {
	db := store.GetInstance()
	if db.Conn() == nil {
		return nil, errors.New("SQLite not open")
	}
	return db.LoadControllerRegistries()
}
