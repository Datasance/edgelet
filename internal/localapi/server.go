package localapi

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/eclipse-iofog/agent/internal/utils/logging"
)

const (
	serverModuleName = "Local API Server"
)

// Server represents the HTTP/WebSocket server
type Server struct {
	httpServer *http.Server
	router     *Router
	port       int
}

// NewServer creates a new Local API server
func NewServer(port int) *Server {
	router := NewRouter()

	addr := fmt.Sprintf(":%d", port)

	return &Server{
		router: router,
		port:   port,
		httpServer: &http.Server{
			Addr:         addr,
			Handler:      router,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
	}
}

// Start starts the HTTP server
func (s *Server) Start() error {
	logging.LogInfo(serverModuleName, "Local api server starting at port: "+s.httpServer.Addr)
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	logging.LogInfo(serverModuleName, "Shutting down local api server")
	return s.httpServer.Shutdown(ctx)
}
