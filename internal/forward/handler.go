package forward

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/postalsys/muti-metroo/internal/crypto"
	"github.com/postalsys/muti-metroo/internal/identity"
	"github.com/postalsys/muti-metroo/internal/logging"
	"github.com/postalsys/muti-metroo/internal/protocol"
	"github.com/postalsys/muti-metroo/internal/recovery"
)

// HandlerConfig contains tunnel exit handler configuration.
type HandlerConfig struct {
	// Endpoints define tunnel exit points - key to target mappings.
	Endpoints []Endpoint

	// ConnectTimeout for outbound connections.
	ConnectTimeout time.Duration

	// IdleTimeout for idle connections.
	IdleTimeout time.Duration

	// MaxConnections limits concurrent connections.
	MaxConnections int

	// Logger for logging.
	Logger *slog.Logger
}

// DefaultHandlerConfig returns sensible defaults.
func DefaultHandlerConfig() HandlerConfig {
	return HandlerConfig{
		ConnectTimeout: 30 * time.Second,
		IdleTimeout:    5 * time.Minute,
		MaxConnections: 1000,
	}
}

// ActiveConnection represents an active tunnel connection.
type ActiveConnection struct {
	StreamID   uint64
	RemoteID   identity.AgentID
	Key        string
	Target     string
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

// Handler handles tunnel exit connections.
type Handler struct {
	cfg     HandlerConfig
	localID identity.AgentID
	writer  StreamWriter
	logger  *slog.Logger
	targets map[string]string // routing key -> target

	mu          sync.RWMutex
	connections map[uint64]*ActiveConnection
	connCount   atomic.Int64

	running  atomic.Bool
	stopOnce sync.Once
	stopCh   chan struct{}
}

// NewHandler creates a new tunnel exit handler.
func NewHandler(cfg HandlerConfig, localID identity.AgentID, writer StreamWriter) *Handler {
	logger := cfg.Logger
	if logger == nil {
		logger = logging.NopLogger()
	}

	// Build targets map
	targets := make(map[string]string)
	for _, ep := range cfg.Endpoints {
		targets[ep.Key] = ep.Target
	}

	return &Handler{
		cfg:         cfg,
		localID:     localID,
		writer:      writer,
		logger:      logger,
		targets:     targets,
		connections: make(map[uint64]*ActiveConnection),
		stopCh:      make(chan struct{}),
	}
}

// Start starts the tunnel exit handler.
func (h *Handler) Start() {
	h.running.Store(true)
}

// Stop stops the tunnel exit handler.
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
		h.connCount.Store(0)
		h.mu.Unlock()
	})
}

// IsRunning returns true if the handler is running.
func (h *Handler) IsRunning() bool {
	return h.running.Load()
}

// GetTarget returns the target for a routing key.
func (h *Handler) GetTarget(key string) (string, bool) {
	target, ok := h.targets[key]
	return target, ok
}

// GetKeys returns all configured routing keys.
func (h *Handler) GetKeys() []string {
	keys := make([]string, 0, len(h.targets))
	for k := range h.targets {
		keys = append(keys, k)
	}
	return keys
}

// HandleStreamOpen processes a tunnel STREAM_OPEN request.
// The TCP dial is performed asynchronously to avoid blocking the frame processing loop.
func (h *Handler) HandleStreamOpen(ctx context.Context, streamID uint64, requestID uint64, remoteID identity.AgentID, key string, remoteEphemeralPub [crypto.KeySize]byte) error {
	if !h.running.Load() {
		return fmt.Errorf("handler not running")
	}

	// Check connection limit
	if h.cfg.MaxConnections > 0 && h.connCount.Load() >= int64(h.cfg.MaxConnections) {
		h.sendOpenErr(remoteID, streamID, requestID, protocol.ErrConnectionLimit, "connection limit exceeded")
		return fmt.Errorf("connection limit exceeded")
	}

	// Look up target for this routing key
	target, ok := h.targets[key]
	if !ok {
		h.sendOpenErr(remoteID, streamID, requestID, protocol.ErrForwardNotFound, "forward key not found")
		return fmt.Errorf("forward key not found: %s", key)
	}

	// Perform the rest asynchronously to avoid blocking the frame processing loop.
	go h.handleStreamOpenAsync(ctx, streamID, requestID, remoteID, key, target, remoteEphemeralPub)

	return nil
}

// handleStreamOpenAsync performs the actual stream open work asynchronously.
func (h *Handler) handleStreamOpenAsync(ctx context.Context, streamID uint64, requestID uint64, remoteID identity.AgentID, key, target string, remoteEphemeralPub [crypto.KeySize]byte) {
	defer recovery.RecoverWithLog(h.logger, "forward.Handler.handleStreamOpenAsync")

	// Generate ephemeral keypair for E2E encryption key exchange
	ephPriv, ephPub, err := crypto.GenerateEphemeralKeypair()
	if err != nil {
		h.sendOpenErr(remoteID, streamID, requestID, protocol.ErrGeneralFailure, "key generation failed")
		return
	}

	// Compute shared secret from ECDH
	sharedSecret, err := crypto.ComputeECDH(ephPriv, remoteEphemeralPub)
	if err != nil {
		crypto.ZeroKey(&ephPriv)
		h.sendOpenErr(remoteID, streamID, requestID, protocol.ErrGeneralFailure, "key exchange failed")
		return
	}

	// Zero out ephemeral private key after computing shared secret
	crypto.ZeroKey(&ephPriv)

	// Derive session key - we are the responder (exit node)
	// Use requestID (not streamID) because streamID changes at each relay hop
	sessionKey := crypto.DeriveSessionKey(sharedSecret, requestID, remoteEphemeralPub, ephPub, false)
	crypto.ZeroKey(&sharedSecret)

	// Connect to target
	dialer := &net.Dialer{Timeout: h.cfg.ConnectTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", target)
	if err != nil {
		errorCode := h.mapDialError(err)
		h.sendOpenErr(remoteID, streamID, requestID, errorCode, err.Error())
		return
	}

	// Get local address for ACK
	localAddr := conn.LocalAddr().(*net.TCPAddr)

	// Track connection with session key
	ac := &ActiveConnection{
		StreamID:   streamID,
		RemoteID:   remoteID,
		Key:        key,
		Target:     target,
		Conn:       conn,
		StartedAt:  time.Now(),
		sessionKey: sessionKey,
	}

	h.mu.Lock()
	h.connections[streamID] = ac
	h.connCount.Add(1)
	h.mu.Unlock()

	h.logger.Debug("forward stream opened",
		"key", key,
		"target", target,
		logging.KeyStreamID, streamID)

	// Send ACK with our ephemeral public key
	if err := h.writer.WriteStreamOpenAck(remoteID, streamID, requestID, localAddr.IP, uint16(localAddr.Port), ephPub); err != nil {
		ac.Close()
		h.removeConnection(streamID)
		return
	}

	// Start reading from target
	go h.readLoop(ac)
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

	// Decrypt data before writing to target
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
		// Client is done sending, close write side of target
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

// readLoop reads data from the target and forwards to the stream.
func (h *Handler) readLoop(ac *ActiveConnection) {
	defer h.closeConnection(ac.StreamID, ac.RemoteID, nil)
	defer recovery.RecoverWithLog(h.logger, "forward.readLoop")

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

	h.logger.Debug("forward stream closed",
		"key", ac.Key,
		"target", ac.Target,
		logging.KeyStreamID, streamID)

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

// sendOpenErr sends a stream open error.
func (h *Handler) sendOpenErr(peerID identity.AgentID, streamID, requestID uint64, code uint16, msg string) {
	h.logger.Debug("forward stream open error",
		logging.KeyStreamID, streamID,
		"error_code", code,
		"message", msg)

	if h.writer != nil {
		h.writer.WriteStreamOpenErr(peerID, streamID, requestID, code, msg)
	}
}

// mapDialError maps a dial error to a protocol error code.
func (h *Handler) mapDialError(err error) uint16 {
	if err == nil {
		return 0
	}

	// Check for timeout via net.Error interface
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return protocol.ErrConnectionTimeout
	}

	// Check error message for common patterns (case-insensitive)
	errLower := strings.ToLower(err.Error())
	if strings.Contains(errLower, "refused") {
		return protocol.ErrConnectionRefused
	}
	if strings.Contains(errLower, "unreachable") {
		return protocol.ErrHostUnreachable
	}
	if strings.Contains(errLower, "timeout") {
		return protocol.ErrConnectionTimeout
	}

	return protocol.ErrGeneralFailure
}

// ConnectionCount returns the number of active connections.
func (h *Handler) ConnectionCount() int64 {
	return h.connCount.Load()
}
