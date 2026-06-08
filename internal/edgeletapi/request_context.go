package edgeletapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
)

const requestIDHeader = "X-Request-Id"

type contextKey string

const (
	requestIDContextKey contextKey = "edgeletapi.requestID"
	routeContextKey     contextKey = "edgeletapi.route"
	authMetaContextKey  contextKey = "edgeletapi.authMeta"
)

func requestIDMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		requestID := strings.TrimSpace(r.Header.Get(requestIDHeader))
		if requestID == "" {
			requestID = generateUUIDv4()
		}
		w.Header().Set(requestIDHeader, requestID)
		next(w, r.WithContext(context.WithValue(r.Context(), requestIDContextKey, requestID)))
	}
}

func withRoute(route string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		next(w, r.WithContext(context.WithValue(r.Context(), routeContextKey, route)))
	}
}

func requestIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDContextKey).(string); ok {
		return v
	}
	return ""
}

func withAuthMeta(r *http.Request, authMeta map[string]any) *http.Request {
	if len(authMeta) == 0 {
		return r
	}
	return r.WithContext(context.WithValue(r.Context(), authMetaContextKey, authMeta))
}

func authMetaFromContext(ctx context.Context) map[string]any {
	meta, ok := ctx.Value(authMetaContextKey).(map[string]any)
	if !ok || len(meta) == 0 {
		return map[string]any{}
	}
	return meta
}

func routeFromContextOrPath(r *http.Request) string {
	if v, ok := r.Context().Value(routeContextKey).(string); ok && strings.TrimSpace(v) != "" {
		return v
	}
	return r.URL.Path
}

func generateUUIDv4() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "00000000-0000-4000-8000-000000000000"
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return hex.EncodeToString(b[0:4]) + "-" +
		hex.EncodeToString(b[4:6]) + "-" +
		hex.EncodeToString(b[6:8]) + "-" +
		hex.EncodeToString(b[8:10]) + "-" +
		hex.EncodeToString(b[10:16])
}
