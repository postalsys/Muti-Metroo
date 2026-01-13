package icmp

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/postalsys/muti-metroo/internal/crypto"
	"github.com/postalsys/muti-metroo/internal/identity"
	"github.com/postalsys/muti-metroo/internal/logging"
	"github.com/postalsys/muti-metroo/internal/protocol"
)

// DataWriter is the interface for sending frames back to the mesh.
type DataWriter interface {
	// WriteICMPOpenAck sends an ICMP open acknowledgment to the specified peer.
	WriteICMPOpenAck(peerID identity.AgentID, streamID uint64, ack *protocol.ICMPOpenAck) error

	// WriteICMPOpenErr sends an ICMP open error to the specified peer.
	WriteICMPOpenErr(peerID identity.AgentID, streamID uint64, err *protocol.ICMPOpenErr) error

	// WriteICMPEcho sends an ICMP echo frame to the specified peer.
	WriteICMPEcho(peerID identity.AgentID, streamID uint64, echo *protocol.ICMPEcho) error

	// WriteICMPClose sends an ICMP close frame to the specified peer.
	WriteICMPClose(peerID identity.AgentID, streamID uint64, reason uint8) error
}

// Handler manages ICMP echo sessions for exit nodes.
type Handler struct {
	mu          sync.RWMutex
	sessions    map[uint64]*Session // by StreamID
	byRequestID map[uint64]*Session // for correlation across hops

	config Config
	writer DataWriter
	logger *slog.Logger

	// Cleanup
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewHandler creates a new ICMP handler.
func NewHandler(cfg Config, writer DataWriter, logger *slog.Logger) *Handler {
	ctx, cancel := context.WithCancel(context.Background())

	h := &Handler{
		sessions:    make(map[uint64]*Session),
		byRequestID: make(map[uint64]*Session),
		config:      cfg,
		writer:      writer,
		logger:      logger.With(slog.String("component", "icmp")),
		ctx:         ctx,
		cancel:      cancel,
	}

	// Start cleanup goroutine if timeout is configured
	if cfg.IdleTimeout > 0 {
		h.wg.Add(1)
		go h.cleanupLoop()
	}

	return h
}

// HandleICMPOpen processes an ICMP_OPEN frame at the exit node.
// Creates an ICMP socket and establishes the session.
func (h *Handler) HandleICMPOpen(
	ctx context.Context,
	peerID identity.AgentID,
	streamID uint64,
	open *protocol.ICMPOpen,
	remoteEphemeralPub [protocol.EphemeralKeySize]byte,
) error {
	destIP := open.GetDestinationIP()

	h.logger.Debug("exit handler received ICMP_OPEN",
		slog.Uint64("stream_id", streamID),
		slog.String("from_peer", peerID.ShortString()),
		slog.Uint64("request_id", open.RequestID),
		slog.String("dest_ip", destIP.String()))

	// Check if ICMP is enabled
	if !h.config.Enabled {
		h.writer.WriteICMPOpenErr(peerID, streamID, &protocol.ICMPOpenErr{
			RequestID: open.RequestID,
			ErrorCode: protocol.ErrICMPDisabled,
			Message:   "ICMP echo is disabled",
		})
		return fmt.Errorf("ICMP echo is disabled")
	}

	// Check session limit
	h.mu.RLock()
	count := len(h.sessions)
	h.mu.RUnlock()

	if h.config.MaxSessions > 0 && count >= h.config.MaxSessions {
		h.writer.WriteICMPOpenErr(peerID, streamID, &protocol.ICMPOpenErr{
			RequestID: open.RequestID,
			ErrorCode: protocol.ErrICMPSessionLimit,
			Message:   "ICMP session limit reached",
		})
		return fmt.Errorf("session limit reached")
	}

	// Create session
	session := NewSession(streamID, open.RequestID, peerID, destIP)

	// Create ICMP socket
	conn, err := NewICMPSocket()
	if err != nil {
		h.writer.WriteICMPOpenErr(peerID, streamID, &protocol.ICMPOpenErr{
			RequestID: open.RequestID,
			ErrorCode: protocol.ErrGeneralFailure,
			Message:   "failed to create ICMP socket",
		})
		return fmt.Errorf("create ICMP socket: %w", err)
	}

	session.SetConn(conn)

	// Perform E2E key exchange if remote provided an ephemeral key
	var ephPub [protocol.EphemeralKeySize]byte
	var zeroKey [protocol.EphemeralKeySize]byte
	hasEncryption := remoteEphemeralPub != zeroKey

	if hasEncryption {
		var keyExchangeErr error
		ephPub, keyExchangeErr = h.performKeyExchange(session, open, remoteEphemeralPub, conn)
		if keyExchangeErr != nil {
			return keyExchangeErr
		}
	} else {
		h.logger.Warn("ICMP_OPEN without ephemeral key, encryption disabled",
			logging.KeyStreamID, streamID)
	}

	// Register session
	h.mu.Lock()
	h.sessions[streamID] = session
	h.byRequestID[open.RequestID] = session
	h.mu.Unlock()

	// Build and send ICMP_OPEN_ACK
	ack := &protocol.ICMPOpenAck{
		RequestID: open.RequestID,
	}
	if hasEncryption {
		ack.EphemeralPubKey = ephPub
	}

	if err := h.writer.WriteICMPOpenAck(peerID, streamID, ack); err != nil {
		h.removeSession(streamID)
		return fmt.Errorf("send ICMP_OPEN_ACK: %w", err)
	}

	session.SetOpen()

	if hasEncryption {
		h.logger.Info("ICMP session established",
			logging.KeyStreamID, streamID,
			logging.KeyRequestID, open.RequestID,
			"dest_ip", destIP.String())
	}

	return nil
}

// performKeyExchange generates an ephemeral keypair and derives the session key.
// Returns the public key for inclusion in the ack, or an error.
func (h *Handler) performKeyExchange(
	session *Session,
	open *protocol.ICMPOpen,
	remoteEphemeralPub [protocol.EphemeralKeySize]byte,
	conn interface{ Close() error },
) ([protocol.EphemeralKeySize]byte, error) {
	var zeroKey [protocol.EphemeralKeySize]byte

	ephPriv, ephPub, err := crypto.GenerateEphemeralKeypair()
	if err != nil {
		conn.Close()
		h.writer.WriteICMPOpenErr(session.PeerID, session.StreamID, &protocol.ICMPOpenErr{
			RequestID: open.RequestID,
			ErrorCode: protocol.ErrGeneralFailure,
			Message:   "failed to generate ephemeral key",
		})
		return zeroKey, fmt.Errorf("generate ephemeral key: %w", err)
	}

	sharedSecret, err := crypto.ComputeECDH(ephPriv, remoteEphemeralPub)
	if err != nil {
		crypto.ZeroKey(&ephPriv)
		conn.Close()
		h.writer.WriteICMPOpenErr(session.PeerID, session.StreamID, &protocol.ICMPOpenErr{
			RequestID: open.RequestID,
			ErrorCode: protocol.ErrGeneralFailure,
			Message:   "key exchange failed",
		})
		return zeroKey, fmt.Errorf("ECDH: %w", err)
	}

	crypto.ZeroKey(&ephPriv)

	// Derive session key (we are responder)
	sessionKey := crypto.DeriveSessionKey(sharedSecret, open.RequestID, remoteEphemeralPub, ephPub, false)
	crypto.ZeroKey(&sharedSecret)

	session.SetSessionKey(sessionKey)

	return ephPub, nil
}

// HandleICMPEcho processes an ICMP_ECHO frame.
// Decrypts the payload, sends the echo request, waits for reply, and returns.
func (h *Handler) HandleICMPEcho(peerID identity.AgentID, streamID uint64, echo *protocol.ICMPEcho) error {
	h.mu.RLock()
	session := h.sessions[streamID]
	h.mu.RUnlock()

	if session == nil {
		return fmt.Errorf("unknown stream ID: %d", streamID)
	}

	if session.IsClosed() {
		return fmt.Errorf("session closed")
	}

	session.UpdateActivity()

	// If this is a reply (shouldn't happen from mesh), ignore
	if echo.IsReply {
		return nil
	}

	// Decrypt payload
	plaintext, err := session.Decrypt(echo.Data)
	if err != nil {
		return fmt.Errorf("decrypt: %w", err)
	}

	conn := session.GetConn()
	if conn == nil {
		return fmt.Errorf("ICMP connection closed")
	}

	// Send echo request
	if err := SendEchoRequest(conn, session.DestIP, echo.Identifier, echo.Sequence, plaintext); err != nil {
		return fmt.Errorf("send echo: %w", err)
	}

	// Wait for reply
	timeout := h.config.EchoTimeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	// Start goroutine to read reply and send back
	go h.waitForReply(session, echo.Identifier, echo.Sequence, timeout)

	return nil
}

// waitForReply waits for an ICMP echo reply and sends it back through the mesh.
func (h *Handler) waitForReply(session *Session, identifier, sequence uint16, timeout time.Duration) {
	conn := session.GetConn()
	if conn == nil {
		return
	}

	reply, err := ReadEchoReplyFiltered(conn, identifier, timeout)
	if err != nil {
		// Timeout or error, don't send anything
		h.logger.Debug("ICMP reply timeout or error",
			logging.KeyStreamID, session.StreamID,
			"identifier", identifier,
			"sequence", sequence,
			"error", err)
		return
	}

	// Encrypt reply payload
	ciphertext, err := session.Encrypt(reply.Payload)
	if err != nil {
		h.logger.Error("failed to encrypt ICMP reply",
			logging.KeyStreamID, session.StreamID,
			"error", err)
		return
	}

	// Send reply back through mesh
	echoReply := &protocol.ICMPEcho{
		Identifier: reply.ID,
		Sequence:   reply.Seq,
		IsReply:    true,
		Data:       ciphertext,
	}

	if err := h.writer.WriteICMPEcho(session.PeerID, session.StreamID, echoReply); err != nil {
		h.logger.Error("failed to send ICMP reply",
			logging.KeyStreamID, session.StreamID,
			"error", err)
	}
}

// HandleICMPClose processes an ICMP_CLOSE frame.
func (h *Handler) HandleICMPClose(peerID identity.AgentID, streamID uint64) error {
	h.mu.RLock()
	session := h.sessions[streamID]
	h.mu.RUnlock()

	if session == nil {
		return nil // Already closed
	}

	h.removeSession(streamID)
	return nil
}

// GetSession returns a session by stream ID.
func (h *Handler) GetSession(streamID uint64) *Session {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return h.sessions[streamID]
}

// GetSessionByRequestID returns a session by request ID.
func (h *Handler) GetSessionByRequestID(requestID uint64) *Session {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return h.byRequestID[requestID]
}

// ActiveCount returns the number of active sessions.
func (h *Handler) ActiveCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return len(h.sessions)
}

// Close shuts down the handler and all sessions.
func (h *Handler) Close() error {
	h.cancel()

	// Close all sessions
	h.mu.Lock()
	for _, session := range h.sessions {
		session.Close()
	}
	h.sessions = make(map[uint64]*Session)
	h.byRequestID = make(map[uint64]*Session)
	h.mu.Unlock()

	// Wait for goroutines
	h.wg.Wait()

	return nil
}

// removeSession removes a session and cleans up resources.
func (h *Handler) removeSession(streamID uint64) {
	h.mu.Lock()
	session := h.sessions[streamID]
	if session != nil {
		delete(h.sessions, streamID)
		delete(h.byRequestID, session.RequestID)
	}
	h.mu.Unlock()

	if session != nil {
		session.Close()
	}
}

// cleanupLoop periodically removes expired sessions.
func (h *Handler) cleanupLoop() {
	defer h.wg.Done()

	ticker := time.NewTicker(h.config.IdleTimeout / 2)
	defer ticker.Stop()

	for {
		select {
		case <-h.ctx.Done():
			return
		case <-ticker.C:
			h.cleanupExpired()
		}
	}
}

// cleanupExpired removes sessions that have exceeded the idle timeout.
func (h *Handler) cleanupExpired() {
	h.mu.RLock()
	var expired []uint64
	for streamID, session := range h.sessions {
		if session.IsExpired(h.config.IdleTimeout) {
			expired = append(expired, streamID)
		}
	}
	h.mu.RUnlock()

	for _, streamID := range expired {
		h.mu.RLock()
		session := h.sessions[streamID]
		h.mu.RUnlock()

		if session != nil {
			// Send close to peer
			h.writer.WriteICMPClose(session.PeerID, streamID, protocol.ICMPCloseTimeout)
			h.removeSession(streamID)
		}
	}
}
