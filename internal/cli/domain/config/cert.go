package config

import (
	"strings"

	"github.com/datasance/edgelet/internal/cli/output"
	"github.com/datasance/edgelet/internal/cli/run"
)

// CertResult carries controller cert install outcome.
type CertResult struct {
	Human string
	Data  map[string]interface{}
}

// ApplyCert decodes and installs a base64-encoded controller certificate.
func ApplyCert(client run.EdgeletAPIClient, base64Cert string) (*CertResult, error) {
	if client == nil {
		return nil, run.NewCLIError(run.CodeInternal, "edgeletapi client is nil", nil)
	}
	certValue := strings.TrimSpace(base64Cert)
	if certValue == "" {
		return nil, run.NewCLIError(run.CodeInvalidArgument, "certificate value cannot be empty", nil)
	}
	data, err := client.Request("POST", "/v1/system/controller/cert", map[string]interface{}{
		"certificate": certValue,
	})
	if err != nil {
		return nil, run.MapAPIError(err)
	}
	return &CertResult{
		Human: output.FormatEdgeletAPIHuman("/v1/system/controller/cert", data),
		Data:  data,
	}, nil
}
