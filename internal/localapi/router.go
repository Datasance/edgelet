package localapi

import (
	"net/http"

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
	commandHandler     *handlers.CommandHandler
	gpsHandler         *handlers.GPSHandler
	bluetoothHandler   *handlers.BluetoothHandler
	configHandler      *handlers.ConfigHandler
	provisionHandler   *handlers.ProvisionHandler
	deprovisionHandler *handlers.DeprovisionHandler
	logHandler         *handlers.LogHandler
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
		commandHandler:     &handlers.CommandHandler{},
		gpsHandler:         handlers.NewGPSHandler(),
		bluetoothHandler:   handlers.NewBluetoothHandler(),
		configHandler:      handlers.NewConfigHandler(),
		provisionHandler:   handlers.NewProvisionHandler(),
		deprovisionHandler: handlers.NewDeprovisionHandler(),
		logHandler:         handlers.NewLogHandler(),
		controlWSHandler:   websocket.NewControlHandler(),
	}
	r.setupRoutes()
	return r
}

// ServeHTTP implements http.Handler
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	logging.LogDebug(routerModuleName, "Handling request: "+req.Method+" "+req.URL.Path)
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

	// REST endpoints with authentication (CLI/admin only)
	r.mux.HandleFunc("/v2/status", chainMiddleware(authMiddleware(r.statusHandler.HandleStatus), loggingMiddleware))
	r.mux.HandleFunc("/v2/info", chainMiddleware(authMiddleware(r.infoHandler.HandleInfo), loggingMiddleware))
	r.mux.HandleFunc("/v2/version", chainMiddleware(authMiddleware(r.versionHandler.HandleVersion), loggingMiddleware))

	// CommandLine endpoint needs specific logging
	r.mux.HandleFunc("/v2/commandline", chainMiddleware(authMiddleware(withActionLogging("commandline", r.commandHandler.HandleCommandLine)), loggingMiddleware))

	// Config endpoints
	// /v2/config/get WITHOUT authentication
	r.mux.HandleFunc("/v2/config/get", chainMiddleware(r.configHandler.HandleConfigGet, loggingMiddleware))
	// /v2/config WITH authentication
	r.mux.HandleFunc("/v2/config", chainMiddleware(authMiddleware(r.configHandler.HandleConfigSet), loggingMiddleware))

	// WebSocket endpoint for control channel (no auth middleware needed, handled in handler)
	r.mux.HandleFunc("/v2/control/socket/", r.controlWSHandler.Handle)

	// Provision/Deprovision endpoints with authentication
	r.mux.HandleFunc("/v2/provision", chainMiddleware(authMiddleware(r.provisionHandler.HandleProvision), loggingMiddleware))
	r.mux.HandleFunc("/v2/deprovision", chainMiddleware(authMiddleware(r.deprovisionHandler.HandleDeprovision), loggingMiddleware))

	// Log endpoint WITHOUT authentication (microservices access this)
	r.mux.HandleFunc("/v2/log", chainMiddleware(withActionLogging("log", r.logHandler.HandleLog), loggingMiddleware))

	// GPS endpoint
	r.mux.HandleFunc("/v2/gps", chainMiddleware(authMiddleware(withActionLogging("gps", r.gpsHandler.HandleGetGPS)), loggingMiddleware))

	// Bluetooth endpoint
	r.mux.HandleFunc("/v2/bluetooth", chainMiddleware(authMiddleware(withActionLogging("restblue", r.bluetoothHandler.HandleGetBluetooth)), loggingMiddleware))
}
