package localapi

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/datasance/edgelet/internal/auth"
	"github.com/datasance/edgelet/internal/utils"
	"github.com/datasance/edgelet/internal/utils/logging"
)

const (
	serverModuleName = "Local API Server"
)

// Server represents the HTTP/WebSocket server
type Server struct {
	httpServer       *http.Server
	unixServer       *http.Server
	unixListener     net.Listener
	tcpListener      net.Listener
	router           *Router
	port             int
	unixSocketPath   string
	enableUnixSocket bool
	tlsCertPath      string
	tlsKeyPath       string
	readyCh          chan struct{}
	readyOnce        sync.Once
}

// NewServer creates a new Local API server
func NewServer(port int) *Server {
	router := NewRouter()

	addr := fmt.Sprintf(":%d", port)
	unixPath := filepath.Join(utils.VarRun, "edgelet.sock")

	return &Server{
		router:           router,
		port:             port,
		unixSocketPath:   unixPath,
		enableUnixSocket: true,
		httpServer: &http.Server{
			Addr:         addr,
			Handler:      router,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
		unixServer: &http.Server{
			Handler:      router,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
		readyCh: make(chan struct{}),
	}
}

// Start starts the HTTP server
func (s *Server) Start() error {
	logging.LogInfo(serverModuleName, "Local api server starting at port: "+s.httpServer.Addr)

	if s.enableUnixSocket {
		if err := s.startUnixSocket(); err != nil {
			return err
		}
		go func() {
			if err := s.unixServer.Serve(s.unixListener); err != nil && !errors.Is(err, http.ErrServerClosed) {
				logging.LogError(serverModuleName, "Unix socket local api listener error", err)
			}
		}()
	}
	_, certPath, keyPath, err := auth.EnsureLocalAPIPKI()
	if err != nil {
		return fmt.Errorf("failed to ensure localapi TLS material: %w", err)
	}
	s.tlsCertPath = certPath
	s.tlsKeyPath = keyPath

	tcpListener, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		return fmt.Errorf("failed to bind local api TCP listener on %s: %w", s.httpServer.Addr, err)
	}
	s.tcpListener = tcpListener

	s.readyOnce.Do(func() { close(s.readyCh) })

	cert, err := tls.LoadX509KeyPair(s.tlsCertPath, s.tlsKeyPath)
	if err != nil {
		_ = s.tcpListener.Close()
		return fmt.Errorf("failed to load localapi TLS keypair: %w", err)
	}

	tlsListener := tls.NewListener(s.tcpListener, &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	})

	if err := s.httpServer.Serve(tlsListener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	logging.LogInfo(serverModuleName, "Shutting down local api server")

	var firstErr error
	if err := s.httpServer.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		firstErr = err
	}
	if s.unixServer != nil {
		if err := s.unixServer.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) && firstErr == nil {
			firstErr = err
		}
	}
	if s.unixListener != nil {
		_ = s.unixListener.Close()
	}
	if s.tcpListener != nil {
		_ = s.tcpListener.Close()
	}
	if s.enableUnixSocket {
		_ = os.Remove(s.unixSocketPath)
	}

	return firstErr
}

// Ready returns a channel closed once all API listeners are bound.
func (s *Server) Ready() <-chan struct{} {
	return s.readyCh
}

func (s *Server) startUnixSocket() error {
	if err := os.MkdirAll(filepath.Dir(s.unixSocketPath), 0755); err != nil { // #nosec G301 -- runtime dir needs traversable permissions
		return fmt.Errorf("failed to create unix socket directory: %w", err)
	}
	_ = os.Remove(s.unixSocketPath)

	listener, err := net.Listen("unix", s.unixSocketPath)
	if err != nil {
		return fmt.Errorf("failed to listen on unix socket %s: %w", s.unixSocketPath, err)
	}
	if chmodErr := os.Chmod(s.unixSocketPath, 0600); chmodErr != nil {
		_ = listener.Close()
		return fmt.Errorf("failed to set unix socket permissions: %w", chmodErr)
	}

	s.unixListener = listener

	logging.LogInfo(serverModuleName, "Local api unix socket listening at: "+s.unixSocketPath)
	return nil
}
