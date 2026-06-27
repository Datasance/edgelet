package run

import (
	"errors"
	"testing"

	"github.com/eclipse-iofog/edgelet/internal/cli/client"
)

func TestMapAPIErrorExecStartTimeout(t *testing.T) {
	err := MapAPIError(&client.EdgeletAPIError{
		Code:    CodeExecStartTimeout,
		Message: "exec start timeout after 15s",
	})
	var cliErr *CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected *CLIError, got %T", err)
	}
	if cliErr.Code != CodeExecStartTimeout {
		t.Fatalf("expected code %s, got %s", CodeExecStartTimeout, cliErr.Code)
	}
	want := "Error[EXEC_START_TIMEOUT]: Interactive shell did not start within 15 seconds. Retry `edgelet ms exec`; if the problem persists, check microservice and engine logs."
	if cliErr.Error() != want {
		t.Fatalf("unexpected error string: %s", cliErr.Error())
	}
}
