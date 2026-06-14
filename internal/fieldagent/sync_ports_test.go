package fieldagent

import (
	"testing"

	"github.com/eclipse-iofog/edgelet/internal/controlplane"
)

func TestParsePortMappingFromController_WireFormat(t *testing.T) {
	pm := parsePortMappingFromController(map[string]any{
		"internal": float64(8008),
		"external": float64(80),
		"protocol": "tcp",
	})
	if pm == nil {
		t.Fatal("expected port mapping")
	}
	if pm.Inside != 8008 || pm.Outside != 80 || pm.UDP {
		t.Fatalf("unexpected mapping: inside=%d outside=%d udp=%v", pm.Inside, pm.Outside, pm.UDP)
	}
}

func TestParsePortMappingFromController_LegacyFormat(t *testing.T) {
	pm := parsePortMappingFromController(map[string]any{
		"portInternal": float64(51121),
		"portExternal": float64(51121),
		"isUdp":        true,
	})
	if pm == nil {
		t.Fatal("expected port mapping")
	}
	if pm.Inside != 51121 || pm.Outside != 51121 || !pm.UDP {
		t.Fatalf("unexpected mapping: inside=%d outside=%d udp=%v", pm.Inside, pm.Outside, pm.UDP)
	}
}

func TestParseMicroservice_PortMappingsWireFormat(t *testing.T) {
	ms, err := parseMicroservice(map[string]any{
		"uuid":    "ms-1",
		"imageId": "alpine:3.19",
		"portMappings": []any{
			map[string]any{
				"internal": float64(controlplane.DefaultContainerAPIPort),
				"external": float64(controlplane.HostAPIPort),
				"protocol": "tcp",
			},
		},
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(ms.PortMappings) != 1 {
		t.Fatalf("expected 1 port mapping, got %d", len(ms.PortMappings))
	}
	if ms.PortMappings[0].Inside != controlplane.DefaultContainerAPIPort {
		t.Fatalf("inside: got %d want %d", ms.PortMappings[0].Inside, controlplane.DefaultContainerAPIPort)
	}
	if ms.PortMappings[0].Outside != controlplane.HostAPIPort {
		t.Fatalf("outside: got %d want %d", ms.PortMappings[0].Outside, controlplane.HostAPIPort)
	}
}
