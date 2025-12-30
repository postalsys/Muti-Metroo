// Package health provides health check HTTP endpoints for Muti Metroo.
package health

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/postalsys/muti-metroo/internal/identity"
	"github.com/postalsys/muti-metroo/internal/protocol"
	"github.com/postalsys/muti-metroo/internal/webui"
)

// StatsProvider provides agent statistics.
type StatsProvider interface {
	// IsRunning returns true if the agent is running.
	IsRunning() bool

	// Stats returns agent statistics.
	Stats() Stats
}

// FileTransferProgress is a callback for reporting file transfer progress.
type FileTransferProgress func(bytesTransferred, totalBytes int64)

// RemoteMetricsProvider provides the ability to fetch metrics from remote agents.
type RemoteMetricsProvider interface {
	// ID returns the local agent's ID.
	ID() identity.AgentID

	// DisplayName returns the local agent's display name, or falls back to short ID.
	DisplayName() string

	// SendControlRequest sends a control request to a remote agent.
	SendControlRequest(ctx context.Context, targetID identity.AgentID, controlType uint8) (*protocol.ControlResponse, error)

	// SendControlRequestWithData sends a control request with data payload to a remote agent.
	SendControlRequestWithData(ctx context.Context, targetID identity.AgentID, controlType uint8, data []byte) (*protocol.ControlResponse, error)

	// GetPeerIDs returns a list of all connected peer IDs.
	GetPeerIDs() []identity.AgentID

	// GetKnownAgentIDs returns a list of all known agent IDs (including remote via routes).
	GetKnownAgentIDs() []identity.AgentID

	// GetPeerDetails returns detailed information about connected peers for the dashboard.
	GetPeerDetails() []PeerDetails

	// GetRouteDetails returns detailed route information for the dashboard.
	GetRouteDetails() []RouteDetails

	// GetAllDisplayNames returns display names for all known agents (from route advertisements).
	GetAllDisplayNames() map[identity.AgentID]string

	// GetAllNodeInfo returns node info for all known agents.
	GetAllNodeInfo() map[identity.AgentID]*protocol.NodeInfo

	// GetLocalNodeInfo returns local node info.
	GetLocalNodeInfo() *protocol.NodeInfo

	// UploadFile uploads a file or directory to a remote agent via stream-based transfer.
	// localPath is the local file/directory path, remotePath is the destination on the remote agent.
	UploadFile(ctx context.Context, targetID identity.AgentID, localPath, remotePath string, password string, progress FileTransferProgress) error

	// DownloadFile downloads a file or directory from a remote agent via stream-based transfer.
	// remotePath is the path on the remote agent, localPath is the local destination.
	DownloadFile(ctx context.Context, targetID identity.AgentID, remotePath, localPath string, password string, progress FileTransferProgress) error
}

// PeerDetails contains detailed information about a connected peer.
type PeerDetails struct {
	ID          identity.AgentID
	DisplayName string
	State       string
	RTT         time.Duration
	IsDialer    bool
	Transport   string // Transport type: "quic", "h2", "ws"
}

// RouteDetails contains detailed route information.
type RouteDetails struct {
	Network  string
	NextHop  identity.AgentID
	Origin   identity.AgentID
	Metric   int
	HopCount int
	Path     []identity.AgentID // Full path from local to origin
}

// RouteAdvertiseTrigger provides the ability to trigger immediate route advertisement.
type RouteAdvertiseTrigger interface {
	// TriggerRouteAdvertise triggers an immediate route advertisement.
	TriggerRouteAdvertise()
}

// Stats contains agent health statistics.
type Stats struct {
	PeerCount      int  `json:"peer_count"`
	StreamCount    int  `json:"stream_count"`
	RouteCount     int  `json:"route_count"`
	SOCKS5Running  bool `json:"socks5_running"`
	ExitHandlerRun bool `json:"exit_handler_running"`
}

// TopologyAgentInfo contains information about an agent for the topology API.
type TopologyAgentInfo struct {
	ID          string   `json:"id"`
	ShortID     string   `json:"short_id"`
	DisplayName string   `json:"display_name"`
	IsLocal     bool     `json:"is_local"`
	IsConnected bool     `json:"is_connected"`
	Hostname    string   `json:"hostname,omitempty"`
	OS          string   `json:"os,omitempty"`
	Arch        string   `json:"arch,omitempty"`
	Version     string   `json:"version,omitempty"`
	UptimeHours float64  `json:"uptime_hours,omitempty"`
	IPAddresses []string `json:"ip_addresses,omitempty"`
}

// TopologyConnection represents a connection between two agents.
type TopologyConnection struct {
	FromAgent string `json:"from_agent"`
	ToAgent   string `json:"to_agent"`
	IsDirect  bool   `json:"is_direct"`
	RTTMs     int64  `json:"rtt_ms,omitempty"`
	Transport string `json:"transport,omitempty"` // Transport type for direct connections: "quic", "h2", "ws"
}

// TopologyResponse is the response for the /api/topology endpoint.
type TopologyResponse struct {
	LocalAgent  TopologyAgentInfo    `json:"local_agent"`
	Agents      []TopologyAgentInfo  `json:"agents"`
	Connections []TopologyConnection `json:"connections"`
}

// DashboardPeerInfo contains information about a connected peer.
type DashboardPeerInfo struct {
	ID          string `json:"id"`
	ShortID     string `json:"short_id"`
	DisplayName string `json:"display_name"`
	State       string `json:"state"`
	RTTMs       int64  `json:"rtt_ms"`
	IsDialer    bool   `json:"is_dialer"`
}

// DashboardRouteInfo contains information about a route.
type DashboardRouteInfo struct {
	Network     string   `json:"network"`
	Origin      string   `json:"origin"`        // Display name of origin
	OriginID    string   `json:"origin_id"`     // Short ID of origin
	HopCount    int      `json:"hop_count"`
	PathDisplay []string `json:"path_display"`  // Display names: [local, peer1, ..., origin]
}

// DashboardResponse is the response for the /api/dashboard endpoint.
type DashboardResponse struct {
	Agent  TopologyAgentInfo   `json:"agent"`
	Stats  Stats               `json:"stats"`
	Peers  []DashboardPeerInfo `json:"peers"`
	Routes []DashboardRouteInfo `json:"routes"`
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
	routeTrigger   RouteAdvertiseTrigger
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

	// RPC endpoint: POST /agents/{agent-id}/rpc
	// Note: The /agents/ handler will also handle RPC by checking for "rpc" suffix

	// Route advertisement trigger endpoint
	mux.HandleFunc("/routes/advertise", s.handleTriggerAdvertise)

	// Dashboard API endpoints
	mux.HandleFunc("/api/topology", s.handleTopology)
	mux.HandleFunc("/api/dashboard", s.handleDashboard)
	mux.HandleFunc("/api/nodes", s.handleNodes)

	// Web UI static files
	uiHandler := webui.Handler()
	mux.HandleFunc("/ui/", func(w http.ResponseWriter, r *http.Request) {
		// Strip /ui prefix
		path := strings.TrimPrefix(r.URL.Path, "/ui")
		if path == "" || path == "/" {
			path = "/index.html"
		}
		r.URL.Path = path
		uiHandler.ServeHTTP(w, r)
	})
	// Redirect /ui to /ui/
	mux.HandleFunc("/ui", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ui/", http.StatusMovedPermanently)
	})

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

// SetRouteAdvertiseTrigger sets the route advertisement trigger.
// This is called after the agent is initialized.
func (s *Server) SetRouteAdvertiseTrigger(trigger RouteAdvertiseTrigger) {
	s.routeTrigger = trigger
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
			"id":           localID.String(),
			"short":        localID.ShortString(),
			"display_name": s.remoteProvider.DisplayName(),
			"local":        true,
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
// For RPC: POST /agents/{agent-id}/rpc
func (s *Server) handleAgentInfo(w http.ResponseWriter, r *http.Request) {
	if s.remoteProvider == nil {
		http.Error(w, "remote provider not configured", http.StatusServiceUnavailable)
		return
	}

	// Parse path: /agents/{agent-id}[/routes|/peers|/rpc]
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

	// Check if this is an RPC request
	if len(parts) > 1 && parts[1] == "rpc" {
		s.handleRPC(w, r, targetID)
		return
	}

	// Check if this is a file transfer request
	if len(parts) > 1 && strings.HasPrefix(parts[1], "file/") {
		filePath := strings.TrimPrefix(parts[1], "file/")
		switch filePath {
		case "upload":
			s.handleFileUpload(w, r, targetID)
			return
		case "download":
			s.handleFileDownload(w, r, targetID)
			return
		}
	}

	// For non-RPC requests, only allow GET
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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

// handleRPC handles Remote Procedure Call requests.
// POST /agents/{agent-id}/rpc
// Request body: JSON with command, args, stdin (base64), timeout, and password
// Response: JSON with exit_code, stdout (base64), stderr (base64), and error
func (s *Server) handleRPC(w http.ResponseWriter, r *http.Request, targetID identity.AgentID) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed - use POST", http.StatusMethodNotAllowed)
		return
	}

	// Read request body
	body, err := io.ReadAll(io.LimitReader(r.Body, 2*1024*1024)) // 2MB limit
	if err != nil {
		http.Error(w, "failed to read request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Validate JSON structure
	var reqData map[string]interface{}
	if err := json.Unmarshal(body, &reqData); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Ensure command is specified
	if _, ok := reqData["command"]; !ok {
		http.Error(w, "missing required field: command", http.StatusBadRequest)
		return
	}

	// Send RPC request to target agent
	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second) // 2 minute timeout for RPC
	defer cancel()

	resp, err := s.remoteProvider.SendControlRequestWithData(ctx, targetID, protocol.ControlTypeRPC, body)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"exit_code": -1,
			"error":     "failed to send request: " + err.Error(),
		})
		return
	}

	if !resp.Success {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"exit_code": -1,
			"error":     string(resp.Data),
		})
		return
	}

	// Return the RPC response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(resp.Data)
}

// handleFileUpload handles file upload requests for files and directories.
// POST /agents/{agent-id}/file/upload
// Content-Type: multipart/form-data
// Form fields:
//   - file: the file to upload (can be a tar archive for directories)
//   - path: remote destination path (required)
//   - password: authentication password (optional)
//   - directory: "true" if uploading a directory tar (optional)
//
// Response: JSON with success, error, bytes_written
func (s *Server) handleFileUpload(w http.ResponseWriter, r *http.Request, targetID identity.AgentID) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed - use POST", http.StatusMethodNotAllowed)
		return
	}

	// Parse multipart form (max 32MB in memory, rest goes to temp files)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "failed to parse multipart form: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Get remote path
	remotePath := r.FormValue("path")
	if remotePath == "" {
		http.Error(w, "missing required field: path", http.StatusBadRequest)
		return
	}

	password := r.FormValue("password")
	isDirectory := r.FormValue("directory") == "true"

	// Get uploaded file
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "failed to get uploaded file: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Create temp file to store upload
	tmpFile, err := os.CreateTemp("", "upload-*")
	if err != nil {
		http.Error(w, "failed to create temp file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath) // Clean up temp file

	// Copy uploaded file to temp
	bytesReceived, err := io.Copy(tmpFile, file)
	tmpFile.Close()
	if err != nil {
		http.Error(w, "failed to save uploaded file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// For directories, extract the tar first
	localPath := tmpPath
	if isDirectory {
		// Create temp directory for extraction
		tmpDir, err := os.MkdirTemp("", "upload-dir-*")
		if err != nil {
			http.Error(w, "failed to create temp directory: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer os.RemoveAll(tmpDir)

		// Open the tar file
		tarFile, err := os.Open(tmpPath)
		if err != nil {
			http.Error(w, "failed to open tar file: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Try to extract (handles gzip internally)
		if err := extractTarWithFallback(tarFile, tmpDir); err != nil {
			tarFile.Close()
			http.Error(w, "failed to extract tar: "+err.Error(), http.StatusBadRequest)
			return
		}
		tarFile.Close()
		localPath = tmpDir
	}

	// Perform stream-based upload
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute) // 30 minute timeout for large files
	defer cancel()

	err = s.remoteProvider.UploadFile(ctx, targetID, localPath, remotePath, password, nil)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":       true,
		"bytes_written": bytesReceived,
		"filename":      header.Filename,
		"remote_path":   remotePath,
	})
}

// handleFileDownload handles file download requests for files and directories.
// POST /agents/{agent-id}/file/download
// Request body: JSON with path and optional password
// Response: Binary file data with Content-Disposition header, or tar.gz for directories
func (s *Server) handleFileDownload(w http.ResponseWriter, r *http.Request, targetID identity.AgentID) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed - use POST", http.StatusMethodNotAllowed)
		return
	}

	// Read request body
	body, err := io.ReadAll(io.LimitReader(r.Body, 1*1024*1024)) // 1MB limit for request
	if err != nil {
		http.Error(w, "failed to read request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Parse request
	var req struct {
		Path     string `json:"path"`
		Password string `json:"password,omitempty"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.Path == "" {
		http.Error(w, "missing required field: path", http.StatusBadRequest)
		return
	}

	// Create temp file/directory for download
	tmpDir, err := os.MkdirTemp("", "download-*")
	if err != nil {
		http.Error(w, "failed to create temp directory: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(tmpDir)

	// Use the basename of the remote path as local filename
	localName := filepath.Base(req.Path)
	if localName == "" || localName == "." || localName == "/" {
		localName = "download"
	}
	localPath := filepath.Join(tmpDir, localName)

	// Perform stream-based download
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute) // 30 minute timeout
	defer cancel()

	err = s.remoteProvider.DownloadFile(ctx, targetID, req.Path, localPath, req.Password, nil)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	// Check if it's a file or directory
	info, err := os.Stat(localPath)
	if err != nil {
		http.Error(w, "failed to stat downloaded file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if info.IsDir() {
		// For directories, create tar.gz and stream back
		w.Header().Set("Content-Type", "application/gzip")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", localName+".tar.gz"))

		// Stream tar.gz directly to response
		gzw := gzip.NewWriter(w)
		defer gzw.Close()

		tw := tar.NewWriter(gzw)
		defer tw.Close()

		// Walk the directory and add files to tar
		filepath.Walk(localPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			// Get relative path
			relPath, err := filepath.Rel(localPath, path)
			if err != nil {
				return err
			}
			if relPath == "." {
				return nil
			}

			// Create tar header
			header, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return err
			}
			header.Name = filepath.ToSlash(relPath)

			// Handle symlinks
			if info.Mode()&os.ModeSymlink != 0 {
				link, err := os.Readlink(path)
				if err != nil {
					return err
				}
				header.Linkname = link
			}

			if err := tw.WriteHeader(header); err != nil {
				return err
			}

			// Write file content
			if info.Mode().IsRegular() {
				f, err := os.Open(path)
				if err != nil {
					return err
				}
				defer f.Close()
				io.Copy(tw, f)
			}

			return nil
		})
	} else {
		// For files, stream directly
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", localName))
		w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))
		w.Header().Set("X-File-Mode", fmt.Sprintf("%04o", info.Mode().Perm()))

		f, err := os.Open(localPath)
		if err != nil {
			http.Error(w, "failed to open downloaded file: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer f.Close()

		io.Copy(w, f)
	}
}

// extractTarWithFallback tries to extract a tar archive, handling both plain tar and gzip.
func extractTarWithFallback(r io.Reader, destDir string) error {
	// Try reading first few bytes to detect gzip
	buf := make([]byte, 2)
	n, err := r.Read(buf)
	if err != nil && err != io.EOF {
		return err
	}

	// Create a reader that includes the bytes we already read
	var reader io.Reader
	if n > 0 {
		reader = io.MultiReader(bytes.NewReader(buf[:n]), r)
	} else {
		reader = r
	}

	// Check for gzip magic number
	if n >= 2 && buf[0] == 0x1f && buf[1] == 0x8b {
		gzr, err := gzip.NewReader(reader)
		if err != nil {
			return fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gzr.Close()
		reader = gzr
	}

	// Create tar reader
	tr := tar.NewReader(reader)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar header: %w", err)
		}

		targetPath := filepath.Join(destDir, header.Name)

		// Security check
		if !strings.HasPrefix(filepath.Clean(targetPath), filepath.Clean(destDir)) {
			return fmt.Errorf("tar entry attempts path traversal: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, os.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return err
			}
			f, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return err
			}
			os.Remove(targetPath)
			if err := os.Symlink(header.Linkname, targetPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// handleTriggerAdvertise handles POST /routes/advertise to trigger immediate route advertisement.
func (s *Server) handleTriggerAdvertise(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.routeTrigger == nil {
		http.Error(w, "route trigger not configured", http.StatusServiceUnavailable)
		return
	}

	s.routeTrigger.TriggerRouteAdvertise()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "triggered",
		"message": "route advertisement triggered",
	})
}

// handleTopology handles GET /api/topology for the metro map visualization.
func (s *Server) handleTopology(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.remoteProvider == nil {
		http.Error(w, "provider not configured", http.StatusServiceUnavailable)
		return
	}

	localID := s.remoteProvider.ID()
	localName := s.remoteProvider.DisplayName()

	// Get all known display names from route advertisements
	displayNames := s.remoteProvider.GetAllDisplayNames()

	// Get all known node info
	allNodeInfo := s.remoteProvider.GetAllNodeInfo()
	localNodeInfo := s.remoteProvider.GetLocalNodeInfo()

	// Build local agent info
	localAgent := TopologyAgentInfo{
		ID:          localID.String(),
		ShortID:     localID.ShortString(),
		DisplayName: localName,
		IsLocal:     true,
		IsConnected: true,
	}
	// Populate local node info if available
	if localNodeInfo != nil {
		localAgent.Hostname = localNodeInfo.Hostname
		localAgent.OS = localNodeInfo.OS
		localAgent.Arch = localNodeInfo.Arch
		localAgent.Version = localNodeInfo.Version
		localAgent.IPAddresses = localNodeInfo.IPAddresses
		if localNodeInfo.StartTime > 0 {
			localAgent.UptimeHours = float64(time.Now().Unix()-localNodeInfo.StartTime) / 3600.0
		}
	}

	// Track all agents and connections
	agentMap := make(map[string]TopologyAgentInfo)
	agentMap[localID.String()] = localAgent

	// Build set of direct peers
	peerIDs := s.remoteProvider.GetPeerIDs()
	peerSet := make(map[identity.AgentID]bool)
	for _, id := range peerIDs {
		peerSet[id] = true
	}

	// Track unique connections (from -> to)
	connectionSet := make(map[string]TopologyConnection)

	// Add direct peer connections
	peerDetails := s.remoteProvider.GetPeerDetails()
	for _, peer := range peerDetails {
		// Add peer to agent map
		displayName := peer.DisplayName
		if displayName == "" {
			displayName = peer.ID.ShortString()
		}
		if _, exists := agentMap[peer.ID.String()]; !exists {
			agentMap[peer.ID.String()] = TopologyAgentInfo{
				ID:          peer.ID.String(),
				ShortID:     peer.ID.ShortString(),
				DisplayName: displayName,
				IsLocal:     false,
				IsConnected: true,
			}
		}
		// Add direct connection
		connKey := localID.ShortString() + "->" + peer.ID.ShortString()
		connectionSet[connKey] = TopologyConnection{
			FromAgent: localID.ShortString(),
			ToAgent:   peer.ID.ShortString(),
			IsDirect:  true,
			RTTMs:     peer.RTT.Milliseconds(),
			Transport: peer.Transport,
		}
	}

	// Extract agents and connections from route paths
	routeDetails := s.remoteProvider.GetRouteDetails()
	for _, route := range routeDetails {
		// Add all agents in the path
		for _, agentID := range route.Path {
			if _, exists := agentMap[agentID.String()]; !exists {
				// Look up display name from route advertisements
				displayName := displayNames[agentID]
				if displayName == "" {
					displayName = agentID.ShortString()
				}
				agentMap[agentID.String()] = TopologyAgentInfo{
					ID:          agentID.String(),
					ShortID:     agentID.ShortString(),
					DisplayName: displayName,
					IsLocal:     false,
					IsConnected: peerSet[agentID],
				}
			}
		}

		// Build connections between consecutive agents in path
		// Path is ordered from local toward origin
		for i := 0; i < len(route.Path)-1; i++ {
			fromID := route.Path[i]
			toID := route.Path[i+1]
			connKey := fromID.ShortString() + "->" + toID.ShortString()
			if _, exists := connectionSet[connKey]; !exists {
				conn := TopologyConnection{
					FromAgent: fromID.ShortString(),
					ToAgent:   toID.ShortString(),
					IsDirect:  false,
				}
				// Try to find transport/RTT from the "from" agent's peer info
				if fromNodeInfo, ok := allNodeInfo[fromID]; ok {
					for _, peer := range fromNodeInfo.Peers {
						var peerAgentID identity.AgentID
						copy(peerAgentID[:], peer.PeerID[:])
						if peerAgentID == toID {
							conn.Transport = peer.Transport
							conn.RTTMs = peer.RTTMs
							break
						}
					}
				}
				connectionSet[connKey] = conn
			}
		}
	}

	// Populate node info for all agents in the map
	for agentID, nodeInfo := range allNodeInfo {
		if existing, ok := agentMap[agentID.String()]; ok {
			existing.Hostname = nodeInfo.Hostname
			existing.OS = nodeInfo.OS
			existing.Arch = nodeInfo.Arch
			existing.Version = nodeInfo.Version
			existing.IPAddresses = nodeInfo.IPAddresses
			if nodeInfo.StartTime > 0 {
				existing.UptimeHours = float64(time.Now().Unix()-nodeInfo.StartTime) / 3600.0
			}
			// Update display name from node info if not already set
			if existing.DisplayName == existing.ShortID && nodeInfo.DisplayName != "" {
				existing.DisplayName = nodeInfo.DisplayName
			}
			agentMap[agentID.String()] = existing
		}
	}

	// Convert maps to slices
	agents := make([]TopologyAgentInfo, 0, len(agentMap))
	for _, agent := range agentMap {
		agents = append(agents, agent)
	}

	connections := make([]TopologyConnection, 0, len(connectionSet))
	for _, conn := range connectionSet {
		connections = append(connections, conn)
	}

	response := TopologyResponse{
		LocalAgent:  localAgent,
		Agents:      agents,
		Connections: connections,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// handleDashboard handles GET /api/dashboard for the dashboard overview.
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.remoteProvider == nil || s.provider == nil {
		http.Error(w, "provider not configured", http.StatusServiceUnavailable)
		return
	}

	localID := s.remoteProvider.ID()
	localName := s.remoteProvider.DisplayName()
	stats := s.provider.Stats()

	// Build peer info
	peers := []DashboardPeerInfo{}
	peerDetails := s.remoteProvider.GetPeerDetails()
	for _, peer := range peerDetails {
		peers = append(peers, DashboardPeerInfo{
			ID:          peer.ID.String(),
			ShortID:     peer.ID.ShortString(),
			DisplayName: peer.DisplayName,
			State:       peer.State,
			RTTMs:       peer.RTT.Milliseconds(),
			IsDialer:    peer.IsDialer,
		})
	}

	// Get display names for building path display
	displayNames := s.remoteProvider.GetAllDisplayNames()

	// Helper to get display name or fall back to short ID
	getDisplayName := func(id identity.AgentID) string {
		if id == localID {
			return localName
		}
		if name, ok := displayNames[id]; ok && name != "" {
			return name
		}
		return id.ShortString()
	}

	// Build route info
	routes := []DashboardRouteInfo{}
	routeDetails := s.remoteProvider.GetRouteDetails()
	for _, route := range routeDetails {
		// Build path display: [local, peer1, peer2, ..., origin]
		pathDisplay := []string{localName} // Start with local
		for _, agentID := range route.Path {
			pathDisplay = append(pathDisplay, getDisplayName(agentID))
		}

		routes = append(routes, DashboardRouteInfo{
			Network:     route.Network,
			Origin:      getDisplayName(route.Origin),
			OriginID:    route.Origin.ShortString(),
			HopCount:    route.HopCount,
			PathDisplay: pathDisplay,
		})
	}

	// Sort routes deterministically by network, then by origin
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Network != routes[j].Network {
			return routes[i].Network < routes[j].Network
		}
		return routes[i].OriginID < routes[j].OriginID
	})

	response := DashboardResponse{
		Agent: TopologyAgentInfo{
			ID:          localID.String(),
			ShortID:     localID.ShortString(),
			DisplayName: localName,
			IsLocal:     true,
			IsConnected: true,
		},
		Stats:  stats,
		Peers:  peers,
		Routes: routes,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// NodesResponse is the response for the /api/nodes endpoint.
type NodesResponse struct {
	Nodes []TopologyAgentInfo `json:"nodes"`
}

// handleNodes handles GET /api/nodes for detailed node info listing.
func (s *Server) handleNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.remoteProvider == nil {
		http.Error(w, "provider not configured", http.StatusServiceUnavailable)
		return
	}

	localID := s.remoteProvider.ID()
	localName := s.remoteProvider.DisplayName()

	// Get all known node info
	allNodeInfo := s.remoteProvider.GetAllNodeInfo()
	localNodeInfo := s.remoteProvider.GetLocalNodeInfo()

	// Build list of all nodes
	nodes := []TopologyAgentInfo{}

	// Build set of direct peers
	peerIDs := s.remoteProvider.GetPeerIDs()
	peerSet := make(map[identity.AgentID]bool)
	for _, id := range peerIDs {
		peerSet[id] = true
	}

	// Add local node
	localNode := TopologyAgentInfo{
		ID:          localID.String(),
		ShortID:     localID.ShortString(),
		DisplayName: localName,
		IsLocal:     true,
		IsConnected: true,
	}
	if localNodeInfo != nil {
		localNode.Hostname = localNodeInfo.Hostname
		localNode.OS = localNodeInfo.OS
		localNode.Arch = localNodeInfo.Arch
		localNode.Version = localNodeInfo.Version
		localNode.IPAddresses = localNodeInfo.IPAddresses
		if localNodeInfo.StartTime > 0 {
			localNode.UptimeHours = float64(time.Now().Unix()-localNodeInfo.StartTime) / 3600.0
		}
	}
	nodes = append(nodes, localNode)

	// Add all known remote nodes
	for agentID, nodeInfo := range allNodeInfo {
		if agentID == localID {
			continue // Skip local, already added
		}

		displayName := nodeInfo.DisplayName
		if displayName == "" {
			displayName = agentID.ShortString()
		}

		node := TopologyAgentInfo{
			ID:          agentID.String(),
			ShortID:     agentID.ShortString(),
			DisplayName: displayName,
			IsLocal:     false,
			IsConnected: peerSet[agentID],
			Hostname:    nodeInfo.Hostname,
			OS:          nodeInfo.OS,
			Arch:        nodeInfo.Arch,
			Version:     nodeInfo.Version,
			IPAddresses: nodeInfo.IPAddresses,
		}
		if nodeInfo.StartTime > 0 {
			node.UptimeHours = float64(time.Now().Unix()-nodeInfo.StartTime) / 3600.0
		}
		nodes = append(nodes, node)
	}

	response := NodesResponse{
		Nodes: nodes,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}
