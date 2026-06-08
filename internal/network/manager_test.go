package network

import "testing"

func TestValidateNetworkInterfaceConfig(t *testing.T) {
	mgr := GetInstance()

	if err := mgr.ValidateNetworkInterfaceConfig("http://127.0.0.1:51121", "dynamic"); err != nil {
		t.Fatalf("expected dynamic interface config to be accepted, got error: %v", err)
	}

	if err := mgr.ValidateNetworkInterfaceConfig("http://127.0.0.1:51121", "iface-does-not-exist-98765"); err == nil {
		t.Fatal("expected unknown interface to be rejected")
	}
}
