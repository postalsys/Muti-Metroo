// Package health provides health check HTTP endpoints for Muti Metroo.
package health

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/pprof"
	"sync/atomic"
	"time"
)

// StatsProvider provides agent statistics.
type StatsProvider interface {
	// IsRunning returns true if the agent is running.
	IsRunning() bool

	// Stats returns agent statistics.
	Stats() Stats
}

// Stats contains agent health statistics.
type Stats struct {
	PeerCount      int  `json:"peer_count"`
	StreamCount    int  `json:"stream_count"`
	RouteCount     int  `json:"route_count"`
	SOCKS5Running  bool `json:"socks5_running"`
	ExitHandlerRun bool `json:"exit_handler_running"`
}

// ServerConfig contains health server configuration.
type ServerConfig struct {
	// Address to listen on (e.g., ":8080")
	Address string

	// ReadTimeout for HTTP reads
	ReadTimeout time.Duration

	// WriteTimeout for HTTP writes
	WriteTimeout time.Duration
}

// DefaultServerConfig returns sensible defaults.
func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		Address:      ":8080",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
}

// Server is an HTTP server for health check endpoints.
type Server struct {
	cfg      ServerConfig
	provider StatsProvider
	server   *http.Server
	listener net.Listener
	running  atomic.Bool
}

// NewServer creates a new health check server.
func NewServer(cfg ServerConfig, provider StatsProvider) *Server {
	s := &Server{
		cfg:      cfg,
		provider: provider,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/ready", s.handleReady)

	// pprof debug endpoints
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	s.server = &http.Server{
		Addr:         cfg.Address,
		Handler:      mux,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}

	return s
}

// Start starts the health check server.
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.cfg.Address)
	if err != nil {
		return err
	}
	s.listener = ln
	s.running.Store(true)

	go s.server.Serve(ln)

	return nil
}

// Stop stops the health check server.
func (s *Server) Stop() error {
	if !s.running.Swap(false) {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return s.server.Shutdown(ctx)
}

// Address returns the server's listen address.
func (s *Server) Address() net.Addr {
	if s.listener != nil {
		return s.listener.Addr()
	}
	return nil
}

// IsRunning returns true if the server is running.
func (s *Server) IsRunning() bool {
	return s.running.Load()
}

// handleHealth handles the basic health check endpoint.
// Returns 200 if the server is responding.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK\n"))
}

// handleHealthz handles the detailed health check endpoint.
// Returns 200 with JSON stats if healthy, 503 if not running.
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.provider == nil || !s.provider.IsRunning() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "unavailable",
			"running": false,
		})
		return
	}

	stats := s.provider.Stats()
	response := map[string]interface{}{
		"status":               "healthy",
		"running":              true,
		"peer_count":           stats.PeerCount,
		"stream_count":         stats.StreamCount,
		"route_count":          stats.RouteCount,
		"socks5_running":       stats.SOCKS5Running,
		"exit_handler_running": stats.ExitHandlerRun,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// handleReady handles the readiness probe endpoint.
// Returns 200 if the agent is ready to serve traffic.
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.provider == nil || !s.provider.IsRunning() {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("NOT READY\n"))
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("READY\n"))
}

// Handler returns the HTTP handler for embedding in other servers.
func (s *Server) Handler() http.Handler {
	return s.server.Handler
}
