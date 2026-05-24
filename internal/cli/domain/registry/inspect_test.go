package registry

import (
	"strings"
	"testing"

	"github.com/datasance/edgelet/internal/cli/run"
)

type inspectFakeClient struct {
	gets map[string]map[string]interface{}
}

func (f *inspectFakeClient) IsDaemonRunning() bool { return true }

func (f *inspectFakeClient) RequestV3(method, path string, _ interface{}) (map[string]interface{}, error) {
	key := method + " " + path
	if data, ok := f.gets[key]; ok {
		return data, nil
	}
	return nil, run.NewCLIError(run.CodeNotFound, "not found", nil)
}

func (f *inspectFakeClient) RequestV3MultipartFile(string, string, string, string, map[string]string) (map[string]interface{}, error) {
	return nil, run.NewCLIError(run.CodeInternal, "unexpected multipart request", nil)
}

func TestInspect_PasswordPlain(t *testing.T) {
	client := &inspectFakeClient{
		gets: map[string]map[string]interface{}{
			"GET /v1/deploy/registries/7": {
				"id":        7,
				"url":       "https://registry.example/v2/",
				"isPublic":  false,
				"userName":  "user",
				"userEmail": "user@example.com",
				"password":  "secret",
			},
		},
	}
	result, err := Inspect(client, "7", true)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !strings.Contains(result.Human, "PASSWORD: secret") {
		t.Fatalf("expected plain password in output, got: %q", result.Human)
	}
}

func TestInspect_RequiresID(t *testing.T) {
	_, err := Inspect(&inspectFakeClient{}, "  ", false)
	if err == nil || !strings.Contains(err.Error(), "registry id is required") {
		t.Fatalf("expected id required error, got: %v", err)
	}
}
