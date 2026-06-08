//revive:disable:nested-structs
package models

import (
	"errors"
	"strings"
)

// LocalRegistryManifest represents local registry deployment YAML.
type LocalRegistryManifest struct {
	APIVersion string `yaml:"apiVersion" json:"apiVersion"`
	Kind       string `yaml:"kind" json:"kind"`
	Spec       struct {
		URL       string `yaml:"url" json:"url"`
		UserName  string `yaml:"username,omitempty" json:"username,omitempty"`
		Password  string `yaml:"password,omitempty" json:"password,omitempty"`
		UserEmail string `yaml:"email,omitempty" json:"email,omitempty"`
		Private   bool   `yaml:"private" json:"private"`
	} `yaml:"spec" json:"spec"`
}

func (m *LocalRegistryManifest) Validate() error {
	if m == nil {
		return errors.New("manifest is nil")
	}
	if strings.TrimSpace(m.APIVersion) == "" {
		return errors.New("apiVersion is required")
	}
	if strings.TrimSpace(m.Kind) == "" {
		return errors.New("kind is required")
	}
	if strings.TrimSpace(m.Kind) != "Registry" {
		return errors.New("kind must be Registry")
	}
	switch strings.TrimSpace(m.APIVersion) {
	case "edgelet.iofog.org/v1":
	default:
		return errors.New("apiVersion must be edgelet.iofog.org/v1")
	}
	if strings.TrimSpace(m.Spec.URL) == "" {
		return errors.New("spec.url is required")
	}
	if m.Spec.Private {
		if strings.TrimSpace(m.Spec.UserName) == "" {
			return errors.New("spec.username is required when spec.private=true")
		}
		if strings.TrimSpace(m.Spec.Password) == "" {
			return errors.New("spec.password is required when spec.private=true")
		}
	}
	return nil
}
