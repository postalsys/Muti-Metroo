package exit

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/postalsys/muti-metroo/internal/crypto"
	"github.com/postalsys/muti-metroo/internal/identity"
	"github.com/postalsys/muti-metroo/internal/logging"
	"github.com/postalsys/muti-metroo/internal/protocol"
	"github.com/postalsys/muti-metroo/internal/recovery"
)

// HandlerConfig contains exit handler configuration.
type HandlerConfig struct {
	// AllowedRoutes defines which destinations are allowed (CIDR format)
	AllowedRoutes []*net.IPNet

	// ConnectTimeout for outbound connections
	ConnectTimeout time.Duration

	// IdleTimeout for idle connections
	IdleTimeout time.Duration

	// MaxConnections limits concurrent connections
	MaxConnections int

	// DNS configuration
	DNS DNSConfig

	// Logger for logging
	Logger *slog.Logger
}

// DefaultHandlerConfig returns sensible defaults.
func DefaultHandlerConfig() HandlerConfig {
	return HandlerConfig{
		AllowedRoutes:  nil, // nil means deny all (security: deny by default)
		ConnectTimeout: 30 * time.Second,
		IdleTimeout:    5 * time.Minute,
		MaxConnections: 1000,
		DNS:            DefaultDNSConfig(),
	}
}

// StreamWriter is an interface for writing to virtual streams.
type StreamWriter interface {
	// WriteStreamData writes data to a stream.
	WriteStreamData(peerID identity.AgentID, streamID uint64, data []byte, flags uint8) error

	// WriteStreamOpenAck sends a successful open acknowledgment with ephemeral public key for E2E encryption.
	WriteStreamOpenAck(peerID identity.AgentID, streamID uint64, requestID uint64, boundIP net.IP, boundPort uint16, ephemeralPubKey [crypto.KeySize]byte) error

	// WriteStreamOpenErr sends a failed open acknowledgment.
	WriteStreamOpenErr(peerID identity.AgentID, streamID uint64, requestID uint64, errorCode uint16, message string) error

	// WriteStreamClose sends a close frame.
	WriteStreamClose(peerID identity.AgentID, streamID uint64) error
}

// ActiveConnection represents an active exit connection.
type ActiveConnection struct {
	StreamID   uint64
	RemoteID   identity.AgentID
	DestAddr   string
	DestPort   uint16
	Conn       net.Conn
	StartedAt  time.Time
	closed     atomic.Bool
	closeOnce  sync.Once
	sessionKey *crypto.SessionKey // E2E encryption session key
}

// Close closes the connection.
func (ac *ActiveConnection) Close() error {
	var err error
	ac.closeOnce.Do(func() {
		ac.closed.Store(true)
		if ac.Conn != nil {
			err = ac.Conn.Close()
		}
	})
	return err
}

// IsClosed returns true if the connection is closed.
func (ac *ActiveConnection) IsClosed() bool {
	return ac.closed.Load()
}

// Handler handles exit node connections.
type Handler struct {
	cfg      HandlerConfig
	localID  identity.AgentID
	resolver *Resolver
	writer   StreamWriter
	logger   *slog.Logger

	mu          sync.RWMutex
	connections map[uint64]*ActiveConnection
	connCount   atomic.Int64

	running  atomic.Bool
	stopOnce sync.Once
	stopCh   chan struct{}
}

// NewHandler creates a new exit handler.
func NewHandler(cfg HandlerConfig, localID identity.AgentID, writer StreamWriter) *Handler {
	logger := cfg.Logger
	if logger == nil {
		logger = logging.NopLogger()
	}

	return &Handler{
		cfg:         cfg,
		localID:     localID,
		resolver:    NewResolver(cfg.DNS),
		writer:      writer,
		logger:      logger,
		connections: make(map[uint64]*ActiveConnection),
		stopCh:      make(chan struct{}),
	}
}

// Start starts the exit handler.
func (h *Handler) Start() {
	h.running.Store(true)
}

// Stop stops the exit handler.
func (h *Handler) Stop() {
	h.stopOnce.Do(func() {
		h.running.Store(false)
		close(h.stopCh)

		// Close all connections
		h.mu.Lock()
		for _, conn := range h.connections {
			conn.Close()
		}
		h.connections = make(map[uint64]*ActiveConnection)
		h.mu.Unlock()
	})
}

// IsRunning returns true if the handler is running.
func (h *Handler) IsRunning() bool {
	return h.running.Load()
}

// HandleStreamOpen processes a STREAM_OPEN request.
func (h *Handler) HandleStreamOpen(ctx context.Context, streamID uint64, requestID uint64, remoteID identity.AgentID, destAddr string, destPort uint16, remoteEphemeralPub [crypto.KeySize]byte) error {
	if !h.running.Load() {
		return fmt.Errorf("handler not running")
	}

	// Check connection limit
	if h.cfg.MaxConnections > 0 && h.connCount.Load() >= int64(h.cfg.MaxConnections) {
		h.sendOpenErr(remoteID, streamID, requestID, protocol.ErrConnectionLimit, "connection limit exceeded")
		return fmt.Errorf("connection limit exceeded")
	}

	// Resolve address
	ip, err := h.resolver.Resolve(ctx, destAddr)
	if err != nil {
		h.sendOpenErr(remoteID, streamID, requestID, protocol.ErrHostUnreachable, err.Error())
		return fmt.Errorf("resolve %s: %w", destAddr, err)
	}

	// Check if destination is allowed
	if !h.isAllowed(ip) {
		h.sendOpenErr(remoteID, streamID, requestID, protocol.ErrNotAllowed, "destination not allowed")
		return fmt.Errorf("destination not allowed: %s", ip)
	}

	// Generate ephemeral keypair for E2E encryption key exchange
	ephPriv, ephPub, err := crypto.GenerateEphemeralKeypair()
	if err != nil {
		h.sendOpenErr(remoteID, streamID, requestID, protocol.ErrGeneralFailure, "key generation failed")
		return fmt.Errorf("generate ephemeral key: %w", err)
	}

	// Compute shared secret from ECDH
	sharedSecret, err := crypto.ComputeECDH(ephPriv, remoteEphemeralPub)
	if err != nil {
		crypto.ZeroKey(&ephPriv)
		h.sendOpenErr(remoteID, streamID, requestID, protocol.ErrGeneralFailure, "key exchange failed")
		return fmt.Errorf("compute ECDH: %w", err)
	}

	// Zero out ephemeral private key after computing shared secret
	crypto.ZeroKey(&ephPriv)

	// Derive session key - we are the responder (exit node)
	// Use requestID (not streamID) because streamID changes at each relay hop
	sessionKey := crypto.DeriveSessionKey(sharedSecret, requestID, remoteEphemeralPub, ephPub, false)
	crypto.ZeroKey(&sharedSecret)

	// Connect to destination
	addr := fmt.Sprintf("%s:%d", ip.String(), destPort)
	dialer := &net.Dialer{Timeout: h.cfg.ConnectTimeout}

	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		errorCode := h.mapDialError(err)
		h.sendOpenErr(remoteID, streamID, requestID, errorCode, err.Error())
		return fmt.Errorf("dial %s: %w", addr, err)
	}

	// Get local address for ACK
	localAddr := conn.LocalAddr().(*net.TCPAddr)

	// Track connection with session key
	ac := &ActiveConnection{
		StreamID:   streamID,
		RemoteID:   remoteID,
		DestAddr:   destAddr,
		DestPort:   destPort,
		Conn:       conn,
		StartedAt:  time.Now(),
		sessionKey: sessionKey,
	}

	h.mu.Lock()
	h.connections[streamID] = ac
	h.connCount.Add(1)
	h.mu.Unlock()

	// Send ACK with our ephemeral public key
	if err := h.writer.WriteStreamOpenAck(remoteID, streamID, requestID, localAddr.IP, uint16(localAddr.Port), ephPub); err != nil {
		ac.Close()
		h.removeConnection(streamID)
		return fmt.Errorf("write ack: %w", err)
	}

	// Start reading from destination
	go h.readLoop(ac)

	return nil
}

// HandleStreamData processes incoming stream data.
func (h *Handler) HandleStreamData(peerID identity.AgentID, streamID uint64, data []byte, flags uint8) error {
	h.mu.RLock()
	ac := h.connections[streamID]
	h.mu.RUnlock()

	if ac == nil {
		return fmt.Errorf("unknown stream %d", streamID)
	}

	if ac.IsClosed() {
		return fmt.Errorf("stream %d closed", streamID)
	}

	// Decrypt data before writing to destination
	if len(data) > 0 {
		if ac.sessionKey == nil {
			h.closeConnection(streamID, peerID, fmt.Errorf("no session key"))
			return fmt.Errorf("no session key for stream %d", streamID)
		}

		plaintext, err := ac.sessionKey.Decrypt(data)
		if err != nil {
			h.closeConnection(streamID, peerID, err)
			return fmt.Errorf("decrypt: %w", err)
		}

		if _, err := ac.Conn.Write(plaintext); err != nil {
			h.closeConnection(streamID, peerID, err)
			return err
		}
	}

	// Handle FIN flag
	if flags&protocol.FlagFinWrite != 0 {
		// Client is done sending, close write side of destination
		if tcpConn, ok := ac.Conn.(*net.TCPConn); ok {
			tcpConn.CloseWrite()
		}
	}

	return nil
}

// HandleStreamClose processes a stream close request.
func (h *Handler) HandleStreamClose(peerID identity.AgentID, streamID uint64) {
	h.closeConnection(streamID, peerID, nil)
}

// HandleStreamReset processes a stream reset request.
func (h *Handler) HandleStreamReset(peerID identity.AgentID, streamID uint64, errorCode uint16) {
	h.closeConnection(streamID, peerID, fmt.Errorf("reset with code %d", errorCode))
}

// readLoop reads data from the destination and forwards to the stream.
func (h *Handler) readLoop(ac *ActiveConnection) {
	defer h.closeConnection(ac.StreamID, ac.RemoteID, nil)
	defer recovery.RecoverWithLog(h.logger, "exit.readLoop")

	// Account for encryption overhead when reading
	// Each encrypted message must fit in a single frame to be decryptable
	maxPlaintext := protocol.MaxPayloadSize - crypto.EncryptionOverhead
	buf := make([]byte, maxPlaintext)
	for {
		select {
		case <-h.stopCh:
			return
		default:
		}

		// Set read deadline for idle timeout
		if h.cfg.IdleTimeout > 0 {
			ac.Conn.SetReadDeadline(time.Now().Add(h.cfg.IdleTimeout))
		}

		n, err := ac.Conn.Read(buf)
		if n > 0 {
			// Encrypt data before forwarding
			if ac.sessionKey == nil {
				h.logger.Error("no session key in readLoop",
					logging.KeyStreamID, ac.StreamID)
				return
			}

			ciphertext, encErr := ac.sessionKey.Encrypt(buf[:n])
			if encErr != nil {
				h.logger.Error("encrypt failed in readLoop",
					logging.KeyStreamID, ac.StreamID,
					logging.KeyError, encErr)
				return
			}

			// Forward encrypted data to stream
			if writeErr := h.writer.WriteStreamData(ac.RemoteID, ac.StreamID, ciphertext, 0); writeErr != nil {
				return
			}
		}

		if err != nil {
			if err == io.EOF {
				// Send FIN_WRITE (no data to encrypt)
				h.writer.WriteStreamData(ac.RemoteID, ac.StreamID, nil, protocol.FlagFinWrite)
			}
			return
		}
	}
}

// closeConnection closes a connection and cleans up.
func (h *Handler) closeConnection(streamID uint64, peerID identity.AgentID, err error) {
	ac := h.removeConnection(streamID)
	if ac == nil {
		return
	}

	ac.Close()

	// Notify stream is closed
	if h.writer != nil {
		h.writer.WriteStreamClose(peerID, streamID)
	}
}

// removeConnection removes a connection from tracking.
func (h *Handler) removeConnection(streamID uint64) *ActiveConnection {
	h.mu.Lock()
	defer h.mu.Unlock()

	ac, ok := h.connections[streamID]
	if !ok {
		return nil
	}

	delete(h.connections, streamID)
	h.connCount.Add(-1)
	return ac
}

// isAllowed checks if an IP is allowed by the configured routes.
// Security: Returns false (deny) when no routes are configured.
func (h *Handler) isAllowed(ip net.IP) bool {
	if len(h.cfg.AllowedRoutes) == 0 {
		return false // Deny by default when no routes configured
	}

	for _, route := range h.cfg.AllowedRoutes {
		if route.Contains(ip) {
			return true
		}
	}

	return false
}

// mapDialError maps dial errors to protocol error codes.
func (h *Handler) mapDialError(err error) uint16 {
	if netErr, ok := err.(*net.OpError); ok {
		if netErr.Timeout() {
			return protocol.ErrConnectionTimeout
		}
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return protocol.ErrHostUnreachable
	}

	return protocol.ErrConnectionRefused
}

// sendOpenErr sends an open error response.
func (h *Handler) sendOpenErr(peerID identity.AgentID, streamID, requestID uint64, errorCode uint16, message string) {
	if h.writer != nil {
		h.writer.WriteStreamOpenErr(peerID, streamID, requestID, errorCode, message)
	}
}

// ConnectionCount returns the number of active connections.
func (h *Handler) ConnectionCount() int64 {
	return h.connCount.Load()
}

// GetConnection returns an active connection by stream ID.
func (h *Handler) GetConnection(streamID uint64) *ActiveConnection {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.connections[streamID]
}

// SetWriter sets the stream writer.
func (h *Handler) SetWriter(writer StreamWriter) {
	h.writer = writer
}

// ParseAllowedRoutes parses a list of CIDR strings into IPNets.
func ParseAllowedRoutes(routes []string) ([]*net.IPNet, error) {
	var result []*net.IPNet
	for _, r := range routes {
		_, ipNet, err := net.ParseCIDR(r)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR %q: %w", r, err)
		}
		result = append(result, ipNet)
	}
	return result, nil
}
