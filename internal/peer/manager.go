// Package peer manages peer connections and handshakes for Muti Metroo.
package peer

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/postalsys/muti-metroo/internal/identity"
	"github.com/postalsys/muti-metroo/internal/logging"
	"github.com/postalsys/muti-metroo/internal/protocol"
	"github.com/postalsys/muti-metroo/internal/recovery"
	"github.com/postalsys/muti-metroo/internal/transport"
)

// PeerInfo contains information about a configured peer.
type PeerInfo struct {
	Address      string
	ExpectedID   identity.AgentID
	Capabilities []string
	Persistent   bool // If true, auto-reconnect on disconnect
	DialOptions  *transport.DialOptions
}

// ManagerConfig contains configuration for the peer manager.
type ManagerConfig struct {
	LocalID           identity.AgentID
	DisplayName       string
	Capabilities      []string
	Transport         transport.Transport
	DialOptions       transport.DialOptions
	HandshakeTimeout  time.Duration
	KeepaliveInterval time.Duration
	KeepaliveTimeout  time.Duration
	ReconnectConfig   ReconnectConfig
	Logger            *slog.Logger
	OnPeerConnected   func(*Connection)
	OnPeerDisconnect  func(*Connection, error)
	OnFrame           func(*Connection, *protocol.Frame)
}

// DefaultManagerConfig returns a config with sensible defaults.
func DefaultManagerConfig(localID identity.AgentID, tr transport.Transport) ManagerConfig {
	return ManagerConfig{
		LocalID:           localID,
		Capabilities:      []string{},
		Transport:         tr,
		HandshakeTimeout:  10 * time.Second,
		KeepaliveInterval: 30 * time.Second,
		KeepaliveTimeout:  10 * time.Second,
		ReconnectConfig:   DefaultReconnectConfig(),
	}
}

// Manager manages connections to multiple peers.
type Manager struct {
	cfg        ManagerConfig
	handshaker *Handshaker
	logger     *slog.Logger

	mu          sync.RWMutex
	peers       map[identity.AgentID]*Connection
	peerInfos   map[string]*PeerInfo // Address -> PeerInfo
	reconnector *Reconnector

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewManager creates a new peer manager.
func NewManager(cfg ManagerConfig) *Manager {
	ctx, cancel := context.WithCancel(context.Background())

	logger := cfg.Logger
	if logger == nil {
		logger = logging.NopLogger()
	}

	m := &Manager{
		cfg:        cfg,
		handshaker: NewHandshaker(cfg.LocalID, cfg.DisplayName, cfg.Capabilities, cfg.HandshakeTimeout),
		logger:     logger,
		peers:      make(map[identity.AgentID]*Connection),
		peerInfos:  make(map[string]*PeerInfo),
		ctx:        ctx,
		cancel:     cancel,
	}

	// Create reconnector with callback to this manager
	m.reconnector = NewReconnector(cfg.ReconnectConfig, m.handleReconnect)

	return m
}

// AddPeer adds a peer configuration to the manager.
func (m *Manager) AddPeer(info PeerInfo) {
	m.mu.Lock()
	m.peerInfos[info.Address] = &info
	m.mu.Unlock()
}

// RemovePeer removes a peer configuration.
func (m *Manager) RemovePeer(addr string) {
	m.mu.Lock()
	delete(m.peerInfos, addr)
	m.mu.Unlock()
}

// Connect initiates a connection to a peer at the given address.
func (m *Manager) Connect(ctx context.Context, addr string) (*Connection, error) {
	m.mu.RLock()
	info := m.peerInfos[addr]
	m.mu.RUnlock()

	var expectedID identity.AgentID
	if info != nil {
		expectedID = info.ExpectedID
	}

	connCfg := ConnectionConfig{
		LocalID:          m.cfg.LocalID,
		ExpectedPeerID:   expectedID,
		Capabilities:     m.cfg.Capabilities,
		HandshakeTimeout: m.cfg.HandshakeTimeout,
		OnFrame:          m.cfg.OnFrame,
		OnDisconnect:     m.handleDisconnect,
	}

	// Use per-peer DialOptions if available, otherwise use manager defaults
	dialOpts := m.cfg.DialOptions
	if info != nil && info.DialOptions != nil {
		dialOpts = *info.DialOptions
	}

	conn, err := m.handshaker.DialAndHandshake(ctx, m.cfg.Transport, addr, connCfg, dialOpts)
	if err != nil {
		// Schedule reconnect if this is a persistent peer
		if info != nil && info.Persistent {
			m.reconnector.Schedule(addr)
		}
		return nil, err
	}

	// Store the config address for reconnection purposes
	conn.SetConfigAddr(addr)

	m.registerConnection(conn)
	return conn, nil
}

// ConnectWithTransport connects to a peer using the specified transport.
// This allows using different transports for different peers (e.g., WebSocket for proxy traversal).
func (m *Manager) ConnectWithTransport(ctx context.Context, tr transport.Transport, addr string) (*Connection, error) {
	m.mu.RLock()
	info := m.peerInfos[addr]
	m.mu.RUnlock()

	var expectedID identity.AgentID
	if info != nil {
		expectedID = info.ExpectedID
	}

	connCfg := ConnectionConfig{
		LocalID:          m.cfg.LocalID,
		ExpectedPeerID:   expectedID,
		Capabilities:     m.cfg.Capabilities,
		HandshakeTimeout: m.cfg.HandshakeTimeout,
		OnFrame:          m.cfg.OnFrame,
		OnDisconnect:     m.handleDisconnect,
	}

	// Use per-peer DialOptions if available, otherwise use manager defaults
	dialOpts := m.cfg.DialOptions
	if info != nil && info.DialOptions != nil {
		dialOpts = *info.DialOptions
	}

	conn, err := m.handshaker.DialAndHandshake(ctx, tr, addr, connCfg, dialOpts)
	if err != nil {
		// Schedule reconnect if this is a persistent peer
		if info != nil && info.Persistent {
			m.reconnector.Schedule(addr)
		}
		return nil, err
	}

	// Store the config address for reconnection purposes
	conn.SetConfigAddr(addr)

	m.registerConnection(conn)
	return conn, nil
}

// Accept accepts an incoming connection and performs handshake.
func (m *Manager) Accept(ctx context.Context, peerConn transport.PeerConn) (*Connection, error) {
	connCfg := ConnectionConfig{
		LocalID:          m.cfg.LocalID,
		Capabilities:     m.cfg.Capabilities,
		HandshakeTimeout: m.cfg.HandshakeTimeout,
		OnFrame:          m.cfg.OnFrame,
		OnDisconnect:     m.handleDisconnect,
	}

	conn, err := m.handshaker.AcceptHandshake(ctx, peerConn, connCfg)
	if err != nil {
		return nil, err
	}

	m.registerConnection(conn)
	return conn, nil
}

// registerConnection adds a connection to the manager.
func (m *Manager) registerConnection(conn *Connection) {
	m.mu.Lock()
	// Check if we already have a connection to this peer
	if existing, ok := m.peers[conn.RemoteID]; ok {
		// Close the existing connection
		existing.Close()
	}
	m.peers[conn.RemoteID] = conn
	m.mu.Unlock()

	// Start connection management goroutines
	m.wg.Add(2)
	go m.readLoop(conn)
	go m.keepaliveLoop(conn)

	// Notify callback
	if m.cfg.OnPeerConnected != nil {
		m.cfg.OnPeerConnected(conn)
	}
}

// handleDisconnect is called when a connection is closed.
func (m *Manager) handleDisconnect(conn *Connection, err error) {
	m.mu.Lock()
	// Remove from peers map if this is still the active connection
	if existing, ok := m.peers[conn.RemoteID]; ok && existing == conn {
		delete(m.peers, conn.RemoteID)
	}

	// Find the peer info using the config address (original dial address).
	// This is necessary because RemoteAddr() returns the resolved IP,
	// but peerInfos is keyed by the config address (which may be a hostname).
	var peerInfo *PeerInfo
	configAddr := conn.ConfigAddr()
	if configAddr != "" {
		peerInfo = m.peerInfos[configAddr]
	}
	m.mu.Unlock()

	// Notify callback
	if m.cfg.OnPeerDisconnect != nil {
		m.cfg.OnPeerDisconnect(conn, err)
	}

	// Schedule reconnect if persistent, using the config address
	if peerInfo != nil && peerInfo.Persistent && configAddr != "" {
		m.reconnector.Schedule(configAddr)
	}
}

// handleReconnect attempts to reconnect to a peer.
func (m *Manager) handleReconnect(addr string) error {
	ctx, cancel := context.WithTimeout(m.ctx, m.cfg.HandshakeTimeout+m.cfg.DialOptions.Timeout)
	defer cancel()

	_, err := m.Connect(ctx, addr)
	return err
}

// readLoop reads frames from a connection.
func (m *Manager) readLoop(conn *Connection) {
	defer m.wg.Done()
	defer recovery.RecoverWithLog(m.logger, "peer.readLoop")

	// Wait for handshake to complete and reader to be initialized
	select {
	case <-conn.Ready():
		// Handshake complete, reader is now safe to use
	case <-conn.Done():
		return
	case <-m.ctx.Done():
		return
	}

	for {
		select {
		case <-conn.Done():
			return
		case <-m.ctx.Done():
			return
		default:
		}

		frame, err := conn.reader.Read()
		if err != nil {
			conn.Close()
			m.handleDisconnect(conn, err)
			return
		}

		conn.updateActivity()

		// Handle control frames internally
		switch frame.Type {
		case protocol.FrameKeepalive:
			ka, err := protocol.DecodeKeepalive(frame.Payload)
			if err == nil {
				conn.SendKeepaliveAck(ka.Timestamp)
			}
		case protocol.FrameKeepaliveAck:
			ka, err := protocol.DecodeKeepalive(frame.Payload)
			if err == nil {
				conn.UpdateRTT(ka.Timestamp)
			}
		default:
			// Pass to callback
			if conn.onFrame != nil {
				conn.onFrame(conn, frame)
			}
		}
	}
}

// keepaliveLoop sends periodic keepalives.
func (m *Manager) keepaliveLoop(conn *Connection) {
	defer m.wg.Done()
	defer recovery.RecoverWithLog(m.logger, "peer.keepaliveLoop")

	ticker := time.NewTicker(m.cfg.KeepaliveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-conn.Done():
			return
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			// Check if connection is still active
			if conn.State() != StateConnected {
				return
			}

			// Check for timeout
			if time.Since(conn.LastActivity()) > m.cfg.KeepaliveInterval+m.cfg.KeepaliveTimeout {
				conn.Close()
				m.handleDisconnect(conn, fmt.Errorf("keepalive timeout"))
				return
			}

			// Send keepalive
			if err := conn.SendKeepalive(); err != nil {
				conn.Close()
				m.handleDisconnect(conn, err)
				return
			}
		}
	}
}

// GetPeer returns a connection by peer ID.
func (m *Manager) GetPeer(id identity.AgentID) *Connection {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.peers[id]
}

// GetAllPeers returns all connected peers.
func (m *Manager) GetAllPeers() []*Connection {
	m.mu.RLock()
	defer m.mu.RUnlock()

	peers := make([]*Connection, 0, len(m.peers))
	for _, conn := range m.peers {
		peers = append(peers, conn)
	}
	return peers
}

// PeerCount returns the number of connected peers.
func (m *Manager) PeerCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.peers)
}

// Broadcast sends a frame to all connected peers.
func (m *Manager) Broadcast(frame *protocol.Frame) error {
	m.mu.RLock()
	peers := make([]*Connection, 0, len(m.peers))
	for _, conn := range m.peers {
		peers = append(peers, conn)
	}
	m.mu.RUnlock()

	var lastErr error
	for _, conn := range peers {
		if err := conn.WriteFrame(frame); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// Disconnect closes a connection to a specific peer.
func (m *Manager) Disconnect(id identity.AgentID) error {
	m.mu.Lock()
	conn, ok := m.peers[id]
	if ok {
		delete(m.peers, id)
	}
	m.mu.Unlock()

	if !ok {
		return fmt.Errorf("peer not found: %s", id.String())
	}

	return conn.Close()
}

// Close shuts down the manager and all connections.
func (m *Manager) Close() error {
	m.cancel()

	// Stop reconnector
	m.reconnector.Stop()

	// Close all connections
	m.mu.Lock()
	for _, conn := range m.peers {
		conn.Close()
	}
	m.peers = make(map[identity.AgentID]*Connection)
	m.mu.Unlock()

	// Wait for goroutines
	m.wg.Wait()

	return nil
}

// SendToPeer sends a frame to a specific peer.
// Implements flood.PeerSender interface.
func (m *Manager) SendToPeer(peerID identity.AgentID, frame *protocol.Frame) error {
	m.mu.RLock()
	conn := m.peers[peerID]
	m.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("peer not found: %s", peerID.ShortString())
	}

	return conn.WriteFrame(frame)
}

// GetPeerIDs returns all connected peer IDs.
// Implements flood.PeerSender interface.
func (m *Manager) GetPeerIDs() []identity.AgentID {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]identity.AgentID, 0, len(m.peers))
	for id := range m.peers {
		ids = append(ids, id)
	}
	return ids
}

// SetFrameCallback sets the callback for incoming frames.
func (m *Manager) SetFrameCallback(callback func(identity.AgentID, *protocol.Frame)) {
	// Wrap the callback to match the internal interface
	m.cfg.OnFrame = func(conn *Connection, frame *protocol.Frame) {
		if callback != nil {
			callback(conn.RemoteID, frame)
		}
	}
}
