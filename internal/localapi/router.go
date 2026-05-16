package localapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/eclipse-iofog/agent/internal/localapi/handlers"
	"github.com/eclipse-iofog/agent/internal/localapi/websocket"
	"github.com/eclipse-iofog/agent/internal/utils/logging"
)

const (
	routerModuleName = "Local API Router"
)

// Router handles HTTP routing
type Router struct {
	mux                *http.ServeMux
	statusHandler      *handlers.StatusHandler
	infoHandler        *handlers.InfoHandler
	healthLiveHandler  http.HandlerFunc
	healthReadyHandler http.HandlerFunc
	metricsHandler     http.HandlerFunc
	versionHandler     *handlers.VersionHandler
	authHandler        *handlers.AuthHandler
	v3Handler          *handlers.V3Handler
	controlWSHandler   *websocket.ControlHandler
}

// NewRouter creates a new router
func NewRouter() *Router {
	r := &Router{
		mux:                http.NewServeMux(),
		statusHandler:      &handlers.StatusHandler{},
		infoHandler:        &handlers.InfoHandler{},
		healthLiveHandler:  handlers.HealthLiveHandler,
		healthReadyHandler: handlers.HealthReadyHandler,
		metricsHandler:     handlers.MetricsHandler,
		versionHandler:     &handlers.VersionHandler{},
		authHandler:        handlers.NewAuthHandler(),
		v3Handler:          handlers.NewV3Handler(),
		controlWSHandler:   websocket.NewControlHandler(),
	}
	r.setupRoutes()
	return r
}

// ServeHTTP implements http.Handler
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	fields := map[string]interface{}{
		"event":     "localapi.debug",
		"method":    req.Method,
		"path":      req.URL.Path,
		"requestId": strings.TrimSpace(req.Header.Get(requestIDHeader)),
	}
	if authHeader := strings.TrimSpace(req.Header.Get("Authorization")); strings.HasPrefix(authHeader, "Bearer ") {
		for key, value := range safeTokenMeta(strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))) {
			fields[key] = value
		}
	}
	payload, _ := json.Marshal(fields)
	logging.LogDebug(routerModuleName, string(payload))
	r.mux.ServeHTTP(w, req)
}

// withActionLogging wraps a handler with "Start/Finished Processing..." info logs
func withActionLogging(actionName string, handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		moduleName := "Local API : LocalApiServerHandler"
		logging.LogInfo(moduleName, "Start Processing "+actionName+" request")
		handler(w, req)
		logging.LogInfo(moduleName, "Finished Processing "+actionName+" request")
	}
}

// setupRoutes registers all routes
func (r *Router) setupRoutes() {
	// Health and metrics (no auth — used by orchestrators and monitoring)
	r.mux.HandleFunc("/health/live", r.healthLiveHandler)
	r.mux.HandleFunc("/health/ready", r.healthReadyHandler)
	r.mux.HandleFunc("/metrics", r.metricsHandler)

	// LocalAPI v3 baseline routes (canonical namespace: /v3)
	r.mux.HandleFunc("/v3/system/status", chainMiddleware(withRoute("/v3/system/status", r.statusHandler.HandleStatus), authMiddlewareV3, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v3/system/info", chainMiddleware(withRoute("/v3/system/info", r.infoHandler.HandleInfo), authMiddlewareV3, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v3/system/version", chainMiddleware(withRoute("/v3/system/version", r.versionHandler.HandleVersion), authMiddlewareV3, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v3/system/provision", chainMiddleware(withRoute("/v3/system/provision", r.v3Handler.HandleSystemProvision), authMiddlewareV3, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v3/system/reload", chainMiddleware(withRoute("/v3/system/reload", r.v3Handler.HandleSystemReload), authMiddlewareV3, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v3/system/prune", chainMiddleware(withRoute("/v3/system/prune", r.v3Handler.HandleSystemPrune), authMiddlewareV3, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v3/system/logs", chainMiddleware(withRoute("/v3/system/logs", r.v3Handler.HandleSystemLogs), authMiddlewareV3, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v3/system/logs:stream", chainMiddleware(withRoute("/v3/system/logs:stream", r.v3Handler.HandleSystemLogs), authMiddlewareV3, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v3/system/gps", chainMiddleware(withRoute("/v3/system/gps", r.v3Handler.HandleSystemGPS), authMiddlewareV3, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v3/system/config", chainMiddleware(withRoute("/v3/system/config", r.v3Handler.HandleConfig), authMiddlewareV3, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v3/system/controller/cert", chainMiddleware(withRoute("/v3/system/controller/cert", r.v3Handler.HandleSystemControllerCert), authMiddlewareV3, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v3/system/config/switch", chainMiddleware(withRoute("/v3/system/config/switch", r.v3Handler.HandleSystemConfigSwitch), authMiddlewareV3, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v3/images", chainMiddleware(withRoute("/v3/images", r.v3Handler.HandleImages), authMiddlewareV3, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v3/images:pull", chainMiddleware(withRoute("/v3/images:pull", r.v3Handler.HandleImagePull), authMiddlewareV3, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v3/images:pull/", chainMiddleware(withRoute("/v3/images:pull/", r.v3Handler.HandleImagePullStatus), authMiddlewareV3, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v3/images:load", chainMiddleware(withRoute("/v3/images:load", r.v3Handler.HandleImageLoad), authMiddlewareV3, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v3/images:prune", chainMiddleware(withRoute("/v3/images:prune", r.v3Handler.HandleImagePrune), authMiddlewareV3, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v3/images:remove", chainMiddleware(withRoute("/v3/images:remove", r.v3Handler.HandleImageRemove), authMiddlewareV3, accessLoggingMiddleware, requestIdMiddleware))

	r.mux.HandleFunc("/v3/microservices/config", chainMiddleware(withRoute("/v3/microservices/config", r.v3Handler.HandleMicroserviceConfigSelf), authMiddlewareV3, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v3/microservices/control", r.controlWSHandler.Handle)

	r.mux.HandleFunc("/v3/ms", chainMiddleware(withRoute("/v3/ms", r.v3Handler.HandleMicroservices), authMiddlewareV3, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v3/ms/", chainMiddleware(withRoute("/v3/ms/", r.v3Handler.HandleMicroservices), authMiddlewareV3, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v3/deploy/microservices:apply", chainMiddleware(withRoute("/v3/deploy/microservices:apply", r.v3Handler.HandleDeployMicroservicesApply), authMiddlewareV3, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v3/deploy/microservices:apply/", chainMiddleware(withRoute("/v3/deploy/microservices:apply/", r.v3Handler.HandleDeployMicroservicesApplyStatus), authMiddlewareV3, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v3/deploy/microservices:validate", chainMiddleware(withRoute("/v3/deploy/microservices:validate", r.v3Handler.HandleDeployMicroservicesValidate), authMiddlewareV3, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v3/deploy/microservices", chainMiddleware(withRoute("/v3/deploy/microservices", r.v3Handler.HandleDeployMicroservices), authMiddlewareV3, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v3/deploy/microservices/", chainMiddleware(withRoute("/v3/deploy/microservices/", r.v3Handler.HandleDeployMicroservices), authMiddlewareV3, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v3/deploy/registries:apply", chainMiddleware(withRoute("/v3/deploy/registries:apply", r.v3Handler.HandleDeployRegistriesApply), authMiddlewareV3, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v3/deploy/registries:validate", chainMiddleware(withRoute("/v3/deploy/registries:validate", r.v3Handler.HandleDeployRegistriesValidate), authMiddlewareV3, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v3/deploy/registries", chainMiddleware(withRoute("/v3/deploy/registries", r.v3Handler.HandleDeployRegistries), authMiddlewareV3, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v3/deploy/registries/", chainMiddleware(withRoute("/v3/deploy/registries/", r.v3Handler.HandleDeployRegistries), authMiddlewareV3, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v3/auth/whoami", chainMiddleware(withRoute("/v3/auth/whoami", r.authHandler.HandleWhoAmI), authMiddlewareV3, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v3/auth/tokens", chainMiddleware(withRoute("/v3/auth/tokens", r.v3Handler.HandleAuthTokens), authMiddlewareV3, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v3/auth/tokens/revoke", chainMiddleware(withRoute("/v3/auth/tokens/revoke", r.v3Handler.HandleAuthTokensRevoke), authMiddlewareV3, accessLoggingMiddleware, requestIdMiddleware))
}
