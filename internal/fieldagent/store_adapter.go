package fieldagent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/eclipse-iofog/agent/internal/models"
	"github.com/eclipse-iofog/agent/internal/store"
	"github.com/eclipse-iofog/agent/internal/utils"
	"github.com/eclipse-iofog/agent/internal/utils/logging"
)

const adapterModuleName = "StoreAdapter"

// saveMicroservicesToStore persists the given slice to SQLite.
func saveMicroservicesToStore(microservices []*models.Microservice) error {
	db := store.GetInstance()
	if db.Conn() == nil {
		return fmt.Errorf("SQLite not open")
	}
	return db.SaveMicroservices(microservices)
}

// loadMicroservicesFromStore retrieves all microservices from SQLite.
func loadMicroservicesFromStore() ([]*models.Microservice, error) {
	db := store.GetInstance()
	if db.Conn() == nil {
		return nil, fmt.Errorf("SQLite not open")
	}
	return db.LoadMicroservices()
}

// saveRegistriesToStore persists the given slice to SQLite.
func saveRegistriesToStore(registries []*models.Registry) error {
	db := store.GetInstance()
	if db.Conn() == nil {
		return fmt.Errorf("SQLite not open")
	}
	return db.SaveRegistries(registries)
}

// loadRegistriesFromStore retrieves all registries from SQLite.
func loadRegistriesFromStore() ([]*models.Registry, error) {
	db := store.GetInstance()
	if db.Conn() == nil {
		return nil, fmt.Errorf("SQLite not open")
	}
	return db.LoadRegistries()
}

// MigrateJSONToSQLite checks whether legacy JSON cache files exist and, if so,
// imports their contents into SQLite and deletes the JSON files.
// This runs once on first startup after upgrading to the SQLite-backed release.
func MigrateJSONToSQLite() {
	cacheDir := getCachePath()

	migrateFile(
		filepath.Join(cacheDir, utils.MicroserviceFile),
		"microservices",
		func(data []map[string]interface{}) error {
			microservices := make([]*models.Microservice, 0, len(data))
			for _, d := range data {
				ms, err := parseMicroservice(d)
				if err != nil {
					logging.LogWarn(adapterModuleName, fmt.Sprintf("Skipping invalid microservice during JSON migration: %v", err))
					continue
				}
				microservices = append(microservices, ms)
			}
			return saveMicroservicesToStore(microservices)
		},
	)

	migrateFile(
		filepath.Join(cacheDir, "registries.json"),
		"registries",
		func(data []map[string]interface{}) error {
			registries := make([]*models.Registry, 0, len(data))
			for _, d := range data {
				registries = append(registries, parseRegistry(d))
			}
			return saveRegistriesToStore(registries)
		},
	)
}

// migrateFile reads a legacy JSON cache file (if present), calls importer with
// the parsed array, and removes the file on success.
func migrateFile(
	filePath string,
	label string,
	importer func([]map[string]interface{}) error,
) {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return
	}

	logging.LogInfo(adapterModuleName, fmt.Sprintf("Migrating legacy %s JSON file to SQLite: %s", label, filePath))

	data, _, err := ReadFile(filepath.Base(filePath))
	if err != nil || data == nil {
		logging.LogWarn(adapterModuleName, fmt.Sprintf("Could not read %s for migration, skipping: %v", label, err))
		return
	}

	// ReadFile returns raw JSON; unmarshal into array
	var rawArray []map[string]interface{}
	if err := json.Unmarshal(data, &rawArray); err != nil {
		logging.LogWarn(adapterModuleName, fmt.Sprintf("Could not parse %s JSON for migration: %v", label, err))
		return
	}

	if err := importer(rawArray); err != nil {
		logging.LogError(adapterModuleName, fmt.Sprintf("Failed to import %s into SQLite", label), err)
		return
	}

	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		logging.LogWarn(adapterModuleName, fmt.Sprintf("Could not remove legacy %s file: %v", label, err))
	} else {
		logging.LogInfo(adapterModuleName, fmt.Sprintf("Legacy %s file migrated and removed", label))
	}
}
