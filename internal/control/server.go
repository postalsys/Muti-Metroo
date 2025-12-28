// Package control provides a Unix socket control interface for Muti Metroo.
package control

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/coinstash/muti-metroo/internal/identity"
)

// AgentInfo provides agent information for the control interface.
type AgentInfo interface {
	// ID returns the agent's identity.
	ID() identity.AgentID

	// IsRunning returns true if the agent is running.
	IsRunning() bool

	// GetPeerIDs returns all connected peer IDs.
	GetPeerIDs() []identity.AgentID

	// GetRouteInfo returns route information.
	GetRouteInfo() []RouteInfo
}

// RouteInfo contains route information for display.
type RouteInfo struct {
	Network     string `json:"network"`
	NextHop     string `json:"next_hop"`
	Origin      string `json:"origin"`
	Metric      int    `json:"metric"`
	HopCount    int    `json:"hop_count"`
}

// StatusResponse is the response for the status endpoint.
type StatusResponse struct {
	AgentID     string `json:"agent_id"`
	Running     bool   `json:"running"`
	PeerCount   int    `json:"peer_count"`
	RouteCount  int    `json:"route_count"`
}

// PeersResponse is the response for the peers endpoint.
type PeersResponse struct {
	Peers []string `json:"peers"`
}

// RoutesResponse is the response for the routes endpoint.
type RoutesResponse struct {
	Routes []RouteInfo `json:"routes"`
}

// ServerConfig contains control server configuration.
type ServerConfig struct {
	// SocketPath is the path to the Unix socket file.
	SocketPath string

	// ReadTimeout for HTTP reads.
	ReadTimeout time.Duration

	// WriteTimeout for HTTP writes.
	WriteTimeout time.Duration
}

// DefaultServerConfig returns sensible defaults.
func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		SocketPath:   "./data/control.sock",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
}

// Server is a Unix socket HTTP server for control commands.
type Server struct {
	cfg      ServerConfig
	agent    AgentInfo
	server   *http.Server
	listener net.Listener
	running  atomic.Bool
}

// NewServer creates a new control server.
func NewServer(cfg ServerConfig, agent AgentInfo) *Server {
	s := &Server{
		cfg:   cfg,
		agent: agent,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/status", s.handleStatus)
	mux.HandleFunc("/peers", s.handlePeers)
	mux.HandleFunc("/routes", s.handleRoutes)

	s.server = &http.Server{
		Handler:      mux,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}

	return s
}

// Start starts the control server.
func (s *Server) Start() error {
	// Remove existing socket file if it exists
	if err := os.Remove(s.cfg.SocketPath); err != nil && !os.IsNotExist(err) {
		return err
	}

	ln, err := net.Listen("unix", s.cfg.SocketPath)
	if err != nil {
		return err
	}
	s.listener = ln
	s.running.Store(true)

	go s.server.Serve(ln)

	return nil
}

// Stop stops the control server.
func (s *Server) Stop() error {
	if !s.running.Swap(false) {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Shutdown server
	if err := s.server.Shutdown(ctx); err != nil {
		return err
	}

	// Remove socket file
	if err := os.Remove(s.cfg.SocketPath); err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

// IsRunning returns true if the server is running.
func (s *Server) IsRunning() bool {
	return s.running.Load()
}

// SocketPath returns the socket path.
func (s *Server) SocketPath() string {
	return s.cfg.SocketPath
}

// handleStatus handles the status endpoint.
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	response := StatusResponse{
		AgentID:    s.agent.ID().ShortString(),
		Running:    s.agent.IsRunning(),
		PeerCount:  len(s.agent.GetPeerIDs()),
		RouteCount: len(s.agent.GetRouteInfo()),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handlePeers handles the peers endpoint.
func (s *Server) handlePeers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	peerIDs := s.agent.GetPeerIDs()
	peers := make([]string, len(peerIDs))
	for i, id := range peerIDs {
		peers[i] = id.ShortString()
	}

	response := PeersResponse{
		Peers: peers,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleRoutes handles the routes endpoint.
func (s *Server) handleRoutes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	response := RoutesResponse{
		Routes: s.agent.GetRouteInfo(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
