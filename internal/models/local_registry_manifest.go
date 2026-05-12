package models

import (
	"fmt"
	"strings"
)

// LocalRegistryManifest represents local registry deployment YAML.
type LocalRegistryManifest struct {
	APIVersion string `yaml:"apiVersion" json:"apiVersion"`
	Kind       string `yaml:"kind" json:"kind"`
	Metadata   struct {
		Name string `yaml:"name" json:"name"`
	} `yaml:"metadata" json:"metadata"`
	Spec struct {
		ID       int    `yaml:"id" json:"id"`
		URL      string `yaml:"url" json:"url"`
		IsPublic bool   `yaml:"isPublic" json:"isPublic"`
		UserName string `yaml:"userName,omitempty" json:"userName,omitempty"`
		Password string `yaml:"password,omitempty" json:"password,omitempty"`
		UserEmail string `yaml:"userEmail,omitempty" json:"userEmail,omitempty"`
	} `yaml:"spec" json:"spec"`
}

func (m *LocalRegistryManifest) Validate() error {
	if m == nil {
		return fmt.Errorf("manifest is nil")
	}
	if strings.TrimSpace(m.APIVersion) == "" {
		return fmt.Errorf("apiVersion is required")
	}
	if strings.TrimSpace(m.Kind) == "" {
		return fmt.Errorf("kind is required")
	}
	if strings.TrimSpace(m.Kind) != "Registry" {
		return fmt.Errorf("kind must be Registry")
	}
	if m.Spec.ID <= 0 {
		return fmt.Errorf("spec.id must be > 0")
	}
	if strings.TrimSpace(m.Spec.URL) == "" {
		return fmt.Errorf("spec.url is required")
	}
	return nil
}
