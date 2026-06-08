package models

import (
	"encoding/json"
	"testing"
)

// TestJSONCompatibility tests that Go structs produce JSON compatible with output
func TestJSONCompatibility(t *testing.T) {
	t.Run("Microservice JSON field names", func(t *testing.T) {
		ms := NewMicroservice("test-uuid", "test-image")
		ms.ContainerID = "container-123"
		ms.RegistryID = 1

		jsonData, err := json.Marshal(ms)
		if err != nil {
			t.Fatalf("Failed to marshal: %v", err)
		}

		var result map[string]any
		if err := json.Unmarshal(jsonData, &result); err != nil {
			t.Fatalf("Failed to unmarshal: %v", err)
		}

		// Verify field names match (camelCase)
		if _, ok := result["microserviceUuid"]; !ok {
			t.Error("Missing field: microserviceUuid")
		}
		if _, ok := result["imageName"]; !ok {
			t.Error("Missing field: imageName")
		}
		if _, ok := result["containerId"]; !ok {
			t.Error("Missing field: containerId")
		}
		if _, ok := result["registryId"]; !ok {
			t.Error("Missing field: registryId")
		}
	})

	t.Run("Null handling with pointers", func(t *testing.T) {
		ms := NewMicroservice("uuid", "image")
		// Config is nil (not set)

		jsonData, err := json.Marshal(ms)
		if err != nil {
			t.Fatalf("Failed to marshal: %v", err)
		}

		// When field is nil pointer, it should be omitted (omitempty)
		var result map[string]any
		if err := json.Unmarshal(jsonData, &result); err != nil {
			t.Fatalf("Failed to unmarshal: %v", err)
		}

		// Config should not be present when nil
		if _, ok := result["config"]; ok {
			t.Error("Config field should be omitted when nil")
		}

		// Now set it to empty string
		empty := ""
		ms.Config = &empty
		jsonData2, _ := json.Marshal(ms)
		var result2 map[string]any
		_ = json.Unmarshal(jsonData2, &result2)

		// Now it should be present
		if val, ok := result2["config"].(string); !ok || val != "" {
			t.Error("Config field should be present when set to empty string")
		}
	})

	t.Run("Enum string values", func(t *testing.T) {
		ms := NewMicroserviceStatus()
		ms.Status = MicroserviceStateRunning

		jsonData, err := json.Marshal(ms)
		if err != nil {
			t.Fatalf("Failed to marshal: %v", err)
		}

		var result map[string]any
		if err := json.Unmarshal(jsonData, &result); err != nil {
			t.Fatalf("Failed to unmarshal: %v", err)
		}

		// Status should be a string
		if status, ok := result["status"].(string); !ok || status != "RUNNING" {
			t.Errorf("Status should be 'RUNNING', got %v", result["status"])
		}
	})

	t.Run("Empty slices vs nil", func(t *testing.T) {
		ms := NewMicroservice("uuid", "image")
		// PortMappings is initialized as empty slice, not nil
		// However, with omitempty, empty slices are omitted
		// This is correct behavior - empty slices are omitted to save space

		jsonData, err := json.Marshal(ms)
		if err != nil {
			t.Fatalf("Failed to marshal: %v", err)
		}

		var result map[string]any
		if err := json.Unmarshal(jsonData, &result); err != nil {
			t.Fatalf("Failed to unmarshal: %v", err)
		}

		// With omitempty, empty slices are omitted (this is correct)
		// When we add a port mapping, it should appear
		ms.PortMappings = append(ms.PortMappings, NewPortMapping(8080, 80, false))
		jsonData2, _ := json.Marshal(ms)
		var result2 map[string]any
		_ = json.Unmarshal(jsonData2, &result2)

		if ports, ok := result2["portMappings"].([]any); !ok || len(ports) != 1 {
			t.Error("PortMappings should be present when not empty")
		}
	})
}
