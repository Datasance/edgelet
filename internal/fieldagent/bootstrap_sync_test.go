package fieldagent

import (
	"testing"

	"github.com/eclipse-iofog/edgelet/internal/models"
)

func TestIsControllerConnectedCacheOnly(t *testing.T) {
	fa := GetInstance()
	fa.state.SetControllerStatus(models.ControllerStatusNotConnected)
	fa.state.SetControllerVerified(false)

	if !fa.IsControllerConnected(true) {
		t.Fatal("expected cache reads to be allowed when fromFile=true")
	}
	if fa.IsControllerConnected(false) {
		t.Fatal("expected live controller I/O to be blocked when not verified")
	}

	fa.state.SetControllerStatus(models.ControllerStatusOK)
	fa.state.SetControllerVerified(true)
	if !fa.IsControllerConnected(false) {
		t.Fatal("expected live controller I/O when verified and status OK")
	}
}

func TestIsControllerConnectedNotProvisioned(t *testing.T) {
	fa := GetInstance()
	fa.state.SetControllerStatus(models.ControllerStatusNotProvisioned)
	fa.state.SetControllerVerified(false)

	if fa.IsControllerConnected(false) {
		t.Fatal("expected not provisioned agent to report controller disconnected")
	}
}
