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

	"github.com/postalsys/muti-metroo/internal/crypto"
	"github.com/postalsys/muti-metroo/internal/identity"
	"github.com/postalsys/muti-metroo/internal/protocol"
	"github.com/postalsys/muti-metroo/internal/webui"
)

// splashPageTemplate is the HTML template for the root splash page.
const splashPageTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Muti Metroo</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            background: linear-gradient(135deg, #1a1a2e 0%%, #16213e 100%%);
            color: #e4e4e7;
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
        }
        .container {
            text-align: center;
            padding: 40px 20px;
            max-width: 480px;
        }
        .logo {
            width: 160px;
            height: 160px;
            margin-bottom: 24px;
        }
        h1 {
            font-size: 2.5rem;
            font-weight: 700;
            margin-bottom: 8px;
            color: #ffffff;
        }
        .tagline {
            font-size: 1.1rem;
            color: #a1a1aa;
            margin-bottom: 16px;
        }
        .description {
            font-size: 0.95rem;
            color: #71717a;
            line-height: 1.6;
            margin-bottom: 32px;
        }
        .button {
            display: inline-block;
            padding: 12px 28px;
            background: #3b82f6;
            color: #ffffff;
            text-decoration: none;
            border-radius: 8px;
            font-weight: 500;
            font-size: 1rem;
            transition: background 0.2s ease;
        }
        .button:hover {
            background: #2563eb;
        }
    </style>
</head>
<body>
    <div class="container">
        <img src="/ui/img/logo.png" alt="Muti Metroo" class="logo">
        <h1>Muti Metroo</h1>
        <p class="tagline">Userspace Mesh Networking Agent</p>
        <p class="description">End-to-end encrypted tunnels across heterogeneous transports with multi-hop routing.</p>
        %s
    </div>
</body>
</html>
`

// StatsProvider provides agent statistics.
type StatsProvider interface {
	// IsRunning returns true if the agent is running.
	IsRunning() bool

	// Stats returns agent statistics.
	Stats() Stats
}

// FileTransferProgress is a callback for reporting file transfer progress.
type FileTransferProgress func(bytesTransferred, totalBytes int64)

// SOCKS5Info contains SOCKS5 proxy configuration for display.
type SOCKS5Info struct {
	Enabled bool
	Address string
}

// UDPInfo contains UDP relay configuration for display.
type UDPInfo struct {
	Enabled bool
}

// RemoteStatusProvider provides the ability to fetch status from remote agents.
type RemoteStatusProvider interface {
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

	// GetDomainRouteDetails returns detailed domain route information for the dashboard.
	GetDomainRouteDetails() []DomainRouteDetails

	// GetAllDisplayNames returns display names for all known agents (from route advertisements).
	GetAllDisplayNames() map[identity.AgentID]string

	// GetAllNodeInfo returns node info for all known agents.
	GetAllNodeInfo() map[identity.AgentID]*protocol.NodeInfo

	// GetLocalNodeInfo returns local node info.
	GetLocalNodeInfo() *protocol.NodeInfo

	// GetSOCKS5Info returns SOCKS5 configuration for the local agent.
	GetSOCKS5Info() SOCKS5Info

	// GetUDPInfo returns UDP relay configuration for the local agent.
	GetUDPInfo() UDPInfo

	// UploadFile uploads a file or directory to a remote agent via stream-based transfer.
	// localPath is the local file/directory path, remotePath is the destination on the remote agent.
	UploadFile(ctx context.Context, targetID identity.AgentID, localPath, remotePath string, opts TransferOptions, progress FileTransferProgress) error

	// DownloadFile downloads a file or directory from a remote agent via stream-based transfer.
	// remotePath is the path on the remote agent, localPath is the local destination.
	DownloadFile(ctx context.Context, targetID identity.AgentID, remotePath, localPath string, opts TransferOptions, progress FileTransferProgress) error

	// DownloadFileStream opens a streaming download from a remote agent.
	// Returns a reader that streams file data directly without writing to disk.
	// The caller must call Close() on the result when done.
	DownloadFileStream(ctx context.Context, targetID identity.AgentID, remotePath string, opts TransferOptions) (*DownloadStreamResult, error)
}

// DownloadStreamResult contains the result of a streaming download request.
type DownloadStreamResult struct {
	Reader       io.ReadCloser
	Size         int64  // Size of the (possibly compressed) data stream
	OriginalSize int64  // Original uncompressed file size
	Mode         uint32 // File permission mode
	IsDirectory  bool   // True if downloading a directory (tar.gz)
	Compressed   bool   // True if data is gzip compressed
	Close        func() // Cleanup function to call when done
}

// TransferOptions contains options for file upload/download operations.
type TransferOptions struct {
	Password     string // Authentication password
	RateLimit    int64  // Max bytes per second (0 = unlimited)
	Offset       int64  // Resume from this byte offset (for downloads)
	OriginalSize int64  // Expected file size for resume validation
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

// DomainRouteDetails contains detailed domain route information for the dashboard.
type DomainRouteDetails struct {
	Pattern    string
	IsWildcard bool
	NextHop    identity.AgentID
	Origin     identity.AgentID
	Metric     int
	HopCount   int
	Path       []identity.AgentID // Full path from local to origin
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
	ID           string   `json:"id"`
	ShortID      string   `json:"short_id"`
	DisplayName  string   `json:"display_name"`
	IsLocal      bool     `json:"is_local"`
	IsConnected  bool     `json:"is_connected"`
	Hostname     string   `json:"hostname,omitempty"`
	OS           string   `json:"os,omitempty"`
	Arch         string   `json:"arch,omitempty"`
	Version      string   `json:"version,omitempty"`
	UptimeHours  float64  `json:"uptime_hours,omitempty"`
	IPAddresses  []string `json:"ip_addresses,omitempty"`
	Roles        []string `json:"roles,omitempty"`         // Agent roles: "ingress", "exit", "transit"
	SOCKS5Addr   string   `json:"socks5_addr,omitempty"`   // SOCKS5 listen address (for ingress)
	ExitRoutes   []string `json:"exit_routes,omitempty"`   // CIDR routes (for exit)
	DomainRoutes []string `json:"domain_routes,omitempty"` // Domain patterns (for exit)
	UDPEnabled   bool     `json:"udp_enabled,omitempty"`   // UDP relay enabled (for exit)
}

// TopologyConnection represents a connection between two agents.
type TopologyConnection struct {
	FromAgent    string `json:"from_agent"`
	ToAgent      string `json:"to_agent"`
	IsDirect     bool   `json:"is_direct"`
	RTTMs        int64  `json:"rtt_ms,omitempty"`
	Unresponsive bool   `json:"unresponsive,omitempty"` // RTT > 60s indicates connection is stuck
	Transport    string `json:"transport,omitempty"`    // Transport type for direct connections: "quic", "h2", "ws"
}

// TopologyResponse is the response for the /api/topology endpoint.
type TopologyResponse struct {
	LocalAgent  TopologyAgentInfo    `json:"local_agent"`
	Agents      []TopologyAgentInfo  `json:"agents"`
	Connections []TopologyConnection `json:"connections"`
}

// DashboardPeerInfo contains information about a connected peer.
type DashboardPeerInfo struct {
	ID           string `json:"id"`
	ShortID      string `json:"short_id"`
	DisplayName  string `json:"display_name"`
	State        string `json:"state"`
	RTTMs        int64  `json:"rtt_ms"`
	Unresponsive bool   `json:"unresponsive,omitempty"` // RTT > 60s indicates connection is stuck
	IsDialer     bool   `json:"is_dialer"`
}

// DashboardRouteInfo contains information about a route.
type DashboardRouteInfo struct {
	Network     string   `json:"network"`
	RouteType   string   `json:"route_type"` // "cidr" or "domain"
	Origin      string   `json:"origin"`     // Display name of origin
	OriginID    string   `json:"origin_id"`  // Short ID of origin
	HopCount    int      `json:"hop_count"`
	PathDisplay []string `json:"path_display"` // Display names: [local, peer1, ..., origin]
	PathIDs     []string `json:"path_ids"`     // Short IDs for path highlighting
	TCP         bool     `json:"tcp"`          // TCP support (always true)
	UDP         bool     `json:"udp"`          // UDP support (exit has UDP enabled)
}

// DashboardDomainRouteInfo contains information about a domain route.
type DashboardDomainRouteInfo struct {
	Pattern     string   `json:"pattern"` // Domain pattern (e.g., "*.example.com")
	IsWildcard  bool     `json:"is_wildcard"`
	Origin      string   `json:"origin"`    // Display name of origin
	OriginID    string   `json:"origin_id"` // Short ID of origin
	HopCount    int      `json:"hop_count"`
	PathDisplay []string `json:"path_display"` // Display names: [local, peer1, ..., origin]
	PathIDs     []string `json:"path_ids"`     // Short IDs for path highlighting
	TCP         bool     `json:"tcp"`          // TCP support (always true)
	UDP         bool     `json:"udp"`          // UDP support (exit has UDP enabled)
}

// DashboardResponse is the response for the /api/dashboard endpoint.
type DashboardResponse struct {
	Agent        TopologyAgentInfo          `json:"agent"`
	Stats        Stats                      `json:"stats"`
	Peers        []DashboardPeerInfo        `json:"peers"`
	Routes       []DashboardRouteInfo       `json:"routes"`
	DomainRoutes []DashboardDomainRouteInfo `json:"domain_routes,omitempty"`
}

// ServerConfig contains health server configuration.
type ServerConfig struct {
	// Address to listen on (e.g., ":8080")
	Address string

	// ReadTimeout for HTTP reads
	ReadTimeout time.Duration

	// WriteTimeout for HTTP writes
	WriteTimeout time.Duration

	// Endpoint group toggles. Disabled endpoints return 404 with logging.
	// /health, /healthz, /ready are always enabled.

	// EnablePprof enables the /debug/pprof/* endpoints
	EnablePprof bool

	// EnableDashboard enables the /ui/*, /api/* endpoints
	EnableDashboard bool

	// EnableRemoteAPI enables the /agents/*, /routes/advertise endpoints
	EnableRemoteAPI bool
}

// DefaultServerConfig returns sensible defaults.
func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		Address:         ":8080",
		ReadTimeout:     10 * time.Second,
		WriteTimeout:    10 * time.Second,
		EnablePprof:     true,
		EnableDashboard: true,
		EnableRemoteAPI: true,
	}
}

// Server is an HTTP server for health check endpoints.
type Server struct {
	cfg            ServerConfig
	provider       StatsProvider
	remoteProvider RemoteStatusProvider
	routeTrigger   RouteAdvertiseTrigger
	shellProvider  ShellProvider     // For shell WebSocket sessions
	sealedBox      *crypto.SealedBox // For checking decrypt capability
	server         *http.Server
	listener       net.Listener
	running        atomic.Bool
}

// disabledHandler returns a handler that returns 404 for disabled endpoints.
func disabledHandler(_ string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// requireGET returns true if the request method is GET, otherwise sends a 405 error.
func requireGET(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	return true
}

// requirePOST returns true if the request method is POST, otherwise sends a 405 error.
func requirePOST(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	return true
}

// calculateUptimeHours calculates uptime in hours from a Unix start time.
func calculateUptimeHours(startTime int64) float64 {
	if startTime <= 0 {
		return 0
	}
	return float64(time.Now().Unix()-startTime) / 3600.0
}

// populateNodeInfo fills in TopologyAgentInfo fields from NodeInfo.
func populateNodeInfo(agent *TopologyAgentInfo, nodeInfo *protocol.NodeInfo) {
	if nodeInfo == nil {
		return
	}
	agent.Hostname = nodeInfo.Hostname
	agent.OS = nodeInfo.OS
	agent.Arch = nodeInfo.Arch
	agent.Version = nodeInfo.Version
	agent.IPAddresses = nodeInfo.IPAddresses
	agent.UptimeHours = calculateUptimeHours(nodeInfo.StartTime)
	if agent.DisplayName == agent.ShortID && nodeInfo.DisplayName != "" {
		agent.DisplayName = nodeInfo.DisplayName
	}
	if nodeInfo.UDPEnabled {
		agent.UDPEnabled = true
	}
}

// buildLocalAgentInfo constructs TopologyAgentInfo for the local agent.
func (s *Server) buildLocalAgentInfo(localID identity.AgentID, displayName string, stats Stats, socks5Info SOCKS5Info, udpInfo UDPInfo) TopologyAgentInfo {
	agent := TopologyAgentInfo{
		ID:          localID.String(),
		ShortID:     localID.ShortString(),
		DisplayName: displayName,
		IsLocal:     true,
		IsConnected: true,
	}
	populateNodeInfo(&agent, s.remoteProvider.GetLocalNodeInfo())
	agent.Roles = s.buildAgentRoles(true, stats.SOCKS5Running, stats.ExitHandlerRun)
	if socks5Info.Enabled {
		agent.SOCKS5Addr = socks5Info.Address
	}
	if udpInfo.Enabled {
		agent.UDPEnabled = true
	}
	return agent
}

// NewServer creates a new health check server.
func NewServer(cfg ServerConfig, provider StatsProvider) *Server {
	s := &Server{
		cfg:      cfg,
		provider: provider,
	}

	mux := http.NewServeMux()

	// Health endpoints - always enabled (minimal footprint)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/ready", s.handleReady)

	// Remote API endpoints: /agents, /agents/*, /routes/advertise
	if cfg.EnableRemoteAPI {
		mux.HandleFunc("/agents", s.handleListAgents)
		mux.HandleFunc("/agents/", s.handleAgentInfo)
		mux.HandleFunc("/routes/advertise", s.handleTriggerAdvertise)
	} else {
		mux.HandleFunc("/agents", disabledHandler("agents"))
		mux.HandleFunc("/agents/", disabledHandler("agents"))
		mux.HandleFunc("/routes/advertise", disabledHandler("routes_advertise"))
	}

	// Dashboard API and Web UI endpoints
	if cfg.EnableDashboard {
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
	} else {
		mux.HandleFunc("/api/", disabledHandler("dashboard_api"))
		mux.HandleFunc("/ui", disabledHandler("dashboard"))
		mux.HandleFunc("/ui/", disabledHandler("dashboard"))
	}

	// pprof debug endpoints
	if cfg.EnablePprof {
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	} else {
		mux.HandleFunc("/debug/", disabledHandler("pprof"))
	}

	// Root splash page - shows logo, name, and optional dashboard link
	// The handler checks for exact "/" path and returns 404 for other unmatched paths
	mux.HandleFunc("/", s.handleSplash)

	s.server = &http.Server{
		Addr:         cfg.Address,
		Handler:      mux,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}

	return s
}

// SetRemoteProvider sets the remote status provider.
// This is called after the agent is initialized.
func (s *Server) SetRemoteProvider(provider RemoteStatusProvider) {
	s.remoteProvider = provider
}

// SetRouteAdvertiseTrigger sets the route advertisement trigger.
// This is called after the agent is initialized.
func (s *Server) SetRouteAdvertiseTrigger(trigger RouteAdvertiseTrigger) {
	s.routeTrigger = trigger
}

// SetSealedBox sets the sealed box for checking decrypt capability.
// This is called after the agent is initialized.
func (s *Server) SetSealedBox(sealedBox *crypto.SealedBox) {
	s.sealedBox = sealedBox
}

// CanDecryptManagement returns true if management key decryption is available.
func (s *Server) CanDecryptManagement() bool {
	return s.sealedBox != nil && s.sealedBox.CanDecrypt()
}

// shouldRestrictTopology returns true if topology info should be restricted to local-only.
// This happens when management key encryption is enabled but we don't have the private key.
func (s *Server) shouldRestrictTopology() bool {
	// If no sealed box configured, no restriction (management key not enabled)
	if s.sealedBox == nil {
		return false
	}
	// If sealed box configured but can't decrypt, restrict to local only
	return !s.sealedBox.CanDecrypt()
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
	if !requireGET(w, r) {
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK\n"))
}

// handleHealthz handles the detailed health check endpoint.
// Returns 200 with JSON stats if healthy, 503 if not running.
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if !requireGET(w, r) {
		return
	}

	if s.provider == nil || !s.provider.IsRunning() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
			"status":  "unavailable",
			"running": false,
		})
		return
	}

	stats := s.provider.Stats()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":               "healthy",
		"running":              true,
		"peer_count":           stats.PeerCount,
		"stream_count":         stats.StreamCount,
		"route_count":          stats.RouteCount,
		"socks5_running":       stats.SOCKS5Running,
		"exit_handler_running": stats.ExitHandlerRun,
	})
}

// handleReady handles the readiness probe endpoint.
// Returns 200 if the agent is ready to serve traffic.
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if !requireGET(w, r) {
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	if s.provider == nil || !s.provider.IsRunning() {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("NOT READY\n"))
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("READY\n"))
}

// handleSplash handles the root "/" splash page.
// Shows Muti Metroo logo, name, description, and optional dashboard link.
func (s *Server) handleSplash(w http.ResponseWriter, r *http.Request) {
	// Only handle exact "/" path, return 404 for other unmatched paths
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if !requireGET(w, r) {
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	dashboardLink := ""
	if s.cfg.EnableDashboard {
		dashboardLink = `<a href="/ui/" class="button">Open Dashboard</a>`
	}
	fmt.Fprintf(w, splashPageTemplate, dashboardLink)
}

// Handler returns the HTTP handler for embedding in other servers.
func (s *Server) Handler() http.Handler {
	return s.server.Handler
}

// handleListAgents lists all known agents in the mesh.
func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	if !requireGET(w, r) {
		return
	}
	if s.remoteProvider == nil {
		http.Error(w, "remote provider not configured", http.StatusServiceUnavailable)
		return
	}

	localID := s.remoteProvider.ID()
	agents := s.remoteProvider.GetKnownAgentIDs()

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

	writeJSON(w, http.StatusOK, result)
}

// handleAgentInfo handles fetching status from a specific agent.
// URL format: /agents/{agent-id} or /agents/{agent-id}/routes or /agents/{agent-id}/peers
func (s *Server) handleAgentInfo(w http.ResponseWriter, r *http.Request) {
	if s.remoteProvider == nil {
		http.Error(w, "remote provider not configured", http.StatusServiceUnavailable)
		return
	}

	// Parse path: /agents/{agent-id}[/routes|/peers|/shell|/file/*]
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

	// Dispatch to sub-handlers based on path suffix
	if len(parts) > 1 {
		switch {
		case strings.HasPrefix(parts[1], "file/upload"):
			s.handleFileUpload(w, r, targetID)
			return
		case strings.HasPrefix(parts[1], "file/download"):
			s.handleFileDownload(w, r, targetID)
			return
		case parts[1] == "shell":
			s.handleShellWebSocket(w, r, targetID)
			return
		}
	}

	// For status/routes/peers requests, only allow GET
	if !requireGET(w, r) {
		return
	}

	// Determine control type from path suffix
	controlType := protocol.ControlTypeStatus
	if len(parts) > 1 {
		switch parts[1] {
		case "routes":
			controlType = protocol.ControlTypeRoutes
		case "peers":
			controlType = protocol.ControlTypePeers
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
	if !requirePOST(w, r) {
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

	// Parse rate limit
	var rateLimit int64
	if rl := r.FormValue("rate_limit"); rl != "" {
		fmt.Sscanf(rl, "%d", &rateLimit)
	}

	// Parse resume options
	resumeUpload := r.FormValue("resume") == "true"
	var originalSize int64
	if os := r.FormValue("original_size"); os != "" {
		fmt.Sscanf(os, "%d", &originalSize)
	}
	_ = resumeUpload // TODO: implement upload resume on server side

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

	// Build transfer options
	opts := TransferOptions{
		Password:     password,
		RateLimit:    rateLimit,
		OriginalSize: originalSize,
	}

	// Perform stream-based upload
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute) // 30 minute timeout for large files
	defer cancel()

	// Extend write deadline for long file transfers (default WriteTimeout is 10s)
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Now().Add(30 * time.Minute))

	err = s.remoteProvider.UploadFile(ctx, targetID, localPath, remotePath, opts, nil)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
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
	if !requirePOST(w, r) {
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
		Path         string `json:"path"`
		Password     string `json:"password,omitempty"`
		RateLimit    int64  `json:"rate_limit,omitempty"`
		Offset       int64  `json:"offset,omitempty"`
		OriginalSize int64  `json:"original_size,omitempty"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.Path == "" {
		http.Error(w, "missing required field: path", http.StatusBadRequest)
		return
	}

	// Use the basename of the remote path as local filename
	localName := filepath.Base(req.Path)
	if localName == "" || localName == "." || localName == "/" {
		localName = "download"
	}

	// Build transfer options
	opts := TransferOptions{
		Password:     req.Password,
		RateLimit:    req.RateLimit,
		Offset:       req.Offset,
		OriginalSize: req.OriginalSize,
	}

	// Perform streaming download directly (no temp file)
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute) // 30 minute timeout
	defer cancel()

	// Extend write deadline for long file transfers (default WriteTimeout is 10s)
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Now().Add(30 * time.Minute))

	result, err := s.remoteProvider.DownloadFileStream(ctx, targetID, req.Path, opts)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	defer result.Reader.Close()
	if result.Close != nil {
		defer result.Close()
	}

	// Set headers based on result type
	if result.IsDirectory {
		// For directories, stream compressed tar data directly
		w.Header().Set("Content-Type", "application/gzip")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", localName+".tar.gz"))
	} else {
		// For files, stream directly
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", localName))
		w.Header().Set("X-File-Mode", fmt.Sprintf("%04o", result.Mode))
		w.Header().Set("X-Original-Size", fmt.Sprintf("%d", result.OriginalSize))
		// Note: We don't set Content-Length because the stream is decompressed
		// and we don't know the final size until we've read it all
	}

	// Stream data directly to response
	_, err = io.Copy(w, result.Reader)
	if err != nil {
		// Can't return error via HTTP at this point since we already started streaming
		// The connection will just be broken
		return
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
	if !requirePOST(w, r) {
		return
	}
	if s.routeTrigger == nil {
		http.Error(w, "route trigger not configured", http.StatusServiceUnavailable)
		return
	}

	s.routeTrigger.TriggerRouteAdvertise()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "triggered",
		"message": "route advertisement triggered",
	})
}

// handleTopology handles GET /api/topology for the metro map visualization.
func (s *Server) handleTopology(w http.ResponseWriter, r *http.Request) {
	if !requireGET(w, r) {
		return
	}
	if s.remoteProvider == nil {
		http.Error(w, "provider not configured", http.StatusServiceUnavailable)
		return
	}

	localID := s.remoteProvider.ID()
	localName := s.remoteProvider.DisplayName()

	// Get local stats for role determination
	var localStats Stats
	if s.provider != nil {
		localStats = s.provider.Stats()
	}

	socks5Info := s.remoteProvider.GetSOCKS5Info()
	udpInfo := s.remoteProvider.GetUDPInfo()

	// Build base local agent info
	localAgent := s.buildLocalAgentInfo(localID, localName, localStats, socks5Info, udpInfo)

	// If management key encryption is enabled but we can't decrypt,
	// only return local agent info (no peers, routes, or other agents)
	if s.shouldRestrictTopology() {
		writeJSON(w, http.StatusOK, TopologyResponse{
			LocalAgent:  localAgent,
			Agents:      []TopologyAgentInfo{localAgent},
			Connections: []TopologyConnection{},
		})
		return
	}

	// Get all known display names from route advertisements
	displayNames := s.remoteProvider.GetAllDisplayNames()
	allNodeInfo := s.remoteProvider.GetAllNodeInfo()

	// Get route details for determining exit roles
	routeDetails := s.remoteProvider.GetRouteDetails()
	domainRouteDetails := s.remoteProvider.GetDomainRouteDetails()

	// Build maps of exit routes and domain routes per agent (by origin)
	exitRoutesPerAgent := make(map[string][]string)
	domainRoutesPerAgent := make(map[string][]string)
	for _, route := range routeDetails {
		originID := route.Origin.String()
		exitRoutesPerAgent[originID] = append(exitRoutesPerAgent[originID], route.Network)
	}
	for _, route := range domainRouteDetails {
		originID := route.Origin.String()
		domainRoutesPerAgent[originID] = append(domainRoutesPerAgent[originID], route.Pattern)
	}

	// Add exit routes to local agent
	if routes, ok := exitRoutesPerAgent[localID.String()]; ok {
		localAgent.ExitRoutes = routes
	}
	if domains, ok := domainRoutesPerAgent[localID.String()]; ok {
		localAgent.DomainRoutes = domains
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
			FromAgent:    localID.ShortString(),
			ToAgent:      peer.ID.ShortString(),
			IsDirect:     true,
			RTTMs:        peer.RTT.Milliseconds(),
			Unresponsive: peer.RTT.Seconds() > 60,
			Transport:    peer.Transport,
		}
	}

	// Extract agents and connections from route paths
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
							conn.Unresponsive = peer.RTTMs > 60000 // 60 seconds in ms
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
			populateNodeInfo(&existing, nodeInfo)
			agentMap[agentID.String()] = existing
		}
	}

	// Populate exit routes and domain routes for all agents, and determine roles
	for agentID, existing := range agentMap {
		if existing.IsLocal {
			continue // Already handled above
		}

		// Check if agent has exit routes (makes it an exit node)
		hasExitRoutes := false
		if routes, ok := exitRoutesPerAgent[agentID]; ok {
			existing.ExitRoutes = routes
			hasExitRoutes = true
		}
		if domains, ok := domainRoutesPerAgent[agentID]; ok {
			existing.DomainRoutes = domains
			hasExitRoutes = true
		}

		// Build roles for remote agents
		// Note: We can't know if remote agents have SOCKS5 without protocol changes
		// For now, we only know exit status from routes
		existing.Roles = s.buildAgentRoles(false, false, hasExitRoutes)

		agentMap[agentID] = existing
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

	writeJSON(w, http.StatusOK, TopologyResponse{
		LocalAgent:  localAgent,
		Agents:      agents,
		Connections: connections,
	})
}

// buildAgentRoles constructs the roles array based on agent capabilities.
// All agents can act as transit, but we only include it if they're not ingress or exit.
func (s *Server) buildAgentRoles(isLocal bool, hasSOCKS5 bool, hasExitRoutes bool) []string {
	var roles []string
	if hasSOCKS5 {
		roles = append(roles, "ingress")
	}
	if hasExitRoutes {
		roles = append(roles, "exit")
	}
	// If no specific roles, mark as transit
	if len(roles) == 0 {
		roles = append(roles, "transit")
	}
	return roles
}

// handleDashboard handles GET /api/dashboard for the dashboard overview.
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if !requireGET(w, r) {
		return
	}
	if s.remoteProvider == nil || s.provider == nil {
		http.Error(w, "provider not configured", http.StatusServiceUnavailable)
		return
	}

	localID := s.remoteProvider.ID()
	localName := s.remoteProvider.DisplayName()
	stats := s.provider.Stats()

	localAgentInfo := TopologyAgentInfo{
		ID:          localID.String(),
		ShortID:     localID.ShortString(),
		DisplayName: localName,
		IsLocal:     true,
		IsConnected: true,
	}

	// If management key encryption is enabled but we can't decrypt,
	// only return local agent info and stats (no peers or routes)
	if s.shouldRestrictTopology() {
		writeJSON(w, http.StatusOK, DashboardResponse{
			Agent:  localAgentInfo,
			Stats:  stats,
			Peers:  []DashboardPeerInfo{},
			Routes: []DashboardRouteInfo{},
		})
		return
	}

	// Build peer info
	peers := make([]DashboardPeerInfo, 0, len(s.remoteProvider.GetPeerDetails()))
	for _, peer := range s.remoteProvider.GetPeerDetails() {
		peers = append(peers, DashboardPeerInfo{
			ID:           peer.ID.String(),
			ShortID:      peer.ID.ShortString(),
			DisplayName:  peer.DisplayName,
			State:        peer.State,
			RTTMs:        peer.RTT.Milliseconds(),
			Unresponsive: peer.RTT.Seconds() > 60,
			IsDialer:     peer.IsDialer,
		})
	}

	// Get display names for building path display
	displayNames := s.remoteProvider.GetAllDisplayNames()
	allNodeInfo := s.remoteProvider.GetAllNodeInfo()

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

	// Helper to get UDP info for an agent
	getUDPEnabled := func(id identity.AgentID) bool {
		info, ok := allNodeInfo[id]
		return ok && info.UDPEnabled
	}

	// Helper to build path display and IDs from agent path
	buildPath := func(path []identity.AgentID) (pathDisplay, pathIDs []string) {
		pathDisplay = []string{localName}
		pathIDs = []string{localID.ShortString()}
		for _, agentID := range path {
			pathDisplay = append(pathDisplay, getDisplayName(agentID))
			pathIDs = append(pathIDs, agentID.ShortString())
		}
		return pathDisplay, pathIDs
	}

	// Build route info (CIDR and domain routes)
	routeDetails := s.remoteProvider.GetRouteDetails()
	domainRouteDetails := s.remoteProvider.GetDomainRouteDetails()
	routes := make([]DashboardRouteInfo, 0, len(routeDetails)+len(domainRouteDetails))

	for _, route := range routeDetails {
		pathDisplay, pathIDs := buildPath(route.Path)
		routes = append(routes, DashboardRouteInfo{
			Network:     route.Network,
			RouteType:   "cidr",
			Origin:      getDisplayName(route.Origin),
			OriginID:    route.Origin.ShortString(),
			HopCount:    route.HopCount,
			PathDisplay: pathDisplay,
			PathIDs:     pathIDs,
			TCP:         true,
			UDP:         getUDPEnabled(route.Origin),
		})
	}

	for _, route := range domainRouteDetails {
		pathDisplay, pathIDs := buildPath(route.Path)
		routes = append(routes, DashboardRouteInfo{
			Network:     route.Pattern,
			RouteType:   "domain",
			Origin:      getDisplayName(route.Origin),
			OriginID:    route.Origin.ShortString(),
			HopCount:    route.HopCount,
			PathDisplay: pathDisplay,
			PathIDs:     pathIDs,
			TCP:         true,
			UDP:         getUDPEnabled(route.Origin),
		})
	}

	// Sort routes: CIDR first (by network), then domains (by pattern)
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].RouteType != routes[j].RouteType {
			return routes[i].RouteType == "cidr"
		}
		if routes[i].Network != routes[j].Network {
			return routes[i].Network < routes[j].Network
		}
		return routes[i].OriginID < routes[j].OriginID
	})

	// Build legacy domain routes for backward compatibility
	domainRoutes := make([]DashboardDomainRouteInfo, 0, len(domainRouteDetails))
	for _, route := range domainRouteDetails {
		pathDisplay, pathIDs := buildPath(route.Path)
		domainRoutes = append(domainRoutes, DashboardDomainRouteInfo{
			Pattern:     route.Pattern,
			IsWildcard:  route.IsWildcard,
			Origin:      getDisplayName(route.Origin),
			OriginID:    route.Origin.ShortString(),
			HopCount:    route.HopCount,
			PathDisplay: pathDisplay,
			PathIDs:     pathIDs,
			TCP:         true,
			UDP:         getUDPEnabled(route.Origin),
		})
	}

	// Sort domain routes deterministically by pattern, then by origin
	sort.Slice(domainRoutes, func(i, j int) bool {
		if domainRoutes[i].Pattern != domainRoutes[j].Pattern {
			return domainRoutes[i].Pattern < domainRoutes[j].Pattern
		}
		return domainRoutes[i].OriginID < domainRoutes[j].OriginID
	})

	writeJSON(w, http.StatusOK, DashboardResponse{
		Agent:        localAgentInfo,
		Stats:        stats,
		Peers:        peers,
		Routes:       routes,
		DomainRoutes: domainRoutes,
	})
}

// NodesResponse is the response for the /api/nodes endpoint.
type NodesResponse struct {
	Nodes []TopologyAgentInfo `json:"nodes"`
}

// handleNodes handles GET /api/nodes for detailed node info listing.
func (s *Server) handleNodes(w http.ResponseWriter, r *http.Request) {
	if !requireGET(w, r) {
		return
	}
	if s.remoteProvider == nil {
		http.Error(w, "provider not configured", http.StatusServiceUnavailable)
		return
	}

	localID := s.remoteProvider.ID()
	localName := s.remoteProvider.DisplayName()

	// Build local node info
	localNode := TopologyAgentInfo{
		ID:          localID.String(),
		ShortID:     localID.ShortString(),
		DisplayName: localName,
		IsLocal:     true,
		IsConnected: true,
	}
	populateNodeInfo(&localNode, s.remoteProvider.GetLocalNodeInfo())

	// If management key encryption is enabled but we can't decrypt,
	// only return local node info
	if s.shouldRestrictTopology() {
		writeJSON(w, http.StatusOK, NodesResponse{Nodes: []TopologyAgentInfo{localNode}})
		return
	}

	// Get all known node info
	allNodeInfo := s.remoteProvider.GetAllNodeInfo()

	// Build set of direct peers
	peerIDs := s.remoteProvider.GetPeerIDs()
	peerSet := make(map[identity.AgentID]bool)
	for _, id := range peerIDs {
		peerSet[id] = true
	}

	// Build list of all nodes, starting with local
	nodes := []TopologyAgentInfo{localNode}

	// Add all known remote nodes
	for agentID, nodeInfo := range allNodeInfo {
		if agentID == localID {
			continue
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
		}
		populateNodeInfo(&node, nodeInfo)
		nodes = append(nodes, node)
	}

	writeJSON(w, http.StatusOK, NodesResponse{Nodes: nodes})
}
