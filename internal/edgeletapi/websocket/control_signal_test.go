package websocket

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/eclipse-iofog/edgelet/internal/auth"
	"github.com/golang-jwt/jwt/v5"
	gws "github.com/gorilla/websocket"
)

func TestSendControlSignalToAll_SendsOpcode(t *testing.T) {
	const microserviceUUID = "ms-control-signal"
	runControlSignalOpcodeTest(t, microserviceUUID, func(h *ControlHandler) {
		h.SendControlSignalToAll([]string{microserviceUUID})
	}, OpcodeControlSignal)
}

func TestSendResourceSignal_SendsOpcode(t *testing.T) {
	const microserviceUUID = "ms-resource-signal"
	runControlSignalOpcodeTest(t, microserviceUUID, func(h *ControlHandler) {
		h.SendResourceSignal()
	}, OpcodeResourceSignal)
}

func runControlSignalOpcodeTest(
	t *testing.T,
	microserviceUUID string,
	send func(*ControlHandler),
	wantOpcode byte,
) {
	t.Helper()

	originalValidate := validateLocalJWTFn
	originalAuthorize := authorizeV3WSFn
	defer func() {
		validateLocalJWTFn = originalValidate
		authorizeV3WSFn = originalAuthorize
	}()
	validateLocalJWTFn = func(string) (*auth.LocalJWTValidationResult, error) {
		return &auth.LocalJWTValidationResult{
			Claims: serviceAccountControlClaims(microserviceUUID),
		}, nil
	}
	authorizeV3WSFn = func(jwt.MapClaims) bool { return true }

	handler, mgr := newTestControlHandler(t)
	server := httptest.NewServer(http.HandlerFunc(handler.Handle))
	t.Cleanup(func() {
		server.Close()
		waitForControlConnectionsDrained(t, mgr)
	})

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/v1/microservices/control"
	h := http.Header{}
	h.Set("Authorization", "Bearer mock")
	conn, _, err := gws.DefaultDialer.Dial(wsURL, h)
	if err != nil {
		t.Fatalf("dial control websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	waitForControlConnectionRegistered(t, mgr, microserviceUUID)

	type readResult struct {
		msg []byte
		err error
	}
	done := make(chan readResult, 1)
	go func() {
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, msg, readErr := conn.ReadMessage()
		done <- readResult{msg: msg, err: readErr}
	}()

	send(handler)

	result := <-done
	if result.err != nil {
		t.Fatalf("read control websocket signal: %v", result.err)
	}
	if len(result.msg) != 1 || result.msg[0] != wantOpcode {
		t.Fatalf("expected opcode 0x%x, got %v", wantOpcode, result.msg)
	}
}
