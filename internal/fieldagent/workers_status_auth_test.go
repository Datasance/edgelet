package fieldagent

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eclipse-iofog/edgelet/internal/config"
	"github.com/eclipse-iofog/edgelet/internal/models"
	"github.com/eclipse-iofog/edgelet/internal/version"
)

func newStatusPostTestAgent(postFn func(context.Context, map[string]any) error) (*FieldAgent, *int) {
	deprovisionCalls := 0
	fa := &FieldAgent{
		config: config.GetInstance(),
		state:  NewState(),
		postStatusFn: func(ctx context.Context, status map[string]any) error {
			if postFn == nil {
				return nil
			}
			return postFn(ctx, status)
		},
		deprovisionFn: func(clearCredentials bool) error {
			deprovisionCalls++
			return nil
		},
	}
	fa.state.SetControllerStatus(models.ControllerStatusOK)
	fa.state.SetControllerVerified(true)
	return fa, &deprovisionCalls
}

func agentJWTAuthError() error {
	return ParseControllerAPIError(http.StatusUnauthorized, `{"error":"Unauthorized","code":"AGENT_JWT_SIGNATURE_INVALID","message":"Agent JWT signature invalid","retryable":false}`)
}

func TestPostStatusHelper_503NeverDeprovisions(t *testing.T) {
	fa, deprovisionCalls := newStatusPostTestAgent(func(_ context.Context, _ map[string]any) error {
		return ParseControllerAPIError(http.StatusServiceUnavailable, `{"error":"ServiceUnavailable","code":"CONTROLLER_DB_BUSY","message":"Database temporarily unavailable","retryable":true}`)
	})

	fa.PostStatusHelper()

	if *deprovisionCalls != 0 {
		t.Fatalf("expected no deprovision on 503, got %d", *deprovisionCalls)
	}
}

func TestPostStatusHelper_401FirstAttemptDefersDeprovision(t *testing.T) {
	fa, deprovisionCalls := newStatusPostTestAgent(func(_ context.Context, _ map[string]any) error {
		return agentJWTAuthError()
	})

	fa.PostStatusHelper()

	if *deprovisionCalls != 0 {
		t.Fatalf("expected no deprovision on first 401, got %d", *deprovisionCalls)
	}
	if got := fa.statusAuthFailureCount(); got != 1 {
		t.Fatalf("expected streak count 1, got %d", got)
	}
}

func TestPostStatusHelper_Five401Over60sDeprovisionsOnce(t *testing.T) {
	start := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	now := start
	fa, deprovisionCalls := newStatusPostTestAgent(func(_ context.Context, _ map[string]any) error {
		return agentJWTAuthError()
	})
	fa.statusAuthNowFn = func() time.Time { return now }

	for i := 0; i < 4; i++ {
		fa.PostStatusHelper()
		if *deprovisionCalls != 0 {
			t.Fatalf("attempt %d: expected no deprovision before gate, got %d", i+1, *deprovisionCalls)
		}
		now = now.Add(15 * time.Second)
	}

	fa.PostStatusHelper()
	if *deprovisionCalls != 1 {
		t.Fatalf("expected one deprovision after 5 failures over 60s, got %d", *deprovisionCalls)
	}
}

func TestPostStatusHelper_SuccessResetsStatusAuthStreak(t *testing.T) {
	attempt := 0
	fa, deprovisionCalls := newStatusPostTestAgent(func(_ context.Context, _ map[string]any) error {
		attempt++
		switch attempt {
		case 3:
			return nil
		default:
			return agentJWTAuthError()
		}
	})

	fa.PostStatusHelper()
	fa.PostStatusHelper()
	if got := fa.statusAuthFailureCount(); got != 2 {
		t.Fatalf("expected streak count 2 before success, got %d", got)
	}

	fa.PostStatusHelper()
	if got := fa.statusAuthFailureCount(); got != 0 {
		t.Fatalf("expected streak reset after success, got %d", got)
	}

	fa.PostStatusHelper()
	if got := fa.statusAuthFailureCount(); got != 1 {
		t.Fatalf("expected streak count 1 after post-success 401, got %d", got)
	}
	if *deprovisionCalls != 0 {
		t.Fatalf("expected no deprovision after single post-reset 401, got %d", *deprovisionCalls)
	}
}

func TestPostStatusHelper_OTAPendingSuppressesDeprovision(t *testing.T) {
	dir := t.TempDir()
	pendingFile := filepath.Join(dir, "ota-reprovision-pending")
	version.SetOTAReprovisionPendingPath(pendingFile)
	t.Cleanup(func() { version.SetOTAReprovisionPendingPath("") })

	if err := version.WriteOTAReprovisionPending("key-1", "upgrade", "1.0.0", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("write pending OTA: %v", err)
	}

	start := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	now := start
	fa, deprovisionCalls := newStatusPostTestAgent(func(_ context.Context, _ map[string]any) error {
		return agentJWTAuthError()
	})
	fa.statusAuthNowFn = func() time.Time { return now }

	for i := 0; i < 5; i++ {
		fa.PostStatusHelper()
		now = now.Add(20 * time.Second)
	}

	if *deprovisionCalls != 0 {
		t.Fatalf("expected no deprovision while OTA pending, got %d", *deprovisionCalls)
	}
	if got := fa.statusAuthFailureCount(); got != 0 {
		t.Fatalf("expected streak not incremented during OTA suppress, got %d", got)
	}
}

func TestProvisionSuccessResetsStatusAuthStreak(t *testing.T) {
	fa := &FieldAgent{
		config: config.GetInstance(),
		state:  NewState(),
	}
	fa.recordStatusAuthFailure(time.Now())
	if got := fa.statusAuthFailureCount(); got != 1 {
		t.Fatalf("expected initial streak count 1, got %d", got)
	}

	fa.resetStatusAuthFailure()
	if got := fa.statusAuthFailureCount(); got != 0 {
		t.Fatalf("expected streak reset after provision success hook, got %d", got)
	}
}

func TestShouldDeprovisionForStatusAuth(t *testing.T) {
	fa := &FieldAgent{}
	start := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)

	for i := 0; i < 4; i++ {
		fa.recordStatusAuthFailure(start.Add(time.Duration(i) * time.Second))
	}
	if fa.shouldDeprovisionForStatusAuth(start.Add(60 * time.Second)) {
		t.Fatal("expected gate closed with only 4 failures")
	}

	fa.recordStatusAuthFailure(start.Add(4 * time.Second))
	if fa.shouldDeprovisionForStatusAuth(start.Add(59 * time.Second)) {
		t.Fatal("expected gate closed before 60s window")
	}
	if !fa.shouldDeprovisionForStatusAuth(start.Add(60 * time.Second)) {
		t.Fatal("expected gate open after 5 failures over 60s")
	}
}

func TestShouldSuppressStatusAuthDeprovision(t *testing.T) {
	fa := &FieldAgent{}

	fa.setProvisionInFlight(true)
	if !fa.shouldSuppressStatusAuthDeprovision() {
		t.Fatal("expected suppress while provision in flight")
	}
	fa.setProvisionInFlight(false)

	dir := t.TempDir()
	pendingFile := filepath.Join(dir, "ota-reprovision-pending")
	version.SetOTAReprovisionPendingPath(pendingFile)
	t.Cleanup(func() { version.SetOTAReprovisionPendingPath("") })

	if err := os.WriteFile(pendingFile, []byte(`{"provisionKey":"k","command":"upgrade"}`), 0o600); err != nil {
		t.Fatalf("write pending file: %v", err)
	}
	if !fa.shouldSuppressStatusAuthDeprovision() {
		t.Fatal("expected suppress while OTA pending file present")
	}
}

func TestPostStatusHelper_Legacy401UsesGate(t *testing.T) {
	fa, deprovisionCalls := newStatusPostTestAgent(func(_ context.Context, _ map[string]any) error {
		return ParseControllerAPIError(http.StatusUnauthorized, `{"name":"AuthenticationError","message":"Expired provision key"}`)
	})

	fa.PostStatusHelper()

	if *deprovisionCalls != 0 {
		t.Fatalf("expected no deprovision on first legacy 401, got %d", *deprovisionCalls)
	}
	if got := fa.statusAuthFailureCount(); got != 1 {
		t.Fatalf("expected streak count 1 for legacy 401, got %d", got)
	}
}

func TestPostStatusHelper_UnstructuredUnauthorizedUsesGate(t *testing.T) {
	fa, deprovisionCalls := newStatusPostTestAgent(func(_ context.Context, _ map[string]any) error {
		return errors.New("unauthorized: invalid JWT")
	})

	fa.PostStatusHelper()

	if *deprovisionCalls != 0 {
		t.Fatalf("expected no deprovision on first unstructured 401, got %d", *deprovisionCalls)
	}
}
