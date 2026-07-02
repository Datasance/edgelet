package fieldagent

import (
	"testing"

	"github.com/eclipse-iofog/edgelet/internal/models"
)

func TestProcessMicroserviceConfig_NotifiesOnlyChangedUUIDs(t *testing.T) {
	fa := GetInstance()
	fa.containerConfigMu.Lock()
	fa.containerConfigMap = map[string]string{
		"ms-1": `{"a":1}`,
		"ms-2": `{"b":2}`,
	}
	fa.containerConfigMu.Unlock()

	var notified []string
	fa.SetOnConfigsUpdate(func(changedUUIDs []string) error {
		notified = append(notified, changedUUIDs...)
		return nil
	})

	cfg1 := `{"a":1}`
	cfg2 := `{"b":3}`
	cfg3 := `{"c":4}`
	microservices := []*models.Microservice{
		models.NewMicroservice("ms-1", "image:1"),
		models.NewMicroservice("ms-2", "image:2"),
		models.NewMicroservice("ms-3", "image:3"),
	}
	microservices[0].Config = &cfg1
	microservices[1].Config = &cfg2
	microservices[2].Config = &cfg3

	if err := fa.processMicroserviceConfig(microservices); err != nil {
		t.Fatalf("processMicroserviceConfig: %v", err)
	}

	if len(notified) != 1 {
		t.Fatalf("expected 1 changed UUID, got %v", notified)
	}
	if notified[0] != "ms-2" {
		t.Fatalf("unexpected changed UUIDs: %v", notified)
	}
}

func TestProcessMicroserviceConfig_SkipsCallbackOnInitialLoad(t *testing.T) {
	fa := GetInstance()
	fa.containerConfigMu.Lock()
	fa.containerConfigMap = map[string]string{}
	fa.containerConfigMu.Unlock()

	called := false
	fa.SetOnConfigsUpdate(func(changedUUIDs []string) error {
		called = true
		return nil
	})

	cfg := `{"x":1}`
	ms := models.NewMicroservice("ms-new", "image:1")
	ms.Config = &cfg

	if err := fa.processMicroserviceConfig([]*models.Microservice{ms}); err != nil {
		t.Fatalf("processMicroserviceConfig: %v", err)
	}
	if called {
		t.Fatal("expected no callback on first config load")
	}

	got, ok := fa.GetContainerConfig("ms-new")
	if !ok || got != cfg {
		t.Fatalf("expected config stored, got %q ok=%v", got, ok)
	}
}
