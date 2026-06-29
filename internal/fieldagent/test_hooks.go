package fieldagent

import (
	"context"
	"net/http"

	"github.com/eclipse-iofog/edgelet/internal/auth"
	"github.com/eclipse-iofog/edgelet/internal/models"
)

// ResetControllerRegisterStateForTest clears in-memory register markers.
func ResetControllerRegisterStateForTest() {
	GetInstance().resetControllerRegisterState()
}

// MarkControllerRegisterSucceededForTest marks a controller UUID as registered in memory.
func MarkControllerRegisterSucceededForTest(uuid string) {
	fa := GetInstance()
	if fa.controllerRegister == nil {
		fa.controllerRegister = newControllerRegisterState()
	}
	fa.controllerRegister.markSucceeded(uuid)
}

// ConfigureRegisterTestClient wires a test HTTP client for controller/register calls.
func ConfigureRegisterTestClient(baseURL string, httpClient *http.Client) {
	fa := GetInstance()
	fa.apiClient = &APIClient{
		baseURL:    baseURL,
		httpClient: httpClient,
		jwtManager: auth.GetJWTManager(),
	}
}

// ConfigureRegisterTestRuntime wires field agent state for controller register integration tests.
func ConfigureRegisterTestRuntime(ctx context.Context, connected bool) {
	fa := GetInstance()
	if ctx != nil {
		fa.ctx = ctx
	}
	if connected {
		fa.state.SetControllerStatus(models.ControllerStatusOK)
		fa.state.SetControllerVerified(true)
	} else {
		fa.state.SetControllerStatus(models.ControllerStatusNotProvisioned)
		fa.state.SetControllerVerified(false)
	}
}
