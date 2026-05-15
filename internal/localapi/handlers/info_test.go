package handlers

import (
	"net"
	"testing"

	"github.com/eclipse-iofog/agent/internal/network"
)

func TestFormatInfoNetworkInterface(t *testing.T) {
	t.Run("dynamic with detected interface", func(t *testing.T) {
		detected := &network.NetworkInterfaceInfo{
			Interface: &net.Interface{Name: "eth0"},
		}
		got := formatInfoNetworkInterface("dynamic", detected)
		if got != "dynamic (eth0)" {
			t.Fatalf("expected dynamic interface display, got %q", got)
		}
	})

	t.Run("dynamic without detected interface", func(t *testing.T) {
		got := formatInfoNetworkInterface("dynamic", nil)
		if got != "dynamic" {
			t.Fatalf("expected dynamic fallback display, got %q", got)
		}
	})

	t.Run("dynamic with empty detected name", func(t *testing.T) {
		detected := &network.NetworkInterfaceInfo{
			Interface: &net.Interface{Name: "  "},
		}
		got := formatInfoNetworkInterface("dynamic", detected)
		if got != "dynamic" {
			t.Fatalf("expected dynamic fallback for empty interface name, got %q", got)
		}
	})

	t.Run("static configured value unchanged", func(t *testing.T) {
		got := formatInfoNetworkInterface("ens3", &network.NetworkInterfaceInfo{
			Interface: &net.Interface{Name: "eth0"},
		})
		if got != "ens3" {
			t.Fatalf("expected static interface value, got %q", got)
		}
	})
}
