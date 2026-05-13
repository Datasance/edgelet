package models

import (
	"fmt"
	"strings"
)

// LocalRegistryManifest represents local registry deployment YAML.
type LocalRegistryManifest struct {
	APIVersion string `yaml:"apiVersion" json:"apiVersion"`
	Kind       string `yaml:"kind" json:"kind"`
	Spec struct {
		URL      string `yaml:"url" json:"url"`
		UserName string `yaml:"username,omitempty" json:"username,omitempty"`
		Password string `yaml:"password,omitempty" json:"password,omitempty"`
		UserEmail string `yaml:"email,omitempty" json:"email,omitempty"`
		Private  bool   `yaml:"private" json:"private"`
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
	switch strings.TrimSpace(m.APIVersion) {
	case "datasance.com/v3", "iofog.org/v3":
	default:
		return fmt.Errorf("apiVersion must be datasance.com/v3 or iofog.org/v3")
	}
	if strings.TrimSpace(m.Spec.URL) == "" {
		return fmt.Errorf("spec.url is required")
	}
	return nil
}
