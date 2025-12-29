// Package health provides health check HTTP endpoints for Muti Metroo.
package health

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/pprof"
	"strings"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/coinstash/muti-metroo/internal/identity"
	"github.com/coinstash/muti-metroo/internal/protocol"
)

// StatsProvider provides agent statistics.
type StatsProvider interface {
	// IsRunning returns true if the agent is running.
	IsRunning() bool

	// Stats returns agent statistics.
	Stats() Stats
}

// RemoteMetricsProvider provides the ability to fetch metrics from remote agents.
type RemoteMetricsProvider interface {
	// ID returns the local agent's ID.
	ID() identity.AgentID

	// SendControlRequest sends a control request to a remote agent.
	SendControlRequest(ctx context.Context, targetID identity.AgentID, controlType uint8) (*protocol.ControlResponse, error)

	// GetPeerIDs returns a list of all connected peer IDs.
	GetPeerIDs() []identity.AgentID

	// GetKnownAgentIDs returns a list of all known agent IDs (including remote via routes).
	GetKnownAgentIDs() []identity.AgentID
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
	cfg            ServerConfig
	provider       StatsProvider
	remoteProvider RemoteMetricsProvider
	server         *http.Server
	listener       net.Listener
	running        atomic.Bool
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

	// Prometheus metrics endpoint (local)
	mux.Handle("/metrics", promhttp.Handler())

	// Remote metrics endpoint: /metrics/{agent-id}
	mux.HandleFunc("/metrics/", s.handleRemoteMetrics)

	// Agent status endpoints
	mux.HandleFunc("/agents", s.handleListAgents)
	mux.HandleFunc("/agents/", s.handleAgentInfo)

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

// SetRemoteProvider sets the remote metrics provider.
// This is called after the agent is initialized.
func (s *Server) SetRemoteProvider(provider RemoteMetricsProvider) {
	s.remoteProvider = provider
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

// handleRemoteMetrics handles fetching metrics from remote agents.
// URL format: /metrics/{agent-id}
func (s *Server) handleRemoteMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.remoteProvider == nil {
		http.Error(w, "remote metrics not configured", http.StatusServiceUnavailable)
		return
	}

	// Parse agent ID from path: /metrics/{agent-id}
	path := strings.TrimPrefix(r.URL.Path, "/metrics/")
	if path == "" || path == "/" {
		http.Error(w, "agent ID required: /metrics/{agent-id}", http.StatusBadRequest)
		return
	}

	// Parse the agent ID
	targetID, err := identity.ParseAgentID(path)
	if err != nil {
		http.Error(w, "invalid agent ID format", http.StatusBadRequest)
		return
	}

	// Check if target is local agent
	if targetID == s.remoteProvider.ID() {
		// Redirect to local metrics
		promhttp.Handler().ServeHTTP(w, r)
		return
	}

	// Fetch metrics from remote agent
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	resp, err := s.remoteProvider.SendControlRequest(ctx, targetID, protocol.ControlTypeMetrics)
	if err != nil {
		http.Error(w, "failed to fetch metrics: "+err.Error(), http.StatusBadGateway)
		return
	}

	if !resp.Success {
		http.Error(w, "remote agent error: "+string(resp.Data), http.StatusBadGateway)
		return
	}

	// Return the metrics in Prometheus text format
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(resp.Data)
}

// handleListAgents lists all known agents in the mesh.
func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.remoteProvider == nil {
		http.Error(w, "remote provider not configured", http.StatusServiceUnavailable)
		return
	}

	agents := s.remoteProvider.GetKnownAgentIDs()
	localID := s.remoteProvider.ID()

	// Build response with local agent first
	result := []map[string]interface{}{
		{
			"id":    localID.String(),
			"short": localID.ShortString(),
			"local": true,
		},
	}

	for _, id := range agents {
		if id != localID {
			result = append(result, map[string]interface{}{
				"id":    id.String(),
				"short": id.ShortString(),
				"local": false,
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(result)
}

// handleAgentInfo handles fetching status from a specific agent.
// URL format: /agents/{agent-id} or /agents/{agent-id}/routes or /agents/{agent-id}/peers
func (s *Server) handleAgentInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.remoteProvider == nil {
		http.Error(w, "remote provider not configured", http.StatusServiceUnavailable)
		return
	}

	// Parse path: /agents/{agent-id}[/routes|/peers]
	path := strings.TrimPrefix(r.URL.Path, "/agents/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "agent ID required", http.StatusBadRequest)
		return
	}

	targetID, err := identity.ParseAgentID(parts[0])
	if err != nil {
		http.Error(w, "invalid agent ID format", http.StatusBadRequest)
		return
	}

	// Determine control type
	controlType := protocol.ControlTypeStatus
	if len(parts) > 1 {
		switch parts[1] {
		case "routes":
			controlType = protocol.ControlTypeRoutes
		case "peers":
			controlType = protocol.ControlTypePeers
		case "metrics":
			controlType = protocol.ControlTypeMetrics
		}
	}

	// Fetch from remote agent
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	resp, err := s.remoteProvider.SendControlRequest(ctx, targetID, controlType)
	if err != nil {
		http.Error(w, "failed to fetch: "+err.Error(), http.StatusBadGateway)
		return
	}

	if !resp.Success {
		http.Error(w, "remote agent error: "+string(resp.Data), http.StatusBadGateway)
		return
	}

	// Determine content type based on control type
	if controlType == protocol.ControlTypeMetrics {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	} else {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(http.StatusOK)
	w.Write(resp.Data)
}
