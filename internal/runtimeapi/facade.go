package runtimeapi

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/eclipse-iofog/agent/internal/config"
	"github.com/eclipse-iofog/agent/internal/fieldagent"
	"github.com/eclipse-iofog/agent/internal/models"
	"github.com/eclipse-iofog/agent/internal/network"
	"github.com/eclipse-iofog/agent/internal/processmanager"
	"github.com/eclipse-iofog/agent/internal/pruning"
	"github.com/eclipse-iofog/agent/internal/statusreporter"
	"github.com/eclipse-iofog/agent/internal/store"
	"github.com/eclipse-iofog/agent/internal/workloadmeta"
	"github.com/eclipse-iofog/agent/pkg/engine"
	"github.com/eclipse-iofog/agent/pkg/imageref"
	"github.com/google/uuid"
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
func (f *Facade) Deprovision() error {
	return f.fa.Deprovision(false)
}

// Prune triggers dangling-image prune and returns structured details.
func (f *Facade) Prune() map[string]interface{} {
	pm := processmanager.GetInstance()
	report, err := pm.PruneDanglingImages()
	if err != nil {
		return map[string]interface{}{
			"status":  "failed",
			"mode":    "dangling",
			"message": err.Error(),
		}
	}
	deleted := make([]string, 0)
	reclaimed := int64(0)
	deletedCount := 0
	if report != nil {
		deleted = append(deleted, report.Deleted...)
		reclaimed = report.SpaceReclaimedBytes
		deletedCount = report.DeletedCount
	}
	sort.Strings(deleted)
	return map[string]interface{}{
		"status":              "ok",
		"mode":                "dangling",
		"deleted":             deleted,
		"deletedCount":        deletedCount,
		"spaceReclaimedBytes": reclaimed,
		"spaceReclaimedHuman": humanBytes(reclaimed),
		"engine":              currentEngineName(f.cfg),
		"message":             "pruned dangling images",
	}
}

// ListImages returns normalized image list for local runtime engine.
func (f *Facade) ListImages() ([]map[string]interface{}, error) {
	items, err := processmanager.GetInstance().ListImages()
	if err != nil {
		return nil, err
	}
	out := make([]map[string]interface{}, 0, len(items))
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
		out = append(out, map[string]interface{}{
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
		return "", fmt.Errorf("image is required")
	}
	resolvedImage := imageRef
	var reg *models.Registry
	if registryID != nil {
		if *registryID <= 0 {
			return "", fmt.Errorf("registryId must be greater than zero")
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
			return "", fmt.Errorf("platform must follow os/arch[/variant] format")
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
		return "", fmt.Errorf("image is required")
	}
	resolvedImage := imageRef
	var reg *models.Registry
	if registryID != nil {
		if *registryID <= 0 {
			return "", fmt.Errorf("registryId must be greater than zero")
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
			return "", fmt.Errorf("platform must follow os/arch[/variant] format")
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
		return nil, fmt.Errorf("path is required")
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to access path: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("path must point to a regular file")
	}
	return processmanager.GetInstance().LoadImageFromPath(path)
}

// RemoveImage removes an image by selector (id, id prefix, name:tag, or digest).
func (f *Facade) RemoveImage(selector string) (string, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return "", fmt.Errorf("selector is required")
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
		sort.Strings(matches)
		candidates := matches
		if len(candidates) > 5 {
			candidates = candidates[:5]
		}
		return "", fmt.Errorf("ambiguous image id prefix %q; candidates: %s", selector, strings.Join(candidates, ", "))
	}
}

// ListRuntimeMicroservices returns process-manager tracked microservices.
func (f *Facade) ListRuntimeMicroservices() []map[string]interface{} {
	pmStatus := f.sr.GetProcessManagerStatus()
	capHint := 0
	if pmStatus != nil {
		capHint = len(pmStatus.MicroservicesStatus)
	}
	result := make([]map[string]interface{}, 0, capHint)
	msByUUID := make(map[string]*models.Microservice)
	for _, ms := range f.fa.GetLatestMicroservices() {
		msByUUID[ms.MicroserviceUUID] = ms
	}
	if pmStatus != nil {
		for uuid, status := range pmStatus.MicroservicesStatus {
			name := ""
			application := ""
			image := ""
			if ms, ok := msByUUID[uuid]; ok {
				name = ms.MicroserviceName
				application = ms.ApplicationName
				image = ms.ImageName
			}
			entry := map[string]interface{}{
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

	if locals, err := f.db.ListLocalDeployedMicroservices(); err == nil {
		for _, item := range locals {
			entry := map[string]interface{}{
				"uuid":        item.LocalUUID,
				"name":        item.MicroserviceName,
				"application": item.ApplicationName,
				"source":      strings.TrimSpace(item.SourceName),
				"type":        "local",
				"state":       strings.TrimSpace(item.State),
				"containerId": strings.TrimSpace(item.ContainerID),
				"image":       strings.TrimSpace(item.ImageName),
			}
			if entry["source"] == "" {
				entry["source"] = "local-cli"
			}
			result = append(result, entry)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i]["uuid"].(string) < result[j]["uuid"].(string)
	})
	return result
}

// GetRuntimeMicroservice returns one process-manager tracked microservice.
func (f *Facade) GetRuntimeMicroservice(id string) (map[string]interface{}, error) {
	uuid, err := f.ResolveMicroserviceID(id)
	if err != nil {
		return nil, err
	}
	containerInspect := map[string]interface{}{}
	if cont, contErr := processmanager.GetInstance().GetContainerForMicroservice(uuid); contErr == nil && cont != nil {
		if rawInspect, rawErr := processmanager.GetInstance().InspectContainerRaw(cont.ID); rawErr == nil && rawInspect != nil {
			containerInspect = rawInspect
		} else {
			containerInspect = map[string]interface{}{
				"id":     cont.ID,
				"names":  cont.Names,
				"image":  cont.Image,
				"status": cont.Status,
				"state":  cont.State,
				"labels": cont.Labels,
			}
		}
	}
	if local, localErr := f.db.GetLocalDeployedMicroservice(uuid); localErr == nil && local != nil {
		return map[string]interface{}{
			"uuid":         local.LocalUUID,
			"name":         local.MicroserviceName,
			"application":  local.ApplicationName,
			"source":       local.SourceName,
			"type":         "local",
			"state":        local.State,
			"containerId":  local.ContainerID,
			"image":        local.ImageName,
			"manifestYAML": local.ManifestYAML,
			"raw": map[string]interface{}{
				"localDeployment":      local,
				"engineInspect":        containerInspect,
				"engineType":           currentEngineName(f.cfg),
				"inspectSchemaVersion": "v1",
			},
		}, nil
	}
	pmStatus := f.sr.GetProcessManagerStatus()
	if pmStatus == nil {
		return nil, fmt.Errorf("process manager status unavailable")
	}
	status := pmStatus.GetMicroserviceStatus(uuid)
	if status == nil || status.Status == models.MicroserviceStateUnknown {
		return nil, fmt.Errorf("microservice not found")
	}
	name := ""
	application := ""
	image := ""
	if ms := f.fa.FindLatestMicroserviceByUUID(uuid); ms != nil {
		name = ms.MicroserviceName
		application = ms.ApplicationName
		image = ms.ImageName
	}
	return map[string]interface{}{
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
		"raw": map[string]interface{}{
			"processManager":       status,
			"engineInspect":        containerInspect,
			"engineType":           currentEngineName(f.cfg),
			"inspectSchemaVersion": "v1",
		},
	}, nil
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
		return "", fmt.Errorf("microservice id is required")
	}

	if f.fa.FindLatestMicroserviceByUUID(trimmed) != nil {
		return trimmed, nil
	}
	if local, err := f.db.GetLocalDeployedMicroservice(trimmed); err == nil && local != nil {
		return trimmed, nil
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

	if strings.Contains(trimmed, ".") {
		parts := strings.SplitN(trimmed, ".", 2)
		app := strings.TrimSpace(parts[0])
		name := strings.TrimSpace(parts[1])
		if app == "" || name == "" {
			return "", fmt.Errorf("invalid selector %q", selector)
		}
		matches := make([]string, 0)
		for _, ms := range f.fa.GetLatestMicroservices() {
			if strings.EqualFold(ms.ApplicationName, app) && strings.EqualFold(ms.MicroserviceName, name) {
				matches = append(matches, ms.MicroserviceUUID)
			}
		}
		switch len(matches) {
		case 0:
			return "", fmt.Errorf("microservice not found")
		case 1:
			return matches[0], nil
		default:
			sort.Strings(matches)
			return "", &ErrAmbiguousMicroserviceSelector{Matches: matches}
		}
	}

	return "", fmt.Errorf("microservice not found")
}

func (f *Facade) StartRuntimeMicroservice(selector string) (string, error) {
	uuid, err := f.ResolveMicroserviceID(selector)
	if err != nil {
		return "", err
	}
	if err := processmanager.GetInstance().StartMicroservice(uuid); err != nil {
		return "", err
	}
	return uuid, nil
}

func (f *Facade) StopRuntimeMicroservice(selector string) (string, error) {
	uuid, err := f.ResolveMicroserviceID(selector)
	if err != nil {
		return "", err
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
	if err := processmanager.GetInstance().RestartMicroservice(uuid); err != nil {
		return "", err
	}
	return uuid, nil
}

func (f *Facade) RemoveRuntimeMicroservice(selector string) (string, error) {
	uuid, err := f.ResolveMicroserviceID(selector)
	if err != nil {
		return "", err
	}
	if local, localErr := f.db.GetLocalDeployedMicroservice(uuid); localErr == nil && local != nil {
		if strings.TrimSpace(local.ContainerID) != "" {
			_ = processmanager.GetInstance().RemoveContainerByContainerID(local.ContainerID)
		}
		if err := f.db.DeleteLocalDeployedMicroservice(uuid); err != nil {
			return "", err
		}
		return uuid, nil
	}
	if err := processmanager.GetInstance().RemoveMicroservice(uuid); err != nil {
		return "", err
	}
	return uuid, nil
}

func (f *Facade) GetRuntimeMicroserviceLogs(selector string, tailLines int, since, until string) (string, []map[string]interface{}, error) {
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
		return "", nil, err
	}
	deploymentID := uuid.NewString()
	if dryRun {
		emitDeployProgress(progress, DeployStageDone, "dry-run completed")
		return deploymentID, doc, nil
	}
	if sourceName == "" {
		sourceName = "local-cli"
	}
	arch := strings.ToLower(strings.TrimSpace(f.cfg.Arch))
	image := doc.ResolveImageForArch(arch)
	localMS := manifestToMicroservice(doc, deploymentID, image)
	registry := models.NewRegistry(2, "from_cache", true, "", "", "")
	if doc.Spec.Images.Registry != nil {
		regID := *doc.Spec.Images.Registry
		if regID <= 0 {
			return "", nil, fmt.Errorf("invalid registry id %d", regID)
		}
		reg, regErr := f.db.GetLocalRegistry(regID)
		if regErr != nil || reg == nil {
			return "", nil, fmt.Errorf("invalid registry id %d", regID)
		}
		registry = reg
		localMS.RegistryID = reg.ID
	}
	localItem := &models.LocalDeployedMicroservice{
		LocalUUID:        deploymentID,
		ApplicationName:  "",
		MicroserviceName: doc.Metadata.Name,
		SourceName:       sourceName,
		ManifestYAML:     manifest,
		ImageName:        image,
		State:            "starting",
	}
	emitDeployProgress(progress, DeployStagePersisting, "saving deployment metadata")
	if err := f.UpsertLocalDeployment(localItem); err != nil {
		return "", nil, err
	}
	hostIP := network.GetInstance().GetCurrentIPAddress()

	containerID, launchErr := processmanager.GetInstance().LaunchLocalMicroserviceWithProgress(localMS, registry, hostIP, func(stage string, message string) {
		emitDeployProgress(progress, stage, message)
	})
	if launchErr != nil {
		localItem.State = "failed"
		emitDeployProgress(progress, DeployStagePersisting, "saving failed deployment state")
		_ = f.UpsertLocalDeployment(localItem)
		return "", nil, launchErr
	}
	localItem.ContainerID = containerID
	localItem.State = "running"
	emitDeployProgress(progress, DeployStagePersisting, "saving running deployment state")
	if err := f.UpsertLocalDeployment(localItem); err != nil {
		if container, lookupErr := processmanager.GetInstance().GetContainerByID(containerID); lookupErr == nil && container != nil {
			localItem.ContainerID = container.ID
			localItem.State = "running"
			if retryErr := f.UpsertLocalDeployment(localItem); retryErr == nil {
				emitDeployProgress(progress, DeployStageDone, "deployment completed")
				return deploymentID, doc, nil
			}
		}
		return "", nil, fmt.Errorf("runtime started (containerId=%s) but persistence failed: %w", containerID, err)
	}
	emitDeployProgress(progress, DeployStageDone, "deployment completed")
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
	ms := models.NewMicroservice(deploymentID, image)
	ms.MicroserviceName = doc.Metadata.Name
	ms.ApplicationName = "local"
	ms.Labels = cloneStringMap(doc.Metadata.Labels)
	ms.RegistryID = 2
	ms.HostNetworkMode = doc.Spec.Container.HostNetworkMode
	ms.IsPrivileged = doc.Spec.Container.IsPrivileged
	ms.Args = append(ms.Args, doc.Spec.Container.Commands...)
	ms.CapAdd = append(ms.CapAdd, doc.Spec.Container.CapAdd...)
	ms.CapDrop = append(ms.CapDrop, doc.Spec.Container.CapDrop...)
	ms.CdiDevs = append(ms.CdiDevs, doc.Spec.Container.CDIDevices...)
	ms.Schedule = doc.Spec.Schedule
	if strings.TrimSpace(doc.Spec.Container.RunAsUser) != "" {
		runAs := strings.TrimSpace(doc.Spec.Container.RunAsUser)
		ms.RunAsUser = &runAs
	}
	if strings.TrimSpace(doc.Spec.Container.Runtime) != "" {
		runtime := strings.TrimSpace(doc.Spec.Container.Runtime)
		ms.Runtime = &runtime
	}
	if strings.TrimSpace(doc.Spec.Container.Platform) != "" {
		platform := strings.TrimSpace(doc.Spec.Container.Platform)
		ms.Platform = &platform
	}
	if strings.TrimSpace(doc.Spec.Container.IpcMode) != "" {
		ipcMode := strings.TrimSpace(doc.Spec.Container.IpcMode)
		ms.IpcMode = &ipcMode
	}
	if strings.TrimSpace(doc.Spec.Container.PidMode) != "" {
		pidMode := strings.TrimSpace(doc.Spec.Container.PidMode)
		ms.PidMode = &pidMode
	}
	if strings.TrimSpace(doc.Spec.Container.CPUSetCpus) != "" {
		cpuSet := strings.TrimSpace(doc.Spec.Container.CPUSetCpus)
		ms.CPUSetCpus = &cpuSet
	}
	if doc.Spec.Container.MemoryLimit > 0 {
		mem := doc.Spec.Container.MemoryLimit
		ms.MemoryLimit = &mem
	}
	for _, envVar := range doc.Spec.Container.Env {
		ms.EnvVars = append(ms.EnvVars, &models.EnvVar{Key: envVar.Key, Value: envVar.Value})
	}
	for _, volume := range doc.Spec.Container.Volumes {
		ms.VolumeMappings = append(ms.VolumeMappings, &models.VolumeMapping{
			HostDestination:      volume.HostDestination,
			ContainerDestination: volume.ContainerDestination,
			AccessMode:           volume.AccessMode,
			Type:                 models.VolumeMappingType(strings.ToUpper(strings.TrimSpace(volume.Type))),
		})
	}
	for _, port := range doc.Spec.Container.Ports {
		ms.PortMappings = append(ms.PortMappings, &models.PortMapping{
			Inside:  port.Internal,
			Outside: port.External,
			UDP:     strings.EqualFold(strings.TrimSpace(port.Protocol), "udp"),
		})
	}
	for _, host := range doc.Spec.Container.ExtraHosts {
		if strings.TrimSpace(host.Name) == "" || strings.TrimSpace(host.Address) == "" {
			continue
		}
		ms.ExtraHosts = append(ms.ExtraHosts, strings.TrimSpace(host.Name)+":"+strings.TrimSpace(host.Address))
	}
	return ms
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(v)
	}
	return out
}
