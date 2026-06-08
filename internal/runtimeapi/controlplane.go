package runtimeapi

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/eclipse-iofog/edgelet/internal/models"
	"github.com/eclipse-iofog/edgelet/internal/processmanager"
	"github.com/eclipse-iofog/edgelet/internal/utils/logging"
	"github.com/google/uuid"
)

var ErrControlPlaneNotFound = errors.New("control plane deployment not found")

// ErrControlPlaneIdentityImmutable indicates metadata.name or metadata.namespace changed on patch.
type ErrControlPlaneIdentityImmutable struct {
	ExistingNamespace string
	ExistingName      string
	NewNamespace      string
	NewName           string
}

func (e *ErrControlPlaneIdentityImmutable) Error() string {
	return fmt.Sprintf(
		"metadata.name and metadata.namespace cannot change on patch (existing namespace=%q name=%q; manifest namespace=%q name=%q); delete the control plane and re-apply",
		e.ExistingNamespace, e.ExistingName, e.NewNamespace, e.NewName,
	)
}

// ControlPlaneApplyResult is the control plane apply outcome.
type ControlPlaneApplyResult struct {
	Mode           string
	ControllerUUID string
	Namespace      string
	Name           string
	Image          string
	State          string
	RuntimeState   string
	ContainerID    string
	Generation     int64
}

// ParseAndValidateControlPlaneManifest validates ControlPlane YAML.
func (f *Facade) ParseAndValidateControlPlaneManifest(manifest string) (*models.ControlPlaneManifest, error) {
	return models.ParseControlPlaneManifest(manifest)
}

// GetControlPlaneDeployment returns the singleton control plane row.
func (f *Facade) GetControlPlaneDeployment() (*models.ControlPlaneDeployment, error) {
	item, found, err := f.db.GetSystemControlPlane()
	if err != nil {
		return nil, err
	}
	if !found || item == nil {
		return nil, ErrControlPlaneNotFound
	}
	return item, nil
}

// ControlPlaneStatusMap builds the GET /v1/system/controlplane response body.
func ControlPlaneStatusMap(item *models.ControlPlaneDeployment) map[string]any {
	if item == nil {
		return map[string]any{}
	}
	item.NormalizeDefaults()
	return map[string]any{
		"controllerUuid":     item.ControllerUUID,
		"namespace":          item.Namespace,
		"name":               item.Name,
		"image":              item.Image,
		"containerId":        item.ContainerID,
		"state":              item.State,
		"desiredState":       item.DesiredState,
		"runtimeState":       item.RuntimeState,
		"lastError":          item.LastError,
		"restartCount":       item.RestartCount,
		"generation":         item.Generation,
		"observedGeneration": item.ObservedGeneration,
		"lastTransitionAt":   item.LastTransitionAt,
		"source":             "controlplane",
		"type":               "controlplane",
	}
}

// GetControlPlaneManifestMasked returns secrets-redacted manifest YAML.
func (f *Facade) GetControlPlaneManifestMasked() (string, error) {
	item, err := f.GetControlPlaneDeployment()
	if err != nil {
		return "", err
	}
	return models.MaskedControlPlaneManifestYAML(item.ManifestYAML)
}

// DeleteControlPlane removes the controller deployment, container, and volumes.
func (f *Facade) DeleteControlPlane() error {
	pm := processmanager.GetInstance()
	if err := pm.DeleteControlPlane(); err != nil {
		if errors.Is(err, processmanager.ErrControlPlaneNotFound) {
			return ErrControlPlaneNotFound
		}
		return err
	}
	return nil
}

// ApplyControlPlaneManifest validates, persists, and launches the controller.
func (f *Facade) ApplyControlPlaneManifest(manifest, sourceName string, dryRun bool, progress DeployProgressCallback) (*ControlPlaneApplyResult, error) {
	emitDeployProgress(progress, DeployStageParsing, "validating control plane manifest")
	doc, err := f.ParseAndValidateControlPlaneManifest(manifest)
	if err != nil {
		logging.LogWarn(runtimeAPIModuleName, fmt.Sprintf("control plane manifest validation failed: %v", err))
		return nil, err
	}
	doc.NormalizeDefaults()

	var existing *models.ControlPlaneDeployment
	if f.db.Conn() != nil {
		item, found, getErr := f.db.GetSystemControlPlane()
		if getErr != nil {
			return nil, getErr
		}
		if found {
			existing = item
		}
	}

	mode := "create"
	controllerUUID := uuid.NewString()
	nextGeneration := int64(1)
	restartCount := 0
	if existing != nil {
		mode = "patch"
		controllerUUID = strings.TrimSpace(existing.ControllerUUID)
		if controllerUUID == "" {
			return nil, errors.New("existing control plane deployment is missing controller_uuid")
		}
		existing.NormalizeDefaults()
		if existing.Namespace != doc.Metadata.Namespace || existing.Name != doc.Metadata.Name {
			return nil, &ErrControlPlaneIdentityImmutable{
				ExistingNamespace: existing.Namespace,
				ExistingName:      existing.Name,
				NewNamespace:      doc.Metadata.Namespace,
				NewName:           doc.Metadata.Name,
			}
		}
		nextGeneration = existing.Generation + 1
		restartCount = existing.RestartCount
	}

	image := doc.ManifestControllerImage()
	result := &ControlPlaneApplyResult{
		Mode:           mode,
		ControllerUUID: controllerUUID,
		Namespace:      doc.Metadata.Namespace,
		Name:           doc.Metadata.Name,
		Image:          image,
		Generation:     nextGeneration,
	}

	if dryRun {
		result.State = "dry-run"
		result.RuntimeState = "dry-run"
		emitDeployProgress(progress, DeployStageDone, "dry-run completed")
		return result, nil
	}

	nowSec := time.Now().Unix()
	item := &models.ControlPlaneDeployment{
		ControllerUUID:     controllerUUID,
		Namespace:          doc.Metadata.Namespace,
		Name:               doc.Metadata.Name,
		ManifestYAML:       manifest,
		Image:              image,
		State:              "starting",
		DesiredState:       "running",
		RuntimeState:       "starting",
		RestartCount:       restartCount,
		LastTransitionAt:   nowSec,
		LastStartAttemptAt: nowSec,
		Generation:         nextGeneration,
		ObservedGeneration: 0,
	}
	if existing != nil && strings.TrimSpace(existing.ContainerID) != "" {
		item.ContainerID = strings.TrimSpace(existing.ContainerID)
	}

	emitDeployProgress(progress, DeployStagePersisting, "saving control plane deployment")
	if err := f.db.UpsertSystemControlPlane(item); err != nil {
		return nil, err
	}

	launchProgress := func(stage, message string) {
		normalized := strings.TrimSpace(strings.ToLower(stage))
		if normalized == "" {
			return
		}
		emitDeployProgress(progress, normalized, message)
	}
	if err := processmanager.GetInstance().SyncApplyControlPlaneDeployment(item, launchProgress); err != nil {
		return nil, err
	}

	got, err := f.GetControlPlaneDeployment()
	if err != nil {
		return nil, err
	}
	result.State = got.State
	result.RuntimeState = got.RuntimeState
	result.ContainerID = got.ContainerID
	result.Generation = got.Generation
	emitDeployProgress(progress, DeployStageDone, "control plane apply completed")
	return result, nil
}
