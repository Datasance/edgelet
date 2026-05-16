package models

import "testing"

func validLocalRegistryManifest() *LocalRegistryManifest {
	return &LocalRegistryManifest{
		APIVersion: "iofog.org/v3",
		Kind:       "Registry",
		Spec: struct {
			URL       string `yaml:"url" json:"url"`
			UserName  string `yaml:"username,omitempty" json:"username,omitempty"`
			Password  string `yaml:"password,omitempty" json:"password,omitempty"`
			UserEmail string `yaml:"email,omitempty" json:"email,omitempty"`
			Private   bool   `yaml:"private" json:"private"`
		}{
			URL:     "registry.example.com",
			Private: false,
		},
	}
}

func TestLocalRegistryManifestValidate_PrivateRequiresUsernameAndPassword(t *testing.T) {
	doc := validLocalRegistryManifest()
	doc.Spec.Private = true
	doc.Spec.Password = "secret"

	err := doc.Validate()
	if err == nil || err.Error() != "spec.username is required when spec.private=true" {
		t.Fatalf("expected missing username validation error, got: %v", err)
	}

	doc.Spec.UserName = "alice"
	doc.Spec.Password = ""
	err = doc.Validate()
	if err == nil || err.Error() != "spec.password is required when spec.private=true" {
		t.Fatalf("expected missing password validation error, got: %v", err)
	}
}

func TestLocalRegistryManifestValidate_PrivateWithCredentialsAllowsEmptyEmail(t *testing.T) {
	doc := validLocalRegistryManifest()
	doc.Spec.Private = true
	doc.Spec.UserName = "alice"
	doc.Spec.Password = "secret"
	doc.Spec.UserEmail = ""

	if err := doc.Validate(); err != nil {
		t.Fatalf("expected private registry with username/password and empty email to validate, got: %v", err)
	}
}
