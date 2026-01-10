// Package agent implements the main agent orchestration for Muti Metroo.
package agent

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/postalsys/muti-metroo/internal/certutil"
	"github.com/postalsys/muti-metroo/internal/config"
	"github.com/postalsys/muti-metroo/internal/crypto"
	"github.com/postalsys/muti-metroo/internal/exit"
	"github.com/postalsys/muti-metroo/internal/filetransfer"
	"github.com/postalsys/muti-metroo/internal/flood"
	"github.com/postalsys/muti-metroo/internal/health"
	"github.com/postalsys/muti-metroo/internal/identity"
	"github.com/postalsys/muti-metroo/internal/logging"
	"github.com/postalsys/muti-metroo/internal/peer"
	"github.com/postalsys/muti-metroo/internal/protocol"
	"github.com/postalsys/muti-metroo/internal/recovery"
	"github.com/postalsys/muti-metroo/internal/routing"
	"github.com/postalsys/muti-metroo/internal/shell"
	"github.com/postalsys/muti-metroo/internal/socks5"
	"github.com/postalsys/muti-metroo/internal/stream"
	"github.com/postalsys/muti-metroo/internal/sysinfo"
	"github.com/postalsys/muti-metroo/internal/transport"
	"github.com/postalsys/muti-metroo/internal/tunnel"
	"github.com/postalsys/muti-metroo/internal/udp"
)

// directDialTimeout is the timeout for direct TCP connections (no mesh route).
// Using 10 seconds as it's long enough for real connections but short enough
// for unreachable addresses to fail quickly.
const directDialTimeout = 10 * time.Second

// relayStream tracks a stream being relayed through this agent.
type relayStream struct {
	UpstreamPeer   identity.AgentID
	UpstreamID     uint64 // Stream ID from upstream
	DownstreamPeer identity.AgentID
	DownstreamID   uint64 // Stream ID to downstream
}

// fileTransferStream tracks an active file transfer stream.
type fileTransferStream struct {
	StreamID     uint64
	PeerID       identity.AgentID
	RequestID    uint64
	IsUpload     bool // true for upload (receiving data), false for download (sending data)
	Meta         *filetransfer.TransferMetadata
	MetaReceived bool // true after metadata frame received
	Closed       bool
	// Streaming upload fields (write directly to disk instead of buffering)
	TempFile     *os.File // Temp file for streaming upload data
	BytesWritten int64    // Bytes written to temp file
	// E2E encryption
	sessionKey *crypto.SessionKey // E2E encryption session key
}

// pendingControlRequest tracks an outbound control request awaiting response.
type pendingControlRequest struct {
	RequestID   uint64
	ControlType uint8
	ResponseCh  chan *protocol.ControlResponse
	Timeout     time.Time
}

// forwardedControlRequest tracks a request we forwarded so we can route the response back.
type forwardedControlRequest struct {
	RequestID  uint64
	SourcePeer identity.AgentID // Peer who sent us the request
}

// Agent is the main Muti Metroo agent that orchestrates all components.
type Agent struct {
	cfg     *config.Config
	id      identity.AgentID
	keypair *identity.Keypair // X25519 keypair for E2E encryption
	dataDir string
	logger  *slog.Logger

	// Transport layer - supports QUIC, WebSocket, and HTTP/2
	transports map[transport.TransportType]transport.Transport
	listeners  []transport.Listener

	// Core components
	peerMgr      *peer.Manager
	routeMgr     *routing.Manager
	streamMgr    *stream.Manager
	flooder      *flood.Flooder
	socks5Srv    *socks5.Server
	exitHandler  *exit.Handler
	healthServer *health.Server
	sealedBox    *crypto.SealedBox // Management key encryption (nil if not configured)

	// File transfer (stream-based)
	fileStreamHandler *filetransfer.StreamHandler
	fileStreamsMu     sync.RWMutex
	fileStreams       map[uint64]*fileTransferStream // StreamID -> active transfer

	// Shell (stream-based)
	shellHandler       *shell.Handler
	shellClientMu      sync.RWMutex
	shellClientStreams map[uint64]*health.ShellStreamAdapter // StreamID -> active client session

	// UDP relay (for exit nodes)
	udpHandler *udp.Handler

	// Tunnel (port forwarding)
	tunnelHandler   *tunnel.Handler
	tunnelListeners []*tunnel.Listener

	// Relay stream tracking
	relayMu           sync.RWMutex
	relayByUpstream   map[uint64]*relayStream // Upstream stream ID -> relay
	relayByDownstream map[uint64]*relayStream // Downstream stream ID -> relay
	nextRelayID       uint64

	// Control request tracking
	controlMu        sync.RWMutex
	pendingControl   map[uint64]*pendingControlRequest   // Request ID -> pending request (for requests we initiated)
	forwardedControl map[uint64]*forwardedControlRequest // Request ID -> source peer (for requests we forwarded)
	nextControlID    uint64

	// Route advertisement trigger channel
	routeAdvertiseCh chan struct{}

	// State
	running  atomic.Bool
	stopOnce sync.Once
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// New creates a new agent with the given configuration.
func New(cfg *config.Config) (*Agent, error) {
	// Load or create agent identity
	var agentID identity.AgentID
	var err error

	// If config specifies an explicit ID (not "auto"), use it
	if cfg.Agent.ID != "" && cfg.Agent.ID != "auto" {
		agentID, err = identity.ParseAgentID(cfg.Agent.ID)
		if err != nil {
			return nil, fmt.Errorf("parse agent ID from config: %w", err)
		}
		// Store the config ID to data directory for consistency
		if err := agentID.Store(cfg.Agent.DataDir); err != nil {
			return nil, fmt.Errorf("store agent ID: %w", err)
		}
	} else {
		// Auto-generate or load from data directory
		agentID, _, err = identity.LoadOrCreate(cfg.Agent.DataDir)
		if err != nil {
			return nil, fmt.Errorf("load identity: %w", err)
		}
	}

	// Load or create X25519 keypair for E2E encryption
	keypair, _, err := identity.LoadOrCreateKeypair(cfg.Agent.DataDir)
	if err != nil {
		return nil, fmt.Errorf("load keypair: %w", err)
	}

	// Initialize logger
	logger := logging.NewLogger(cfg.Agent.LogLevel, cfg.Agent.LogFormat)

	a := &Agent{
		cfg:                cfg,
		id:                 agentID,
		keypair:            keypair,
		dataDir:            cfg.Agent.DataDir,
		logger:             logger,
		stopCh:             make(chan struct{}),
		routeAdvertiseCh:   make(chan struct{}, 1), // Buffered to avoid blocking
		relayByUpstream:    make(map[uint64]*relayStream),
		relayByDownstream:  make(map[uint64]*relayStream),
		pendingControl:     make(map[uint64]*pendingControlRequest),
		forwardedControl:   make(map[uint64]*forwardedControlRequest),
		fileStreams:        make(map[uint64]*fileTransferStream),
		shellClientStreams: make(map[uint64]*health.ShellStreamAdapter),
	}

	// Initialize components
	if err := a.initComponents(); err != nil {
		return nil, err
	}

	return a, nil
}

// initComponents initializes all agent components.
func (a *Agent) initComponents() error {
	// Initialize all transports
	a.transports = make(map[transport.TransportType]transport.Transport)
	a.transports[transport.TransportQUIC] = transport.NewQUICTransport()
	a.transports[transport.TransportWebSocket] = transport.NewWebSocketTransport()
	a.transports[transport.TransportHTTP2] = transport.NewH2Transport()

	// Initialize routing manager
	a.routeMgr = routing.NewManager(a.id)

	// Initialize stream manager
	streamCfg := stream.ManagerConfig{
		MaxStreamsPerPeer: a.cfg.Limits.MaxStreamsPerPeer,
		MaxStreamsTotal:   a.cfg.Limits.MaxStreamsTotal,
		BufferSize:        a.cfg.Limits.BufferSize,
		IdleTimeout:       a.cfg.Connections.IdleThreshold,
	}
	a.streamMgr = stream.NewManager(streamCfg, a.id)

	// Initialize peer manager with default QUIC transport
	// Other transports are used via ConnectWithTransport()
	peerCfg := peer.DefaultManagerConfig(a.id, a.transports[transport.TransportQUIC])
	peerCfg.DisplayName = a.cfg.Agent.DisplayName
	peerCfg.KeepaliveInterval = a.cfg.Connections.IdleThreshold
	peerCfg.KeepaliveTimeout = a.cfg.Connections.Timeout
	peerCfg.KeepaliveJitter = a.cfg.Connections.KeepaliveJitter
	peerCfg.Logger = a.logger
	peerCfg.ReconnectConfig = peer.ReconnectConfig{
		InitialDelay: a.cfg.Connections.Reconnect.InitialDelay,
		MaxDelay:     a.cfg.Connections.Reconnect.MaxDelay,
		Multiplier:   a.cfg.Connections.Reconnect.Multiplier,
		Jitter:       a.cfg.Connections.Reconnect.Jitter,
		MaxAttempts:  a.cfg.Connections.Reconnect.MaxRetries,
	}
	peerCfg.OnPeerDisconnect = a.handlePeerDisconnect
	a.peerMgr = peer.NewManager(peerCfg)

	// Initialize management key encryption (sealed box) if configured
	if a.cfg.HasManagementKey() {
		pubKey, err := a.cfg.GetManagementPublicKey()
		if err != nil {
			return fmt.Errorf("get management public key: %w", err)
		}
		if a.cfg.CanDecryptManagement() {
			privKey, err := a.cfg.GetManagementPrivateKey()
			if err != nil {
				return fmt.Errorf("get management private key: %w", err)
			}
			a.sealedBox = crypto.NewSealedBoxWithPrivate(pubKey, privKey)
			a.logger.Info("management key encryption enabled (can decrypt)")
		} else {
			a.sealedBox = crypto.NewSealedBox(pubKey)
			a.logger.Info("management key encryption enabled (encrypt only)")
		}
		// Pass sealed box to routing manager for decryption attempts
		a.routeMgr.SetSealedBox(a.sealedBox)
	}

	// Initialize flooder (needs peer manager for sending)
	floodCfg := flood.DefaultFloodConfig()
	floodCfg.LocalDisplayName = a.cfg.Agent.DisplayName
	floodCfg.Logger = a.logger
	floodCfg.SealedBox = a.sealedBox // Pass sealed box for encryption
	a.flooder = flood.NewFlooder(floodCfg, a.id, a.routeMgr, a.peerMgr)

	// Initialize SOCKS5 server if enabled
	if a.cfg.SOCKS5.Enabled {
		auths := a.buildSOCKS5Auth()
		socksCfg := socks5.ServerConfig{
			Address:        a.cfg.SOCKS5.Address,
			MaxConnections: a.cfg.SOCKS5.MaxConnections,
			ConnectTimeout: 30 * time.Second,
			IdleTimeout:    a.cfg.Connections.IdleThreshold,
			Authenticators: auths,
			Dialer:         a, // Agent implements socks5.Dialer
		}
		a.socks5Srv = socks5.NewServer(socksCfg)
	}

	// Initialize exit handler if enabled
	if a.cfg.Exit.Enabled {
		routes, err := exit.ParseAllowedRoutes(a.cfg.Exit.Routes)
		if err != nil {
			return fmt.Errorf("parse exit routes: %w", err)
		}

		// Parse domain patterns for exit access control
		var domainPatterns []exit.DomainPattern
		for _, pattern := range a.cfg.Exit.DomainRoutes {
			isWildcard, baseDomain := routing.ParseDomainPattern(pattern)
			domainPatterns = append(domainPatterns, exit.DomainPattern{
				Pattern:    pattern,
				IsWildcard: isWildcard,
				BaseDomain: baseDomain,
			})
		}

		exitCfg := exit.HandlerConfig{
			AllowedRoutes:  routes,
			AllowedDomains: domainPatterns,
			ConnectTimeout: 30 * time.Second,
			IdleTimeout:    a.cfg.Connections.IdleThreshold,
			MaxConnections: a.cfg.Limits.MaxStreamsTotal,
			Logger:         a.logger,
			DNS: exit.DNSConfig{
				Servers: a.cfg.Exit.DNS.Servers,
				Timeout: a.cfg.Exit.DNS.Timeout,
			},
		}
		a.exitHandler = exit.NewHandler(exitCfg, a.id, nil)
	}

	// Add local CIDR routes
	for _, route := range a.cfg.Exit.Routes {
		network := routing.MustParseCIDR(route)
		a.routeMgr.AddLocalRoute(network, 0)
	}

	// Add local domain routes
	for _, pattern := range a.cfg.Exit.DomainRoutes {
		a.routeMgr.AddLocalDomainRoute(pattern, 0)
	}

	// Wire up exit handler with Agent as StreamWriter
	if a.exitHandler != nil {
		a.exitHandler.SetWriter(a)
	}

	// Initialize HTTP server if enabled
	if a.cfg.HTTP.Enabled {
		healthCfg := health.ServerConfig{
			Address:         a.cfg.HTTP.Address,
			ReadTimeout:     a.cfg.HTTP.ReadTimeout,
			WriteTimeout:    a.cfg.HTTP.WriteTimeout,
			EnablePprof:     a.cfg.HTTP.PprofEnabled(),
			EnableDashboard: a.cfg.HTTP.DashboardEnabled(),
			EnableRemoteAPI: a.cfg.HTTP.RemoteAPIEnabled(),
		}
		provider := &agentStatsProvider{agent: a}
		a.healthServer = health.NewServer(healthCfg, provider)
		a.healthServer.SetRemoteProvider(a)        // Enable remote status via control channel
		a.healthServer.SetRouteAdvertiseTrigger(a) // Enable route advertisement trigger
		a.healthServer.SetSealedBox(a.sealedBox)   // Enable management key decrypt checks
		a.healthServer.SetShellProvider(a)         // Enable remote shell via HTTP API
	}

	// Initialize file transfer handler (stream-based)
	ftStreamCfg := filetransfer.StreamConfig{
		Enabled:      a.cfg.FileTransfer.Enabled,
		MaxFileSize:  a.cfg.FileTransfer.MaxFileSize,
		AllowedPaths: a.cfg.FileTransfer.AllowedPaths,
		PasswordHash: a.cfg.FileTransfer.PasswordHash,
		Compression:  true, // Default to compression
	}
	a.fileStreamHandler = filetransfer.NewStreamHandler(ftStreamCfg)

	// Initialize shell handler
	shellCfg := shell.Config{
		Enabled:      a.cfg.Shell.Enabled,
		Whitelist:    a.cfg.Shell.Whitelist,
		PasswordHash: a.cfg.Shell.PasswordHash,
		Timeout:      a.cfg.Shell.Timeout,
		MaxSessions:  a.cfg.Shell.MaxSessions,
	}
	shellExecutor := shell.NewExecutor(shellCfg)
	a.shellHandler = shell.NewHandler(shellExecutor, a, a.logger)

	// Initialize UDP handler for exit nodes
	if a.cfg.UDP.Enabled {
		udpCfg := udp.Config{
			Enabled:         a.cfg.UDP.Enabled,
			MaxAssociations: a.cfg.UDP.MaxAssociations,
			IdleTimeout:     a.cfg.UDP.IdleTimeout,
			MaxDatagramSize: a.cfg.UDP.MaxDatagramSize,
		}
		a.udpHandler = udp.NewHandler(udpCfg, a, a.logger)
	}

	// Set UDP handler on SOCKS5 server (for ingress UDP ASSOCIATE)
	if a.socks5Srv != nil {
		a.socks5Srv.SetUDPHandler(a)
	}

	// Initialize tunnel exit handler if endpoints are configured
	if len(a.cfg.Tunnel.Endpoints) > 0 {
		endpoints := make([]tunnel.Endpoint, len(a.cfg.Tunnel.Endpoints))
		for i, ep := range a.cfg.Tunnel.Endpoints {
			endpoints[i] = tunnel.Endpoint{
				Key:    ep.Key,
				Target: ep.Target,
			}
		}

		handlerCfg := tunnel.HandlerConfig{
			Endpoints:      endpoints,
			ConnectTimeout: 30 * time.Second,
			IdleTimeout:    a.cfg.Connections.IdleThreshold,
			MaxConnections: a.cfg.Limits.MaxStreamsTotal,
			Logger:         a.logger,
		}
		a.tunnelHandler = tunnel.NewHandler(handlerCfg, a.id, a)

		// Register local tunnel routes
		for _, ep := range a.cfg.Tunnel.Endpoints {
			a.routeMgr.AddLocalTunnelRoute(ep.Key, ep.Target, 0)
		}
	}

	// Initialize tunnel listeners
	for _, lisCfg := range a.cfg.Tunnel.Listeners {
		cfg := tunnel.ListenerConfig{
			Key:            lisCfg.Key,
			Address:        lisCfg.Address,
			MaxConnections: lisCfg.MaxConnections,
			Logger:         a.logger,
		}
		listener := tunnel.NewListener(cfg, a)
		a.tunnelListeners = append(a.tunnelListeners, listener)
	}

	return nil
}

// buildSOCKS5Auth builds SOCKS5 authenticators from config.
func (a *Agent) buildSOCKS5Auth() []socks5.Authenticator {
	if !a.cfg.SOCKS5.Auth.Enabled {
		return []socks5.Authenticator{&socks5.NoAuthAuthenticator{}}
	}

	// Separate plaintext and hashed credentials
	users := make(map[string]string)
	hashedUsers := make(map[string]string)

	for _, u := range a.cfg.SOCKS5.Auth.Users {
		if u.PasswordHash != "" {
			// Prefer password hash if available
			hashedUsers[u.Username] = u.PasswordHash
		} else if u.Password != "" {
			// Fall back to plaintext password (deprecated)
			users[u.Username] = u.Password
		}
	}

	return socks5.CreateAuthenticators(socks5.AuthConfig{
		Enabled:     true,
		Required:    true,
		Users:       users,
		HashedUsers: hashedUsers,
	})
}

// Start starts all agent components.
func (a *Agent) Start() error {
	if a.running.Load() {
		return fmt.Errorf("agent already running")
	}

	a.running.Store(true)

	a.logger.Info("starting agent",
		logging.KeyAgentID, a.id.ShortString(),
		logging.KeyComponent, "agent")

	// Set frame callback on peer manager
	a.peerMgr.SetFrameCallback(a.processFrame)

	// Start listeners
	for _, listenerCfg := range a.cfg.Listeners {
		if err := a.startListener(listenerCfg); err != nil {
			a.logger.Error("failed to start listener",
				logging.KeyAddress, listenerCfg.Address,
				logging.KeyTransport, listenerCfg.Transport,
				logging.KeyError, err)
			a.running.Store(false)
			return fmt.Errorf("start listener %s: %w", listenerCfg.Address, err)
		}
		a.logger.Info("listener started",
			logging.KeyAddress, listenerCfg.Address,
			logging.KeyTransport, listenerCfg.Transport)
	}

	// Connect to configured peers
	for _, peerCfg := range a.cfg.Peers {
		a.wg.Add(1)
		go a.connectToPeer(peerCfg)
	}

	// Start SOCKS5 server if enabled
	if a.socks5Srv != nil {
		if err := a.socks5Srv.Start(); err != nil {
			a.logger.Error("failed to start SOCKS5 server",
				logging.KeyAddress, a.cfg.SOCKS5.Address,
				logging.KeyError, err)
			a.running.Store(false)
			return fmt.Errorf("start socks5: %w", err)
		}
		a.logger.Info("SOCKS5 server started",
			logging.KeyAddress, a.cfg.SOCKS5.Address)

		// Start UDP destination association cleanup loop
		a.startUDPDestCleanupLoop()
	}

	// Start exit handler if enabled
	if a.exitHandler != nil {
		a.exitHandler.Start()
		a.logger.Info("exit handler started",
			logging.KeyCount, len(a.cfg.Exit.Routes),
			"domain_routes", len(a.cfg.Exit.DomainRoutes))
	}

	// Start tunnel handler if enabled
	if a.tunnelHandler != nil {
		a.tunnelHandler.Start()
		a.logger.Info("tunnel handler started",
			"endpoints", len(a.cfg.Tunnel.Endpoints))
	}

	// Start tunnel listeners
	for _, listener := range a.tunnelListeners {
		if err := listener.Start(); err != nil {
			a.logger.Error("failed to start tunnel listener",
				"key", listener.Key(),
				logging.KeyError, err)
			a.running.Store(false)
			return fmt.Errorf("start tunnel listener %s: %w", listener.Key(), err)
		}
		a.logger.Info("tunnel listener started",
			"key", listener.Key(),
			"address", listener.Address().String())
	}

	// Start route advertisement loop and announce initial routes
	a.wg.Add(1)
	go a.routeAdvertiseLoop()
	if a.cfg.Exit.Enabled || len(a.cfg.Tunnel.Endpoints) > 0 {
		a.flooder.AnnounceLocalRoutes() // Initial announcement
	}

	// Start node info advertisement loop and announce initial node info
	// All nodes advertise their info (not just exit nodes)
	a.wg.Add(1)
	go a.nodeInfoAdvertiseLoop()
	// Initial node info announcement (with small delay for peer connections)
	go func() {
		time.Sleep(2 * time.Second) // Wait for initial peer connections
		info := sysinfo.Collect(a.cfg.Agent.DisplayName, a.getPeerConnectionInfo(), a.keypair.PublicKey, a.getUDPConfig())
		a.flooder.AnnounceLocalNodeInfo(info)
		a.logger.Debug("initial node info advertisement sent",
			"display_name", info.DisplayName,
			"hostname", info.Hostname,
			"peers", len(info.Peers))
	}()

	// Start HTTP server if enabled
	if a.healthServer != nil {
		if err := a.healthServer.Start(); err != nil {
			a.logger.Error("failed to start HTTP server",
				logging.KeyAddress, a.cfg.HTTP.Address,
				logging.KeyError, err)
			a.running.Store(false)
			return fmt.Errorf("start HTTP server: %w", err)
		}
		a.logger.Info("HTTP server started",
			logging.KeyAddress, a.healthServer.Address())
	}

	a.logger.Info("agent started",
		logging.KeyAgentID, a.id.ShortString(),
		"peers", len(a.cfg.Peers),
		"listeners", len(a.cfg.Listeners))

	return nil
}

// startListener starts a listener for the given configuration.
func (a *Agent) startListener(cfg config.ListenerConfig) error {
	// For plaintext WebSocket listeners (reverse proxy mode), skip TLS
	var tlsConfig *tls.Config
	if cfg.PlainText {
		a.logger.Warn("starting plaintext WebSocket listener (no TLS)",
			"address", cfg.Address,
			"path", cfg.Path,
			"warning", "only use behind trusted reverse proxy")
	} else {
		// Determine effective mTLS setting (per-listener override or global)
		enableMTLS := a.cfg.TLS.MTLS
		if cfg.TLS.MTLS != nil {
			enableMTLS = *cfg.TLS.MTLS
		}

		// Load TLS config with mTLS support
		var err error
		tlsConfig, err = a.loadListenerTLSConfig(&cfg.TLS, enableMTLS)
		if err != nil {
			return fmt.Errorf("load TLS config: %w", err)
		}
	}

	// Select the appropriate transport based on config
	transportType := transport.TransportType(cfg.Transport)
	if transportType == "" {
		transportType = transport.TransportQUIC // Default to QUIC
	}

	tr, ok := a.transports[transportType]
	if !ok {
		return fmt.Errorf("unsupported transport type: %s", transportType)
	}

	// Start the listener with protocol identifiers from config
	listener, err := tr.Listen(cfg.Address, transport.ListenOptions{
		TLSConfig:     tlsConfig,
		Path:          cfg.Path, // Used by WebSocket and HTTP/2
		MaxStreams:    a.cfg.Limits.MaxStreamsTotal,
		PlainText:     cfg.PlainText,
		ALPNProtocol:  a.cfg.Protocol.ALPN,
		HTTPHeader:    a.cfg.Protocol.HTTPHeader,
		WSSubprotocol: a.cfg.Protocol.WSSubprotocol,
	})
	if err != nil {
		return err
	}

	a.listeners = append(a.listeners, listener)

	// Start accept loop
	a.wg.Add(1)
	go a.acceptLoop(listener)

	return nil
}

// loadListenerTLSConfig loads TLS configuration for a listener.
// Uses per-listener override if available, otherwise falls back to global config.
// If enableMTLS is true, client certificate verification is enabled.
func (a *Agent) loadListenerTLSConfig(override *config.TLSConfig, enableMTLS bool) (*tls.Config, error) {
	var cert tls.Certificate
	var certPEM, keyPEM []byte
	var err error

	// Get effective cert/key (per-listener override or global)
	certPEM, err = a.cfg.GetEffectiveCertPEM(override)
	if err != nil {
		return nil, fmt.Errorf("load certificate: %w", err)
	}
	keyPEM, err = a.cfg.GetEffectiveKeyPEM(override)
	if err != nil {
		return nil, fmt.Errorf("load private key: %w", err)
	}

	if certPEM != nil && keyPEM != nil {
		// Validate EC-only certificates
		if err := certutil.ValidateECKeyPair(certPEM, keyPEM); err != nil {
			return nil, fmt.Errorf("EC validation failed: %w", err)
		}

		cert, err = tls.X509KeyPair(certPEM, keyPEM)
		if err != nil {
			return nil, fmt.Errorf("parse certificate: %w", err)
		}
	} else {
		// Generate self-signed EC cert for development
		certPEM, keyPEM, err = transport.GenerateSelfSignedCert(a.id.ShortString(), 365*24*time.Hour)
		if err != nil {
			return nil, fmt.Errorf("generate self-signed cert: %w", err)
		}
		cert, err = tls.X509KeyPair(certPEM, keyPEM)
		if err != nil {
			return nil, fmt.Errorf("parse generated cert: %w", err)
		}
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{transport.ALPNProtocol},
		MinVersion:   tls.VersionTLS13,
	}

	// Set up mTLS if enabled
	if enableMTLS {
		// Load CA certificate for client verification (always from global config)
		caPEM, err := a.cfg.TLS.GetCAPEM()
		if err != nil {
			return nil, fmt.Errorf("load CA certificate: %w", err)
		}
		if caPEM == nil {
			return nil, fmt.Errorf("tls.ca is required when mTLS is enabled")
		}

		// Validate CA is EC
		if err := certutil.ValidateECCertificate(caPEM); err != nil {
			return nil, fmt.Errorf("CA EC validation failed: %w", err)
		}

		certPool := x509.NewCertPool()
		if !certPool.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}

		tlsConfig.ClientCAs = certPool
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	}

	return tlsConfig, nil
}

// acceptLoop accepts incoming connections from a listener.
func (a *Agent) acceptLoop(listener transport.Listener) {
	defer a.wg.Done()
	defer recovery.RecoverWithLog(a.logger, "acceptLoop")

	for {
		select {
		case <-a.stopCh:
			return
		default:
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		peerConn, err := listener.Accept(ctx)
		cancel()

		if err != nil {
			select {
			case <-a.stopCh:
				return
			default:
				// Log error and continue
				a.logger.Debug("accept error",
					logging.KeyLocalAddr, listener.Addr(),
					logging.KeyError, err)
				continue
			}
		}

		// Handle the connection in a goroutine
		a.wg.Add(1)
		go a.handleIncomingConnection(peerConn)
	}
}

// handleIncomingConnection processes an incoming peer connection.
func (a *Agent) handleIncomingConnection(peerConn transport.PeerConn) {
	defer a.wg.Done()
	defer recovery.RecoverWithLog(a.logger, "handleIncomingConnection")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := a.peerMgr.Accept(ctx, peerConn)
	if err != nil {
		a.logger.Debug("failed to accept peer connection",
			logging.KeyError, err)
		peerConn.Close()
		return
	}

	a.logger.Info("peer connected",
		logging.KeyPeerID, conn.RemoteID.ShortString(),
		logging.KeyRemoteAddr, conn.RemoteAddr())

	// Send full routing table to new peer
	a.flooder.SendFullTable(conn.RemoteID)

	// Send all known node info to new peer
	a.flooder.SendNodeInfoToNewPeer(conn.RemoteID)
}

// connectToPeer initiates a connection to a configured peer.
func (a *Agent) connectToPeer(cfg config.PeerConfig) {
	defer a.wg.Done()
	defer recovery.RecoverWithLog(a.logger, "connectToPeer")

	a.logger.Debug("connecting to peer",
		logging.KeyAddress, cfg.Address,
		logging.KeyTransport, cfg.Transport)

	// Parse expected peer ID (if specified)
	var expectedID identity.AgentID
	if cfg.ID != "" && cfg.ID != "auto" {
		var err error
		expectedID, err = identity.ParseAgentID(cfg.ID)
		if err != nil {
			a.logger.Error("failed to parse peer ID",
				logging.KeyPeerID, cfg.ID,
				logging.KeyError, err)
			return
		}
	}
	// If ID is "auto" or empty, expectedID will be zero and peer manager will accept any ID

	// Determine if this is a WebSocket connection through a proxy
	// In this case, the external server might use RSA, so skip EC validation for CA
	isProxiedWS := cfg.Transport == "ws" && cfg.Proxy != ""

	// Determine ALPN protocol to use
	alpn := a.cfg.Protocol.ALPN
	if alpn == "" {
		alpn = transport.DefaultALPNProtocol
	}

	// Build DialOptions from peer config with protocol identifiers
	dialOpts := &transport.DialOptions{
		InsecureSkipVerify: cfg.TLS.InsecureSkipVerify,
		Timeout:            a.cfg.Connections.Timeout,
		ALPNProtocol:       a.cfg.Protocol.ALPN,
		HTTPHeader:         a.cfg.Protocol.HTTPHeader,
		WSSubprotocol:      a.cfg.Protocol.WSSubprotocol,
	}

	// Build TLS config for peer connection
	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS13,
		NextProtos:         []string{alpn},
		InsecureSkipVerify: cfg.TLS.InsecureSkipVerify,
	}

	// Load CA certificate for peer verification (per-peer override or global)
	caPEM, err := a.cfg.GetEffectiveCAPEM(&cfg.TLS)
	if err != nil {
		a.logger.Error("failed to load peer CA certificate",
			logging.KeyPeerID, cfg.ID,
			logging.KeyError, err)
		return
	}
	if caPEM != nil {
		// Validate EC-only for CA (skip for proxied WebSocket - external server may use RSA)
		if !isProxiedWS && !cfg.TLS.InsecureSkipVerify {
			if err := certutil.ValidateECCertificate(caPEM); err != nil {
				a.logger.Error("CA EC validation failed",
					logging.KeyPeerID, cfg.ID,
					logging.KeyError, err)
				return
			}
		}

		certPool := x509.NewCertPool()
		if !certPool.AppendCertsFromPEM(caPEM) {
			a.logger.Error("failed to parse peer CA certificate",
				logging.KeyPeerID, cfg.ID)
			return
		}
		tlsConfig.RootCAs = certPool
	}

	// Load client certificate for mTLS (per-peer override or global)
	certPEM, err := a.cfg.GetEffectiveCertPEM(&cfg.TLS)
	if err != nil {
		a.logger.Error("failed to load peer client certificate",
			logging.KeyPeerID, cfg.ID,
			logging.KeyError, err)
		return
	}
	keyPEM, err := a.cfg.GetEffectiveKeyPEM(&cfg.TLS)
	if err != nil {
		a.logger.Error("failed to load peer client key",
			logging.KeyPeerID, cfg.ID,
			logging.KeyError, err)
		return
	}

	if certPEM != nil && keyPEM != nil {
		// Validate EC-only for client cert (always required for our certs)
		if err := certutil.ValidateECKeyPair(certPEM, keyPEM); err != nil {
			a.logger.Error("client cert EC validation failed",
				logging.KeyPeerID, cfg.ID,
				logging.KeyError, err)
			return
		}

		cert, err := tls.X509KeyPair(certPEM, keyPEM)
		if err != nil {
			a.logger.Error("failed to parse peer client certificate",
				logging.KeyPeerID, cfg.ID,
				logging.KeyError, err)
			return
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	dialOpts.TLSConfig = tlsConfig

	// Select the appropriate transport based on config
	transportType := transport.TransportType(cfg.Transport)
	if transportType == "" {
		transportType = transport.TransportQUIC // Default to QUIC
	}

	// Get transport for this peer (needed for reconnection)
	var peerTransport transport.Transport
	if transportType != transport.TransportQUIC {
		tr, ok := a.transports[transportType]
		if !ok {
			a.logger.Error("unsupported transport type",
				logging.KeyTransport, transportType)
			return
		}
		peerTransport = tr
	}

	// Add peer info to manager (including transport for reconnection)
	a.peerMgr.AddPeer(peer.PeerInfo{
		Address:     cfg.Address,
		ExpectedID:  expectedID,
		Persistent:  true,
		DialOptions: dialOpts,
		Transport:   peerTransport,
	})

	// Attempt connection
	ctx, cancel := context.WithTimeout(context.Background(), a.cfg.Connections.Timeout)
	defer cancel()

	var conn *peer.Connection

	if peerTransport == nil {
		// Use default Connect for QUIC
		conn, err = a.peerMgr.Connect(ctx, cfg.Address)
	} else {
		// Use ConnectWithTransport for other transports
		conn, err = a.peerMgr.ConnectWithTransport(ctx, peerTransport, cfg.Address)
	}

	if err != nil {
		a.logger.Warn("failed to connect to peer",
			logging.KeyAddress, cfg.Address,
			logging.KeyTransport, cfg.Transport,
			logging.KeyError, err)
		// Reconnection will be handled by peer manager
		return
	}

	a.logger.Info("connected to peer",
		logging.KeyPeerID, conn.RemoteID.ShortString(),
		logging.KeyAddress, cfg.Address,
		logging.KeyTransport, cfg.Transport)

	// Send full routing table to new peer
	a.flooder.SendFullTable(conn.RemoteID)

	// Send all known node info to new peer
	a.flooder.SendNodeInfoToNewPeer(conn.RemoteID)
}

// Stop gracefully stops the agent.
func (a *Agent) Stop() error {
	var err error
	a.stopOnce.Do(func() {
		a.logger.Info("stopping agent",
			logging.KeyAgentID, a.id.ShortString())

		a.running.Store(false)
		close(a.stopCh)

		// Withdraw routes before shutdown
		if a.cfg.Exit.Enabled || len(a.cfg.Tunnel.Endpoints) > 0 {
			a.flooder.WithdrawLocalRoutes()
		}

		// Stop components in reverse order
		if a.healthServer != nil {
			a.healthServer.Stop()
		}

		// Stop tunnel listeners
		for _, listener := range a.tunnelListeners {
			listener.Stop()
		}

		// Stop tunnel handler
		if a.tunnelHandler != nil {
			a.tunnelHandler.Stop()
		}

		if a.exitHandler != nil {
			a.exitHandler.Stop()
		}

		if a.socks5Srv != nil {
			a.socks5Srv.Stop()
		}

		if a.flooder != nil {
			a.flooder.Stop()
		}

		if a.peerMgr != nil {
			a.peerMgr.Close()
		}

		// Close listeners
		for _, l := range a.listeners {
			l.Close()
		}
		a.listeners = nil

		// Close all transports
		for _, tr := range a.transports {
			if tr != nil {
				tr.Close()
			}
		}

		a.wg.Wait()

		a.logger.Info("agent stopped",
			logging.KeyAgentID, a.id.ShortString())
	})

	return err
}

// StopWithContext stops with a timeout.
func (a *Agent) StopWithContext(ctx context.Context) error {
	done := make(chan error, 1)
	go func() {
		done <- a.Stop()
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// IsRunning returns true if the agent is running.
func (a *Agent) IsRunning() bool {
	return a.running.Load()
}

// ID returns the agent's identity.
func (a *Agent) ID() identity.AgentID {
	return a.id
}

// DisplayName returns the agent's display name, or falls back to the short ID.
func (a *Agent) DisplayName() string {
	if a.cfg.Agent.DisplayName != "" {
		return a.cfg.Agent.DisplayName
	}
	return a.id.ShortString()
}

// TriggerRouteAdvertise triggers an immediate route advertisement.
// This is useful when local routes change and you want faster propagation
// than waiting for the next scheduled interval.
func (a *Agent) TriggerRouteAdvertise() {
	select {
	case a.routeAdvertiseCh <- struct{}{}:
		a.logger.Debug("route advertisement triggered")
	default:
		// Already pending, skip
	}
}

// routeAdvertiseLoop periodically announces local routes to peers.
// Also responds to manual triggers for immediate advertisement.
// Cleans up stale routes that haven't been refreshed within route_ttl.
func (a *Agent) routeAdvertiseLoop() {
	defer a.wg.Done()
	defer recovery.RecoverWithLog(a.logger, "routeAdvertiseLoop")

	interval := a.cfg.Routing.AdvertiseInterval
	if interval <= 0 {
		interval = 2 * time.Minute // Default if not configured
	}

	// Route TTL for cleanup (use config value or default to 5x advertise interval)
	routeTTL := a.cfg.Routing.RouteTTL
	if routeTTL <= 0 {
		routeTTL = interval * 5
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	a.logger.Debug("route advertise loop started",
		"interval", interval,
		"route_ttl", routeTTL)

	for {
		select {
		case <-a.stopCh:
			return
		case <-ticker.C:
			// Clean up stale routes before advertising
			if removed := a.routeMgr.CleanupStaleRoutes(routeTTL); removed > 0 {
				a.logger.Debug("cleaned up stale routes",
					"removed", removed,
					"ttl", routeTTL)
			}

			if a.cfg.Exit.Enabled {
				a.flooder.AnnounceLocalRoutes()
				a.logger.Debug("periodic route advertisement sent")
			}
		case <-a.routeAdvertiseCh:
			if a.cfg.Exit.Enabled {
				a.flooder.AnnounceLocalRoutes()
				a.logger.Debug("triggered route advertisement sent")
			}
		}
	}
}

// nodeInfoAdvertiseLoop periodically announces local node info to peers.
// All nodes advertise their info, not just exit nodes.
// Also cleans up stale node info from agents that haven't advertised recently.
func (a *Agent) nodeInfoAdvertiseLoop() {
	defer a.wg.Done()
	defer recovery.RecoverWithLog(a.logger, "nodeInfoAdvertiseLoop")

	// Use NodeInfoInterval if set, otherwise fall back to AdvertiseInterval
	interval := a.cfg.Routing.NodeInfoInterval
	if interval <= 0 {
		interval = a.cfg.Routing.AdvertiseInterval
	}
	if interval <= 0 {
		interval = 2 * time.Minute // Default if not configured
	}

	// Node info TTL is 5x the advertise interval (gives agents time to miss a few advertisements)
	nodeInfoTTL := interval * 5

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	a.logger.Debug("node info advertise loop started",
		"interval", interval,
		"node_info_ttl", nodeInfoTTL)

	for {
		select {
		case <-a.stopCh:
			return
		case <-ticker.C:
			// Clean up stale node info before advertising
			if removed := a.routeMgr.CleanupStaleNodeInfo(nodeInfoTTL); removed > 0 {
				a.logger.Debug("cleaned up stale node info entries",
					"removed", removed,
					"ttl", nodeInfoTTL)
			}

			// Collect and announce local node info with current peer connections
			info := sysinfo.Collect(a.cfg.Agent.DisplayName, a.getPeerConnectionInfo(), a.keypair.PublicKey, a.getUDPConfig())
			a.flooder.AnnounceLocalNodeInfo(info)
			a.logger.Debug("periodic node info advertisement sent",
				"display_name", info.DisplayName,
				"hostname", info.Hostname,
				"peers", len(info.Peers))
		}
	}
}

// processFrame dispatches incoming frames to the appropriate handler.
func (a *Agent) processFrame(peerID identity.AgentID, frame *protocol.Frame) {
	switch frame.Type {
	case protocol.FrameStreamOpen:
		a.handleStreamOpen(peerID, frame)
	case protocol.FrameStreamOpenAck:
		a.handleStreamOpenAck(peerID, frame)
	case protocol.FrameStreamOpenErr:
		a.handleStreamOpenErr(peerID, frame)
	case protocol.FrameStreamData:
		a.handleStreamData(peerID, frame)
	case protocol.FrameStreamClose:
		a.handleStreamClose(peerID, frame)
	case protocol.FrameStreamReset:
		a.handleStreamReset(peerID, frame)
	case protocol.FrameRouteAdvertise:
		a.handleRouteAdvertise(peerID, frame)
	case protocol.FrameRouteWithdraw:
		a.handleRouteWithdraw(peerID, frame)
	case protocol.FrameNodeInfoAdvertise:
		a.handleNodeInfoAdvertise(peerID, frame)
	case protocol.FrameKeepalive:
		a.handleKeepalive(peerID, frame)
	case protocol.FrameKeepaliveAck:
		a.handleKeepaliveAck(peerID, frame)
	case protocol.FrameControlRequest:
		a.handleControlRequest(peerID, frame)
	case protocol.FrameControlResponse:
		a.handleControlResponse(peerID, frame)
	// UDP frames
	case protocol.FrameUDPOpen:
		a.handleUDPOpen(peerID, frame)
	case protocol.FrameUDPOpenAck:
		a.handleUDPOpenAck(peerID, frame)
	case protocol.FrameUDPOpenErr:
		a.handleUDPOpenErr(peerID, frame)
	case protocol.FrameUDPDatagram:
		a.handleUDPDatagram(peerID, frame)
	case protocol.FrameUDPClose:
		a.handleUDPClose(peerID, frame)
	}
}

// handleStreamOpen processes a STREAM_OPEN request.
func (a *Agent) handleStreamOpen(peerID identity.AgentID, frame *protocol.Frame) {
	open, err := protocol.DecodeStreamOpen(frame.Payload)
	if err != nil {
		a.logger.Debug("failed to decode stream open",
			logging.KeyPeerID, peerID.ShortString(),
			logging.KeyError, err)
		return
	}

	// Check if we are the exit node (path is empty or we're the target)
	if len(open.RemainingPath) == 0 || (len(open.RemainingPath) == 1 && open.RemainingPath[0] == a.id) {
		// Check if this is a file transfer or shell stream
		if open.AddressType == protocol.AddrTypeDomain {
			destAddr := addressToString(open.AddressType, open.Address)
			if destAddr == protocol.FileTransferUpload {
				a.handleFileUploadStreamOpen(peerID, frame.StreamID, open.RequestID, open.EphemeralPubKey)
				return
			}
			if destAddr == protocol.FileTransferDownload {
				a.handleFileDownloadStreamOpen(peerID, frame.StreamID, open.RequestID, open.EphemeralPubKey)
				return
			}
			// Shell streams
			if destAddr == protocol.ShellStream {
				a.handleShellStreamOpen(peerID, frame.StreamID, open.RequestID, false, open.EphemeralPubKey)
				return
			}
			if destAddr == protocol.ShellInteractive {
				a.handleShellStreamOpen(peerID, frame.StreamID, open.RequestID, true, open.EphemeralPubKey)
				return
			}
			// Tunnel streams
			if strings.HasPrefix(destAddr, protocol.TunnelStreamPrefix) {
				key := strings.TrimPrefix(destAddr, protocol.TunnelStreamPrefix)
				if a.tunnelHandler != nil {
					ctx := context.Background()
					a.tunnelHandler.HandleStreamOpen(ctx, frame.StreamID, open.RequestID, peerID, key, open.EphemeralPubKey)
				} else {
					// No tunnel handler - send error
					errPayload := &protocol.StreamOpenErr{
						RequestID: open.RequestID,
						ErrorCode: protocol.ErrTunnelNotFound,
						Message:   "tunnel key not configured",
					}
					errFrame := &protocol.Frame{
						Type:     protocol.FrameStreamOpenErr,
						StreamID: frame.StreamID,
						Payload:  errPayload.Encode(),
					}
					a.peerMgr.SendToPeer(peerID, errFrame)
				}
				return
			}
		}

		// We are the exit node for TCP traffic
		if a.exitHandler != nil {
			ctx := context.Background()
			// Convert address bytes to string based on address type
			destAddr := addressToString(open.AddressType, open.Address)
			a.exitHandler.HandleStreamOpen(ctx, frame.StreamID, open.RequestID, peerID, destAddr, open.Port, open.EphemeralPubKey)
		}
		return
	}

	// Forward to next hop
	nextHop := open.RemainingPath[0]

	// Get connection to next hop
	conn := a.peerMgr.GetPeer(nextHop)
	if conn == nil {
		// No route to next hop, send error back
		errPayload := &protocol.StreamOpenErr{
			RequestID: open.RequestID,
			ErrorCode: protocol.ErrHostUnreachable,
			Message:   "no route to next hop",
		}
		errFrame := &protocol.Frame{
			Type:     protocol.FrameStreamOpenErr,
			StreamID: frame.StreamID,
			Payload:  errPayload.Encode(),
		}
		a.peerMgr.SendToPeer(peerID, errFrame)
		return
	}

	// Generate new downstream stream ID
	downstreamID := conn.NextStreamID()

	// Create relay entry
	relay := &relayStream{
		UpstreamPeer:   peerID,
		UpstreamID:     frame.StreamID,
		DownstreamPeer: nextHop,
		DownstreamID:   downstreamID,
	}

	a.relayMu.Lock()
	a.relayByUpstream[frame.StreamID] = relay
	a.relayByDownstream[downstreamID] = relay
	a.relayMu.Unlock()

	// Update remaining path (remove the next hop)
	newPath := open.RemainingPath[1:]

	// Build forwarded STREAM_OPEN (preserve ephemeral key for E2E encryption)
	fwdOpen := &protocol.StreamOpen{
		RequestID:       open.RequestID,
		AddressType:     open.AddressType,
		Address:         open.Address,
		Port:            open.Port,
		RemainingPath:   newPath,
		EphemeralPubKey: open.EphemeralPubKey,
	}

	fwdFrame := &protocol.Frame{
		Type:     protocol.FrameStreamOpen,
		StreamID: downstreamID,
		Payload:  fwdOpen.Encode(),
	}

	if err := a.peerMgr.SendToPeer(nextHop, fwdFrame); err != nil {
		// Clean up relay entry on failure
		a.relayMu.Lock()
		delete(a.relayByUpstream, frame.StreamID)
		delete(a.relayByDownstream, downstreamID)
		a.relayMu.Unlock()

		// Send error back
		errPayload := &protocol.StreamOpenErr{
			RequestID: open.RequestID,
			ErrorCode: protocol.ErrConnectionRefused,
			Message:   err.Error(),
		}
		errFrame := &protocol.Frame{
			Type:     protocol.FrameStreamOpenErr,
			StreamID: frame.StreamID,
			Payload:  errPayload.Encode(),
		}
		a.peerMgr.SendToPeer(peerID, errFrame)
	}
}

// addressToString converts address bytes to a string representation.
func addressToString(addrType uint8, addr []byte) string {
	switch addrType {
	case protocol.AddrTypeIPv4:
		if len(addr) >= 4 {
			return net.IP(addr[:4]).String()
		}
	case protocol.AddrTypeIPv6:
		if len(addr) >= 16 {
			return net.IP(addr[:16]).String()
		}
	case protocol.AddrTypeDomain:
		// Domain addresses have length prefix byte that needs to be skipped
		if len(addr) > 0 {
			return string(addr[1:])
		}
	}
	return ""
}

// handleStreamOpenAck processes a STREAM_OPEN_ACK.
func (a *Agent) handleStreamOpenAck(peerID identity.AgentID, frame *protocol.Frame) {
	// Check if this is a relay stream response - ACK comes from downstream
	a.relayMu.RLock()
	relay := a.relayByDownstream[frame.StreamID]
	a.relayMu.RUnlock()

	// Verify ACK is from the expected downstream peer
	if relay != nil && peerID == relay.DownstreamPeer {
		// Forward ACK to upstream with upstream stream ID
		fwdFrame := &protocol.Frame{
			Type:     protocol.FrameStreamOpenAck,
			StreamID: relay.UpstreamID,
			Payload:  frame.Payload,
		}
		a.peerMgr.SendToPeer(relay.UpstreamPeer, fwdFrame)
		return
	}

	ack, err := protocol.DecodeStreamOpenAck(frame.Payload)
	if err != nil {
		return
	}

	// Convert bound address bytes to net.IP
	var boundIP net.IP
	if len(ack.BoundAddr) > 0 {
		boundIP = net.IP(ack.BoundAddr)
	}

	a.streamMgr.HandleStreamOpenAck(ack.RequestID, boundIP, ack.BoundPort, ack.EphemeralPubKey)
}

// handleStreamOpenErr processes a STREAM_OPEN_ERR.
func (a *Agent) handleStreamOpenErr(peerID identity.AgentID, frame *protocol.Frame) {
	// Check if this is a relay stream response - ERR comes from downstream
	a.relayMu.Lock()
	relay := a.relayByDownstream[frame.StreamID]

	// Verify ERR is from the expected downstream peer
	if relay != nil && peerID == relay.DownstreamPeer {
		// Clean up relay entry
		delete(a.relayByUpstream, relay.UpstreamID)
		delete(a.relayByDownstream, frame.StreamID)
		a.relayMu.Unlock()

		// Forward error to upstream with upstream stream ID
		fwdFrame := &protocol.Frame{
			Type:     protocol.FrameStreamOpenErr,
			StreamID: relay.UpstreamID,
			Payload:  frame.Payload,
		}
		a.peerMgr.SendToPeer(relay.UpstreamPeer, fwdFrame)
		return
	}
	a.relayMu.Unlock()

	errPayload, err := protocol.DecodeStreamOpenErr(frame.Payload)
	if err != nil {
		a.logger.Debug("failed to decode stream open error",
			logging.KeyPeerID, peerID.ShortString(),
			logging.KeyError, err)
		return
	}

	a.logger.Debug("stream open failed",
		logging.KeyStreamID, frame.StreamID,
		logging.KeyRequestID, errPayload.RequestID,
		"error_code", errPayload.ErrorCode,
		"message", errPayload.Message)

	a.streamMgr.HandleStreamOpenErr(errPayload.RequestID, errPayload.ErrorCode, errPayload.Message)
}

// handleStreamData processes stream data.
func (a *Agent) handleStreamData(peerID identity.AgentID, frame *protocol.Frame) {
	// Check if this is a relay stream - could be from upstream or downstream
	// We must check both the stream ID AND the peer ID to determine direction,
	// because upstream and downstream may use the same stream ID number
	a.relayMu.RLock()
	upRelay := a.relayByUpstream[frame.StreamID]
	downRelay := a.relayByDownstream[frame.StreamID]
	a.relayMu.RUnlock()

	// Check if data is from upstream (matches upRelay's upstream peer)
	if upRelay != nil && peerID == upRelay.UpstreamPeer {
		// Data from upstream, forward to downstream
		fwdFrame := &protocol.Frame{
			Type:     protocol.FrameStreamData,
			StreamID: upRelay.DownstreamID,
			Flags:    frame.Flags,
			Payload:  frame.Payload,
		}
		a.peerMgr.SendToPeer(upRelay.DownstreamPeer, fwdFrame)
		return
	}

	// Check if data is from downstream (matches downRelay's downstream peer)
	if downRelay != nil && peerID == downRelay.DownstreamPeer {
		// Data from downstream, forward to upstream
		fwdFrame := &protocol.Frame{
			Type:     protocol.FrameStreamData,
			StreamID: downRelay.UpstreamID,
			Flags:    frame.Flags,
			Payload:  frame.Payload,
		}
		a.peerMgr.SendToPeer(downRelay.UpstreamPeer, fwdFrame)
		return
	}

	// Check if this is an exit handler stream
	if a.exitHandler != nil && a.exitHandler.GetConnection(frame.StreamID) != nil {
		a.exitHandler.HandleStreamData(peerID, frame.StreamID, frame.Payload, frame.Flags)
		return
	}

	// Check if this is a tunnel handler stream
	if a.tunnelHandler != nil {
		if err := a.tunnelHandler.HandleStreamData(peerID, frame.StreamID, frame.Payload, frame.Flags); err == nil {
			return
		}
	}

	// Check if this is a file transfer stream
	if a.getFileTransferStream(frame.StreamID) != nil {
		a.handleFileTransferStreamData(peerID, frame.StreamID, frame.Payload, frame.Flags)
		return
	}

	// Check if this is a shell stream (where we are the target/server)
	if a.shellHandler != nil {
		a.shellHandler.HandleStreamData(peerID, frame.StreamID, frame.Payload, frame.Flags)
		// Note: shellHandler tracks its own streams and will ignore unknown stream IDs
	}

	// Check if this is a shell client stream (where we initiated to a remote shell)
	if a.handleShellClientData(frame.StreamID, frame.Payload, frame.Flags) {
		return
	}

	a.streamMgr.HandleStreamData(frame.StreamID, frame.Flags, frame.Payload)
}

// handleStreamClose processes a stream close.
func (a *Agent) handleStreamClose(peerID identity.AgentID, frame *protocol.Frame) {
	a.logger.Debug("handleStreamClose received",
		logging.KeyPeerID, peerID.ShortString(),
		logging.KeyStreamID, frame.StreamID)

	// Check if this is a relay stream - must check peerID to determine direction
	a.relayMu.Lock()
	upRelay := a.relayByUpstream[frame.StreamID]
	downRelay := a.relayByDownstream[frame.StreamID]

	// Check if close is from upstream
	if upRelay != nil && peerID == upRelay.UpstreamPeer {
		// Close from upstream, forward to downstream and clean up
		delete(a.relayByUpstream, frame.StreamID)
		delete(a.relayByDownstream, upRelay.DownstreamID)
		a.relayMu.Unlock()

		fwdFrame := &protocol.Frame{
			Type:     protocol.FrameStreamClose,
			StreamID: upRelay.DownstreamID,
		}
		a.peerMgr.SendToPeer(upRelay.DownstreamPeer, fwdFrame)
		return
	}

	// Check if close is from downstream
	if downRelay != nil && peerID == downRelay.DownstreamPeer {
		a.logger.Debug("handleStreamClose: forwarding from downstream to upstream",
			logging.KeyStreamID, frame.StreamID,
			"upstream_id", downRelay.UpstreamID,
			"upstream_peer", downRelay.UpstreamPeer.ShortString())
		// Close from downstream, forward to upstream and clean up
		delete(a.relayByUpstream, downRelay.UpstreamID)
		delete(a.relayByDownstream, frame.StreamID)
		a.relayMu.Unlock()

		fwdFrame := &protocol.Frame{
			Type:     protocol.FrameStreamClose,
			StreamID: downRelay.UpstreamID,
		}
		a.peerMgr.SendToPeer(downRelay.UpstreamPeer, fwdFrame)
		return
	}
	a.relayMu.Unlock()

	// Check if this is an exit handler stream
	if a.exitHandler != nil && a.exitHandler.GetConnection(frame.StreamID) != nil {
		a.exitHandler.HandleStreamClose(peerID, frame.StreamID)
		return
	}

	// Check if this is a tunnel handler stream
	if a.tunnelHandler != nil {
		a.tunnelHandler.HandleStreamClose(peerID, frame.StreamID)
	}

	// Check if this is a file transfer stream
	if a.getFileTransferStream(frame.StreamID) != nil {
		a.cleanupFileTransferStream(frame.StreamID)
		return
	}

	// Check if this is a shell stream (where we are the target/server)
	if a.shellHandler != nil {
		a.shellHandler.HandleStreamClose(frame.StreamID)
	}

	// Check if this is a shell client stream (where we initiated to a remote shell)
	if a.handleShellClientClose(frame.StreamID) {
		// Also clean up the stream manager entry (shell client streams are tracked in both places)
		a.streamMgr.HandleStreamClose(frame.StreamID)
		return
	}

	a.streamMgr.HandleStreamClose(frame.StreamID)
}

// handleStreamReset processes a stream reset.
func (a *Agent) handleStreamReset(peerID identity.AgentID, frame *protocol.Frame) {
	reset, err := protocol.DecodeStreamReset(frame.Payload)
	if err != nil {
		return
	}

	// Check if this is a relay stream - must check peerID to determine direction
	a.relayMu.Lock()
	upRelay := a.relayByUpstream[frame.StreamID]
	downRelay := a.relayByDownstream[frame.StreamID]

	// Check if reset is from upstream
	if upRelay != nil && peerID == upRelay.UpstreamPeer {
		// Reset from upstream, forward to downstream and clean up
		delete(a.relayByUpstream, frame.StreamID)
		delete(a.relayByDownstream, upRelay.DownstreamID)
		a.relayMu.Unlock()

		fwdFrame := &protocol.Frame{
			Type:     protocol.FrameStreamReset,
			StreamID: upRelay.DownstreamID,
			Payload:  frame.Payload,
		}
		a.peerMgr.SendToPeer(upRelay.DownstreamPeer, fwdFrame)
		return
	}

	// Check if reset is from downstream
	if downRelay != nil && peerID == downRelay.DownstreamPeer {
		// Reset from downstream, forward to upstream and clean up
		delete(a.relayByUpstream, downRelay.UpstreamID)
		delete(a.relayByDownstream, frame.StreamID)
		a.relayMu.Unlock()

		fwdFrame := &protocol.Frame{
			Type:     protocol.FrameStreamReset,
			StreamID: downRelay.UpstreamID,
			Payload:  frame.Payload,
		}
		a.peerMgr.SendToPeer(downRelay.UpstreamPeer, fwdFrame)
		return
	}
	a.relayMu.Unlock()

	// Check if this is an exit handler stream
	if a.exitHandler != nil && a.exitHandler.GetConnection(frame.StreamID) != nil {
		a.exitHandler.HandleStreamReset(peerID, frame.StreamID, reset.ErrorCode)
		return
	}

	// Check if this is a tunnel handler stream
	if a.tunnelHandler != nil {
		a.tunnelHandler.HandleStreamReset(peerID, frame.StreamID, reset.ErrorCode)
	}

	// Check if this is a file transfer stream
	if a.getFileTransferStream(frame.StreamID) != nil {
		a.cleanupFileTransferStream(frame.StreamID)
		return
	}

	a.streamMgr.HandleStreamReset(frame.StreamID, reset.ErrorCode)
}

// handleRouteAdvertise processes a route advertisement.
func (a *Agent) handleRouteAdvertise(peerID identity.AgentID, frame *protocol.Frame) {
	adv, err := protocol.DecodeRouteAdvertise(frame.Payload)
	if err != nil {
		a.logger.Debug("failed to decode route advertise",
			logging.KeyPeerID, peerID.ShortString(),
			logging.KeyError, err)
		return
	}

	// Log with available info (path may be encrypted)
	encrypted := adv.EncPath != nil && adv.EncPath.Encrypted
	a.logger.Debug("received route advertisement",
		logging.KeyPeerID, peerID.ShortString(),
		"origin", adv.OriginAgent.ShortString(),
		logging.KeyCount, len(adv.Routes),
		"encrypted", encrypted)

	a.flooder.HandleRouteAdvertise(peerID, adv.OriginAgent, adv.OriginDisplayName, adv.Sequence, adv.Routes, adv.EncPath, adv.SeenBy)
}

// handleRouteWithdraw processes a route withdrawal.
func (a *Agent) handleRouteWithdraw(peerID identity.AgentID, frame *protocol.Frame) {
	withdraw, err := protocol.DecodeRouteWithdraw(frame.Payload)
	if err != nil {
		return
	}

	a.flooder.HandleRouteWithdraw(peerID, withdraw.OriginAgent, withdraw.Sequence, withdraw.Routes, withdraw.SeenBy)
}

// handleNodeInfoAdvertise processes a node info advertisement.
func (a *Agent) handleNodeInfoAdvertise(peerID identity.AgentID, frame *protocol.Frame) {
	adv, err := protocol.DecodeNodeInfoAdvertise(frame.Payload)
	if err != nil {
		a.logger.Debug("failed to decode node info advertise",
			logging.KeyPeerID, peerID.ShortString(),
			logging.KeyError, err)
		return
	}

	// Log with available info (may be encrypted)
	encrypted := adv.EncInfo != nil && adv.EncInfo.Encrypted
	a.logger.Debug("received node info advertisement",
		logging.KeyPeerID, peerID.ShortString(),
		"origin", adv.OriginAgent.ShortString(),
		"encrypted", encrypted)

	a.flooder.HandleNodeInfoAdvertise(peerID, adv.OriginAgent, adv.Sequence, adv.EncInfo, adv.SeenBy)
}

// handleKeepalive processes a keepalive.
// Note: This is kept for completeness but is currently not called.
// The peer.Manager handles keepalive frames internally and sends acks
// before frames reach the agent's callback. See manager.go readLoop.
func (a *Agent) handleKeepalive(peerID identity.AgentID, frame *protocol.Frame) {
	ka, err := protocol.DecodeKeepalive(frame.Payload)
	if err != nil {
		return
	}

	// Send keepalive ack - uses the same Keepalive type with the original timestamp
	ackPayload := (&protocol.Keepalive{
		Timestamp: ka.Timestamp,
	}).Encode()

	ackFrame := &protocol.Frame{
		Type:     protocol.FrameKeepaliveAck,
		StreamID: protocol.ControlStreamID,
		Payload:  ackPayload,
	}

	a.peerMgr.SendToPeer(peerID, ackFrame)
}

// handleKeepaliveAck processes a keepalive ack.
// Note: This is kept for completeness but is currently not called.
// The peer.Manager handles keepalive acks internally and updates RTT
// before frames reach the agent's callback. See manager.go readLoop.
func (a *Agent) handleKeepaliveAck(peerID identity.AgentID, frame *protocol.Frame) {
	// RTT tracking is handled by peer.Manager.readLoop which calls
	// conn.UpdateRTT(ka.Timestamp) for FrameKeepaliveAck frames.
}

// handleControlRequest processes a CONTROL_REQUEST from a peer.
func (a *Agent) handleControlRequest(peerID identity.AgentID, frame *protocol.Frame) {
	req, err := protocol.DecodeControlRequest(frame.Payload)
	if err != nil {
		a.logger.Debug("failed to decode control request",
			logging.KeyPeerID, peerID.ShortString(),
			logging.KeyError, err)
		return
	}

	a.logger.Debug("received control request",
		"from", peerID.ShortString(),
		"request_id", req.RequestID,
		"type", req.ControlType,
		"target", req.TargetAgent.ShortString(),
		"path_len", len(req.Path))

	// Check if this request is for us or needs forwarding
	var zeroID identity.AgentID
	if req.TargetAgent != zeroID && req.TargetAgent != a.id {
		// Need to forward to target agent
		var nextHop identity.AgentID
		var remainingPath []identity.AgentID

		if len(req.Path) > 0 {
			// Use the path provided in the request
			nextHop = req.Path[0]
			remainingPath = req.Path[1:]
		} else {
			// Path is empty, look up route to target
			conn := a.peerMgr.GetPeer(req.TargetAgent)
			if conn != nil {
				// Target is a direct peer
				nextHop = req.TargetAgent
			} else {
				// Look up route
				routes := a.routeMgr.Table().GetRoutesFromAgent(req.TargetAgent)
				if len(routes) == 0 {
					a.sendControlResponse(peerID, req.RequestID, req.ControlType, false, []byte("no route to target"))
					return
				}
				nextHop = routes[0].NextHop
				// Calculate remaining path from route
				// route.Path is [local, ..., origin], we need path from nextHop to target
				rPath := routes[0].Path
				for i, id := range rPath {
					if id == nextHop && i+1 < len(rPath) {
						remainingPath = rPath[i+1:]
						break
					}
				}
			}
		}

		conn := a.peerMgr.GetPeer(nextHop)
		if conn == nil {
			a.sendControlResponse(peerID, req.RequestID, req.ControlType, false, []byte("next hop not connected"))
			return
		}

		// Track this forwarded request so we can route the response back
		a.controlMu.Lock()
		a.forwardedControl[req.RequestID] = &forwardedControlRequest{
			RequestID:  req.RequestID,
			SourcePeer: peerID,
		}
		a.controlMu.Unlock()

		// Forward the request
		a.logger.Debug("forwarding control request",
			"to", nextHop.ShortString(),
			"target", req.TargetAgent.ShortString(),
			"remaining_path", len(remainingPath),
			"source_peer", peerID.ShortString())

		fwdReq := &protocol.ControlRequest{
			RequestID:   req.RequestID,
			ControlType: req.ControlType,
			TargetAgent: req.TargetAgent,
			Path:        remainingPath,
			Data:        req.Data, // Preserve data payload for RPC and other control types
		}

		fwdFrame := &protocol.Frame{
			Type:     protocol.FrameControlRequest,
			StreamID: protocol.ControlStreamID,
			Payload:  fwdReq.Encode(),
		}
		a.peerMgr.SendToPeer(nextHop, fwdFrame)
		return
	}

	// Handle locally
	var data []byte
	var success bool

	switch req.ControlType {
	case protocol.ControlTypeStatus:
		data, success = a.getLocalStatus()
	case protocol.ControlTypePeers:
		data, success = a.getLocalPeers()
	case protocol.ControlTypeRoutes:
		data, success = a.getLocalRoutes()
	default:
		data = []byte("unknown control type")
		success = false
	}

	a.sendControlResponse(peerID, req.RequestID, req.ControlType, success, data)
}

// handleControlResponse processes a CONTROL_RESPONSE from a peer.
func (a *Agent) handleControlResponse(peerID identity.AgentID, frame *protocol.Frame) {
	resp, err := protocol.DecodeControlResponse(frame.Payload)
	if err != nil {
		a.logger.Debug("failed to decode control response",
			logging.KeyPeerID, peerID.ShortString(),
			logging.KeyError, err)
		return
	}

	a.logger.Debug("received control response",
		"from", peerID.ShortString(),
		"request_id", resp.RequestID,
		"success", resp.Success,
		"data_len", len(resp.Data))

	// Check if we have a pending request (we initiated) or a forwarded request (we relayed)
	a.controlMu.Lock()
	pending, hasPending := a.pendingControl[resp.RequestID]
	if hasPending {
		delete(a.pendingControl, resp.RequestID)
	}

	forwarded, hasForwarded := a.forwardedControl[resp.RequestID]
	if hasForwarded {
		delete(a.forwardedControl, resp.RequestID)
	}
	a.controlMu.Unlock()

	if hasPending && pending.ResponseCh != nil {
		// We initiated this request, deliver to the waiting caller
		select {
		case pending.ResponseCh <- resp:
		default:
			// Channel full or closed, ignore
		}
		return
	}

	if hasForwarded {
		// We forwarded this request, route response back to source peer
		a.logger.Debug("forwarding control response",
			"to", forwarded.SourcePeer.ShortString(),
			"request_id", resp.RequestID)

		responseFrame := &protocol.Frame{
			Type:     protocol.FrameControlResponse,
			StreamID: protocol.ControlStreamID,
			Payload:  resp.Encode(),
		}
		a.peerMgr.SendToPeer(forwarded.SourcePeer, responseFrame)
	}
}

// sendControlResponse sends a control response to a peer.
func (a *Agent) sendControlResponse(peerID identity.AgentID, requestID uint64, controlType uint8, success bool, data []byte) {
	a.logger.Debug("sending control response",
		"to", peerID.ShortString(),
		"request_id", requestID,
		"success", success,
		"data_len", len(data))

	resp := &protocol.ControlResponse{
		RequestID:   requestID,
		ControlType: controlType,
		Success:     success,
		Data:        data,
	}

	frame := &protocol.Frame{
		Type:     protocol.FrameControlResponse,
		StreamID: protocol.ControlStreamID,
		Payload:  resp.Encode(),
	}

	a.peerMgr.SendToPeer(peerID, frame)
}

// findControlPath finds the next hop and remaining path to reach a target agent for control requests.
// Returns the next hop ID and the path to include in the control request.
func (a *Agent) findControlPath(targetID identity.AgentID) (identity.AgentID, []identity.AgentID, error) {
	// Check if target is a direct peer
	if conn := a.peerMgr.GetPeer(targetID); conn != nil {
		return targetID, nil, nil
	}

	// Find route via routing table
	routes := a.routeMgr.Table().GetRoutesFromAgent(targetID)
	if len(routes) == 0 {
		return identity.AgentID{}, nil, fmt.Errorf("no route to agent %s", targetID.ShortString())
	}

	route := routes[0]
	nextHop := route.NextHop

	// route.Path is [local, hop1, hop2, ..., origin/target]
	// We need the path from nextHop to target for intermediate nodes to forward.
	var path []identity.AgentID
	for i, id := range route.Path {
		if id == nextHop && i+1 < len(route.Path) {
			path = make([]identity.AgentID, len(route.Path)-i-1)
			copy(path, route.Path[i+1:])
			break
		}
	}

	return nextHop, path, nil
}

// SendControlRequest sends a control request to a target agent and waits for response.
func (a *Agent) SendControlRequest(ctx context.Context, targetID identity.AgentID, controlType uint8) (*protocol.ControlResponse, error) {
	return a.SendControlRequestWithData(ctx, targetID, controlType, nil)
}

// SendControlRequestWithData sends a control request with data payload to a target agent.
func (a *Agent) SendControlRequestWithData(ctx context.Context, targetID identity.AgentID, controlType uint8, data []byte) (*protocol.ControlResponse, error) {
	nextHop, path, err := a.findControlPath(targetID)
	if err != nil {
		return nil, err
	}

	a.logger.Debug("sending control request",
		"target", targetID.ShortString(),
		"next_hop", nextHop.ShortString(),
		"path_len", len(path),
		"control_type", controlType,
		"data_len", len(data))

	// Generate request ID and create pending request
	a.controlMu.Lock()
	a.nextControlID++
	requestID := a.nextControlID
	pending := &pendingControlRequest{
		RequestID:   requestID,
		ControlType: controlType,
		ResponseCh:  make(chan *protocol.ControlResponse, 1),
		Timeout:     time.Now().Add(30 * time.Second),
	}
	a.pendingControl[requestID] = pending
	a.controlMu.Unlock()

	// Build and send request
	req := &protocol.ControlRequest{
		RequestID:   requestID,
		ControlType: controlType,
		TargetAgent: targetID,
		Path:        path,
		Data:        data,
	}

	frame := &protocol.Frame{
		Type:     protocol.FrameControlRequest,
		StreamID: protocol.ControlStreamID,
		Payload:  req.Encode(),
	}

	if err := a.peerMgr.SendToPeer(nextHop, frame); err != nil {
		a.controlMu.Lock()
		delete(a.pendingControl, requestID)
		a.controlMu.Unlock()
		return nil, fmt.Errorf("send control request: %w", err)
	}

	// Wait for response
	select {
	case resp := <-pending.ResponseCh:
		return resp, nil
	case <-ctx.Done():
		a.controlMu.Lock()
		delete(a.pendingControl, requestID)
		a.controlMu.Unlock()
		return nil, ctx.Err()
	}
}

// getLocalStatus returns the agent's status as JSON.
func (a *Agent) getLocalStatus() ([]byte, bool) {
	stats := a.Stats()
	data, err := json.Marshal(map[string]interface{}{
		"agent_id":       a.id.String(),
		"running":        a.running.Load(),
		"peer_count":     stats.PeerCount,
		"stream_count":   stats.StreamCount,
		"route_count":    stats.RouteCount,
		"socks5_running": stats.SOCKS5Running,
		"exit_running":   stats.ExitHandlerRun,
	})
	if err != nil {
		return []byte(err.Error()), false
	}
	return data, true
}

// getLocalPeers returns the list of connected peers.
func (a *Agent) getLocalPeers() ([]byte, bool) {
	peerIDs := a.peerMgr.GetPeerIDs()
	peers := make([]string, len(peerIDs))
	for i, id := range peerIDs {
		peers[i] = id.String()
	}
	data, err := json.Marshal(peers)
	if err != nil {
		return []byte(err.Error()), false
	}
	return data, true
}

// getLocalRoutes returns the routing table.
func (a *Agent) getLocalRoutes() ([]byte, bool) {
	routes := a.routeMgr.Table().GetAllRoutes()
	type routeInfo struct {
		Network  string `json:"network"`
		NextHop  string `json:"next_hop"`
		Origin   string `json:"origin"`
		Metric   int    `json:"metric"`
		HopCount int    `json:"hop_count"`
	}
	info := make([]routeInfo, len(routes))
	for i, r := range routes {
		info[i] = routeInfo{
			Network:  r.Network.String(),
			NextHop:  r.NextHop.ShortString(),
			Origin:   r.OriginAgent.ShortString(),
			Metric:   int(r.Metric),
			HopCount: len(r.Path),
		}
	}
	data, err := json.Marshal(info)
	if err != nil {
		return []byte(err.Error()), false
	}
	return data, true
}

// handlePeerDisconnect is called when a peer connection is closed.
// It cleans up any relay streams involving the disconnected peer.
func (a *Agent) handlePeerDisconnect(conn *peer.Connection, err error) {
	peerID := conn.RemoteID

	a.logger.Info("peer disconnected",
		logging.KeyPeerID, peerID.ShortString(),
		logging.KeyError, err)

	// Clean up relay streams involving this peer
	a.cleanupRelaysForPeer(peerID)
}

// cleanupRelaysForPeer removes all relay entries involving the specified peer.
func (a *Agent) cleanupRelaysForPeer(peerID identity.AgentID) {
	a.relayMu.Lock()
	defer a.relayMu.Unlock()

	var cleaned int
	for id, relay := range a.relayByUpstream {
		if relay.UpstreamPeer == peerID || relay.DownstreamPeer == peerID {
			// Remove from both maps
			delete(a.relayByUpstream, id)
			delete(a.relayByDownstream, relay.DownstreamID)
			cleaned++
		}
	}

	if cleaned > 0 {
		a.logger.Debug("cleaned up relay streams",
			logging.KeyPeerID, peerID.ShortString(),
			logging.KeyCount, cleaned)
	}
}

// Dial implements socks5.Dialer for SOCKS5 connections.
// This routes connections through the mesh network.
func (a *Agent) Dial(network, address string) (net.Conn, error) {
	return a.DialContext(context.Background(), network, address)
}

// DialContext implements socks5.Dialer for SOCKS5 connections with context support.
// This allows cancellation when the client disconnects during dial.
func (a *Agent) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	// Parse the address
	host, portStr, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("invalid address: %w", err)
	}

	port, err := net.LookupPort(network, portStr)
	if err != nil {
		return nil, fmt.Errorf("invalid port: %w", err)
	}

	// Check if host is already an IP address
	destIP := net.ParseIP(host)

	// If host is a domain, check domain routes BEFORE DNS resolution
	if destIP == nil {
		domainRoute := a.routeMgr.LookupDomain(host)
		if domainRoute != nil {
			// If domain route points to us (local exit), resolve DNS and dial directly
			if domainRoute.OriginAgent == a.id {
				ips, err := net.LookupIP(host)
				if err != nil {
					return nil, fmt.Errorf("resolve %s: %w", host, err)
				}
				if len(ips) == 0 {
					return nil, fmt.Errorf("no IP addresses for %s", host)
				}
				var dialer net.Dialer
				return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), portStr))
			}

			// Route via domain route - exit node will resolve DNS
			return a.dialViaDomainRouteWithContext(ctx, network, host, port, domainRoute)
		}

		// No domain route - resolve DNS at ingress
		ips, err := net.LookupIP(host)
		if err != nil {
			return nil, fmt.Errorf("resolve %s: %w", host, err)
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("no IP addresses for %s", host)
		}
		destIP = ips[0]
	}

	// Look up CIDR route in routing table
	route := a.routeMgr.Lookup(destIP)

	// If no route, or route is to ourselves (local exit), do direct dial
	if route == nil || route.OriginAgent == a.id {
		dialer := &net.Dialer{Timeout: directDialTimeout}
		return dialer.DialContext(ctx, network, address)
	}

	// Route through mesh - get next hop connection
	conn := a.peerMgr.GetPeer(route.NextHop)
	if conn == nil {
		// Next hop not connected, fall back to direct
		dialer := &net.Dialer{Timeout: directDialTimeout}
		return dialer.DialContext(ctx, network, address)
	}

	// Build the path for STREAM_OPEN
	var remainingPath []identity.AgentID
	if len(route.Path) > 1 {
		remainingPath = make([]identity.AgentID, len(route.Path)-1)
		copy(remainingPath, route.Path[1:])
	}

	// Generate stream ID
	streamID := conn.NextStreamID()

	// Build address bytes
	var addrType uint8
	var addrBytes []byte
	if ip4 := destIP.To4(); ip4 != nil {
		addrType = protocol.AddrTypeIPv4
		addrBytes = ip4
	} else {
		addrType = protocol.AddrTypeIPv6
		addrBytes = destIP
	}

	// Generate ephemeral keypair for E2E encryption key exchange
	ephPriv, ephPub, err := crypto.GenerateEphemeralKeypair()
	if err != nil {
		return nil, fmt.Errorf("generate ephemeral key: %w", err)
	}

	// Create the stream in stream manager
	pending := a.streamMgr.OpenStream(streamID, route.NextHop, host, uint16(port), 30*time.Second)

	// Store ephemeral keys in pending request for later key derivation
	a.streamMgr.SetPendingEphemeralKeys(pending.RequestID, ephPriv, ephPub)

	// Build and send STREAM_OPEN with ephemeral public key
	openPayload := &protocol.StreamOpen{
		RequestID:       pending.RequestID,
		AddressType:     addrType,
		Address:         addrBytes,
		Port:            uint16(port),
		RemainingPath:   remainingPath,
		EphemeralPubKey: ephPub,
	}

	frame := &protocol.Frame{
		Type:     protocol.FrameStreamOpen,
		StreamID: streamID,
		Payload:  openPayload.Encode(),
	}

	if err := a.peerMgr.SendToPeer(route.NextHop, frame); err != nil {
		a.streamMgr.CancelPendingRequest(pending.RequestID)
		return nil, fmt.Errorf("send stream open: %w", err)
	}

	// Wait for response with context support
	var result *stream.StreamOpenResult
	select {
	case result = <-pending.ResultCh:
		// Got result
	case <-ctx.Done():
		// Context cancelled - clean up and return
		a.streamMgr.CancelPendingRequest(pending.RequestID)
		crypto.ZeroKey(&ephPriv)
		return nil, ctx.Err()
	}

	if result.Error != nil {
		crypto.ZeroKey(&ephPriv)
		return nil, result.Error
	}

	// Derive session key from ECDH with exit node's ephemeral public key
	sharedSecret, err := crypto.ComputeECDH(ephPriv, result.RemoteEphemeral)
	if err != nil {
		crypto.ZeroKey(&ephPriv)
		return nil, fmt.Errorf("compute ECDH: %w", err)
	}

	crypto.ZeroKey(&ephPriv)

	// Derive session key - we are the initiator
	sessionKey := crypto.DeriveSessionKey(sharedSecret, pending.RequestID, ephPub, result.RemoteEphemeral, true)
	crypto.ZeroKey(&sharedSecret)

	// Store session key in stream
	result.Stream.SetSessionKey(sessionKey)

	// Return a MeshConn wrapper
	return &meshConn{
		agent:    a,
		stream:   result.Stream,
		peerID:   route.NextHop,
		streamID: streamID,
		localAddr: &net.TCPAddr{
			IP:   result.BoundIP,
			Port: int(result.BoundPort),
		},
		remoteAddr: &net.TCPAddr{
			IP:   destIP,
			Port: port,
		},
	}, nil
}

// dialViaDomainRoute routes a connection through a domain route.
// The domain name is passed to the exit node for DNS resolution.
func (a *Agent) dialViaDomainRoute(network, host string, port int, route *routing.DomainRoute) (net.Conn, error) {
	return a.dialViaDomainRouteWithContext(context.Background(), network, host, port, route)
}

// dialViaDomainRouteWithContext routes a connection through a domain route with context support.
func (a *Agent) dialViaDomainRouteWithContext(ctx context.Context, network, host string, port int, route *routing.DomainRoute) (net.Conn, error) {
	// Get next hop connection
	conn := a.peerMgr.GetPeer(route.NextHop)
	if conn == nil {
		return nil, fmt.Errorf("next hop %s not connected", route.NextHop.ShortString())
	}

	// Build the path for STREAM_OPEN
	var remainingPath []identity.AgentID
	if len(route.Path) > 1 {
		remainingPath = make([]identity.AgentID, len(route.Path)-1)
		copy(remainingPath, route.Path[1:])
	}

	// Generate stream ID
	streamID := conn.NextStreamID()

	// Build domain address bytes (length-prefixed string)
	addrBytes := make([]byte, 1+len(host))
	addrBytes[0] = byte(len(host))
	copy(addrBytes[1:], host)

	// Generate ephemeral keypair for E2E encryption key exchange
	ephPriv, ephPub, err := crypto.GenerateEphemeralKeypair()
	if err != nil {
		return nil, fmt.Errorf("generate ephemeral key: %w", err)
	}

	// Create the stream in stream manager
	pending := a.streamMgr.OpenStream(streamID, route.NextHop, host, uint16(port), 30*time.Second)

	// Store ephemeral keys in pending request for later key derivation
	a.streamMgr.SetPendingEphemeralKeys(pending.RequestID, ephPriv, ephPub)

	// Build and send STREAM_OPEN with domain address
	openPayload := &protocol.StreamOpen{
		RequestID:       pending.RequestID,
		AddressType:     protocol.AddrTypeDomain,
		Address:         addrBytes,
		Port:            uint16(port),
		RemainingPath:   remainingPath,
		EphemeralPubKey: ephPub,
	}

	frame := &protocol.Frame{
		Type:     protocol.FrameStreamOpen,
		StreamID: streamID,
		Payload:  openPayload.Encode(),
	}

	if err := a.peerMgr.SendToPeer(route.NextHop, frame); err != nil {
		a.streamMgr.CancelPendingRequest(pending.RequestID)
		return nil, fmt.Errorf("send stream open: %w", err)
	}

	// Wait for response with context support
	var result *stream.StreamOpenResult
	select {
	case result = <-pending.ResultCh:
		// Got result
	case <-ctx.Done():
		// Context cancelled - clean up and return
		a.streamMgr.CancelPendingRequest(pending.RequestID)
		crypto.ZeroKey(&ephPriv)
		return nil, ctx.Err()
	}

	if result.Error != nil {
		crypto.ZeroKey(&ephPriv)
		return nil, result.Error
	}

	// Derive session key from ECDH with exit node's ephemeral public key
	sharedSecret, err := crypto.ComputeECDH(ephPriv, result.RemoteEphemeral)
	if err != nil {
		crypto.ZeroKey(&ephPriv)
		return nil, fmt.Errorf("compute ECDH: %w", err)
	}

	crypto.ZeroKey(&ephPriv)

	// Derive session key - we are the initiator
	sessionKey := crypto.DeriveSessionKey(sharedSecret, pending.RequestID, ephPub, result.RemoteEphemeral, true)
	crypto.ZeroKey(&sharedSecret)

	// Store session key in stream
	result.Stream.SetSessionKey(sessionKey)

	// Return a MeshConn wrapper
	return &meshConn{
		agent:    a,
		stream:   result.Stream,
		peerID:   route.NextHop,
		streamID: streamID,
		localAddr: &net.TCPAddr{
			IP:   result.BoundIP,
			Port: int(result.BoundPort),
		},
		remoteAddr: &domainAddr{
			domain: host,
			port:   port,
		},
	}, nil
}

// DialTunnel implements tunnel.TunnelDialer for tunnel connections.
// This routes connections through the mesh network to a tunnel endpoint.
func (a *Agent) DialTunnel(ctx context.Context, key string) (net.Conn, error) {
	// Look up tunnel route
	route := a.routeMgr.LookupTunnel(key)
	if route == nil {
		return nil, fmt.Errorf("no route for tunnel: %s", key)
	}

	// Get next hop connection
	conn := a.peerMgr.GetPeer(route.NextHop)
	if conn == nil {
		return nil, fmt.Errorf("next hop %s not connected", route.NextHop.ShortString())
	}

	// Build the path for STREAM_OPEN
	var remainingPath []identity.AgentID
	if len(route.Path) > 1 {
		remainingPath = make([]identity.AgentID, len(route.Path)-1)
		copy(remainingPath, route.Path[1:])
	}

	// Generate stream ID
	streamID := conn.NextStreamID()

	// Build tunnel address (domain type with "tunnel:" prefix)
	tunnelAddr := protocol.TunnelStreamPrefix + key
	addrBytes := make([]byte, 1+len(tunnelAddr))
	addrBytes[0] = byte(len(tunnelAddr))
	copy(addrBytes[1:], tunnelAddr)

	// Generate ephemeral keypair for E2E encryption key exchange
	ephPriv, ephPub, err := crypto.GenerateEphemeralKeypair()
	if err != nil {
		return nil, fmt.Errorf("generate ephemeral key: %w", err)
	}

	// Create the stream in stream manager (port 0 for tunnel connections)
	pending := a.streamMgr.OpenStream(streamID, route.NextHop, tunnelAddr, 0, 30*time.Second)

	// Store ephemeral keys in pending request for later key derivation
	a.streamMgr.SetPendingEphemeralKeys(pending.RequestID, ephPriv, ephPub)

	// Build and send STREAM_OPEN with tunnel address
	openPayload := &protocol.StreamOpen{
		RequestID:       pending.RequestID,
		AddressType:     protocol.AddrTypeDomain,
		Address:         addrBytes,
		Port:            0, // Not used for tunnels
		RemainingPath:   remainingPath,
		EphemeralPubKey: ephPub,
	}

	frame := &protocol.Frame{
		Type:     protocol.FrameStreamOpen,
		StreamID: streamID,
		Payload:  openPayload.Encode(),
	}

	if err := a.peerMgr.SendToPeer(route.NextHop, frame); err != nil {
		a.streamMgr.CancelPendingRequest(pending.RequestID)
		return nil, fmt.Errorf("send stream open: %w", err)
	}

	// Wait for response with context support
	var result *stream.StreamOpenResult
	select {
	case result = <-pending.ResultCh:
		// Got result
	case <-ctx.Done():
		// Context cancelled - clean up and return
		a.streamMgr.CancelPendingRequest(pending.RequestID)
		crypto.ZeroKey(&ephPriv)
		return nil, ctx.Err()
	}

	if result.Error != nil {
		crypto.ZeroKey(&ephPriv)
		return nil, result.Error
	}

	// Derive session key from ECDH with exit node's ephemeral public key
	sharedSecret, err := crypto.ComputeECDH(ephPriv, result.RemoteEphemeral)
	if err != nil {
		crypto.ZeroKey(&ephPriv)
		return nil, fmt.Errorf("compute ECDH: %w", err)
	}

	crypto.ZeroKey(&ephPriv)

	// Derive session key - we are the initiator
	sessionKey := crypto.DeriveSessionKey(sharedSecret, pending.RequestID, ephPub, result.RemoteEphemeral, true)
	crypto.ZeroKey(&sharedSecret)

	// Store session key in stream
	result.Stream.SetSessionKey(sessionKey)

	// Return a MeshConn wrapper
	return &meshConn{
		agent:    a,
		stream:   result.Stream,
		peerID:   route.NextHop,
		streamID: streamID,
		localAddr: &net.TCPAddr{
			IP:   result.BoundIP,
			Port: int(result.BoundPort),
		},
		remoteAddr: &tunnelAddr_{
			key: key,
		},
	}, nil
}

// tunnelAddr_ implements net.Addr for tunnel connections.
type tunnelAddr_ struct {
	key string
}

func (t *tunnelAddr_) Network() string { return "tcp" }
func (t *tunnelAddr_) String() string  { return "tunnel:" + t.key }

// domainAddr implements net.Addr for domain-based connections.
type domainAddr struct {
	domain string
	port   int
}

func (d *domainAddr) Network() string { return "tcp" }
func (d *domainAddr) String() string  { return fmt.Sprintf("%s:%d", d.domain, d.port) }

// meshConn implements net.Conn for mesh-routed connections.
type meshConn struct {
	agent      *Agent
	stream     *stream.Stream
	peerID     identity.AgentID
	streamID   uint64
	localAddr  net.Addr
	remoteAddr net.Addr
	readBuf    []byte
	readOffset int

	// Deadlines for read/write operations
	mu            sync.Mutex
	readDeadline  time.Time
	writeDeadline time.Time
}

// Read reads data from the mesh connection.
func (c *meshConn) Read(b []byte) (int, error) {
	// If we have buffered data, return from that
	if c.readOffset < len(c.readBuf) {
		n := copy(b, c.readBuf[c.readOffset:])
		c.readOffset += n
		if c.readOffset >= len(c.readBuf) {
			c.readBuf = nil
			c.readOffset = 0
		}
		return n, nil
	}

	// Create context with deadline if set
	c.mu.Lock()
	deadline := c.readDeadline
	c.mu.Unlock()

	var ctx context.Context
	var cancel context.CancelFunc
	if !deadline.IsZero() {
		ctx, cancel = context.WithDeadline(context.Background(), deadline)
		defer cancel()
	} else {
		ctx = context.Background()
	}

	// Read new data from stream
	data, err := c.stream.Read(ctx)
	if err != nil {
		return 0, err
	}

	// Decrypt the received data
	sessionKey := c.stream.GetSessionKey()
	if sessionKey == nil {
		return 0, fmt.Errorf("no session key for stream %d", c.streamID)
	}

	plaintext, err := sessionKey.Decrypt(data)
	if err != nil {
		return 0, fmt.Errorf("decrypt: %w", err)
	}

	n := copy(b, plaintext)
	if n < len(plaintext) {
		c.readBuf = plaintext
		c.readOffset = n
	}
	return n, nil
}

// Write writes data to the mesh connection.
// Chunks data larger than MaxPayloadSize into multiple frames.
func (c *meshConn) Write(b []byte) (int, error) {
	if !c.stream.CanWrite() {
		return 0, fmt.Errorf("stream closed for writing")
	}

	if len(b) == 0 {
		return 0, nil
	}

	// Get session key for encryption
	sessionKey := c.stream.GetSessionKey()
	if sessionKey == nil {
		return 0, fmt.Errorf("no session key for stream %d", c.streamID)
	}

	// Account for encryption overhead when chunking
	maxPlaintext := protocol.MaxPayloadSize - crypto.EncryptionOverhead

	// Chunk data into max plaintext size pieces, encrypt each, and send
	for offset := 0; offset < len(b); {
		end := offset + maxPlaintext
		if end > len(b) {
			end = len(b)
		}

		chunk := b[offset:end]

		// Encrypt the chunk
		ciphertext, err := sessionKey.Encrypt(chunk)
		if err != nil {
			return offset, fmt.Errorf("encrypt: %w", err)
		}

		frame := &protocol.Frame{
			Type:     protocol.FrameStreamData,
			StreamID: c.streamID,
			Payload:  ciphertext,
		}

		if err := c.agent.peerMgr.SendToPeer(c.peerID, frame); err != nil {
			// Return bytes written so far
			return offset, err
		}

		offset = end
	}

	return len(b), nil
}

// Close closes the mesh connection.
func (c *meshConn) Close() error {
	// Send close frame
	frame := &protocol.Frame{
		Type:     protocol.FrameStreamClose,
		StreamID: c.streamID,
	}
	c.agent.peerMgr.SendToPeer(c.peerID, frame)

	return c.stream.Close()
}

// CloseWrite signals that we're done writing (half-close).
// Sends FIN_WRITE flag through the mesh to signal remote side.
func (c *meshConn) CloseWrite() error {
	// Get session key for encryption
	sessionKey := c.stream.GetSessionKey()
	if sessionKey == nil {
		return fmt.Errorf("no session key for stream %d", c.streamID)
	}

	// Send empty encrypted data with FIN_WRITE flag
	emptyEncrypted, err := sessionKey.Encrypt([]byte{})
	if err != nil {
		return fmt.Errorf("encrypt fin: %w", err)
	}

	frame := &protocol.Frame{
		Type:     protocol.FrameStreamData,
		Flags:    protocol.FlagFinWrite,
		StreamID: c.streamID,
		Payload:  emptyEncrypted,
	}

	if err := c.agent.peerMgr.SendToPeer(c.peerID, frame); err != nil {
		return err
	}

	// Update local stream state
	c.stream.CloseWrite()
	return nil
}

// LocalAddr returns the local address.
func (c *meshConn) LocalAddr() net.Addr {
	return c.localAddr
}

// RemoteAddr returns the remote address.
func (c *meshConn) RemoteAddr() net.Addr {
	return c.remoteAddr
}

// SetDeadline sets read and write deadlines.
func (c *meshConn) SetDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.readDeadline = t
	c.writeDeadline = t
	return nil
}

// SetReadDeadline sets the read deadline.
func (c *meshConn) SetReadDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.readDeadline = t
	return nil
}

// SetWriteDeadline sets the write deadline.
func (c *meshConn) SetWriteDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.writeDeadline = t
	return nil
}

// Stats returns agent statistics.
func (a *Agent) Stats() AgentStats {
	return AgentStats{
		PeerCount:      a.peerMgr.PeerCount(),
		StreamCount:    a.streamMgr.StreamCount(),
		RouteCount:     a.routeMgr.TotalRoutes(),
		SOCKS5Running:  a.socks5Srv != nil && a.socks5Srv.IsRunning(),
		ExitHandlerRun: a.exitHandler != nil && a.exitHandler.IsRunning(),
	}
}

// HealthStats returns health statistics for the health.StatsProvider interface.
func (a *Agent) HealthStats() health.Stats {
	return health.Stats{
		PeerCount:      a.peerMgr.PeerCount(),
		StreamCount:    a.streamMgr.StreamCount(),
		RouteCount:     a.routeMgr.TotalRoutes(),
		SOCKS5Running:  a.socks5Srv != nil && a.socks5Srv.IsRunning(),
		ExitHandlerRun: a.exitHandler != nil && a.exitHandler.IsRunning(),
	}
}

// agentStatsProvider adapts Agent to health.StatsProvider interface.
type agentStatsProvider struct {
	agent *Agent
}

// IsRunning implements health.StatsProvider.
func (p *agentStatsProvider) IsRunning() bool {
	return p.agent.IsRunning()
}

// Stats implements health.StatsProvider.
func (p *agentStatsProvider) Stats() health.Stats {
	return p.agent.HealthStats()
}

// AgentStats contains agent statistics.
type AgentStats struct {
	PeerCount      int
	StreamCount    int
	RouteCount     int
	SOCKS5Running  bool
	ExitHandlerRun bool
}

// GetRoutes returns all routes for debugging.
func (a *Agent) GetRoutes() []*routing.Route {
	return a.routeMgr.Table().GetAllRoutes()
}

// GetPeerIDs returns all connected peer IDs for debugging.
func (a *Agent) GetPeerIDs() []identity.AgentID {
	return a.peerMgr.GetPeerIDs()
}

// GetKnownAgentIDs returns all known agent IDs (peers and route origins).
// This implements health.RemoteMetricsProvider.
func (a *Agent) GetKnownAgentIDs() []identity.AgentID {
	seen := make(map[identity.AgentID]struct{})
	var result []identity.AgentID

	// Add connected peers
	for _, id := range a.peerMgr.GetPeerIDs() {
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			result = append(result, id)
		}
	}

	// Add route origins
	routes := a.routeMgr.Table().GetAllRoutes()
	for _, r := range routes {
		if _, ok := seen[r.OriginAgent]; !ok {
			seen[r.OriginAgent] = struct{}{}
			result = append(result, r.OriginAgent)
		}
	}

	return result
}

// GetPeerDetails returns detailed information about connected peers.
// This implements health.RemoteMetricsProvider.
func (a *Agent) GetPeerDetails() []health.PeerDetails {
	peers := a.peerMgr.GetAllPeers()
	details := make([]health.PeerDetails, len(peers))
	for i, p := range peers {
		displayName := p.RemoteDisplayName
		if displayName == "" {
			displayName = p.RemoteID.ShortString()
		}
		details[i] = health.PeerDetails{
			ID:          p.RemoteID,
			DisplayName: displayName,
			State:       p.State().String(),
			RTT:         p.RTT(),
			IsDialer:    p.IsDialer(),
			Transport:   string(p.TransportType()),
		}
	}
	return details
}

// GetRouteDetails returns detailed route information for the dashboard.
// This implements health.RemoteMetricsProvider.
func (a *Agent) GetRouteDetails() []health.RouteDetails {
	routes := a.routeMgr.Table().GetAllRoutes()
	details := make([]health.RouteDetails, len(routes))
	for i, r := range routes {
		// Copy the path slice to avoid sharing underlying array
		pathCopy := make([]identity.AgentID, len(r.Path))
		copy(pathCopy, r.Path)
		details[i] = health.RouteDetails{
			Network:  r.Network.String(),
			NextHop:  r.NextHop,
			Origin:   r.OriginAgent,
			Metric:   int(r.Metric),
			HopCount: len(r.Path),
			Path:     pathCopy,
		}
	}
	return details
}

// GetDomainRouteDetails returns detailed domain route information for the dashboard.
func (a *Agent) GetDomainRouteDetails() []health.DomainRouteDetails {
	routes := a.routeMgr.DomainTable().GetAllRoutes()
	details := make([]health.DomainRouteDetails, 0, len(routes))
	for _, r := range routes {
		// Copy the path slice to avoid sharing underlying array
		pathCopy := make([]identity.AgentID, len(r.Path))
		copy(pathCopy, r.Path)
		details = append(details, health.DomainRouteDetails{
			Pattern:    r.Pattern,
			IsWildcard: r.IsWildcard,
			NextHop:    r.NextHop,
			Origin:     r.OriginAgent,
			Metric:     int(r.Metric),
			HopCount:   len(r.Path),
			Path:       pathCopy,
		})
	}
	return details
}

// GetAllDisplayNames returns display names for all known agents.
// This implements health.RemoteMetricsProvider.
func (a *Agent) GetAllDisplayNames() map[identity.AgentID]string {
	return a.routeMgr.GetAllDisplayNames()
}

// GetAllNodeInfo returns node info for all known agents.
func (a *Agent) GetAllNodeInfo() map[identity.AgentID]*protocol.NodeInfo {
	return a.routeMgr.GetAllNodeInfo()
}

// GetLocalNodeInfo returns local node info.
func (a *Agent) GetLocalNodeInfo() *protocol.NodeInfo {
	return sysinfo.Collect(a.cfg.Agent.DisplayName, a.getPeerConnectionInfo(), a.keypair.PublicKey, a.getUDPConfig())
}

// getUDPConfig returns the UDP configuration for node info advertisements.
func (a *Agent) getUDPConfig() *sysinfo.UDPConfig {
	return &sysinfo.UDPConfig{
		Enabled: a.cfg.UDP.Enabled,
	}
}

// GetSOCKS5Info returns SOCKS5 configuration info for the dashboard.
func (a *Agent) GetSOCKS5Info() health.SOCKS5Info {
	return health.SOCKS5Info{
		Enabled: a.cfg.SOCKS5.Enabled,
		Address: a.cfg.SOCKS5.Address,
	}
}

// GetUDPInfo returns UDP relay configuration info for the dashboard.
func (a *Agent) GetUDPInfo() health.UDPInfo {
	return health.UDPInfo{
		Enabled: a.cfg.UDP.Enabled,
	}
}

// GetTunnelInfo returns tunnel configuration info for the dashboard.
func (a *Agent) GetTunnelInfo() health.TunnelInfo {
	info := health.TunnelInfo{}

	// Get listener keys and addresses
	for _, listener := range a.tunnelListeners {
		info.ListenerKeys = append(info.ListenerKeys, listener.Key())
		if addr := listener.Address(); addr != nil {
			info.ListenerAddresses = append(info.ListenerAddresses, addr.String())
		}
	}

	// Get endpoint keys from tunnel handler
	if a.tunnelHandler != nil {
		info.EndpointKeys = a.tunnelHandler.GetKeys()
	}

	return info
}

// GetTunnelRouteDetails returns detailed tunnel route information for the dashboard.
func (a *Agent) GetTunnelRouteDetails() []health.TunnelRouteDetails {
	localID := a.ID()
	var details []health.TunnelRouteDetails

	// Add local tunnel endpoints (this agent is the exit)
	if a.tunnelHandler != nil {
		for _, key := range a.tunnelHandler.GetKeys() {
			target, _ := a.tunnelHandler.GetTarget(key)
			details = append(details, health.TunnelRouteDetails{
				Key:      key,
				Target:   target,
				NextHop:  localID,
				Origin:   localID,
				Metric:   0,
				HopCount: 0,
				Path:     []identity.AgentID{},
				IsLocal:  true,
			})
		}
	}

	// Add remote tunnel routes from route table
	for _, route := range a.routeMgr.TunnelTable().GetAllRoutes() {
		// Skip local routes (already handled above)
		if route.OriginAgent == localID {
			continue
		}
		details = append(details, health.TunnelRouteDetails{
			Key:      route.Key,
			NextHop:  route.NextHop,
			Origin:   route.OriginAgent,
			Metric:   int(route.Metric),
			HopCount: len(route.Path),
			Path:     route.Path,
			IsLocal:  false,
		})
	}

	return details
}

// getPeerConnectionInfo returns peer connection info for NodeInfo advertisement.
func (a *Agent) getPeerConnectionInfo() []protocol.PeerConnectionInfo {
	peers := a.peerMgr.GetAllPeers()
	info := make([]protocol.PeerConnectionInfo, 0, len(peers))
	for _, p := range peers {
		var peerInfo protocol.PeerConnectionInfo
		copy(peerInfo.PeerID[:], p.RemoteID[:])
		peerInfo.Transport = string(p.TransportType())
		peerInfo.RTTMs = p.RTT().Milliseconds()
		peerInfo.IsDialer = p.IsDialer()
		info = append(info, peerInfo)
	}
	return info
}

// SOCKS5Address returns the SOCKS5 server address, or nil if not running.
func (a *Agent) SOCKS5Address() net.Addr {
	if a.socks5Srv == nil {
		return nil
	}
	return a.socks5Srv.Address()
}

// HealthServerAddress returns the HTTP health server address, or nil if not running.
func (a *Agent) HealthServerAddress() net.Addr {
	if a.healthServer == nil {
		return nil
	}
	return a.healthServer.Address()
}

// WriteStreamData implements exit.StreamWriter.
// Chunks data larger than MaxPayloadSize into multiple frames.
func (a *Agent) WriteStreamData(peerID identity.AgentID, streamID uint64, data []byte, flags uint8) error {
	// Handle empty data with flags (e.g., FIN_WRITE)
	if len(data) == 0 {
		frame := &protocol.Frame{
			Type:     protocol.FrameStreamData,
			StreamID: streamID,
			Flags:    flags,
			Payload:  nil,
		}
		return a.peerMgr.SendToPeer(peerID, frame)
	}

	// Chunk data into MaxPayloadSize pieces
	for offset := 0; offset < len(data); {
		end := offset + protocol.MaxPayloadSize
		if end > len(data) {
			end = len(data)
		}

		chunk := data[offset:end]
		isLast := end >= len(data)

		// Only set flags on the last chunk
		var chunkFlags uint8
		if isLast {
			chunkFlags = flags
		}

		frame := &protocol.Frame{
			Type:     protocol.FrameStreamData,
			StreamID: streamID,
			Flags:    chunkFlags,
			Payload:  chunk,
		}

		if err := a.peerMgr.SendToPeer(peerID, frame); err != nil {
			return err
		}

		offset = end
	}

	return nil
}

// WriteStreamOpenAck implements exit.StreamWriter.
func (a *Agent) WriteStreamOpenAck(peerID identity.AgentID, streamID uint64, requestID uint64, boundIP net.IP, boundPort uint16, ephemeralPubKey [crypto.KeySize]byte) error {
	var addrType uint8
	var addrBytes []byte
	if ip4 := boundIP.To4(); ip4 != nil {
		addrType = protocol.AddrTypeIPv4
		addrBytes = ip4
	} else if len(boundIP) == 16 {
		addrType = protocol.AddrTypeIPv6
		addrBytes = boundIP
	} else {
		addrType = protocol.AddrTypeIPv4
		addrBytes = []byte{0, 0, 0, 0}
	}

	ack := &protocol.StreamOpenAck{
		RequestID:       requestID,
		BoundAddrType:   addrType,
		BoundAddr:       addrBytes,
		BoundPort:       boundPort,
		EphemeralPubKey: ephemeralPubKey,
	}
	frame := &protocol.Frame{
		Type:     protocol.FrameStreamOpenAck,
		StreamID: streamID,
		Payload:  ack.Encode(),
	}
	return a.peerMgr.SendToPeer(peerID, frame)
}

// WriteStreamOpenErr implements exit.StreamWriter.
func (a *Agent) WriteStreamOpenErr(peerID identity.AgentID, streamID uint64, requestID uint64, errorCode uint16, message string) error {
	errPayload := &protocol.StreamOpenErr{
		RequestID: requestID,
		ErrorCode: errorCode,
		Message:   message,
	}
	frame := &protocol.Frame{
		Type:     protocol.FrameStreamOpenErr,
		StreamID: streamID,
		Payload:  errPayload.Encode(),
	}
	return a.peerMgr.SendToPeer(peerID, frame)
}

// WriteStreamClose implements exit.StreamWriter.
func (a *Agent) WriteStreamClose(peerID identity.AgentID, streamID uint64) error {
	frame := &protocol.Frame{
		Type:     protocol.FrameStreamClose,
		StreamID: streamID,
	}
	return a.peerMgr.SendToPeer(peerID, frame)
}

// deriveResponderSessionKey performs E2E key exchange for a responder (receiving a stream open).
// Returns the session key and our ephemeral public key, or an error.
func deriveResponderSessionKey(requestID uint64, remoteEphemeralPub [crypto.KeySize]byte) (*crypto.SessionKey, [crypto.KeySize]byte, error) {
	// Check for zero key (encryption required)
	var zeroKey [crypto.KeySize]byte
	if remoteEphemeralPub == zeroKey {
		return nil, zeroKey, fmt.Errorf("encryption required")
	}

	// Generate ephemeral keypair for E2E encryption
	ephPriv, ephPub, err := crypto.GenerateEphemeralKeypair()
	if err != nil {
		return nil, zeroKey, fmt.Errorf("key generation failed: %w", err)
	}

	// Compute shared secret from ECDH
	sharedSecret, err := crypto.ComputeECDH(ephPriv, remoteEphemeralPub)
	if err != nil {
		crypto.ZeroKey(&ephPriv)
		return nil, zeroKey, fmt.Errorf("key exchange failed: %w", err)
	}

	// Zero out ephemeral private key
	crypto.ZeroKey(&ephPriv)

	// Derive session key - we are the responder
	sessionKey := crypto.DeriveSessionKey(sharedSecret, requestID, remoteEphemeralPub, ephPub, false)
	crypto.ZeroKey(&sharedSecret)

	return sessionKey, ephPub, nil
}

// handleFileTransferStreamOpen is the common handler for file upload/download stream opens.
func (a *Agent) handleFileTransferStreamOpen(peerID identity.AgentID, streamID uint64, requestID uint64, remoteEphemeralPub [crypto.KeySize]byte, isUpload bool) {
	opName := "download"
	if isUpload {
		opName = "upload"
	}

	a.logger.Debug("file "+opName+" stream open",
		logging.KeyPeerID, peerID.ShortString(),
		logging.KeyStreamID, streamID,
		logging.KeyRequestID, requestID)

	// Check if file transfer is enabled
	if a.fileStreamHandler == nil {
		a.WriteStreamOpenErr(peerID, streamID, requestID, protocol.ErrFileTransferDenied, "file transfer disabled")
		return
	}

	// Perform E2E key exchange
	sessionKey, ephPub, err := deriveResponderSessionKey(requestID, remoteEphemeralPub)
	if err != nil {
		a.logger.Warn("rejecting file "+opName+" stream",
			logging.KeyPeerID, peerID.ShortString(),
			logging.KeyStreamID, streamID,
			logging.KeyError, err)
		a.WriteStreamOpenErr(peerID, streamID, requestID, protocol.ErrGeneralFailure, err.Error())
		return
	}

	// Create file transfer stream entry with session key
	fts := &fileTransferStream{
		StreamID:     streamID,
		PeerID:       peerID,
		RequestID:    requestID,
		IsUpload:     isUpload,
		MetaReceived: false,
		sessionKey:   sessionKey,
	}

	a.fileStreamsMu.Lock()
	a.fileStreams[streamID] = fts
	a.fileStreamsMu.Unlock()

	// Send ACK with our ephemeral public key for E2E encryption
	a.WriteStreamOpenAck(peerID, streamID, requestID, nil, 0, ephPub)
}

// handleFileUploadStreamOpen handles a file upload stream open request.
func (a *Agent) handleFileUploadStreamOpen(peerID identity.AgentID, streamID uint64, requestID uint64, remoteEphemeralPub [crypto.KeySize]byte) {
	a.handleFileTransferStreamOpen(peerID, streamID, requestID, remoteEphemeralPub, true)
}

// handleFileDownloadStreamOpen handles a file download stream open request.
func (a *Agent) handleFileDownloadStreamOpen(peerID identity.AgentID, streamID uint64, requestID uint64, remoteEphemeralPub [crypto.KeySize]byte) {
	a.handleFileTransferStreamOpen(peerID, streamID, requestID, remoteEphemeralPub, false)
}

// handleShellStreamOpen handles a shell stream open request.
func (a *Agent) handleShellStreamOpen(peerID identity.AgentID, streamID uint64, requestID uint64, interactive bool, remoteEphemeralPub [crypto.KeySize]byte) {
	modeStr := "normal"
	if interactive {
		modeStr = "interactive"
	}
	a.logger.Debug("shell stream open",
		logging.KeyPeerID, peerID.ShortString(),
		logging.KeyStreamID, streamID,
		logging.KeyRequestID, requestID,
		"mode", modeStr)

	// Delegate to shell handler - performs E2E key exchange
	errCode, localEphemeralPub := a.shellHandler.HandleStreamOpen(peerID, streamID, requestID, interactive, remoteEphemeralPub)
	if errCode != 0 {
		a.WriteStreamOpenErr(peerID, streamID, requestID, errCode, protocol.ErrorCodeName(errCode))
		return
	}

	// Send ACK with our ephemeral public key for E2E encryption
	a.WriteStreamOpenAck(peerID, streamID, requestID, nil, 0, localEphemeralPub)
}

// handleFileTransferStreamData processes data for a file transfer stream.
func (a *Agent) handleFileTransferStreamData(peerID identity.AgentID, streamID uint64, data []byte, flags uint8) {
	a.fileStreamsMu.RLock()
	fts := a.fileStreams[streamID]
	a.fileStreamsMu.RUnlock()

	if fts == nil || fts.Closed {
		return
	}

	// Decrypt data using E2E session key
	if fts.sessionKey == nil {
		a.logger.Error("no session key for file transfer stream",
			logging.KeyStreamID, streamID)
		a.closeFileTransferStream(streamID, protocol.ErrGeneralFailure, "no session key")
		return
	}

	plaintext, err := fts.sessionKey.Decrypt(data)
	if err != nil {
		a.logger.Error("failed to decrypt file transfer data",
			logging.KeyStreamID, streamID,
			logging.KeyError, err)
		a.closeFileTransferStream(streamID, protocol.ErrGeneralFailure, "decryption failed")
		return
	}

	// First data frame contains metadata
	if !fts.MetaReceived {
		meta, err := filetransfer.ParseMetadata(plaintext)
		if err != nil {
			a.logger.Error("invalid file transfer metadata",
				logging.KeyStreamID, streamID,
				logging.KeyError, err)
			a.closeFileTransferStream(streamID, protocol.ErrConnectionRefused, "invalid metadata")
			return
		}
		fts.Meta = meta
		fts.MetaReceived = true

		if fts.IsUpload {
			// Validate upload metadata
			if err := a.fileStreamHandler.ValidateUploadMetadata(meta); err != nil {
				a.logger.Error("file upload validation failed",
					logging.KeyStreamID, streamID,
					logging.KeyError, err)
				a.closeFileTransferStream(streamID, protocol.ErrNotAllowed, err.Error())
				return
			}

			// Create temp file for streaming upload (write directly to disk)
			tmpFile, err := os.CreateTemp("", "upload-stream-*")
			if err != nil {
				a.logger.Error("failed to create temp file for upload",
					logging.KeyStreamID, streamID,
					logging.KeyError, err)
				a.closeFileTransferStream(streamID, protocol.ErrWriteFailed, "failed to create temp file")
				return
			}
			fts.TempFile = tmpFile

			a.logger.Info("file upload started",
				"path", meta.Path,
				"size", meta.Size,
				"is_directory", meta.IsDirectory,
				"temp_file", tmpFile.Name())
		} else {
			// Validate download metadata and start sending file
			if err := a.fileStreamHandler.ValidateDownloadMetadata(meta); err != nil {
				a.logger.Error("file download validation failed",
					logging.KeyStreamID, streamID,
					logging.KeyError, err)
				a.closeFileTransferStream(streamID, protocol.ErrNotAllowed, err.Error())
				return
			}
			// Start goroutine to send file data
			go a.sendFileDownload(fts)
		}
		return
	}

	// Subsequent data frames contain file content (only for uploads)
	if fts.IsUpload && fts.TempFile != nil {
		// Write decrypted data directly to disk (streaming, no memory buffering)
		n, err := fts.TempFile.Write(plaintext)
		if err != nil {
			a.logger.Error("failed to write upload data to temp file",
				logging.KeyStreamID, streamID,
				logging.KeyError, err)
			a.closeFileTransferStream(streamID, protocol.ErrWriteFailed, "write failed")
			return
		}
		fts.BytesWritten += int64(n)

		// Check for FIN flag (end of upload)
		if flags&protocol.FlagFinWrite != 0 {
			go a.completeFileUpload(fts)
		}
	}
}

// completeFileUpload processes the uploaded data from temp file to final destination.
func (a *Agent) completeFileUpload(fts *fileTransferStream) {
	defer a.cleanupFileTransferStream(fts.StreamID)

	if fts.Meta == nil {
		a.logger.Error("file upload missing metadata", logging.KeyStreamID, fts.StreamID)
		return
	}

	if fts.TempFile == nil {
		a.logger.Error("file upload missing temp file", logging.KeyStreamID, fts.StreamID)
		return
	}

	// Close and reopen temp file for reading
	tmpPath := fts.TempFile.Name()
	fts.TempFile.Close()
	defer os.Remove(tmpPath) // Clean up temp file when done

	tmpFile, err := os.Open(tmpPath)
	if err != nil {
		a.logger.Error("failed to reopen temp file",
			logging.KeyStreamID, fts.StreamID,
			logging.KeyError, err)
		a.WriteStreamOpenErr(fts.PeerID, fts.StreamID, fts.RequestID, protocol.ErrWriteFailed, err.Error())
		return
	}
	defer tmpFile.Close()

	// Write file to final destination
	written, err := a.fileStreamHandler.WriteUploadedFile(
		fts.Meta.Path,
		tmpFile,
		fts.Meta.Mode,
		fts.Meta.IsDirectory,
		fts.Meta.Compress,
	)

	if err != nil {
		a.logger.Error("file upload write failed",
			logging.KeyStreamID, fts.StreamID,
			logging.KeyError, err)
		a.WriteStreamOpenErr(fts.PeerID, fts.StreamID, fts.RequestID, protocol.ErrWriteFailed, err.Error())
		return
	}

	a.logger.Info("file upload completed",
		"path", fts.Meta.Path,
		"bytes_received", fts.BytesWritten,
		"bytes_written", written)

	// Send close to signal completion
	a.WriteStreamClose(fts.PeerID, fts.StreamID)
}

// sendFileDownload sends file data to the requester.
func (a *Agent) sendFileDownload(fts *fileTransferStream) {
	defer a.cleanupFileTransferStream(fts.StreamID)

	if fts.Meta == nil {
		a.logger.Error("file download missing metadata", logging.KeyStreamID, fts.StreamID)
		return
	}

	// Check session key for E2E encryption
	if fts.sessionKey == nil {
		a.logger.Error("no session key for file download stream",
			logging.KeyStreamID, fts.StreamID)
		return
	}

	var reader io.Reader
	var size int64
	var mode uint32
	var isDir bool
	var err error
	var originalSize int64

	// Check if this is a resume request
	if fts.Meta.Offset > 0 {
		// Validate that file hasn't changed
		info, statErr := os.Stat(fts.Meta.Path)
		if statErr != nil {
			a.logger.Error("file download stat failed",
				logging.KeyStreamID, fts.StreamID,
				logging.KeyError, statErr)
			a.WriteStreamOpenErr(fts.PeerID, fts.StreamID, fts.RequestID, protocol.ErrFileNotFound, statErr.Error())
			return
		}

		originalSize = info.Size()

		// If original size was provided and doesn't match, reject resume
		if fts.Meta.OriginalSize > 0 && info.Size() != fts.Meta.OriginalSize {
			a.logger.Warn("file changed since partial download, rejecting resume",
				logging.KeyStreamID, fts.StreamID,
				"expected_size", fts.Meta.OriginalSize,
				"actual_size", info.Size())
			a.WriteStreamOpenErr(fts.PeerID, fts.StreamID, fts.RequestID, protocol.ErrResumeFailed, "file size changed")
			return
		}

		// Use offset-aware reader
		reader, size, mode, isDir, err = a.fileStreamHandler.ReadFileForDownloadAtOffset(
			fts.Meta.Path, fts.Meta.Offset, fts.Meta.Compress)
		if err != nil {
			a.logger.Error("file download read at offset failed",
				logging.KeyStreamID, fts.StreamID,
				"offset", fts.Meta.Offset,
				logging.KeyError, err)
			a.WriteStreamOpenErr(fts.PeerID, fts.StreamID, fts.RequestID, protocol.ErrGeneralFailure, err.Error())
			return
		}
	} else {
		// Normal download from beginning
		reader, size, mode, isDir, err = a.fileStreamHandler.ReadFileForDownload(fts.Meta.Path, fts.Meta.Compress)
		if err != nil {
			a.logger.Error("file download read failed",
				logging.KeyStreamID, fts.StreamID,
				logging.KeyError, err)
			a.WriteStreamOpenErr(fts.PeerID, fts.StreamID, fts.RequestID, protocol.ErrFileNotFound, err.Error())
			return
		}

		// Get original size for non-resume downloads
		if !isDir {
			if info, err := os.Stat(fts.Meta.Path); err == nil {
				originalSize = info.Size()
			}
		}
	}

	// Apply rate limiting if requested
	if fts.Meta.RateLimit > 0 {
		ctx := context.Background() // TODO: use proper context with cancellation
		reader = filetransfer.NewRateLimitedReader(ctx, reader, fts.Meta.RateLimit)
	}

	// Send response metadata as first data frame
	respMeta := &filetransfer.TransferMetadata{
		Path:         fts.Meta.Path,
		Mode:         mode,
		Size:         size,
		IsDirectory:  isDir,
		Compress:     fts.Meta.Compress,
		OriginalSize: originalSize, // Include original size for resume tracking
	}
	metaData, err := filetransfer.EncodeMetadata(respMeta)
	if err != nil {
		a.logger.Error("failed to encode response metadata",
			logging.KeyStreamID, fts.StreamID,
			logging.KeyError, err)
		return
	}
	// Encrypt metadata before sending
	encryptedMeta, err := fts.sessionKey.Encrypt(metaData)
	if err != nil {
		a.logger.Error("failed to encrypt response metadata",
			logging.KeyStreamID, fts.StreamID,
			logging.KeyError, err)
		return
	}
	a.WriteStreamData(fts.PeerID, fts.StreamID, encryptedMeta, 0)

	a.logger.Info("file download started",
		"path", fts.Meta.Path,
		"size", size,
		"offset", fts.Meta.Offset,
		"rate_limit", fts.Meta.RateLimit,
		"is_directory", isDir)

	// Stream file data in chunks
	// Leave room for encryption overhead (nonce + auth tag) plus protocol overhead
	buf := make([]byte, protocol.MaxPayloadSize-100-crypto.NonceSize-crypto.TagSize)
	for {
		n, readErr := reader.Read(buf)
		if n > 0 {
			// Encrypt file data before sending
			encryptedData, encErr := fts.sessionKey.Encrypt(buf[:n])
			if encErr != nil {
				a.logger.Error("failed to encrypt file data",
					logging.KeyStreamID, fts.StreamID,
					logging.KeyError, encErr)
				return
			}

			flags := uint8(0)
			if readErr == io.EOF {
				flags = protocol.FlagFinWrite
			}
			if err := a.WriteStreamData(fts.PeerID, fts.StreamID, encryptedData, flags); err != nil {
				a.logger.Error("failed to send file data",
					logging.KeyStreamID, fts.StreamID,
					logging.KeyError, err)
				return
			}
		}
		if readErr != nil {
			if readErr != io.EOF {
				a.logger.Error("file read error",
					logging.KeyStreamID, fts.StreamID,
					logging.KeyError, readErr)
			}
			break
		}
	}

	// Close any readers that implement io.Closer
	if closer, ok := reader.(io.Closer); ok {
		closer.Close()
	}

	a.logger.Info("file download completed", "path", fts.Meta.Path)

	// Send close to signal completion
	a.WriteStreamClose(fts.PeerID, fts.StreamID)
}

// closeFileTransferStream sends an error and cleans up.
func (a *Agent) closeFileTransferStream(streamID uint64, errCode uint16, message string) {
	a.fileStreamsMu.RLock()
	fts := a.fileStreams[streamID]
	a.fileStreamsMu.RUnlock()

	if fts == nil {
		return
	}

	// If stream has a session key, it's already open and we need to send
	// an encrypted error response instead of STREAM_OPEN_ERR
	if fts.sessionKey != nil {
		// Send error as encrypted metadata response
		errMeta := &filetransfer.TransferMetadata{
			Error: message,
		}
		metaData, err := filetransfer.EncodeMetadata(errMeta)
		if err == nil {
			encryptedMeta, encErr := fts.sessionKey.Encrypt(metaData)
			if encErr == nil {
				a.WriteStreamData(fts.PeerID, fts.StreamID, encryptedMeta, protocol.FlagFinWrite)
			}
		}
		a.WriteStreamClose(fts.PeerID, streamID)
	} else {
		// Stream not yet open, use STREAM_OPEN_ERR
		a.WriteStreamOpenErr(fts.PeerID, streamID, fts.RequestID, errCode, message)
	}
	a.cleanupFileTransferStream(streamID)
}

// cleanupFileTransferStream removes a file transfer stream from tracking.
func (a *Agent) cleanupFileTransferStream(streamID uint64) {
	a.fileStreamsMu.Lock()
	fts, ok := a.fileStreams[streamID]
	if ok {
		fts.Closed = true
		delete(a.fileStreams, streamID)
	}
	a.fileStreamsMu.Unlock()

	// Clean up temp file if it exists (outside lock to avoid holding it during I/O)
	if ok && fts.TempFile != nil {
		tmpPath := fts.TempFile.Name()
		fts.TempFile.Close()
		os.Remove(tmpPath)
	}
}

// getFileTransferStream returns a file transfer stream by ID.
func (a *Agent) getFileTransferStream(streamID uint64) *fileTransferStream {
	a.fileStreamsMu.RLock()
	defer a.fileStreamsMu.RUnlock()
	return a.fileStreams[streamID]
}

// ============================================================================
// File Transfer Client Methods (initiating transfers to remote agents)
// ============================================================================

// UploadFile uploads a local file or directory to a remote agent via stream-based transfer.
// The transfer uses the mesh network to reach the target agent.
func (a *Agent) UploadFile(ctx context.Context, targetID identity.AgentID, localPath, remotePath string, opts health.TransferOptions, progress health.FileTransferProgress) error {
	// Check if file transfer is enabled locally (for validation config)
	if a.fileStreamHandler == nil {
		return fmt.Errorf("file transfer is disabled")
	}

	// Get file info
	info, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("cannot access local path: %w", err)
	}

	// Find path to target agent
	nextHop, remainingPath, conn, err := a.findPathToAgent(targetID)
	if err != nil {
		return fmt.Errorf("no route to agent %s: %w", targetID.ShortString(), err)
	}

	// Allocate stream ID
	streamID := conn.NextStreamID()

	// Generate ephemeral keypair for E2E encryption key exchange
	ephPriv, ephPub, err := crypto.GenerateEphemeralKeypair()
	if err != nil {
		return fmt.Errorf("generate ephemeral key: %w", err)
	}

	// Create pending stream (5 min timeout for large files)
	pending := a.streamMgr.OpenStream(streamID, nextHop, protocol.FileTransferUpload, 0, 5*time.Minute)

	// Store ephemeral keys in pending request for later key derivation
	a.streamMgr.SetPendingEphemeralKeys(pending.RequestID, ephPriv, ephPub)

	// Build and send STREAM_OPEN with ephemeral public key
	// Domain addresses need length prefix byte
	domainAddr := protocol.FileTransferUpload
	domainBytes := append([]byte{byte(len(domainAddr))}, []byte(domainAddr)...)
	openPayload := &protocol.StreamOpen{
		RequestID:       pending.RequestID,
		AddressType:     protocol.AddrTypeDomain,
		Address:         domainBytes,
		Port:            0,
		RemainingPath:   remainingPath,
		EphemeralPubKey: ephPub,
	}

	frame := &protocol.Frame{
		Type:     protocol.FrameStreamOpen,
		StreamID: streamID,
		Payload:  openPayload.Encode(),
	}

	if err := a.peerMgr.SendToPeer(nextHop, frame); err != nil {
		crypto.ZeroKey(&ephPriv)
		return fmt.Errorf("send stream open: %w", err)
	}

	// Wait for STREAM_OPEN_ACK
	var result *stream.StreamOpenResult
	select {
	case result = <-pending.ResultCh:
		if result.Error != nil {
			crypto.ZeroKey(&ephPriv)
			return fmt.Errorf("stream open failed: %w", result.Error)
		}
	case <-ctx.Done():
		crypto.ZeroKey(&ephPriv)
		return ctx.Err()
	}

	// Derive session key from ECDH with remote agent's ephemeral public key
	sharedSecret, err := crypto.ComputeECDH(ephPriv, result.RemoteEphemeral)
	if err != nil {
		crypto.ZeroKey(&ephPriv)
		return fmt.Errorf("compute ECDH: %w", err)
	}

	// Zero out ephemeral private key after computing shared secret
	crypto.ZeroKey(&ephPriv)

	// Derive session key - we are the initiator
	sessionKey := crypto.DeriveSessionKey(sharedSecret, pending.RequestID, ephPub, result.RemoteEphemeral, true)
	crypto.ZeroKey(&sharedSecret)

	// Build metadata
	var fileSize int64 = -1
	isDirectory := info.IsDir()
	var fileMode uint32 = uint32(info.Mode().Perm())
	if !isDirectory {
		fileSize = info.Size()
	}

	meta := &filetransfer.TransferMetadata{
		Path:        remotePath,
		Mode:        fileMode,
		Size:        fileSize,
		IsDirectory: isDirectory,
		Password:    opts.Password,
		Compress:    true,
		RateLimit:   opts.RateLimit,
	}

	// Encode and encrypt metadata
	metaData, err := filetransfer.EncodeMetadata(meta)
	if err != nil {
		a.WriteStreamClose(nextHop, streamID)
		return fmt.Errorf("encode metadata: %w", err)
	}

	encryptedMeta, err := sessionKey.Encrypt(metaData)
	if err != nil {
		a.WriteStreamClose(nextHop, streamID)
		return fmt.Errorf("encrypt metadata: %w", err)
	}

	if err := a.WriteStreamData(nextHop, streamID, encryptedMeta, 0); err != nil {
		return fmt.Errorf("send metadata: %w", err)
	}

	// Brief check for immediate server rejection (e.g., file too large, path not allowed)
	// Server validates metadata and may send error response before we start streaming
	s := a.streamMgr.GetStream(streamID)
	if s != nil {
		responseData, readErr := s.ReadWithTimeout(200 * time.Millisecond)
		if readErr == nil && len(responseData) > 0 {
			// Got response data - check if it's an error
			decryptedResponse, decErr := sessionKey.Decrypt(responseData)
			if decErr == nil {
				responseMeta, parseErr := filetransfer.ParseMetadata(decryptedResponse)
				if parseErr == nil && responseMeta.Error != "" {
					a.WriteStreamClose(nextHop, streamID)
					return fmt.Errorf("remote error: %s", responseMeta.Error)
				}
			}
		}
		// Timeout or no data is normal - server hasn't rejected yet
	}

	// Stream file/directory content with E2E encryption
	var written int64
	if isDirectory {
		// Tar and stream directory
		pr, pw := io.Pipe()

		// Start tar in goroutine
		go func() {
			err := filetransfer.TarDirectory(localPath, pw)
			if err != nil {
				pw.CloseWithError(err)
			} else {
				pw.Close()
			}
		}()

		// Apply rate limiting if requested
		var reader io.Reader = pr
		if opts.RateLimit > 0 {
			reader = filetransfer.NewRateLimitedReader(ctx, pr, opts.RateLimit)
		}

		// Stream tar data in chunks with encryption
		written, err = a.streamFileContent(ctx, nextHop, streamID, reader, -1, progress, sessionKey)
		if err != nil {
			pr.Close()
			return fmt.Errorf("stream directory: %w", err)
		}
	} else {
		// Open file and optionally compress
		f, err := os.Open(localPath)
		if err != nil {
			a.WriteStreamClose(nextHop, streamID)
			return fmt.Errorf("open file: %w", err)
		}
		defer f.Close()

		// Compress with gzip if requested
		if meta.Compress {
			pr, pw := io.Pipe()
			go func() {
				gzw := gzip.NewWriter(pw)
				_, copyErr := io.Copy(gzw, f)
				gzw.Close()
				if copyErr != nil {
					pw.CloseWithError(copyErr)
				} else {
					pw.Close()
				}
			}()

			// Apply rate limiting if requested
			var reader io.Reader = pr
			if opts.RateLimit > 0 {
				reader = filetransfer.NewRateLimitedReader(ctx, pr, opts.RateLimit)
			}

			written, err = a.streamFileContent(ctx, nextHop, streamID, reader, -1, progress, sessionKey)
			if err != nil {
				pr.Close()
				return fmt.Errorf("stream file: %w", err)
			}
		} else {
			// Apply rate limiting if requested
			var reader io.Reader = f
			if opts.RateLimit > 0 {
				reader = filetransfer.NewRateLimitedReader(ctx, f, opts.RateLimit)
			}

			written, err = a.streamFileContent(ctx, nextHop, streamID, reader, fileSize, progress, sessionKey)
			if err != nil {
				return fmt.Errorf("stream file: %w", err)
			}
		}
	}

	a.logger.Info("file upload completed",
		"target", targetID.ShortString(),
		"local_path", localPath,
		"remote_path", remotePath,
		"bytes_sent", written)

	// Wait for stream to close (server sends STREAM_CLOSE on completion)
	finalStream := a.streamMgr.GetStream(streamID)
	if finalStream != nil {
		select {
		case <-finalStream.Done():
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(30 * time.Second):
			// Timeout waiting for close acknowledgement
		}
	}

	return nil
}

// DownloadFile downloads a file or directory from a remote agent via stream-based transfer.
func (a *Agent) DownloadFile(ctx context.Context, targetID identity.AgentID, remotePath, localPath string, opts health.TransferOptions, progress health.FileTransferProgress) error {
	// Check if file transfer is enabled locally
	if a.fileStreamHandler == nil {
		return fmt.Errorf("file transfer is disabled")
	}

	// Find path to target agent
	nextHop, remainingPath, conn, err := a.findPathToAgent(targetID)
	if err != nil {
		return fmt.Errorf("no route to agent %s: %w", targetID.ShortString(), err)
	}

	// Allocate stream ID
	streamID := conn.NextStreamID()

	// Generate ephemeral keypair for E2E encryption key exchange
	ephPriv, ephPub, err := crypto.GenerateEphemeralKeypair()
	if err != nil {
		return fmt.Errorf("generate ephemeral key: %w", err)
	}

	// Create pending stream (5 min timeout for large files)
	pending := a.streamMgr.OpenStream(streamID, nextHop, protocol.FileTransferDownload, 0, 5*time.Minute)

	// Store ephemeral keys in pending request for later key derivation
	a.streamMgr.SetPendingEphemeralKeys(pending.RequestID, ephPriv, ephPub)

	// Build and send STREAM_OPEN with ephemeral public key
	// Domain addresses need length prefix byte
	downloadDomainAddr := protocol.FileTransferDownload
	downloadDomainBytes := append([]byte{byte(len(downloadDomainAddr))}, []byte(downloadDomainAddr)...)
	openPayload := &protocol.StreamOpen{
		RequestID:       pending.RequestID,
		AddressType:     protocol.AddrTypeDomain,
		Address:         downloadDomainBytes,
		Port:            0,
		RemainingPath:   remainingPath,
		EphemeralPubKey: ephPub,
	}

	frame := &protocol.Frame{
		Type:     protocol.FrameStreamOpen,
		StreamID: streamID,
		Payload:  openPayload.Encode(),
	}

	if err := a.peerMgr.SendToPeer(nextHop, frame); err != nil {
		crypto.ZeroKey(&ephPriv)
		return fmt.Errorf("send stream open: %w", err)
	}

	// Wait for STREAM_OPEN_ACK
	var openResult *stream.StreamOpenResult
	select {
	case result := <-pending.ResultCh:
		if result.Error != nil {
			crypto.ZeroKey(&ephPriv)
			return fmt.Errorf("stream open failed: %w", result.Error)
		}
		openResult = result
	case <-ctx.Done():
		crypto.ZeroKey(&ephPriv)
		return ctx.Err()
	}

	// Derive session key from ECDH with remote agent's ephemeral public key
	sharedSecret, err := crypto.ComputeECDH(ephPriv, openResult.RemoteEphemeral)
	if err != nil {
		crypto.ZeroKey(&ephPriv)
		return fmt.Errorf("compute ECDH: %w", err)
	}

	// Zero out ephemeral private key after computing shared secret
	crypto.ZeroKey(&ephPriv)

	// Derive session key - we are the initiator
	sessionKey := crypto.DeriveSessionKey(sharedSecret, pending.RequestID, ephPub, openResult.RemoteEphemeral, true)
	crypto.ZeroKey(&sharedSecret)

	// Send encrypted request metadata (path to download)
	meta := &filetransfer.TransferMetadata{
		Path:         remotePath,
		Password:     opts.Password,
		Compress:     true,
		RateLimit:    opts.RateLimit,
		Offset:       opts.Offset,
		OriginalSize: opts.OriginalSize,
	}

	metaData, err := filetransfer.EncodeMetadata(meta)
	if err != nil {
		a.WriteStreamClose(nextHop, streamID)
		return fmt.Errorf("encode metadata: %w", err)
	}

	encryptedMeta, err := sessionKey.Encrypt(metaData)
	if err != nil {
		a.WriteStreamClose(nextHop, streamID)
		return fmt.Errorf("encrypt metadata: %w", err)
	}

	if err := a.WriteStreamData(nextHop, streamID, encryptedMeta, 0); err != nil {
		return fmt.Errorf("send metadata: %w", err)
	}

	// Wait for response metadata (first data frame) and decrypt
	s := openResult.Stream
	responseData, err := s.ReadWithTimeout(30 * time.Second)
	if err != nil {
		return fmt.Errorf("read response metadata: %w", err)
	}

	decryptedResponse, err := sessionKey.Decrypt(responseData)
	if err != nil {
		return fmt.Errorf("decrypt response metadata: %w", err)
	}

	responseMeta, err := filetransfer.ParseMetadata(decryptedResponse)
	if err != nil {
		return fmt.Errorf("parse response metadata: %w", err)
	}

	a.logger.Info("file download started",
		"target", targetID.ShortString(),
		"remote_path", remotePath,
		"local_path", localPath,
		"size", responseMeta.Size,
		"is_directory", responseMeta.IsDirectory)

	// Receive file data with E2E decryption and write to local path
	var written int64
	if responseMeta.IsDirectory {
		// Receive tar stream and extract
		written, err = a.receiveAndExtractDirectory(ctx, s, localPath, responseMeta.Size, progress, sessionKey)
		if err != nil {
			return fmt.Errorf("receive directory: %w", err)
		}
	} else {
		// Receive file data
		written, err = a.receiveAndWriteFile(ctx, s, localPath, responseMeta.Mode, responseMeta.Size, responseMeta.Compress, progress, sessionKey)
		if err != nil {
			return fmt.Errorf("receive file: %w", err)
		}
	}

	a.logger.Info("file download completed",
		"target", targetID.ShortString(),
		"remote_path", remotePath,
		"local_path", localPath,
		"bytes_received", written)

	// Send STREAM_CLOSE to acknowledge completion
	a.WriteStreamClose(nextHop, streamID)

	return nil
}

// DownloadFileStream opens a streaming download from a remote agent.
// Returns a reader that streams file data directly without writing to disk.
// The caller must call Close() on the result when done.
func (a *Agent) DownloadFileStream(ctx context.Context, targetID identity.AgentID, remotePath string, opts health.TransferOptions) (*health.DownloadStreamResult, error) {
	// Check if file transfer is enabled locally
	if a.fileStreamHandler == nil {
		return nil, fmt.Errorf("file transfer is disabled")
	}

	// Find path to target agent
	nextHop, remainingPath, conn, err := a.findPathToAgent(targetID)
	if err != nil {
		return nil, fmt.Errorf("no route to agent %s: %w", targetID.ShortString(), err)
	}

	// Allocate stream ID
	streamID := conn.NextStreamID()

	// Generate ephemeral keypair for E2E encryption key exchange
	ephPriv, ephPub, err := crypto.GenerateEphemeralKeypair()
	if err != nil {
		return nil, fmt.Errorf("generate ephemeral key: %w", err)
	}

	// Create pending stream (5 min timeout for large files)
	pending := a.streamMgr.OpenStream(streamID, nextHop, protocol.FileTransferDownload, 0, 5*time.Minute)

	// Store ephemeral keys in pending request for later key derivation
	a.streamMgr.SetPendingEphemeralKeys(pending.RequestID, ephPriv, ephPub)

	// Build and send STREAM_OPEN with ephemeral public key
	downloadDomainAddr := protocol.FileTransferDownload
	downloadDomainBytes := append([]byte{byte(len(downloadDomainAddr))}, []byte(downloadDomainAddr)...)
	openPayload := &protocol.StreamOpen{
		RequestID:       pending.RequestID,
		AddressType:     protocol.AddrTypeDomain,
		Address:         downloadDomainBytes,
		Port:            0,
		RemainingPath:   remainingPath,
		EphemeralPubKey: ephPub,
	}

	frame := &protocol.Frame{
		Type:     protocol.FrameStreamOpen,
		StreamID: streamID,
		Payload:  openPayload.Encode(),
	}

	if err := a.peerMgr.SendToPeer(nextHop, frame); err != nil {
		crypto.ZeroKey(&ephPriv)
		return nil, fmt.Errorf("send stream open: %w", err)
	}

	// Wait for STREAM_OPEN_ACK
	var openResult *stream.StreamOpenResult
	select {
	case result := <-pending.ResultCh:
		if result.Error != nil {
			crypto.ZeroKey(&ephPriv)
			return nil, fmt.Errorf("stream open failed: %w", result.Error)
		}
		openResult = result
	case <-ctx.Done():
		crypto.ZeroKey(&ephPriv)
		return nil, ctx.Err()
	}

	// Derive session key from ECDH with remote agent's ephemeral public key
	sharedSecret, err := crypto.ComputeECDH(ephPriv, openResult.RemoteEphemeral)
	if err != nil {
		crypto.ZeroKey(&ephPriv)
		return nil, fmt.Errorf("compute ECDH: %w", err)
	}

	// Zero out ephemeral private key after computing shared secret
	crypto.ZeroKey(&ephPriv)

	// Derive session key - we are the initiator
	sessionKey := crypto.DeriveSessionKey(sharedSecret, pending.RequestID, ephPub, openResult.RemoteEphemeral, true)
	crypto.ZeroKey(&sharedSecret)

	// Send encrypted request metadata (path to download)
	meta := &filetransfer.TransferMetadata{
		Path:         remotePath,
		Password:     opts.Password,
		Compress:     true,
		RateLimit:    opts.RateLimit,
		Offset:       opts.Offset,
		OriginalSize: opts.OriginalSize,
	}

	metaData, err := filetransfer.EncodeMetadata(meta)
	if err != nil {
		a.WriteStreamClose(nextHop, streamID)
		return nil, fmt.Errorf("encode metadata: %w", err)
	}

	encryptedMeta, err := sessionKey.Encrypt(metaData)
	if err != nil {
		a.WriteStreamClose(nextHop, streamID)
		return nil, fmt.Errorf("encrypt metadata: %w", err)
	}

	if err := a.WriteStreamData(nextHop, streamID, encryptedMeta, 0); err != nil {
		return nil, fmt.Errorf("send metadata: %w", err)
	}

	// Wait for response metadata (first data frame) and decrypt
	s := openResult.Stream
	responseData, err := s.ReadWithTimeout(30 * time.Second)
	if err != nil {
		a.WriteStreamClose(nextHop, streamID)
		return nil, fmt.Errorf("read response metadata: %w", err)
	}

	decryptedResponse, err := sessionKey.Decrypt(responseData)
	if err != nil {
		a.WriteStreamClose(nextHop, streamID)
		return nil, fmt.Errorf("decrypt response metadata: %w", err)
	}

	responseMeta, err := filetransfer.ParseMetadata(decryptedResponse)
	if err != nil {
		a.WriteStreamClose(nextHop, streamID)
		return nil, fmt.Errorf("parse response metadata: %w", err)
	}

	// Check for error response from server
	if responseMeta.Error != "" {
		a.WriteStreamClose(nextHop, streamID)
		return nil, fmt.Errorf("remote error: %s", responseMeta.Error)
	}

	a.logger.Info("file download stream started",
		"target", targetID.ShortString(),
		"remote_path", remotePath,
		"size", responseMeta.Size,
		"original_size", responseMeta.OriginalSize,
		"is_directory", responseMeta.IsDirectory,
		"compressed", responseMeta.Compress)

	// Create a stream reader that reads from the stream with E2E decryption
	reader := &streamReader{
		stream:     s,
		ctx:        ctx,
		compressed: responseMeta.Compress,
		sessionKey: sessionKey,
	}

	// Cleanup function to close the stream when done
	cleanup := func() {
		a.WriteStreamClose(nextHop, streamID)
	}

	return &health.DownloadStreamResult{
		Reader:       reader,
		Size:         responseMeta.Size,
		OriginalSize: responseMeta.OriginalSize,
		Mode:         responseMeta.Mode,
		IsDirectory:  responseMeta.IsDirectory,
		Compressed:   responseMeta.Compress,
		Close:        cleanup,
	}, nil
}

// streamReader wraps a stream for io.Reader interface with E2E decryption.
type streamReader struct {
	stream     *stream.Stream
	ctx        context.Context
	compressed bool
	gzReader   io.ReadCloser
	buffer     []byte
	bufOffset  int
	eof        bool
	sessionKey *crypto.SessionKey
}

func (r *streamReader) Read(p []byte) (int, error) {
	// If we have a gzip reader, read from it
	if r.gzReader != nil {
		return r.gzReader.Read(p)
	}

	// If we have buffered data, return it first
	if r.bufOffset < len(r.buffer) {
		n := copy(p, r.buffer[r.bufOffset:])
		r.bufOffset += n
		return n, nil
	}

	if r.eof {
		return 0, io.EOF
	}

	// Read from stream
	data, err := r.stream.ReadWithTimeout(30 * time.Second)
	if err != nil {
		if err == io.EOF {
			r.eof = true
		}
		return 0, err
	}

	// Decrypt data using E2E session key
	if r.sessionKey != nil {
		plaintext, decErr := r.sessionKey.Decrypt(data)
		if decErr != nil {
			return 0, fmt.Errorf("decrypt stream data: %w", decErr)
		}
		data = plaintext
	}

	// If compressed and this is the first read, set up gzip reader
	if r.compressed && r.gzReader == nil {
		// We need to create a gzip reader, but it needs an io.Reader
		// Create a pipe to feed data to the gzip reader
		pr, pw := io.Pipe()

		// Start goroutine to feed data to the pipe
		go func() {
			// Write the first chunk (already decrypted)
			pw.Write(data)

			// Continue reading from stream and writing to pipe
			for {
				chunk, err := r.stream.ReadWithTimeout(30 * time.Second)
				if err != nil {
					if err == io.EOF {
						pw.Close()
					} else {
						pw.CloseWithError(err)
					}
					return
				}
				// Decrypt chunk using E2E session key
				if r.sessionKey != nil {
					plaintext, decErr := r.sessionKey.Decrypt(chunk)
					if decErr != nil {
						pw.CloseWithError(fmt.Errorf("decrypt stream chunk: %w", decErr))
						return
					}
					chunk = plaintext
				}
				if _, err := pw.Write(chunk); err != nil {
					pw.CloseWithError(err)
					return
				}
			}
		}()

		gzr, err := gzip.NewReader(pr)
		if err != nil {
			pr.Close()
			return 0, fmt.Errorf("create gzip reader: %w", err)
		}
		r.gzReader = gzr
		return r.gzReader.Read(p)
	}

	// Not compressed, return data directly
	r.buffer = data
	r.bufOffset = 0
	n := copy(p, r.buffer)
	r.bufOffset = n
	return n, nil
}

func (r *streamReader) Close() error {
	if r.gzReader != nil {
		return r.gzReader.Close()
	}
	return nil
}

// findPathToAgent finds the next hop and remaining path to reach a specific agent by ID.
// Returns: nextHop ID, remaining path (for STREAM_OPEN), peer connection, error.
func (a *Agent) findPathToAgent(targetID identity.AgentID) (identity.AgentID, []identity.AgentID, *peer.Connection, error) {
	// Check if target is a direct peer
	conn := a.peerMgr.GetPeer(targetID)
	if conn != nil {
		// Direct connection, no path needed
		return targetID, nil, conn, nil
	}

	// Look up in routing table - find a route where OriginAgent matches targetID
	allRoutes := a.routeMgr.GetFullRoutesForAdvertise(identity.AgentID{}) // Get all routes
	var bestRoute *routing.Route
	for _, route := range allRoutes {
		if route.OriginAgent == targetID {
			if bestRoute == nil || route.Metric < bestRoute.Metric {
				bestRoute = route
			}
		}
	}

	if bestRoute == nil {
		return identity.AgentID{}, nil, nil, fmt.Errorf("agent %s not found in routing table", targetID.ShortString())
	}

	// Get connection to next hop
	conn = a.peerMgr.GetPeer(bestRoute.NextHop)
	if conn == nil {
		return identity.AgentID{}, nil, nil, fmt.Errorf("next hop %s not connected", bestRoute.NextHop.ShortString())
	}

	// Build remaining path (skip first entry which is next hop)
	var remainingPath []identity.AgentID
	if len(bestRoute.Path) > 1 {
		remainingPath = make([]identity.AgentID, len(bestRoute.Path)-1)
		copy(remainingPath, bestRoute.Path[1:])
	}

	return bestRoute.NextHop, remainingPath, conn, nil
}

// streamFileContent streams data from a reader to the peer in chunks with E2E encryption.
func (a *Agent) streamFileContent(ctx context.Context, peerID identity.AgentID, streamID uint64, r io.Reader, totalSize int64, progress health.FileTransferProgress, sessionKey *crypto.SessionKey) (int64, error) {
	// Leave room for frame overhead and encryption overhead (nonce + auth tag)
	buf := make([]byte, protocol.MaxPayloadSize-100-crypto.EncryptionOverhead)
	var totalWritten int64

	for {
		select {
		case <-ctx.Done():
			return totalWritten, ctx.Err()
		default:
		}

		n, readErr := r.Read(buf)
		if n > 0 {
			// Encrypt data before sending
			encryptedData, encErr := sessionKey.Encrypt(buf[:n])
			if encErr != nil {
				return totalWritten, fmt.Errorf("encrypt data: %w", encErr)
			}

			flags := uint8(0)
			if readErr == io.EOF {
				flags = protocol.FlagFinWrite
			}

			if err := a.WriteStreamData(peerID, streamID, encryptedData, flags); err != nil {
				return totalWritten, fmt.Errorf("write data: %w", err)
			}
			totalWritten += int64(n)

			if progress != nil {
				progress(totalWritten, totalSize)
			}
		}

		if readErr != nil {
			if readErr == io.EOF {
				// Ensure FIN_WRITE is sent even if last read was empty
				if n == 0 {
					// Send empty encrypted payload with FIN flag
					emptyEncrypted, encErr := sessionKey.Encrypt(nil)
					if encErr != nil {
						return totalWritten, fmt.Errorf("encrypt fin: %w", encErr)
					}
					if err := a.WriteStreamData(peerID, streamID, emptyEncrypted, protocol.FlagFinWrite); err != nil {
						return totalWritten, fmt.Errorf("write fin: %w", err)
					}
				}
				break
			}
			return totalWritten, fmt.Errorf("read error: %w", readErr)
		}
	}

	return totalWritten, nil
}

// receiveEncryptedStreamData receives and decrypts data from a stream into a buffer.
// Returns the total bytes received (decrypted).
func (a *Agent) receiveEncryptedStreamData(ctx context.Context, s *stream.Stream, sessionKey *crypto.SessionKey, totalSize int64, progress health.FileTransferProgress) (*bytes.Buffer, int64, error) {
	var dataBuf bytes.Buffer
	var totalReceived int64

	for {
		select {
		case <-ctx.Done():
			return &dataBuf, totalReceived, ctx.Err()
		default:
		}

		data, err := s.ReadWithTimeout(30 * time.Second)
		if err != nil {
			if err == io.EOF {
				break
			}
			// Try to drain remaining data if stream is closed
			if s.IsClosed() {
				for {
					select {
					case remaining := <-s.ReadBuffer():
						plaintext, decErr := sessionKey.Decrypt(remaining)
						if decErr != nil {
							return &dataBuf, totalReceived, fmt.Errorf("decrypt remaining data: %w", decErr)
						}
						dataBuf.Write(plaintext)
						totalReceived += int64(len(plaintext))
					default:
						return &dataBuf, totalReceived, nil
					}
				}
			}
			return &dataBuf, totalReceived, fmt.Errorf("read stream: %w", err)
		}

		if len(data) == 0 {
			continue
		}

		plaintext, decErr := sessionKey.Decrypt(data)
		if decErr != nil {
			return &dataBuf, totalReceived, fmt.Errorf("decrypt data: %w", decErr)
		}

		dataBuf.Write(plaintext)
		totalReceived += int64(len(plaintext))

		if progress != nil {
			progress(totalReceived, totalSize)
		}
	}

	return &dataBuf, totalReceived, nil
}

// receiveAndWriteFile receives file data from a stream, decrypts with E2E session key, and writes to disk.
func (a *Agent) receiveAndWriteFile(ctx context.Context, s *stream.Stream, localPath string, mode uint32, totalSize int64, compressed bool, progress health.FileTransferProgress, sessionKey *crypto.SessionKey) (int64, error) {
	// Create parent directories
	dir := filepath.Dir(localPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return 0, fmt.Errorf("create directory: %w", err)
	}

	// Create the file
	f, err := os.OpenFile(localPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(mode))
	if err != nil {
		return 0, fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	// Receive encrypted data
	dataBuf, _, err := a.receiveEncryptedStreamData(ctx, s, sessionKey, totalSize, progress)
	if err != nil {
		return 0, err
	}

	// Decompress if needed and write to file
	var reader io.Reader = dataBuf
	if compressed {
		gzr, err := gzip.NewReader(dataBuf)
		if err != nil {
			return 0, fmt.Errorf("create gzip reader: %w", err)
		}
		defer gzr.Close()
		reader = gzr
	}

	written, err := io.Copy(f, reader)
	if err != nil {
		return 0, fmt.Errorf("write file: %w", err)
	}

	return written, nil
}

// receiveAndExtractDirectory receives tar stream data, decrypts with E2E session key, and extracts to a directory.
func (a *Agent) receiveAndExtractDirectory(ctx context.Context, s *stream.Stream, localPath string, totalSize int64, progress health.FileTransferProgress, sessionKey *crypto.SessionKey) (int64, error) {
	// Receive encrypted data
	dataBuf, _, err := a.receiveEncryptedStreamData(ctx, s, sessionKey, totalSize, progress)
	if err != nil {
		return 0, err
	}

	// Extract tar.gz to directory
	if err := filetransfer.UntarDirectory(dataBuf, localPath); err != nil {
		return 0, fmt.Errorf("extract directory: %w", err)
	}

	// Calculate extracted size
	size, _ := filetransfer.CalculateDirectorySize(localPath)
	return size, nil
}

// OpenShellStream opens a shell stream to a remote agent.
// Implements health.ShellProvider interface.
func (a *Agent) OpenShellStream(ctx context.Context, targetID identity.AgentID, meta *shell.ShellMeta, interactive bool) (*health.ShellSession, error) {
	// Find route to target using existing helper
	nextHop, remainingPath, conn, err := a.findPathToAgent(targetID)
	if err != nil {
		return nil, fmt.Errorf("no route to agent %s: %w", targetID.ShortString(), err)
	}

	// Determine shell address based on mode
	destAddr := protocol.ShellStream
	if interactive {
		destAddr = protocol.ShellInteractive
	}

	// Allocate stream ID from connection
	streamID := conn.NextStreamID()

	// Generate ephemeral keypair for E2E encryption key exchange
	ephPriv, ephPub, err := crypto.GenerateEphemeralKeypair()
	if err != nil {
		return nil, fmt.Errorf("generate ephemeral key: %w", err)
	}

	// Create pending stream request
	pending := a.streamMgr.OpenStream(streamID, nextHop, destAddr, 0, a.cfg.Limits.StreamOpenTimeout)

	// Store ephemeral keys in pending request for later key derivation
	a.streamMgr.SetPendingEphemeralKeys(pending.RequestID, ephPriv, ephPub)

	// Create adapter for this client session
	adapter := health.NewShellStreamAdapter(streamID, targetID, func() {
		a.cleanupShellClientStream(streamID)
	})

	// Register the client stream
	a.shellClientMu.Lock()
	a.shellClientStreams[streamID] = adapter
	a.shellClientMu.Unlock()

	// Build domain address with length prefix (same as file transfer)
	domainBytes := append([]byte{byte(len(destAddr))}, []byte(destAddr)...)

	// Build and send STREAM_OPEN with ephemeral public key for E2E encryption
	openPayload := &protocol.StreamOpen{
		RequestID:       pending.RequestID,
		AddressType:     protocol.AddrTypeDomain,
		Address:         domainBytes,
		Port:            0,
		RemainingPath:   remainingPath,
		EphemeralPubKey: ephPub,
	}

	frame := &protocol.Frame{
		Type:     protocol.FrameStreamOpen,
		StreamID: streamID,
		Payload:  openPayload.Encode(),
	}

	if err := a.peerMgr.SendToPeer(nextHop, frame); err != nil {
		crypto.ZeroKey(&ephPriv)
		a.cleanupShellClientStream(streamID)
		return nil, fmt.Errorf("send stream open: %w", err)
	}

	// Wait for STREAM_OPEN_ACK
	var result *stream.StreamOpenResult
	select {
	case <-ctx.Done():
		crypto.ZeroKey(&ephPriv)
		a.cleanupShellClientStream(streamID)
		return nil, ctx.Err()
	case result = <-pending.ResultCh:
		if result.Error != nil {
			crypto.ZeroKey(&ephPriv)
			a.cleanupShellClientStream(streamID)
			return nil, result.Error
		}
		// Stream opened successfully
	}

	// Derive session key from ECDH with remote agent's ephemeral public key
	sharedSecret, err := crypto.ComputeECDH(ephPriv, result.RemoteEphemeral)
	if err != nil {
		crypto.ZeroKey(&ephPriv)
		a.cleanupShellClientStream(streamID)
		return nil, fmt.Errorf("compute ECDH: %w", err)
	}

	// Zero out ephemeral private key after computing shared secret
	crypto.ZeroKey(&ephPriv)

	// Derive session key - we are the initiator
	// Use requestID (not streamID) because streamID changes at each relay hop
	sessionKey := crypto.DeriveSessionKey(sharedSecret, pending.RequestID, ephPub, result.RemoteEphemeral, true)
	crypto.ZeroKey(&sharedSecret)

	// Store session key in adapter for E2E encryption
	adapter.SetSessionKey(sessionKey)

	// Encode metadata with shell message format (MsgMeta prefix)
	metaBytes, err := shell.EncodeMeta(meta)
	if err != nil {
		a.cleanupShellClientStream(streamID)
		return nil, fmt.Errorf("encode shell metadata: %w", err)
	}

	// Encrypt metadata before sending
	encryptedMeta, err := sessionKey.Encrypt(metaBytes)
	if err != nil {
		a.cleanupShellClientStream(streamID)
		return nil, fmt.Errorf("encrypt metadata: %w", err)
	}

	// Send encrypted metadata as first data frame
	dataFrame := &protocol.Frame{
		Type:     protocol.FrameStreamData,
		StreamID: streamID,
		Payload:  encryptedMeta,
	}
	if err := a.peerMgr.SendToPeer(nextHop, dataFrame); err != nil {
		a.cleanupShellClientStream(streamID)
		return nil, fmt.Errorf("send metadata: %w", err)
	}

	// Store next hop for sending data
	adapter.SetNextHop(nextHop, a.peerMgr)

	// Start goroutine to forward data from adapter.Send to peer
	go a.forwardShellClientData(streamID, nextHop, adapter)

	a.logger.Debug("shell client stream opened",
		logging.KeyStreamID, streamID,
		"target", targetID.ShortString(),
		"interactive", interactive)

	return adapter.ToSession(), nil
}

// forwardShellClientData forwards data from the shell client adapter to the peer.
func (a *Agent) forwardShellClientData(streamID uint64, nextHop identity.AgentID, adapter *health.ShellStreamAdapter) {
	sessionKey := adapter.GetSessionKey()
	if sessionKey == nil {
		a.logger.Error("no session key for shell client stream",
			logging.KeyStreamID, streamID)
		adapter.Close()
		return
	}

	for {
		data, ok := adapter.PopSend()
		if !ok {
			return // Adapter closed
		}

		// Encrypt data before sending
		encryptedData, err := sessionKey.Encrypt(data)
		if err != nil {
			a.logger.Error("failed to encrypt shell client data",
				logging.KeyStreamID, streamID,
				logging.KeyError, err)
			adapter.Close()
			return
		}

		frame := &protocol.Frame{
			Type:     protocol.FrameStreamData,
			StreamID: streamID,
			Payload:  encryptedData,
		}
		if err := a.peerMgr.SendToPeer(nextHop, frame); err != nil {
			a.logger.Debug("shell client send error",
				logging.KeyStreamID, streamID,
				logging.KeyError, err)
			adapter.Close()
			return
		}
	}
}

// cleanupShellClientStream cleans up a shell client stream.
// This is called from adapter.Close()'s closeFunc callback.
func (a *Agent) cleanupShellClientStream(streamID uint64) {
	a.shellClientMu.Lock()
	delete(a.shellClientStreams, streamID)
	a.shellClientMu.Unlock()
}

// handleShellClientData handles incoming data for a shell client stream.
func (a *Agent) handleShellClientData(streamID uint64, data []byte, flags uint8) bool {
	a.shellClientMu.RLock()
	adapter := a.shellClientStreams[streamID]
	a.shellClientMu.RUnlock()

	if adapter == nil {
		a.logger.Debug("handleShellClientData: no adapter", logging.KeyStreamID, streamID)
		return false
	}

	// Decrypt incoming data using E2E session key
	sessionKey := adapter.GetSessionKey()
	if sessionKey == nil {
		a.logger.Error("handleShellClientData: no session key",
			logging.KeyStreamID, streamID)
		adapter.Close()
		return false
	}

	plaintext, err := sessionKey.Decrypt(data)
	if err != nil {
		a.logger.Error("handleShellClientData: decryption failed",
			logging.KeyStreamID, streamID,
			logging.KeyError, err)
		adapter.Close()
		return false
	}

	a.logger.Debug("handleShellClientData: pushing data", logging.KeyStreamID, streamID, "bytes", len(plaintext))
	// Push decrypted data to adapter for reading by HTTP handler
	adapter.PushReceive(plaintext)

	// Check for exit code in flags or parse from data
	// Exit code is sent as last frame with FIN flag
	if flags&protocol.FlagFinWrite != 0 {
		adapter.Close()
	}

	return true
}

// handleShellClientClose handles close of a shell client stream.
func (a *Agent) handleShellClientClose(streamID uint64) bool {
	a.shellClientMu.RLock()
	adapter := a.shellClientStreams[streamID]
	a.shellClientMu.RUnlock()

	if adapter == nil {
		a.logger.Debug("handleShellClientClose: no adapter found", logging.KeyStreamID, streamID)
		return false
	}

	a.logger.Debug("handleShellClientClose: closing adapter", logging.KeyStreamID, streamID)
	// adapter.Close() will call cleanupShellClientStream via closeFunc
	adapter.Close()
	return true
}
