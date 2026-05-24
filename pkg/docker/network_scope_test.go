package docker

import "testing"

func TestResolveIofogBridgeNetworkName(t *testing.T) {
	tests := []struct {
		name            string
		applicationName string
		hostNetwork     bool
		want            string
	}{
		{name: "managed workload", applicationName: "edge-app", hostNetwork: false, want: edgeletNetworkName},
		{name: "local workload", applicationName: "local", hostNetwork: false, want: edgeletNetworkName},
		{name: "local host-network bypass", applicationName: "local", hostNetwork: true, want: edgeletNetworkName},
		{name: "managed host-network", applicationName: "edge-app", hostNetwork: true, want: edgeletNetworkName},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveIofogBridgeNetworkName(tt.applicationName, tt.hostNetwork); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}
