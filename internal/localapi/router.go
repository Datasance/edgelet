package localapi

import (
	"net/http"

	"github.com/eclipse-iofog/agent-go/internal/localapi/handlers"
	"github.com/eclipse-iofog/agent-go/internal/localapi/websocket"
	"github.com/eclipse-iofog/agent-go/internal/utils/logging"
)

const (
	routerModuleName = "Local API Router"
)

// Router handles HTTP routing
type Router struct {
	mux                  *http.ServeMux
	statusHandler        *handlers.StatusHandler
	infoHandler          *handlers.InfoHandler
	versionHandler       *handlers.VersionHandler
	commandHandler       *handlers.CommandHandler
	messageHandler       *handlers.MessageHandler
	gpsHandler           *handlers.GPSHandler
	bluetoothHandler     *handlers.BluetoothHandler
	configHandler        *handlers.ConfigHandler
	provisionHandler     *handlers.ProvisionHandler
	deprovisionHandler   *handlers.DeprovisionHandler
	edgeResourcesHandler *handlers.EdgeResourcesHandler
	logHandler           *handlers.LogHandler
	controlWSHandler     *websocket.ControlHandler
	messageWSHandler     *websocket.MessageHandler
}

// NewRouter creates a new router
func NewRouter() *Router {
	r := &Router{
		mux:                  http.NewServeMux(),
		statusHandler:        &handlers.StatusHandler{},
		infoHandler:          &handlers.InfoHandler{},
		versionHandler:       &handlers.VersionHandler{},
		commandHandler:       &handlers.CommandHandler{},
		messageHandler:       &handlers.MessageHandler{},
		gpsHandler:           handlers.NewGPSHandler(),
		bluetoothHandler:     handlers.NewBluetoothHandler(),
		configHandler:        handlers.NewConfigHandler(),
		provisionHandler:     handlers.NewProvisionHandler(),
		deprovisionHandler:   handlers.NewDeprovisionHandler(),
		edgeResourcesHandler: handlers.NewEdgeResourcesHandler(),
		logHandler:           handlers.NewLogHandler(),
		controlWSHandler:     websocket.NewControlHandler(),
		messageWSHandler:     websocket.NewMessageHandler(),
	}
	r.setupRoutes()
	return r
}

// ServeHTTP implements http.Handler
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// Add logging middleware (generic debug logging)
	logging.LogDebug(routerModuleName, "Handling request: "+req.Method+" "+req.URL.Path)
	r.mux.ServeHTTP(w, req)
}

// withActionLogging wraps a handler with "Start/Finished Processing..." info logs
func withActionLogging(actionName string, handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		// Use "Local API : LocalApiServerHandler" as module name to match Java
		moduleName := "Local API : LocalApiServerHandler"
		logging.LogInfo(moduleName, "Start Processing "+actionName+" request")
		handler(w, req)
		logging.LogInfo(moduleName, "Finished Processing "+actionName+" request")
	}
}

// setupRoutes registers all routes
func (r *Router) setupRoutes() {
	// REST endpoints with authentication (CLI/admin only)
	r.mux.HandleFunc("/v2/status", chainMiddleware(authMiddleware(r.statusHandler.HandleStatus), loggingMiddleware)) // Status doesn't have specific log in Java snippet?
	r.mux.HandleFunc("/v2/info", chainMiddleware(authMiddleware(r.infoHandler.HandleInfo), loggingMiddleware)) // Info neither?
	r.mux.HandleFunc("/v2/version", chainMiddleware(authMiddleware(r.versionHandler.HandleVersion), loggingMiddleware))
	
	// CommandLine endpoint needs specific logging
	r.mux.HandleFunc("/v2/commandline", chainMiddleware(authMiddleware(withActionLogging("commandline", r.commandHandler.HandleCommandLine)), loggingMiddleware))

	// Message endpoints WITHOUT authentication (microservices access these)
	// Matching Java: these handlers don't call validateAccessToken()
	r.mux.HandleFunc("/v2/messages/next", chainMiddleware(r.messageHandler.HandleMessagesNext, loggingMiddleware))
	r.mux.HandleFunc("/v2/messages/new", chainMiddleware(r.messageHandler.HandleMessagesNew, loggingMiddleware))
	r.mux.HandleFunc("/v2/messages/query", chainMiddleware(r.messageHandler.HandleMessagesQuery, loggingMiddleware))

	// Config endpoints
	// /v2/config/get WITHOUT authentication
	r.mux.HandleFunc("/v2/config/get", chainMiddleware(r.configHandler.HandleConfigGet, loggingMiddleware))
	// /v2/config WITH authentication
	r.mux.HandleFunc("/v2/config", chainMiddleware(authMiddleware(r.configHandler.HandleConfigSet), loggingMiddleware))

	// WebSocket endpoints (no auth middleware needed, handled in handler)
	r.mux.HandleFunc("/v2/control/socket/", r.controlWSHandler.Handle)
	r.mux.HandleFunc("/v2/message/socket/", r.messageWSHandler.Handle)

	// Provision/Deprovision endpoints with authentication
	r.mux.HandleFunc("/v2/provision", chainMiddleware(authMiddleware(r.provisionHandler.HandleProvision), loggingMiddleware))
	r.mux.HandleFunc("/v2/deprovision", chainMiddleware(authMiddleware(r.deprovisionHandler.HandleDeprovision), loggingMiddleware))

	// Edge resources with authentication
	r.mux.HandleFunc("/v2/edgeResources", chainMiddleware(authMiddleware(r.edgeResourcesHandler.HandleEdgeResources), loggingMiddleware))

	// Log endpoint WITHOUT authentication (microservices access this)
	// Matching Java: LogApiHandler doesn't call validateAccessToken()
	r.mux.HandleFunc("/v2/log", chainMiddleware(withActionLogging("log", r.logHandler.HandleLog), loggingMiddleware))

	// GPS endpoint
	r.mux.HandleFunc("/v2/gps", chainMiddleware(authMiddleware(withActionLogging("gps", r.gpsHandler.HandleGetGPS)), loggingMiddleware))

	// Bluetooth endpoint
	r.mux.HandleFunc("/v2/bluetooth", chainMiddleware(authMiddleware(withActionLogging("restblue", r.bluetoothHandler.HandleGetBluetooth)), loggingMiddleware))
}

