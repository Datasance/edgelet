package localapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/datasance/edgelet/internal/localapi/handlers"
	"github.com/datasance/edgelet/internal/localapi/websocket"
	"github.com/datasance/edgelet/internal/utils/logging"
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

	// LocalAPI v1 baseline routes (canonical namespace: /v3)
	r.mux.HandleFunc("/v1/system/status", chainMiddleware(withRoute("/v1/system/status", r.statusHandler.HandleStatus), authMiddlewareV1, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v1/system/info", chainMiddleware(withRoute("/v1/system/info", r.infoHandler.HandleInfo), authMiddlewareV1, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v1/system/version", chainMiddleware(withRoute("/v1/system/version", r.versionHandler.HandleVersion), authMiddlewareV1, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v1/system/provision", chainMiddleware(withRoute("/v1/system/provision", r.v3Handler.HandleSystemProvision), authMiddlewareV1, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v1/system/reload", chainMiddleware(withRoute("/v1/system/reload", r.v3Handler.HandleSystemReload), authMiddlewareV1, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v1/system/prune", chainMiddleware(withRoute("/v1/system/prune", r.v3Handler.HandleSystemPrune), authMiddlewareV1, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v1/system/logs", chainMiddleware(withRoute("/v1/system/logs", r.v3Handler.HandleSystemLogs), authMiddlewareV1, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v1/system/logs:stream", chainMiddleware(withRoute("/v1/system/logs:stream", r.v3Handler.HandleSystemLogs), authMiddlewareV1, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v1/system/gps", chainMiddleware(withRoute("/v1/system/gps", r.v3Handler.HandleSystemGPS), authMiddlewareV1, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v1/system/config", chainMiddleware(withRoute("/v1/system/config", r.v3Handler.HandleConfig), authMiddlewareV1, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v1/system/controller/cert", chainMiddleware(withRoute("/v1/system/controller/cert", r.v3Handler.HandleSystemControllerCert), authMiddlewareV1, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v1/system/config/switch", chainMiddleware(withRoute("/v1/system/config/switch", r.v3Handler.HandleSystemConfigSwitch), authMiddlewareV1, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v1/images", chainMiddleware(withRoute("/v1/images", r.v3Handler.HandleImages), authMiddlewareV1, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v1/images:pull", chainMiddleware(withRoute("/v1/images:pull", r.v3Handler.HandleImagePull), authMiddlewareV1, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v1/images:pull/", chainMiddleware(withRoute("/v1/images:pull/", r.v3Handler.HandleImagePullStatus), authMiddlewareV1, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v1/images:load", chainMiddleware(withRoute("/v1/images:load", r.v3Handler.HandleImageLoad), authMiddlewareV1, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v1/images:prune", chainMiddleware(withRoute("/v1/images:prune", r.v3Handler.HandleImagePrune), authMiddlewareV1, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v1/images:remove", chainMiddleware(withRoute("/v1/images:remove", r.v3Handler.HandleImageRemove), authMiddlewareV1, accessLoggingMiddleware, requestIdMiddleware))

	r.mux.HandleFunc("/v1/microservices/config", chainMiddleware(withRoute("/v1/microservices/config", r.v3Handler.HandleMicroserviceConfigSelf), authMiddlewareV1, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v1/microservices/control", r.controlWSHandler.Handle)

	r.mux.HandleFunc("/v1/ms", chainMiddleware(withRoute("/v1/ms", r.v3Handler.HandleMicroservices), authMiddlewareV1, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v1/ms/", chainMiddleware(withRoute("/v1/ms/", r.v3Handler.HandleMicroservices), authMiddlewareV1, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v1/deploy/microservices:apply", chainMiddleware(withRoute("/v1/deploy/microservices:apply", r.v3Handler.HandleDeployMicroservicesApply), authMiddlewareV1, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v1/deploy/microservices:apply/", chainMiddleware(withRoute("/v1/deploy/microservices:apply/", r.v3Handler.HandleDeployMicroservicesApplyStatus), authMiddlewareV1, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v1/deploy/microservices:validate", chainMiddleware(withRoute("/v1/deploy/microservices:validate", r.v3Handler.HandleDeployMicroservicesValidate), authMiddlewareV1, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v1/deploy/microservices", chainMiddleware(withRoute("/v1/deploy/microservices", r.v3Handler.HandleDeployMicroservices), authMiddlewareV1, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v1/deploy/microservices/", chainMiddleware(withRoute("/v1/deploy/microservices/", r.v3Handler.HandleDeployMicroservices), authMiddlewareV1, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v1/deploy/registries:apply", chainMiddleware(withRoute("/v1/deploy/registries:apply", r.v3Handler.HandleDeployRegistriesApply), authMiddlewareV1, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v1/deploy/registries:validate", chainMiddleware(withRoute("/v1/deploy/registries:validate", r.v3Handler.HandleDeployRegistriesValidate), authMiddlewareV1, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v1/deploy/registries", chainMiddleware(withRoute("/v1/deploy/registries", r.v3Handler.HandleDeployRegistries), authMiddlewareV1, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v1/deploy/registries/", chainMiddleware(withRoute("/v1/deploy/registries/", r.v3Handler.HandleDeployRegistries), authMiddlewareV1, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v1/deploy/runtimeclasses:apply", chainMiddleware(withRoute("/v1/deploy/runtimeclasses:apply", r.v3Handler.HandleDeployRuntimeClassesApply), authMiddlewareV1, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v1/deploy/runtimeclasses:apply/", chainMiddleware(withRoute("/v1/deploy/runtimeclasses:apply/", r.v3Handler.HandleDeployRuntimeClassesApplyStatus), authMiddlewareV1, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v1/deploy/runtimeclasses:delete/", chainMiddleware(withRoute("/v1/deploy/runtimeclasses:delete/", r.v3Handler.HandleDeployRuntimeClassesDeleteStatus), authMiddlewareV1, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v1/deploy/runtimeclasses:validate", chainMiddleware(withRoute("/v1/deploy/runtimeclasses:validate", r.v3Handler.HandleDeployRuntimeClassesValidate), authMiddlewareV1, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v1/deploy/runtimeclasses", chainMiddleware(withRoute("/v1/deploy/runtimeclasses", r.v3Handler.HandleDeployRuntimeClasses), authMiddlewareV1, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v1/deploy/runtimeclasses/", chainMiddleware(withRoute("/v1/deploy/runtimeclasses/", r.v3Handler.HandleDeployRuntimeClasses), authMiddlewareV1, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v1/auth/whoami", chainMiddleware(withRoute("/v1/auth/whoami", r.authHandler.HandleWhoAmI), authMiddlewareV1, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v1/auth/tokens", chainMiddleware(withRoute("/v1/auth/tokens", r.v3Handler.HandleAuthTokens), authMiddlewareV1, accessLoggingMiddleware, requestIdMiddleware))
	r.mux.HandleFunc("/v1/auth/tokens/revoke", chainMiddleware(withRoute("/v1/auth/tokens/revoke", r.v3Handler.HandleAuthTokensRevoke), authMiddlewareV1, accessLoggingMiddleware, requestIdMiddleware))
}
