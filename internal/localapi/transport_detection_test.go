package localapi

import (
	"crypto/tls"
	"net/http/httptest"
	"testing"
)

func TestDetectTransport_UnixSocket(t *testing.T) {
	req := httptest.NewRequest("GET", "http://unix/v1/system/status", nil)
	req.Host = "unix"
	req.RemoteAddr = "@/run/edgelet/edgelet.sock"
	transport, scheme := detectTransport(req)
	if transport != "unix" || scheme != "http+unix" {
		t.Fatalf("unexpected transport/scheme: %s %s", transport, scheme)
	}
}

func TestDetectTransport_TLSAndWSS(t *testing.T) {
	req := httptest.NewRequest("GET", "https://localhost/v1/system/status", nil)
	req.TLS = &tls.ConnectionState{}
	transport, scheme := detectTransport(req)
	if transport != "tcp" || scheme != "https" {
		t.Fatalf("unexpected transport/scheme: %s %s", transport, scheme)
	}

	reqWS := httptest.NewRequest("GET", "https://localhost/v1/microservices/control", nil)
	reqWS.TLS = &tls.ConnectionState{}
	reqWS.Header.Set("Connection", "Upgrade")
	reqWS.Header.Set("Upgrade", "websocket")
	transport, scheme = detectTransport(reqWS)
	if transport != "tcp" || scheme != "wss" {
		t.Fatalf("unexpected websocket transport/scheme: %s %s", transport, scheme)
	}
}
