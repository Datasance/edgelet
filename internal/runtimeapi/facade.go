package runtimeapi

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/eclipse-iofog/agent/internal/config"
	"github.com/eclipse-iofog/agent/internal/fieldagent"
	"github.com/eclipse-iofog/agent/internal/models"
	"github.com/eclipse-iofog/agent/internal/pruning"
	"github.com/eclipse-iofog/agent/internal/statusreporter"
	"github.com/eclipse-iofog/agent/internal/store"
	"gopkg.in/yaml.v3"
)

// Facade is a thin runtime adapter used by LocalAPI v3 handlers.
// It intentionally delegates to existing modules to preserve daemon behavior.
type Facade struct {
	cfg  *config.Config
	fa   *fieldagent.FieldAgent
	sr   *statusreporter.StatusReporter
	db   *store.DB
	prun *pruning.Manager
}

// NewFacade returns a singleton-backed runtime facade.
func NewFacade() *Facade {
	return &Facade{
		cfg:  config.GetInstance(),
		fa:   fieldagent.GetInstance(),
		sr:   statusreporter.GetInstance(),
		db:   store.GetInstance(),
		prun: pruning.GetInstance(),
	}
}

// Provision provisions the agent.
func (f *Facade) Provision(provisioningKey string) error {
	return f.fa.Provision(provisioningKey)
}

// Deprovision deprovisions the agent.
func (f *Facade) Deprovision() error {
	return f.fa.Deprovision(false)
}

// Prune triggers prune via existing pruning manager.
func (f *Facade) Prune() string {
	return f.prun.PruneAgent()
}

// ListRuntimeMicroservices returns process-manager tracked microservices.
func (f *Facade) ListRuntimeMicroservices() []map[string]interface{} {
	pmStatus := f.sr.GetProcessManagerStatus()
	if pmStatus == nil {
		return []map[string]interface{}{}
	}

	result := make([]map[string]interface{}, 0, len(pmStatus.MicroservicesStatus))
	for uuid, status := range pmStatus.MicroservicesStatus {
		entry := map[string]interface{}{
			"uuid":        uuid,
			"name":        "",
			"application": "",
			"source":      "managed",
			"state":       strings.ToLower(string(status.Status)),
			"containerId": status.ContainerID,
		}
		result = append(result, entry)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i]["uuid"].(string) < result[j]["uuid"].(string)
	})
	return result
}

// GetRuntimeMicroservice returns one process-manager tracked microservice.
func (f *Facade) GetRuntimeMicroservice(id string) (map[string]interface{}, error) {
	pmStatus := f.sr.GetProcessManagerStatus()
	if pmStatus == nil {
		return nil, fmt.Errorf("process manager status unavailable")
	}
	status := pmStatus.GetMicroserviceStatus(id)
	if status == nil || status.Status == models.MicroserviceStateUnknown {
		return nil, fmt.Errorf("microservice not found")
	}

	return map[string]interface{}{
		"uuid":         id,
		"source":       "managed",
		"state":        strings.ToLower(string(status.Status)),
		"containerId":  status.ContainerID,
		"percentage":   status.Percentage,
		"errorMessage": status.ErrorMessage,
		"healthStatus": status.HealthStatus,
	}, nil
}

// UpsertLocalDeployment upserts one local deployment record.
func (f *Facade) UpsertLocalDeployment(ms *models.LocalDeployedMicroservice) error {
	return f.db.UpsertLocalDeployedMicroservice(ms)
}

// ListLocalDeployments lists local deployment records.
func (f *Facade) ListLocalDeployments() ([]*models.LocalDeployedMicroservice, error) {
	return f.db.ListLocalDeployedMicroservices()
}

// GetLocalDeployment gets local deployment by id.
func (f *Facade) GetLocalDeployment(id string) (*models.LocalDeployedMicroservice, error) {
	return f.db.GetLocalDeployedMicroservice(id)
}

// DeleteLocalDeployment removes local deployment by id.
func (f *Facade) DeleteLocalDeployment(id string) error {
	return f.db.DeleteLocalDeployedMicroservice(id)
}

// ParseAndValidateLocalManifest validates a local deploy manifest.
func (f *Facade) ParseAndValidateLocalManifest(manifest string) (*models.LocalDeployManifest, error) {
	doc := &models.LocalDeployManifest{}
	if err := yaml.Unmarshal([]byte(manifest), doc); err != nil {
		return nil, fmt.Errorf("invalid manifest YAML: %w", err)
	}
	if err := doc.Validate(); err != nil {
		return nil, err
	}
	return doc, nil
}

// ApplyLocalManifest validates and persists local deployment metadata.
func (f *Facade) ApplyLocalManifest(manifest, sourceName string, dryRun bool) (string, *models.LocalDeployManifest, error) {
	doc, err := f.ParseAndValidateLocalManifest(manifest)
	if err != nil {
		return "", nil, err
	}
	deploymentID := fmt.Sprintf("local-%d", time.Now().UnixNano())
	if dryRun {
		return deploymentID, doc, nil
	}
	if sourceName == "" {
		sourceName = "local-cli"
	}
	if err := f.UpsertLocalDeployment(&models.LocalDeployedMicroservice{
		LocalUUID:        deploymentID,
		ApplicationName:  "",
		MicroserviceName: doc.Metadata.Name,
		SourceName:       sourceName,
		ManifestYAML:     manifest,
		ImageName:        doc.Spec.Image,
		State:            "validated",
	}); err != nil {
		return "", nil, err
	}
	return deploymentID, doc, nil
}

// ParseAndValidateLocalRegistryManifest validates a local registry manifest.
func (f *Facade) ParseAndValidateLocalRegistryManifest(manifest string) (*models.LocalRegistryManifest, error) {
	doc := &models.LocalRegistryManifest{}
	if err := yaml.Unmarshal([]byte(manifest), doc); err != nil {
		return nil, fmt.Errorf("invalid registry manifest YAML: %w", err)
	}
	if err := doc.Validate(); err != nil {
		return nil, err
	}
	return doc, nil
}

// ApplyLocalRegistryManifest validates and stores a registry manifest.
func (f *Facade) ApplyLocalRegistryManifest(manifest string, dryRun bool) (*models.Registry, error) {
	doc, err := f.ParseAndValidateLocalRegistryManifest(manifest)
	if err != nil {
		return nil, err
	}
	reg := models.NewRegistry(doc.Spec.ID, doc.Spec.URL, doc.Spec.IsPublic, doc.Spec.UserName, doc.Spec.Password, doc.Spec.UserEmail)
	if dryRun {
		return reg, nil
	}
	if err := f.db.UpsertRegistry(reg); err != nil {
		return nil, err
	}
	if err := f.db.EnsureDefaultRegistries(); err != nil {
		return nil, err
	}
	return reg, nil
}

// ListRegistries returns persisted registry entries with defaults guaranteed.
func (f *Facade) ListRegistries() ([]*models.Registry, error) {
	if err := f.db.EnsureDefaultRegistries(); err != nil {
		return nil, err
	}
	return f.db.LoadRegistries()
}
