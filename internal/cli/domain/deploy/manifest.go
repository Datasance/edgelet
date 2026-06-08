package deploy

import (
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Target is a EdgeletAPI deploy collection.
type Target string

const (
	TargetMicroservices  Target = "microservices"
	TargetRegistries     Target = "registries"
	TargetRuntimeClasses Target = "runtimeclasses"
	TargetControlPlane   Target = "controlplane"
)

// DetectTargetFromManifest inspects manifest kind to choose the deploy API target.
func DetectTargetFromManifest(path string) (Target, error) {
	kind, err := DetectManifestKind(path)
	if err != nil {
		return TargetMicroservices, err
	}
	switch {
	case strings.EqualFold(kind, "Registry"):
		return TargetRegistries, nil
	case strings.EqualFold(kind, "RuntimeClass"):
		return TargetRuntimeClasses, nil
	case strings.EqualFold(kind, "ControlPlane"):
		return TargetControlPlane, nil
	default:
		return TargetMicroservices, nil
	}
}

// DetectManifestKind reads the manifest kind field.
func DetectManifestKind(path string) (string, error) {
	raw, err := os.ReadFile(path) // #nosec G304 CLI manifest path provided by caller
	if err != nil {
		return "", err
	}
	var doc struct {
		Kind string `yaml:"kind"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return "", err
	}
	return strings.TrimSpace(doc.Kind), nil
}

func (t Target) validatePath() string {
	return "/v1/deploy/" + string(t) + ":validate"
}

func (t Target) applyPath() string {
	return "/v1/deploy/" + string(t) + ":apply"
}

func (t Target) applyStatusPath(operationID string) string {
	return "/v1/deploy/" + string(t) + ":apply/" + operationID
}
