package localapi

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/datasance/edgelet/internal/utils/logging"
)

const (
	reasonMissingBearerPrefix = "missing_bearer_prefix"
	reasonMissingToken        = "missing_token"
	reasonEmptyToken          = "empty_token"
	reasonInvalidToken        = "invalid_token"
	reasonTokenValidation     = "token_validation_error"
	reasonTokenUseMismatch    = "token_use_mismatch"
	reasonAudienceMismatch    = "audience_mismatch"
	reasonIssuerMismatch      = "issuer_mismatch"
	reasonSignatureRequired   = "signature_required"
	reasonUnsignedRejected    = "unsigned_token_rejected"
	reasonRBACUnmappedRoute   = "rbac_unmapped_route"
	reasonRBACDenied          = "rbac_denied"
	reasonMethodNotAllowed    = "method_not_allowed"
	reasonWSUpgradeFailed     = "websocket_upgrade_failed"
	reasonWSUnauthorized      = "websocket_unauthorized"
	reasonInternalError       = "internal_error"
)

type responseCaptureWriter struct {
	http.ResponseWriter
	status  int
	written int
}

func (w *responseCaptureWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseCaptureWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.written += n
	return n, err
}

func (w *responseCaptureWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *responseCaptureWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	return hijacker.Hijack()
}

func (w *responseCaptureWriter) ReadFrom(r io.Reader) (int64, error) {
	readerFrom, ok := w.ResponseWriter.(io.ReaderFrom)
	if !ok {
		return io.Copy(w.ResponseWriter, r)
	}
	n, err := readerFrom.ReadFrom(r)
	w.written += int(n)
	return n, err
}

func (w *responseCaptureWriter) StatusCode() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

type structuredEvent struct {
	Level  string
	Module string
	Fields map[string]interface{}
}

var localAPILogSink = func(event structuredEvent) {
	payload, _ := json.Marshal(event.Fields)
	switch strings.ToLower(event.Level) {
	case "warn":
		logging.LogWarn(event.Module, string(payload))
	case "error":
		logging.LogError(event.Module, string(payload), nil)
	default:
		logging.LogInfo(event.Module, string(payload))
	}
}

func accessLoggingMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := &responseCaptureWriter{ResponseWriter: w}
		next(wrapped, r)
		fields := baseLogFields(r, routeFromContextOrPath(r), wrapped.StatusCode(), wrapped.written, start)
		fields["event"] = "localapi.access"
		localAPILogSink(structuredEvent{
			Level:  "info",
			Module: middlewareModuleName,
			Fields: fields,
		})
	}
}

func emitRejectEvent(r *http.Request, route, reasonCode string, statusCode int, tokenMeta map[string]interface{}) {
	fields := baseLogFields(r, route, statusCode, 0, time.Now())
	fields["event"] = "localapi.reject"
	fields["reasonCode"] = reasonCode
	for key, value := range tokenMeta {
		fields[key] = value
	}
	localAPILogSink(structuredEvent{
		Level:  "warn",
		Module: middlewareModuleName,
		Fields: fields,
	})
}

func emitErrorEvent(r *http.Request, route, summary string) {
	fields := baseLogFields(r, route, http.StatusInternalServerError, 0, time.Now())
	fields["event"] = "localapi.error"
	fields["errorSummary"] = summary
	localAPILogSink(structuredEvent{
		Level:  "error",
		Module: middlewareModuleName,
		Fields: fields,
	})
}

func baseLogFields(r *http.Request, route string, status, bytesOut int, start time.Time) map[string]interface{} {
	transport, scheme := detectTransport(r)
	bytesIn := r.ContentLength
	if bytesIn < 0 {
		bytesIn = 0
	}
	fields := map[string]interface{}{
		"requestId":  requestIDFromContext(r.Context()),
		"transport":  transport,
		"scheme":     scheme,
		"method":     r.Method,
		"path":       r.URL.Path,
		"route":      route,
		"status":     status,
		"durationMs": time.Since(start).Milliseconds(),
		"bytesIn":    bytesIn,
		"bytesOut":   bytesOut,
		"remoteAddr": r.RemoteAddr,
		"userAgent":  r.UserAgent(),
	}
	for key, value := range authMetaFromContext(r.Context()) {
		fields[key] = value
	}
	return fields
}

func detectTransport(r *http.Request) (string, string) {
	host := strings.ToLower(strings.TrimSpace(r.Host))
	remote := strings.ToLower(strings.TrimSpace(r.RemoteAddr))
	if host == "unix" || strings.Contains(remote, ".sock") || strings.HasPrefix(remote, "@") || strings.Contains(remote, "/") {
		return "unix", "http+unix"
	}
	if isWebsocketUpgrade(r) {
		if r.TLS != nil {
			return "tcp", "wss"
		}
		return "tcp", "ws"
	}
	if r.TLS != nil {
		return "tcp", "https"
	}
	return "tcp", "http"
}

func isWebsocketUpgrade(r *http.Request) bool {
	return strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade") &&
		strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

func hashJTI(jti string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(jti)))
	return hex.EncodeToString(sum[:])
}
