package models

import (
	"fmt"
	"strings"
)

// LocalDeployManifest mirrors single-microservice deploy shape used by controller deploy flows.
type LocalDeployManifest struct {
	APIVersion string `yaml:"apiVersion" json:"apiVersion"`
	Kind       string `yaml:"kind" json:"kind"`
	Metadata   struct {
		Name      string            `yaml:"name" json:"name"`
		Namespace string            `yaml:"namespace,omitempty" json:"namespace,omitempty"`
		Labels    map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
	} `yaml:"metadata" json:"metadata"`
	Spec struct {
		Image      string            `yaml:"image" json:"image"`
		Command    []string          `yaml:"command,omitempty" json:"command,omitempty"`
		Env        map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
		RegistryID *int              `yaml:"registryId,omitempty" json:"registryId,omitempty"`
	} `yaml:"spec" json:"spec"`
}

func (m *LocalDeployManifest) Validate() error {
	if m == nil {
		return fmt.Errorf("manifest is nil")
	}
	if strings.TrimSpace(m.APIVersion) == "" {
		return fmt.Errorf("apiVersion is required")
	}
	if strings.TrimSpace(m.Kind) == "" {
		return fmt.Errorf("kind is required")
	}
	if strings.TrimSpace(m.Kind) != "Microservice" {
		return fmt.Errorf("kind must be Microservice")
	}
	if strings.TrimSpace(m.Metadata.Name) == "" {
		return fmt.Errorf("metadata.name is required")
	}
	if strings.TrimSpace(m.Spec.Image) == "" {
		return fmt.Errorf("spec.image is required")
	}
	return nil
}
