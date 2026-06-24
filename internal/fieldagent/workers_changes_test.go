package fieldagent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eclipse-iofog/edgelet/internal/auth"
	"github.com/eclipse-iofog/edgelet/internal/config"
	"github.com/eclipse-iofog/edgelet/internal/models"
)

func TestRunChangesWorker_ContinuesAfterProcessChangesPanic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/agent/config/changes") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)

	cfg := config.GetInstance()
	origFreq := cfg.ChangeFrequency
	cfg.ChangeFrequency = 1
	t.Cleanup(func() { cfg.ChangeFrequency = origFreq })

	var calls atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	fa := &FieldAgent{
		config: cfg,
		state:  NewState(),
		ctx:    ctx,
		apiClient: &APIClient{
			baseURL:    srv.URL,
			httpClient: srv.Client(),
			jwtManager: auth.GetJWTManager(),
		},
		processChangesFn: func(map[string]any) bool {
			if calls.Add(1) == 1 {
				panic("test panic")
			}
			return true
		},
	}
	fa.state.SetControllerStatus(models.ControllerStatusOK)
	fa.state.SetControllerVerified(true)

	fa.wg.Add(1)
	go fa.runChangesWorker()

	deadline := time.Now().Add(5 * time.Second)
	for calls.Load() < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("expected at least 2 processChanges calls after panic recovery, got %d", calls.Load())
		}
		time.Sleep(20 * time.Millisecond)
	}

	cancel()
	done := make(chan struct{})
	go func() {
		fa.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("changes worker did not exit after context cancel")
	}
}
