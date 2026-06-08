//revive:disable:nested-structs
package runtimeapi

import (
	"bytes"
	"cmp"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/eclipse-iofog/edgelet/internal/buildmeta"
	"github.com/eclipse-iofog/edgelet/internal/config"
	"github.com/eclipse-iofog/edgelet/internal/fieldagent"
	"github.com/eclipse-iofog/edgelet/internal/models"
	"github.com/eclipse-iofog/edgelet/internal/network"
	"github.com/eclipse-iofog/edgelet/internal/processmanager"
	"github.com/eclipse-iofog/edgelet/internal/pruning"
	"github.com/eclipse-iofog/edgelet/internal/statusreporter"
	"github.com/eclipse-iofog/edgelet/internal/store"
	"github.com/eclipse-iofog/edgelet/internal/utils/logging"
	"github.com/eclipse-iofog/edgelet/internal/workloadmeta"
	"github.com/eclipse-iofog/edgelet/pkg/engine"
	"github.com/eclipse-iofog/edgelet/pkg/imageref"
	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

const runtimeAPIModuleName = "Runtime API Facade"

var ErrRuntimeClassUnsupported = errors.New("runtimeclass is supported only when containerEngine=edgelet")

const (
	RuntimeClassStageWriteConfig     = "write_config"
	RuntimeClassStageStopRuntime     = "stop_runtime"
	RuntimeClassStageStartRuntime    = "start_runtime"
	RuntimeClassStageWaitCRIReady    = "wait_cri_ready"
	RuntimeClassStageVerifyStability = "verify_stability"
	RuntimeClassStageRollbackConfig  = "rollback_config"
	RuntimeClassStageEscalateRestart = "escalate_restart"
	RuntimeClassStageDone            = "done"
)

func NormalizeRuntimeClassOperationStage(stage string) string {
	normalized := strings.TrimSpace(strings.ToLower(stage))
	switch normalized {
	case RuntimeClassStageWriteConfig,
		RuntimeClassStageStopRuntime,
		RuntimeClassStageStartRuntime,
		RuntimeClassStageWaitCRIReady,
		RuntimeClassStageVerifyStability,
		RuntimeClassStageRollbackConfig,
		RuntimeClassStageEscalateRestart,
		RuntimeClassStageDone:
		return normalized
	case "persisting":
		return RuntimeClassStageWriteConfig
	case "reconfiguring":
		return RuntimeClassStageStopRuntime
	default:
		return ""
	}
}

type ErrReservedRuntimeClassDelete struct {
	Name string
}

func (e *ErrReservedRuntimeClassDelete) Error() string {
	return fmt.Sprintf("runtimeclass delete is not allowed for reserved runtime name: %s", strings.TrimSpace(strings.ToLower(e.Name)))
}

func (e *ErrReservedRuntimeClassDelete) Details() map[string]any {
	return map[string]any{
		"runtimeClassName": strings.TrimSpace(strings.ToLower(e.Name)),
	}
}

type ErrRuntimeClassInUse struct {
	Name                      string
	RuntimeNames              []string
	BlockingMicroserviceUuids []string
}

func (e *ErrRuntimeClassInUse) Error() string {
	name := strings.TrimSpace(strings.ToLower(e.Name))
	runtimeUsed := ""
	if len(e.RuntimeNames) > 0 {
		runtimeUsed = strings.TrimSpace(e.RuntimeNames[0])
	}
	firstUUID := ""
	if len(e.BlockingMicroserviceUuids) > 0 {
		firstUUID = strings.TrimSpace(e.BlockingMicroserviceUuids[0])
	}
	return fmt.Sprintf("cannot delete runtimeclass '%s': microservice uuid=%s is still using runtime '%s'; delete dependent microservices first", name, firstUUID, runtimeUsed)
}

func (e *ErrRuntimeClassInUse) Details() map[string]any {
	return map[string]any{
		"runtimeClassName":          strings.TrimSpace(strings.ToLower(e.Name)),
		"runtimeNames":              append([]string{}, e.RuntimeNames...),
		"blockingMicroserviceUuids": append([]string{}, e.BlockingMicroserviceUuids...),
	}
}

type ErrRuntimeClassOperation struct {
	Stage string
	Err   error
}

func (e *ErrRuntimeClassOperation) Error() string {
	if e == nil || e.Err == nil {
		return "runtimeclass operation failed"
	}
	return e.Err.Error()
}

func (e *ErrRuntimeClassOperation) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *ErrRuntimeClassOperation) Details() map[string]any {
	stage := NormalizeRuntimeClassOperationStage(e.Stage)
	if stage == "" {
		return nil
	}
	return map[string]any{
		"stage": stage,
	}
}

func wrapRuntimeClassOperationError(stage string, err error) error {
	if err == nil {
		return nil
	}
	normalizedStage := NormalizeRuntimeClassOperationStage(stage)
	if normalizedStage == "" {
		normalizedStage = RuntimeClassStageWriteConfig
	}
	return &ErrRuntimeClassOperation{
		Stage: normalizedStage,
		Err:   err,
	}
}

const (
	PruneModeDangling   = "dangling"
	PruneModeContainers = "containers"
	PruneModeVolumes    = "volumes"
	PruneModeAll        = "all"
)

// Facade is a thin runtime adapter used by EdgeletAPI handlers.
// It intentionally delegates to existing modules to preserve daemon behavior.
type Facade struct {
	cfg  *config.Config
	fa   *fieldagent.FieldAgent
	sr   *statusreporter.StatusReporter
	db   *store.DB
	prun *pruning.Manager
}

// DeployProgressCallback reports deploy stage transitions from runtime flow.
type DeployProgressCallback func(stage string, message string)

const (
	DeployStageParsing    = "parsing"
	DeployStagePulling    = "pulling"
	DeployStageCreating   = "creating"
	DeployStageStarting   = "starting"
	DeployStagePersisting = "persisting"
	DeployStageDone       = "done"
)

func emitDeployProgress(cb DeployProgressCallback, stage, message string) {
	if cb != nil {
		cb(stage, message)
	}
}

// ErrAmbiguousMicroserviceSelector indicates selector matched multiple targets.
type ErrAmbiguousMicroserviceSelector struct {
	Matches []string
}

func (e *ErrAmbiguousMicroserviceSelector) Error() string {
	if e == nil || len(e.Matches) == 0 {
		return "ambiguous microservice selector"
	}
	return fmt.Sprintf("ambiguous selector; matches: %s", strings.Join(e.Matches, ", "))
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
func (f *Facade) Deprovision(scope string) error {
	normalized := strings.ToLower(strings.TrimSpace(scope))
	if normalized == "" {
		normalized = fieldagent.DeprovisionScopeAll
	}
	switch normalized {
	case fieldagent.DeprovisionScopeAll, fieldagent.DeprovisionScopeLocal:
		return f.fa.DeprovisionWithScope(false, normalized)
	default:
		return fmt.Errorf("invalid deprovision scope %q (allowed: %s|%s)", scope, fieldagent.DeprovisionScopeAll, fieldagent.DeprovisionScopeLocal)
	}
}

func normalizePruneMode(mode string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(mode))
	if normalized == "" {
		return PruneModeDangling, nil
	}
	switch normalized {
	case PruneModeDangling, PruneModeContainers, PruneModeVolumes, PruneModeAll:
		return normalized, nil
	default:
		return "", fmt.Errorf("invalid prune mode %q (allowed: %s|%s|%s|%s)", mode, PruneModeDangling, PruneModeContainers, PruneModeVolumes, PruneModeAll)
	}
}

// Prune triggers runtime prune for the requested mode.
func (f *Facade) Prune(mode string) (map[string]any, error) {
	normalizedMode, err := normalizePruneMode(mode)
	if err != nil {
		return nil, err
	}
	pm := processmanager.GetInstance()
	result := map[string]any{
		"status": "ok",
		"mode":   normalizedMode,
		"engine": currentEngineName(f.cfg),
	}
	switch normalizedMode {
	case PruneModeDangling:
		report, pruneErr := pm.PruneDanglingImages()
		if pruneErr != nil {
			return nil, pruneErr
		}
		deleted := make([]string, 0)
		reclaimed := int64(0)
		deletedCount := 0
		if report != nil {
			deleted = append(deleted, report.Deleted...)
			reclaimed = report.SpaceReclaimedBytes
			deletedCount = report.DeletedCount
		}
		slices.Sort(deleted)
		result["deleted"] = deleted
		result["deletedCount"] = deletedCount
		result["spaceReclaimedBytes"] = reclaimed
		result["spaceReclaimedHuman"] = humanBytes(reclaimed)
		result["message"] = "pruned dangling images"
	case PruneModeContainers:
		report, pruneErr := pm.PruneContainers()
		if pruneErr != nil {
			return nil, pruneErr
		}
		deleted := make([]string, 0)
		deletedCount := 0
		if report != nil {
			deleted = append(deleted, report.Deleted...)
			deletedCount = report.DeletedCount
		}
		slices.Sort(deleted)
		result["deleted"] = deleted
		result["deletedCount"] = deletedCount
		result["message"] = "pruned containers"
	case PruneModeVolumes:
		report, pruneErr := pm.PruneVolumes()
		if pruneErr != nil {
			return nil, pruneErr
		}
		deleted := make([]string, 0)
		deletedCount := 0
		reclaimed := int64(0)
		if report != nil {
			deleted = append(deleted, report.Deleted...)
			deletedCount = report.DeletedCount
			reclaimed = report.SpaceReclaimedBytes
		}
		slices.Sort(deleted)
		result["deleted"] = deleted
		result["deletedCount"] = deletedCount
		result["spaceReclaimedBytes"] = reclaimed
		result["spaceReclaimedHuman"] = humanBytes(reclaimed)
		result["message"] = "pruned volumes"
	case PruneModeAll:
		var (
			containerReport *engine.ContainerPruneReport
			volumeReport    *engine.VolumePruneReport
			imageReport     *engine.ImagePruneReport
			errorsByStep    = map[string]string{}
		)
		if r, pruneErr := pm.PruneContainers(); pruneErr != nil {
			errorsByStep["containers"] = pruneErr.Error()
		} else {
			containerReport = r
		}
		if r, pruneErr := pm.PruneVolumes(); pruneErr != nil {
			errorsByStep["volumes"] = pruneErr.Error()
		} else {
			volumeReport = r
		}
		if r, pruneErr := pm.PruneDanglingImages(); pruneErr != nil {
			errorsByStep["images"] = pruneErr.Error()
		} else {
			imageReport = r
		}
		containerDeleted := make([]string, 0)
		containerDeletedCount := 0
		if containerReport != nil {
			containerDeleted = append(containerDeleted, containerReport.Deleted...)
			containerDeletedCount = containerReport.DeletedCount
		}
		slices.Sort(containerDeleted)
		volumeDeleted := make([]string, 0)
		volumeDeletedCount := 0
		volumeReclaimed := int64(0)
		if volumeReport != nil {
			volumeDeleted = append(volumeDeleted, volumeReport.Deleted...)
			volumeDeletedCount = volumeReport.DeletedCount
			volumeReclaimed = volumeReport.SpaceReclaimedBytes
		}
		slices.Sort(volumeDeleted)
		imageDeleted := make([]string, 0)
		imageDeletedCount := 0
		imageReclaimed := int64(0)
		if imageReport != nil {
			imageDeleted = append(imageDeleted, imageReport.Deleted...)
			imageDeletedCount = imageReport.DeletedCount
			imageReclaimed = imageReport.SpaceReclaimedBytes
		}
		slices.Sort(imageDeleted)
		result["containersDeleted"] = containerDeleted
		result["containersDeletedCount"] = containerDeletedCount
		result["volumesDeleted"] = volumeDeleted
		result["volumesDeletedCount"] = volumeDeletedCount
		result["imagesDeleted"] = imageDeleted
		result["imagesDeletedCount"] = imageDeletedCount
		result["deletedCount"] = containerDeletedCount + volumeDeletedCount + imageDeletedCount
		result["spaceReclaimedBytes"] = volumeReclaimed + imageReclaimed
		result["spaceReclaimedHuman"] = humanBytes(volumeReclaimed + imageReclaimed)
		if len(errorsByStep) > 0 {
			result["status"] = "partial"
			result["errors"] = errorsByStep
			result["message"] = "pruned containers, volumes, and dangling images with partial failures"
		} else {
			result["message"] = "pruned containers, volumes, and dangling images"
		}
	default:
		return nil, fmt.Errorf("unsupported prune mode %q", normalizedMode)
	}
	return result, nil
}

// ListImages returns normalized image list for local runtime engine.
func (f *Facade) ListImages() ([]map[string]any, error) {
	items, err := processmanager.GetInstance().ListImages()
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		repo := strings.TrimSpace(item.Repository)
		tag := strings.TrimSpace(item.Tag)
		if repo == "" {
			repo = "<none>"
		}
		if tag == "" {
			tag = "<none>"
		}
		shortID := strings.TrimSpace(item.ShortID)
		if shortID == "" {
			shortID = strings.TrimPrefix(strings.TrimSpace(item.ID), "sha256:")
			if len(shortID) > 12 {
				shortID = shortID[:12]
			}
		}
		engineName := strings.TrimSpace(item.Engine)
		if engineName == "" {
			engineName = currentEngineName(f.cfg)
		}
		createdAt := ""
		if !item.CreatedAt.IsZero() {
			createdAt = item.CreatedAt.UTC().Format(time.RFC3339)
		}
		out = append(out, map[string]any{
			"id":         item.ID,
			"shortId":    shortID,
			"repository": repo,
			"tag":        tag,
			"digest":     item.Digest,
			"createdAt":  createdAt,
			"sizeBytes":  item.SizeBytes,
			"sizeHuman":  humanBytes(item.SizeBytes),
			"engine":     engineName,
		})
	}
	return out, nil
}

// PullImage pulls an image with optional registry id and platform selector.
func (f *Facade) PullImage(imageRef string, registryID *int, platform string) (string, error) {
	imageRef = strings.TrimSpace(imageRef)
	if imageRef == "" {
		return "", errors.New("image is required")
	}
	resolvedImage := imageRef
	var reg *models.Registry
	if registryID != nil {
		if *registryID <= 0 {
			return "", errors.New("registryId must be greater than zero")
		}
		item, err := f.db.GetLocalRegistry(*registryID)
		if err != nil || item == nil {
			return "", fmt.Errorf("registryId %d not found", *registryID)
		}
		if strings.EqualFold(strings.TrimSpace(item.URL), "from_cache") {
			return "", fmt.Errorf("registryId %d cannot be used for pull (from_cache)", *registryID)
		}
		reg = item
		resolvedImage, _ = imageref.Resolve(imageRef, item.URL, false)
	}
	if p := strings.TrimSpace(platform); p != "" {
		if !isValidOCIPlatform(p) {
			return "", errors.New("platform must follow os/arch[/variant] format")
		}
	}
	if err := processmanager.GetInstance().PullImage(resolvedImage, reg, strings.TrimSpace(platform)); err != nil {
		return resolvedImage, err
	}
	return resolvedImage, nil
}

// PullImageWithProgress pulls an image while reporting progress updates.
func (f *Facade) PullImageWithProgress(imageRef string, registryID *int, platform string, onProgress func(float32)) (string, error) {
	imageRef = strings.TrimSpace(imageRef)
	if imageRef == "" {
		return "", errors.New("image is required")
	}
	resolvedImage := imageRef
	var reg *models.Registry
	if registryID != nil {
		if *registryID <= 0 {
			return "", errors.New("registryId must be greater than zero")
		}
		item, err := f.db.GetLocalRegistry(*registryID)
		if err != nil || item == nil {
			return "", fmt.Errorf("registryId %d not found", *registryID)
		}
		if strings.EqualFold(strings.TrimSpace(item.URL), "from_cache") {
			return "", fmt.Errorf("registryId %d cannot be used for pull (from_cache)", *registryID)
		}
		reg = item
		resolvedImage, _ = imageref.Resolve(imageRef, item.URL, false)
	}
	if p := strings.TrimSpace(platform); p != "" {
		if !isValidOCIPlatform(p) {
			return "", errors.New("platform must follow os/arch[/variant] format")
		}
	}
	if err := processmanager.GetInstance().PullImageWithProgress(resolvedImage, reg, strings.TrimSpace(platform), onProgress); err != nil {
		return resolvedImage, err
	}
	return resolvedImage, nil
}

// LoadImageFromPath imports a daemon-local image archive path.
func (f *Facade) LoadImageFromPath(path string) ([]engine.LoadedImage, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("path is required")
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to access path: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("path must point to a regular file")
	}
	return processmanager.GetInstance().LoadImageFromPath(path)
}

// RemoveImage removes an image by selector (id, id prefix, name:tag, or digest).
func (f *Facade) RemoveImage(selector string) (string, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return "", errors.New("selector is required")
	}
	pm := processmanager.GetInstance()
	if err := pm.RemoveImage(selector); err == nil {
		return selector, nil
	}

	if !looksLikeImageIDPrefix(selector) {
		return "", fmt.Errorf("image not found: %s", selector)
	}
	items, err := pm.ListImages()
	if err != nil {
		return "", err
	}
	matches := make([]string, 0)
	for _, item := range items {
		if imageIDMatchesPrefix(item.ID, selector) {
			matches = append(matches, item.ID)
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("image not found: %s", selector)
	case 1:
		if err := pm.RemoveImage(matches[0]); err != nil {
			return "", err
		}
		return matches[0], nil
	default:
		slices.Sort(matches)
		candidates := matches
		if len(candidates) > 5 {
			candidates = candidates[:5]
		}
		return "", fmt.Errorf("ambiguous image id prefix %q; candidates: %s", selector, strings.Join(candidates, ", "))
	}
}

// ListRuntimeMicroservices returns process-manager tracked microservices.
func (f *Facade) ListRuntimeMicroservices() []map[string]any {
	pmStatus := f.sr.GetProcessManagerStatus()
	capHint := 0
	if pmStatus != nil {
		capHint = len(pmStatus.MicroservicesStatus)
	}
	result := make([]map[string]any, 0, capHint)
	msByUUID := make(map[string]*models.Microservice)
	for _, ms := range f.fa.GetLatestMicroservices() {
		msByUUID[ms.MicroserviceUUID] = ms
	}
	localByUUID := make(map[string]*models.LocalDeployedMicroservice)
	if locals, err := f.db.ListLocalWorkloads(); err == nil {
		for _, item := range locals {
			localByUUID[item.LocalUUID] = item
		}
	}

	controlPlane, hasControlPlane := f.controlPlaneDeploymentRow()
	controlPlaneUUID := ""
	if hasControlPlane {
		controlPlaneUUID = strings.TrimSpace(controlPlane.ControllerUUID)
	}

	suppressedManaged := 0
	if pmStatus != nil {
		for uuid, status := range pmStatus.MicroservicesStatus {
			if controlPlaneUUID != "" && uuid == controlPlaneUUID {
				suppressedManaged++
				continue
			}
			if shouldSuppressManagedRuntimeEntry(uuid, status, msByUUID, localByUUID) {
				suppressedManaged++
				continue
			}
			name := ""
			application := ""
			image := ""
			if ms, ok := msByUUID[uuid]; ok {
				name = ms.MicroserviceName
				application = ms.ApplicationName
				image = ms.ImageName
			}
			entry := map[string]any{
				"uuid":        uuid,
				"name":        name,
				"application": application,
				"source":      "managed",
				"type":        "managed",
				"state":       strings.ToLower(string(status.Status)),
				"containerId": status.ContainerID,
				"image":       image,
			}
			result = append(result, entry)
		}
	}
	if suppressedManaged > 0 {
		logging.LogDebug(runtimeAPIModuleName, fmt.Sprintf("suppressed %d stale managed runtime entries", suppressedManaged))
	}

	for _, item := range localByUUID {
		state := strings.TrimSpace(item.RuntimeState)
		if state == "" {
			state = strings.TrimSpace(item.State)
		}
		entry := map[string]any{
			"uuid":        item.LocalUUID,
			"name":        item.MicroserviceName,
			"application": item.ApplicationName,
			"source":      strings.TrimSpace(item.SourceName),
			"type":        "local",
			"state":       state,
			"containerId": strings.TrimSpace(item.ContainerID),
			"image":       strings.TrimSpace(item.ImageName),
		}
		if entry["source"] == "" {
			entry["source"] = "local-cli"
		}
		result = append(result, entry)
	}

	if hasControlPlane {
		if entry := controlPlaneRuntimeListEntry(controlPlane); entry != nil {
			result = append(result, entry)
		}
	}
	slices.SortFunc(result, func(a, b map[string]any) int {
		uuidA, ok := a["uuid"].(string)
		if !ok {
			uuidA = ""
		}
		uuidB, ok := b["uuid"].(string)
		if !ok {
			uuidB = ""
		}
		return cmp.Compare(uuidA, uuidB)
	})
	return result
}

// GetRuntimeMicroservice returns one process-manager tracked microservice.
func (f *Facade) GetRuntimeMicroservice(id string) (map[string]any, error) {
	uuid, err := f.ResolveMicroserviceID(id)
	if err != nil {
		return nil, err
	}
	if item, ok := f.controlPlaneDeploymentRow(); ok && strings.TrimSpace(item.ControllerUUID) == uuid {
		entry := controlPlaneRuntimeListEntry(item)
		if entry == nil {
			return nil, errors.New("control plane deployment not found")
		}
		entry["raw"] = map[string]any{
			"engineInspect":        f.engineInspectForMicroservice(uuid),
			"engineType":           currentEngineName(f.cfg),
			"inspectSchemaVersion": "v1",
		}
		return entry, nil
	}
	containerInspect := f.engineInspectForMicroservice(uuid)
	if local, localErr := f.db.GetLocalWorkload(uuid); localErr == nil && local != nil {
		state := strings.TrimSpace(local.RuntimeState)
		if state == "" {
			state = strings.TrimSpace(local.State)
		}
		return map[string]any{
			"uuid":         local.LocalUUID,
			"name":         local.MicroserviceName,
			"application":  local.ApplicationName,
			"source":       local.SourceName,
			"type":         "local",
			"state":        state,
			"containerId":  local.ContainerID,
			"image":        local.ImageName,
			"desiredState": local.DesiredState,
			"runtimeState": local.RuntimeState,
			"lastError":    local.LastError,
			"restartCount": local.RestartCount,
			"manifestYAML": local.ManifestYAML,
			"raw": map[string]any{
				"localDeployment":      local,
				"engineInspect":        containerInspect,
				"engineType":           currentEngineName(f.cfg),
				"inspectSchemaVersion": "v1",
			},
		}, nil
	}
	pmStatus := f.sr.GetProcessManagerStatus()
	if pmStatus == nil {
		return nil, errors.New("process manager status unavailable")
	}
	status := pmStatus.GetMicroserviceStatus(uuid)
	if status == nil || status.Status == models.MicroserviceStateUnknown {
		return nil, errors.New("microservice not found")
	}
	name := ""
	application := ""
	image := ""
	if ms := f.fa.FindLatestMicroserviceByUUID(uuid); ms != nil {
		name = ms.MicroserviceName
		application = ms.ApplicationName
		image = ms.ImageName
	}
	return map[string]any{
		"uuid":         uuid,
		"name":         name,
		"application":  application,
		"source":       "managed",
		"type":         "managed",
		"state":        strings.ToLower(string(status.Status)),
		"containerId":  status.ContainerID,
		"image":        image,
		"percentage":   status.Percentage,
		"errorMessage": status.ErrorMessage,
		"healthStatus": status.HealthStatus,
		"raw": map[string]any{
			"processManager":       status,
			"engineInspect":        containerInspect,
			"engineType":           currentEngineName(f.cfg),
			"inspectSchemaVersion": "v1",
		},
	}, nil
}

func (f *Facade) engineInspectForMicroservice(microserviceUUID string) map[string]any {
	if cont, contErr := processmanager.GetInstance().GetContainerForMicroservice(microserviceUUID); contErr == nil && cont != nil {
		if rawInspect, rawErr := processmanager.GetInstance().InspectContainerRaw(cont.ID); rawErr == nil && rawInspect != nil {
			return rawInspect
		}
		return map[string]any{
			"id":     cont.ID,
			"names":  cont.Names,
			"image":  cont.Image,
			"status": cont.Status,
			"state":  cont.State,
			"labels": cont.Labels,
		}
	}
	return map[string]any{}
}

func currentEngineName(cfg *config.Config) string {
	if cfg == nil {
		return "docker"
	}
	engineName := strings.ToLower(strings.TrimSpace(cfg.ContainerEngine))
	if engineName == "" {
		engineName = "docker"
	}
	return engineName
}

func shouldSuppressManagedRuntimeEntry(
	uuid string,
	status *models.MicroserviceStatus,
	managedByUUID map[string]*models.Microservice,
	localByUUID map[string]*models.LocalDeployedMicroservice,
) bool {
	if uuid == "" {
		return true
	}
	if status == nil {
		return true
	}
	if _, ok := managedByUUID[uuid]; ok {
		return false
	}
	// Local deployments are rendered from local_workloads rows.
	// Suppress process-manager projection for the same UUID to avoid duplicates.
	if _, ok := localByUUID[uuid]; ok {
		return true
	}

	switch status.Status {
	case models.MicroserviceStateDeleted,
		models.MicroserviceStateUnknown,
		models.MicroserviceStateQueued,
		models.MicroserviceStateUpdating,
		models.MicroserviceStateMarkedForDeletion,
		models.MicroserviceStateDeleting:
		return true
	default:
		return false
	}
}

func isValidOCIPlatform(v string) bool {
	parts := strings.Split(strings.TrimSpace(v), "/")
	if len(parts) < 2 || len(parts) > 3 {
		return false
	}
	for _, p := range parts {
		if strings.TrimSpace(p) == "" {
			return false
		}
	}
	return true
}

func humanBytes(size int64) string {
	if size <= 0 {
		return "0 B"
	}
	units := []string{"B", "KB", "MB", "GB", "TB"}
	value := float64(size)
	idx := 0
	for value >= 1024 && idx < len(units)-1 {
		value /= 1024
		idx++
	}
	return strconv.FormatFloat(value, 'f', 1, 64) + " " + units[idx]
}

var imageIDPrefixPattern = regexp.MustCompile(`^[a-f0-9]{3,64}$`)

func looksLikeImageIDPrefix(selector string) bool {
	normalized := strings.TrimSpace(strings.TrimPrefix(strings.ToLower(selector), "sha256:"))
	return imageIDPrefixPattern.MatchString(normalized)
}

func imageIDMatchesPrefix(imageID, prefix string) bool {
	imageID = strings.TrimSpace(strings.ToLower(imageID))
	prefix = strings.TrimSpace(strings.ToLower(prefix))
	if imageID == "" || prefix == "" {
		return false
	}
	if strings.HasPrefix(imageID, prefix) {
		return true
	}
	return strings.HasPrefix(strings.TrimPrefix(imageID, "sha256:"), strings.TrimPrefix(prefix, "sha256:"))
}

// ResolveMicroserviceID resolves selectors in this order:
// exact UUID -> container ID prefix -> exact ApplicationName.MicroserviceName.
func (f *Facade) ResolveMicroserviceID(selector string) (string, error) {
	trimmed := strings.TrimSpace(selector)
	if trimmed == "" {
		return "", errors.New("microservice id is required")
	}

	if f.fa.FindLatestMicroserviceByUUID(trimmed) != nil {
		return trimmed, nil
	}
	if local, err := f.db.GetLocalWorkload(trimmed); err == nil && local != nil {
		return trimmed, nil
	}
	if item, ok := f.controlPlaneDeploymentRow(); ok && strings.TrimSpace(item.ControllerUUID) == trimmed {
		return trimmed, nil
	}

	if strings.Contains(trimmed, ".") {
		parts := strings.SplitN(trimmed, ".", 2)
		app := strings.TrimSpace(parts[0])
		name := strings.TrimSpace(parts[1])
		if app == "" || name == "" {
			return "", fmt.Errorf("invalid selector %q", selector)
		}
		matchSet := make(map[string]struct{})
		matches := make([]string, 0)
		for _, ms := range f.fa.GetLatestMicroservices() {
			if strings.EqualFold(ms.ApplicationName, app) && strings.EqualFold(ms.MicroserviceName, name) {
				if _, exists := matchSet[ms.MicroserviceUUID]; exists {
					continue
				}
				matchSet[ms.MicroserviceUUID] = struct{}{}
				matches = append(matches, ms.MicroserviceUUID)
			}
		}
		if locals, err := f.db.ListLocalWorkloads(); err == nil {
			for _, item := range locals {
				if !strings.EqualFold(strings.TrimSpace(item.ApplicationName), app) ||
					!strings.EqualFold(strings.TrimSpace(item.MicroserviceName), name) {
					continue
				}
				uuid := strings.TrimSpace(item.LocalUUID)
				if uuid == "" {
					continue
				}
				if _, exists := matchSet[uuid]; exists {
					continue
				}
				matchSet[uuid] = struct{}{}
				matches = append(matches, uuid)
			}
		}
		if item, ok := f.controlPlaneDeploymentRow(); ok {
			if strings.EqualFold(strings.TrimSpace(item.Namespace), app) &&
				strings.EqualFold(strings.TrimSpace(item.Name), name) {
				uuid := strings.TrimSpace(item.ControllerUUID)
				if uuid != "" {
					if _, exists := matchSet[uuid]; !exists {
						matches = append(matches, uuid)
					}
				}
			}
		}
		switch len(matches) {
		case 0:
			return "", errors.New("microservice not found")
		case 1:
			return matches[0], nil
		default:
			slices.Sort(matches)
			return "", &ErrAmbiguousMicroserviceSelector{Matches: matches}
		}
	}

	pm := processmanager.GetInstance()
	if cont, matches, err := pm.GetContainerByIDPrefix(trimmed); err != nil {
		return "", err
	} else if len(matches) > 1 {
		return "", &ErrAmbiguousMicroserviceSelector{Matches: matches}
	} else if cont != nil {
		if uuid := workloadmeta.MicroserviceUIDFromLabels(cont.Labels); uuid != "" {
			return uuid, nil
		}
		derived := processmanager.GetInstance().GetMicroserviceUUIDForContainer(*cont)
		if strings.TrimSpace(derived) != "" && derived != cont.ID {
			return strings.TrimSpace(derived), nil
		}
		return "", fmt.Errorf("container %s is not mapped to an iofog microservice", cont.ID)
	}

	return "", errors.New("microservice not found")
}

func (f *Facade) StartRuntimeMicroservice(selector string) (string, error) {
	uuid, err := f.ResolveMicroserviceID(selector)
	if err != nil {
		return "", err
	}
	if err := f.guardControlPlaneMicroserviceMutation(uuid, "start"); err != nil {
		return "", err
	}
	if local, localErr := f.db.GetLocalWorkload(uuid); localErr == nil && local != nil {
		local.DesiredState = "running"
		local.RuntimeState = "starting"
		local.State = local.RuntimeState
		local.LastError = ""
		local.Generation++
		local.LastTransitionAt = time.Now().Unix()
		if err := f.db.UpsertLocalWorkload(local); err != nil {
			return "", err
		}
	}
	if err := processmanager.GetInstance().StartMicroservice(uuid); err != nil {
		return "", err
	}
	if local, localErr := f.db.GetLocalWorkload(uuid); localErr == nil && local != nil {
		local.RuntimeState = "running"
		local.State = local.RuntimeState
		local.LastError = ""
		local.LastTransitionAt = time.Now().Unix()
		_ = f.db.UpsertLocalWorkload(local)
	}
	return uuid, nil
}

func (f *Facade) StopRuntimeMicroservice(selector string) (string, error) {
	uuid, err := f.ResolveMicroserviceID(selector)
	if err != nil {
		return "", err
	}
	if err := f.guardControlPlaneMicroserviceMutation(uuid, "stop"); err != nil {
		return "", err
	}
	if local, localErr := f.db.GetLocalWorkload(uuid); localErr == nil && local != nil {
		local.DesiredState = "stopped"
		local.RuntimeState = "stopping"
		local.State = local.RuntimeState
		local.LastError = ""
		local.Generation++
		local.LastTransitionAt = time.Now().Unix()
		if err := f.db.UpsertLocalWorkload(local); err != nil {
			return "", err
		}
	}
	if err := processmanager.GetInstance().StopMicroservice(uuid); err != nil {
		return "", err
	}
	return uuid, nil
}

func (f *Facade) KillRuntimeMicroservice(selector string) (string, error) {
	uuid, err := f.ResolveMicroserviceID(selector)
	if err != nil {
		return "", err
	}
	if err := f.guardControlPlaneMicroserviceMutation(uuid, "kill"); err != nil {
		return "", err
	}
	if err := processmanager.GetInstance().KillMicroservice(uuid); err != nil {
		return "", err
	}
	return uuid, nil
}

func (f *Facade) RestartRuntimeMicroservice(selector string) (string, error) {
	uuid, err := f.ResolveMicroserviceID(selector)
	if err != nil {
		return "", err
	}
	if err := f.guardControlPlaneMicroserviceMutation(uuid, "restart"); err != nil {
		return "", err
	}
	if local, localErr := f.db.GetLocalWorkload(uuid); localErr == nil && local != nil {
		local.DesiredState = "running"
		local.RuntimeState = "restarting"
		local.State = local.RuntimeState
		local.LastError = ""
		local.RestartCount++
		local.Generation++
		local.LastTransitionAt = time.Now().Unix()
		if err := f.db.UpsertLocalWorkload(local); err != nil {
			return "", err
		}
	}
	if err := processmanager.GetInstance().RestartMicroservice(uuid); err != nil {
		return "", err
	}
	if local, localErr := f.db.GetLocalWorkload(uuid); localErr == nil && local != nil {
		local.RuntimeState = "running"
		local.State = local.RuntimeState
		local.LastError = ""
		local.LastTransitionAt = time.Now().Unix()
		_ = f.db.UpsertLocalWorkload(local)
	}
	return uuid, nil
}

func (f *Facade) RemoveRuntimeMicroservice(selector string) (string, error) {
	uuid, err := f.ResolveMicroserviceID(selector)
	if err != nil {
		return "", err
	}
	if err := f.guardControlPlaneMicroserviceMutation(uuid, "rm"); err != nil {
		return "", err
	}
	if local, localErr := f.db.GetLocalWorkload(uuid); localErr == nil && local != nil {
		nowSec := time.Now().Unix()
		local.DesiredState = "deleted"
		local.RuntimeState = "deleting"
		local.State = local.RuntimeState
		local.LastTransitionAt = nowSec
		local.DeletedAt = &nowSec
		_ = f.db.UpsertLocalWorkload(local)
		if strings.TrimSpace(local.ContainerID) != "" {
			_ = processmanager.GetInstance().RemoveContainerByContainerID(local.ContainerID)
		}
		if err := f.db.DeleteLocalWorkload(uuid); err != nil {
			return "", err
		}
		return uuid, nil
	}
	if err := processmanager.GetInstance().RemoveMicroservice(uuid); err != nil {
		return "", err
	}
	return uuid, nil
}

func (f *Facade) GetRuntimeMicroserviceLogs(selector string, tailLines int, since, until string) (string, []map[string]any, error) {
	uuid, err := f.ResolveMicroserviceID(selector)
	if err != nil {
		return "", nil, err
	}
	if tailLines <= 0 {
		tailLines = 100
	}
	entries, err := processmanager.GetInstance().TailMicroserviceLogs(uuid, &engine.TailConfig{
		Follow: false,
		Lines:  tailLines,
		Since:  since,
		Until:  until,
	})
	if err != nil {
		return "", nil, err
	}
	return uuid, entries, nil
}

// UpsertLocalDeployment upserts one local deployment record.
func (f *Facade) UpsertLocalDeployment(ms *models.LocalDeployedMicroservice) error {
	return f.db.UpsertLocalWorkload(ms)
}

// ListLocalDeployments lists local deployment records.
func (f *Facade) ListLocalDeployments() ([]*models.LocalDeployedMicroservice, error) {
	return f.db.ListLocalWorkloads()
}

// GetLocalDeployment gets local deployment by id.
func (f *Facade) GetLocalDeployment(id string) (*models.LocalDeployedMicroservice, error) {
	return f.db.GetLocalWorkload(id)
}

// DeleteLocalDeployment removes local deployment by id.
func (f *Facade) DeleteLocalDeployment(id string) error {
	return f.db.DeleteLocalWorkload(id)
}

func (f *Facade) ensureRuntimeClassSupported() error {
	if !buildmeta.HasEmbeddedEngine() || !strings.EqualFold(strings.TrimSpace(f.cfg.ContainerEngine), "edgelet") {
		return ErrRuntimeClassUnsupported
	}
	return nil
}

// ParseAndValidateLocalRuntimeClassManifest validates a RuntimeClass manifest.
func (f *Facade) ParseAndValidateLocalRuntimeClassManifest(manifest string) (*models.LocalRuntimeClassManifest, error) {
	if err := f.ensureRuntimeClassSupported(); err != nil {
		return nil, err
	}
	doc := &models.LocalRuntimeClassManifest{}
	dec := yaml.NewDecoder(bytes.NewReader([]byte(manifest)))
	dec.KnownFields(true)
	if err := dec.Decode(doc); err != nil {
		return nil, fmt.Errorf("invalid runtimeclass manifest YAML: %w", err)
	}
	if err := doc.Validate(); err != nil {
		return nil, err
	}
	return doc, nil
}

// ApplyLocalRuntimeClassManifest validates and stores a RuntimeClass manifest.
func (f *Facade) ApplyLocalRuntimeClassManifest(manifest string, dryRun bool) (*models.LocalRuntimeClass, error) {
	doc, err := f.ParseAndValidateLocalRuntimeClassManifest(manifest)
	if err != nil {
		return nil, err
	}
	item := &models.LocalRuntimeClass{
		Name:    doc.Metadata.Name,
		Handler: doc.Handler,
	}
	if dryRun {
		item.Normalize()
		return item, nil
	}
	if _, getErr := f.db.GetLocalRuntimeClass(item.Name); getErr != nil && !errors.Is(getErr, sql.ErrNoRows) {
		return nil, wrapRuntimeClassOperationError(RuntimeClassStageWriteConfig, fmt.Errorf("failed to read existing runtimeclass %s: %w", item.Name, getErr))
	}
	if err := f.db.UpsertLocalRuntimeClass(item); err != nil {
		return nil, wrapRuntimeClassOperationError(RuntimeClassStageWriteConfig, err)
	}
	item.Normalize()
	return item, nil
}

// ListRuntimeClasses returns persisted RuntimeClass entries.
func (f *Facade) ListRuntimeClasses() ([]*models.LocalRuntimeClass, error) {
	if err := f.ensureRuntimeClassSupported(); err != nil {
		return nil, err
	}
	return f.db.ListLocalRuntimeClasses()
}

// GetRuntimeClass returns one RuntimeClass entry by name.
func (f *Facade) GetRuntimeClass(name string) (*models.LocalRuntimeClass, error) {
	if err := f.ensureRuntimeClassSupported(); err != nil {
		return nil, err
	}
	return f.db.GetLocalRuntimeClass(name)
}

// ValidateRuntimeClassDelete validates whether one RuntimeClass can be deleted.
func (f *Facade) ValidateRuntimeClassDelete(name string) (*models.LocalRuntimeClass, error) {
	if err := f.ensureRuntimeClassSupported(); err != nil {
		return nil, err
	}
	normalizedName := strings.TrimSpace(strings.ToLower(name))
	if isReservedRuntimeName(normalizedName) {
		return nil, &ErrReservedRuntimeClassDelete{Name: normalizedName}
	}
	existing, err := f.db.GetLocalRuntimeClass(normalizedName)
	if err != nil {
		return nil, err
	}
	if err := f.ensureRuntimeClassNotInUse(existing); err != nil {
		return nil, err
	}
	return existing, nil
}

// DeleteRuntimeClass removes one RuntimeClass entry by name.
func (f *Facade) DeleteRuntimeClass(name string) error {
	_, err := f.ValidateRuntimeClassDelete(name)
	if err != nil {
		return err
	}
	if err := f.db.DeleteLocalRuntimeClass(name); err != nil {
		return wrapRuntimeClassOperationError(RuntimeClassStageWriteConfig, err)
	}
	return nil
}

func isReservedRuntimeName(name string) bool {
	switch strings.TrimSpace(strings.ToLower(name)) {
	case "crun":
		return true
	default:
		return false
	}
}

func (f *Facade) ensureRuntimeClassNotInUse(item *models.LocalRuntimeClass) error {
	if item == nil {
		return nil
	}
	items, err := f.db.ListLocalWorkloads()
	if err != nil {
		return fmt.Errorf("failed to list local deployments while checking runtimeclass delete: %w", err)
	}
	runtimeNames := []string{strings.TrimSpace(item.RuntimeName)}
	runtimeSet := make(map[string]struct{}, len(runtimeNames))
	for _, name := range runtimeNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		runtimeSet[name] = struct{}{}
	}
	if len(runtimeSet) == 0 {
		return nil
	}

	blockingSet := make(map[string]struct{})
	for _, local := range items {
		if local == nil || local.DeletedAt != nil {
			continue
		}
		state := strings.TrimSpace(strings.ToLower(local.RuntimeState))
		if state == "" {
			state = strings.TrimSpace(strings.ToLower(local.State))
		}
		if state != "running" {
			continue
		}
		runtime := runtimeFromManifestYAML(local.ManifestYAML)
		if runtime == "" {
			continue
		}
		if _, used := runtimeSet[runtime]; !used {
			continue
		}
		if uuid := strings.TrimSpace(local.LocalUUID); uuid != "" {
			blockingSet[uuid] = struct{}{}
		}
	}
	if len(blockingSet) == 0 {
		return nil
	}
	blockingUUIDs := make([]string, 0, len(blockingSet))
	for uuid := range blockingSet {
		blockingUUIDs = append(blockingUUIDs, uuid)
	}
	slices.Sort(blockingUUIDs)
	return &ErrRuntimeClassInUse{
		Name:                      item.Name,
		RuntimeNames:              sortedUniqueNonEmpty(runtimeNames),
		BlockingMicroserviceUuids: blockingUUIDs,
	}
}

func runtimeFromManifestYAML(manifest string) string {
	doc := struct {
		Spec struct {
			Container struct {
				Runtime string `yaml:"runtime"`
			} `yaml:"container"`
		} `yaml:"spec"`
	}{}
	if err := yaml.Unmarshal([]byte(strings.TrimSpace(manifest)), &doc); err != nil {
		return ""
	}
	return strings.TrimSpace(strings.ToLower(doc.Spec.Container.Runtime))
}

func sortedUniqueNonEmpty(items []string) []string {
	set := make(map[string]struct{}, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		set[item] = struct{}{}
	}
	result := make([]string, 0, len(set))
	for item := range set {
		result = append(result, item)
	}
	slices.Sort(result)
	return result
}

// ParseAndValidateLocalManifest validates a local deploy manifest.
func (f *Facade) ParseAndValidateLocalManifest(manifest string) (*models.LocalDeployManifest, error) {
	doc := &models.LocalDeployManifest{}
	dec := yaml.NewDecoder(bytes.NewReader([]byte(manifest)))
	dec.KnownFields(true)
	if err := dec.Decode(doc); err != nil {
		return nil, fmt.Errorf("invalid manifest YAML: %w", err)
	}
	if err := doc.Validate(); err != nil {
		return nil, err
	}
	return doc, nil
}

// ApplyLocalManifest validates and persists local deployment metadata.
func (f *Facade) ApplyLocalManifest(manifest, sourceName string, dryRun bool, progress DeployProgressCallback) (string, *models.LocalDeployManifest, error) {
	emitDeployProgress(progress, DeployStageParsing, "validating manifest")
	doc, err := f.ParseAndValidateLocalManifest(manifest)
	if err != nil {
		logging.LogWarn(runtimeAPIModuleName, fmt.Sprintf("local deploy manifest validation failed: %v", err))
		return "", nil, err
	}
	var existing *models.LocalDeployedMicroservice
	deploymentID := uuid.NewString()
	if f.db.Conn() != nil {
		existingItems, findErr := f.db.FindLocalWorkloadsByAppAndName(workloadmeta.LocalDeployApplicationName, doc.Metadata.Name)
		if findErr != nil {
			return "", nil, fmt.Errorf("failed to resolve existing local deployment: %w", findErr)
		}
		if len(existingItems) > 1 {
			matches := make([]string, 0, len(existingItems))
			for _, item := range existingItems {
				if item == nil {
					continue
				}
				if id := strings.TrimSpace(item.LocalUUID); id != "" {
					matches = append(matches, id)
				}
			}
			slices.Sort(matches)
			return "", nil, &ErrAmbiguousMicroserviceSelector{Matches: matches}
		}
		if len(existingItems) == 1 {
			existing = existingItems[0]
			if existing != nil && strings.TrimSpace(existing.LocalUUID) != "" {
				deploymentID = strings.TrimSpace(existing.LocalUUID)
			}
		}
	}
	mode := "create"
	if existing != nil {
		mode = "patch"
	}
	logging.LogInfo(
		runtimeAPIModuleName,
		fmt.Sprintf("local deploy start deploymentId=%s source=%s name=%s mode=%s", deploymentID, strings.TrimSpace(sourceName), strings.TrimSpace(doc.Metadata.Name), mode),
	)
	if dryRun {
		emitDeployProgress(progress, DeployStageDone, "dry-run completed")
		logging.LogInfo(runtimeAPIModuleName, fmt.Sprintf("local deploy dry-run completed deploymentId=%s name=%s mode=%s", deploymentID, strings.TrimSpace(doc.Metadata.Name), mode))
		return deploymentID, doc, nil
	}
	if sourceName == "" {
		sourceName = "local-cli"
	}
	image := doc.ManifestImage()
	localMS := manifestToMicroservice(doc, deploymentID, image)
	registry := models.NewRegistry(2, "from_cache", true, "", "", "")
	if doc.Spec.Registry != nil {
		regID := *doc.Spec.Registry
		if regID <= 0 {
			logging.LogWarn(runtimeAPIModuleName, fmt.Sprintf("local deploy invalid registry id deploymentId=%s registryId=%d", deploymentID, regID))
			return "", nil, fmt.Errorf("invalid registry id %d", regID)
		}
		reg, regErr := f.db.GetLocalRegistry(regID)
		if regErr != nil || reg == nil {
			logging.LogWarn(runtimeAPIModuleName, fmt.Sprintf("local deploy missing registry deploymentId=%s registryId=%d", deploymentID, regID))
			return "", nil, fmt.Errorf("invalid registry id %d", regID)
		}
		registry = reg
		localMS.RegistryID = reg.ID
	}
	nextGeneration := int64(1)
	restartCount := 0
	if existing != nil {
		nextGeneration = existing.Generation + 1
		restartCount = existing.RestartCount
	}
	localItem := &models.LocalDeployedMicroservice{
		LocalUUID:          deploymentID,
		ApplicationName:    workloadmeta.LocalDeployApplicationName,
		MicroserviceName:   doc.Metadata.Name,
		SourceName:         sourceName,
		ManifestYAML:       manifest,
		ImageName:          image,
		State:              "starting",
		DesiredState:       "running",
		RuntimeState:       "starting",
		LastError:          "",
		RestartCount:       restartCount,
		LastTransitionAt:   time.Now().Unix(),
		Generation:         nextGeneration,
		ObservedGeneration: 0,
	}
	emitDeployProgress(progress, DeployStagePersisting, "saving deployment metadata")
	if err := f.UpsertLocalDeployment(localItem); err != nil {
		logging.LogWarn(runtimeAPIModuleName, fmt.Sprintf("local deploy persist initial failed deploymentId=%s err=%v", deploymentID, err))
		return "", nil, err
	}
	if existing != nil && strings.TrimSpace(existing.ContainerID) != "" {
		if removeErr := processmanager.GetInstance().RemoveContainerByContainerID(strings.TrimSpace(existing.ContainerID)); removeErr != nil {
			logging.LogWarn(
				runtimeAPIModuleName,
				fmt.Sprintf("local deploy patch remove previous container failed deploymentId=%s previousContainerId=%s err=%v", deploymentID, strings.TrimSpace(existing.ContainerID), removeErr),
			)
			return "", nil, fmt.Errorf("failed to remove previous local runtime for %s: %w", deploymentID, removeErr)
		}
	}
	hostIP := network.GetInstance().GetCurrentIPAddress()

	localItem.LastStartAttemptAt = time.Now().Unix()
	if err := f.UpsertLocalDeployment(localItem); err != nil {
		logging.LogWarn(runtimeAPIModuleName, fmt.Sprintf("local deploy persist start attempt failed deploymentId=%s err=%v", deploymentID, err))
		return "", nil, err
	}

	containerID, launchErr := processmanager.GetInstance().LaunchLocalMicroserviceWithProgress(localMS, registry, hostIP, func(stage string, message string) {
		emitDeployProgress(progress, stage, message)
	})
	if launchErr != nil {
		localItem.State = "failed"
		localItem.RuntimeState = "failed"
		localItem.LastError = launchErr.Error()
		localItem.LastTransitionAt = time.Now().Unix()
		emitDeployProgress(progress, DeployStagePersisting, "saving failed deployment state")
		_ = f.UpsertLocalDeployment(localItem)
		logging.LogWarn(runtimeAPIModuleName, fmt.Sprintf("local deploy runtime launch failed deploymentId=%s name=%s err=%v", deploymentID, strings.TrimSpace(doc.Metadata.Name), launchErr))
		return "", nil, launchErr
	}
	localItem.ContainerID = containerID
	localItem.State = "running"
	localItem.RuntimeState = "running"
	localItem.ObservedGeneration = localItem.Generation
	localItem.LastError = ""
	localItem.LastTransitionAt = time.Now().Unix()
	emitDeployProgress(progress, DeployStagePersisting, "saving running deployment state")
	if err := f.UpsertLocalDeployment(localItem); err != nil {
		if container, lookupErr := processmanager.GetInstance().GetContainerByID(containerID); lookupErr == nil && container != nil {
			localItem.ContainerID = container.ID
			localItem.State = "running"
			if retryErr := f.UpsertLocalDeployment(localItem); retryErr == nil {
				emitDeployProgress(progress, DeployStageDone, "deployment completed")
				logging.LogInfo(runtimeAPIModuleName, fmt.Sprintf("local deploy completed deploymentId=%s containerId=%s", deploymentID, containerID))
				return deploymentID, doc, nil
			}
		}
		logging.LogWarn(runtimeAPIModuleName, fmt.Sprintf("local deploy runtime started but persist failed deploymentId=%s containerId=%s err=%v", deploymentID, containerID, err))
		return "", nil, fmt.Errorf("runtime started (containerId=%s) but persistence failed: %w", containerID, err)
	}
	emitDeployProgress(progress, DeployStageDone, "deployment completed")
	logging.LogInfo(runtimeAPIModuleName, fmt.Sprintf("local deploy completed deploymentId=%s containerId=%s", deploymentID, containerID))
	return deploymentID, doc, nil
}

// ParseAndValidateLocalRegistryManifest validates a local registry manifest.
func (f *Facade) ParseAndValidateLocalRegistryManifest(manifest string) (*models.LocalRegistryManifest, error) {
	doc := &models.LocalRegistryManifest{}
	dec := yaml.NewDecoder(bytes.NewReader([]byte(manifest)))
	dec.KnownFields(true)
	if err := dec.Decode(doc); err != nil {
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
	registryID, err := f.nextLocalRegistryID()
	if err != nil {
		return nil, err
	}
	reg := models.NewRegistry(registryID, doc.Spec.URL, !doc.Spec.Private, doc.Spec.UserName, doc.Spec.Password, doc.Spec.UserEmail)
	if dryRun {
		return reg, nil
	}
	if err := f.db.UpsertLocalRegistry(reg); err != nil {
		return nil, err
	}
	if err := f.db.EnsureDefaultLocalRegistries(); err != nil {
		return nil, err
	}
	return reg, nil
}

func (f *Facade) nextLocalRegistryID() (int, error) {
	items, err := f.ListRegistries()
	if err != nil {
		return 0, err
	}
	maxID := 2
	for _, item := range items {
		if item.ID > maxID {
			maxID = item.ID
		}
	}
	return maxID + 1, nil
}

// ListRegistries returns persisted registry entries with defaults guaranteed.
func (f *Facade) ListRegistries() ([]*models.Registry, error) {
	if err := f.db.EnsureDefaultLocalRegistries(); err != nil {
		return nil, err
	}
	return f.db.LoadLocalRegistries()
}

// GetRegistry returns one local registry entry.
func (f *Facade) GetRegistry(id int) (*models.Registry, error) {
	return f.db.GetLocalRegistry(id)
}

// DeleteRegistry removes one local registry entry.
func (f *Facade) DeleteRegistry(id int) error {
	if id <= 2 {
		return fmt.Errorf("default registry %d cannot be removed", id)
	}
	return f.db.DeleteLocalRegistry(id)
}
func manifestToMicroservice(doc *models.LocalDeployManifest, deploymentID, image string) *models.Microservice {
	return models.BuildMicroserviceFromLocalManifest(doc, deploymentID, image)
}
