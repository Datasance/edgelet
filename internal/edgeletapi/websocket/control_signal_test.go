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

	done := make(chan []byte, 1)
	go func() {
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, msg, readErr := conn.ReadMessage()
		if readErr != nil {
			t.Errorf("read control signal: %v", readErr)
			done <- nil
			return
		}
		done <- msg
	}()

	handler.SendControlSignalToAll([]string{microserviceUUID})

	msg := <-done
	if len(msg) != 1 || msg[0] != OpcodeControlSignal {
		t.Fatalf("expected control opcode 0x%x, got %v", OpcodeControlSignal, msg)
	}
	_ = conn.Close()
}

func TestSendResourceSignal_SendsOpcode(t *testing.T) {
	const microserviceUUID = "ms-resource-signal"

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

	done := make(chan []byte, 1)
	go func() {
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, msg, readErr := conn.ReadMessage()
		if readErr != nil {
			t.Errorf("read resource signal: %v", readErr)
			done <- nil
			return
		}
		done <- msg
	}()

	handler.SendResourceSignal()

	msg := <-done
	if len(msg) != 1 || msg[0] != OpcodeResourceSignal {
		t.Fatalf("expected resource opcode 0x%x, got %v", OpcodeResourceSignal, msg)
	}
	_ = conn.Close()
}
