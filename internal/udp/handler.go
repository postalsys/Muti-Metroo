package udp

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/postalsys/muti-metroo/internal/crypto"
	"github.com/postalsys/muti-metroo/internal/identity"
	"github.com/postalsys/muti-metroo/internal/logging"
	"github.com/postalsys/muti-metroo/internal/protocol"
)

// DataWriter is the interface for sending frames back to the mesh.
type DataWriter interface {
	// WriteUDPDatagram sends a UDP datagram frame to the specified peer.
	WriteUDPDatagram(peerID identity.AgentID, streamID uint64, datagram *protocol.UDPDatagram) error

	// WriteUDPClose sends a UDP close frame to the specified peer.
	WriteUDPClose(peerID identity.AgentID, streamID uint64, reason uint8) error

	// WriteUDPOpenAck sends a UDP open acknowledgment to the specified peer.
	WriteUDPOpenAck(peerID identity.AgentID, streamID uint64, ack *protocol.UDPOpenAck) error

	// WriteUDPOpenErr sends a UDP open error to the specified peer.
	WriteUDPOpenErr(peerID identity.AgentID, streamID uint64, err *protocol.UDPOpenErr) error
}

// Handler manages UDP associations for exit nodes.
type Handler struct {
	mu           sync.RWMutex
	associations map[uint64]*Association // by StreamID
	byRequestID  map[uint64]*Association // for correlation across hops

	config Config
	writer DataWriter
	logger *slog.Logger

	// Cleanup
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewHandler creates a new UDP handler.
func NewHandler(cfg Config, writer DataWriter, logger *slog.Logger) *Handler {
	ctx, cancel := context.WithCancel(context.Background())

	h := &Handler{
		associations: make(map[uint64]*Association),
		byRequestID:  make(map[uint64]*Association),
		config:       cfg,
		writer:       writer,
		logger:       logger.With(slog.String("component", "udp")),
		ctx:          ctx,
		cancel:       cancel,
	}

	// Start cleanup goroutine if timeout is configured
	if cfg.IdleTimeout > 0 {
		h.wg.Add(1)
		go h.cleanupLoop()
	}

	return h
}

// HandleUDPOpen processes a UDP_OPEN frame at the exit node.
// Creates a UDP socket and establishes the association.
func (h *Handler) HandleUDPOpen(
	ctx context.Context,
	peerID identity.AgentID,
	streamID uint64,
	open *protocol.UDPOpen,
	remoteEphemeralPub [protocol.EphemeralKeySize]byte,
) error {
	h.logger.Debug("exit handler received UDP_OPEN",
		slog.Uint64("stream_id", streamID),
		slog.String("from_peer", peerID.ShortString()),
		slog.Uint64("request_id", open.RequestID))

	// Check if UDP is enabled
	if !h.config.Enabled {
		h.writer.WriteUDPOpenErr(peerID, streamID, &protocol.UDPOpenErr{
			RequestID: open.RequestID,
			ErrorCode: protocol.ErrUDPDisabled,
			Message:   "UDP relay is disabled",
		})
		return fmt.Errorf("UDP relay is disabled")
	}

	// Check association limit
	h.mu.RLock()
	count := len(h.associations)
	h.mu.RUnlock()

	if h.config.MaxAssociations > 0 && count >= h.config.MaxAssociations {
		h.writer.WriteUDPOpenErr(peerID, streamID, &protocol.UDPOpenErr{
			RequestID: open.RequestID,
			ErrorCode: protocol.ErrResourceLimit,
			Message:   "UDP association limit reached",
		})
		return fmt.Errorf("association limit reached")
	}

	// Create association
	assoc := NewAssociation(streamID, open.RequestID, peerID)

	// Create UDP socket
	udpConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		h.writer.WriteUDPOpenErr(peerID, streamID, &protocol.UDPOpenErr{
			RequestID: open.RequestID,
			ErrorCode: protocol.ErrGeneralFailure,
			Message:   "failed to create UDP socket",
		})
		return fmt.Errorf("create UDP socket: %w", err)
	}

	assoc.SetUDPConn(udpConn)

	// Perform E2E key exchange
	var zeroKey [protocol.EphemeralKeySize]byte
	if remoteEphemeralPub != zeroKey {
		// Generate ephemeral keypair for this session
		ephPriv, ephPub, err := crypto.GenerateEphemeralKeypair()
		if err != nil {
			udpConn.Close()
			h.writer.WriteUDPOpenErr(peerID, streamID, &protocol.UDPOpenErr{
				RequestID: open.RequestID,
				ErrorCode: protocol.ErrGeneralFailure,
				Message:   "failed to generate ephemeral key",
			})
			return fmt.Errorf("generate ephemeral key: %w", err)
		}

		// Compute shared secret
		sharedSecret, err := crypto.ComputeECDH(ephPriv, remoteEphemeralPub)
		if err != nil {
			crypto.ZeroKey(&ephPriv)
			udpConn.Close()
			h.writer.WriteUDPOpenErr(peerID, streamID, &protocol.UDPOpenErr{
				RequestID: open.RequestID,
				ErrorCode: protocol.ErrGeneralFailure,
				Message:   "key exchange failed",
			})
			return fmt.Errorf("ECDH: %w", err)
		}

		// Zero out private key immediately
		crypto.ZeroKey(&ephPriv)

		// Derive session key (we are responder)
		sessionKey := crypto.DeriveSessionKey(sharedSecret, open.RequestID, remoteEphemeralPub, ephPub, false)
		crypto.ZeroKey(&sharedSecret)

		assoc.SetSessionKey(sessionKey)

		// Register association
		h.mu.Lock()
		h.associations[streamID] = assoc
		h.byRequestID[open.RequestID] = assoc
		h.mu.Unlock()

		// Send UDP_OPEN_ACK with our ephemeral public key
		localAddr := udpConn.LocalAddr().(*net.UDPAddr)
		var addrType uint8
		var boundAddr []byte
		if ip4 := localAddr.IP.To4(); ip4 != nil {
			addrType = protocol.AddrTypeIPv4
			boundAddr = ip4
		} else {
			addrType = protocol.AddrTypeIPv6
			boundAddr = localAddr.IP.To16()
		}
		ack := &protocol.UDPOpenAck{
			RequestID:       open.RequestID,
			BoundAddrType:   addrType,
			BoundAddr:       boundAddr,
			BoundPort:       uint16(localAddr.Port),
			EphemeralPubKey: ephPub,
		}

		if err := h.writer.WriteUDPOpenAck(peerID, streamID, ack); err != nil {
			h.removeAssociation(streamID)
			return fmt.Errorf("send UDP_OPEN_ACK: %w", err)
		}

		assoc.SetOpen()

		// Start read loop
		h.wg.Add(1)
		go h.readLoop(assoc)

		h.logger.Info("UDP association established",
			logging.KeyStreamID, streamID,
			logging.KeyRequestID, open.RequestID,
			"local_addr", localAddr.String())
	} else {
		// No encryption (should we allow this?)
		h.logger.Warn("UDP_OPEN without ephemeral key, encryption disabled",
			logging.KeyStreamID, streamID)

		// Register association
		h.mu.Lock()
		h.associations[streamID] = assoc
		h.byRequestID[open.RequestID] = assoc
		h.mu.Unlock()

		// Send UDP_OPEN_ACK without ephemeral key
		localAddr := udpConn.LocalAddr().(*net.UDPAddr)
		var addrType uint8
		var boundAddr []byte
		if ip4 := localAddr.IP.To4(); ip4 != nil {
			addrType = protocol.AddrTypeIPv4
			boundAddr = ip4
		} else {
			addrType = protocol.AddrTypeIPv6
			boundAddr = localAddr.IP.To16()
		}
		ack := &protocol.UDPOpenAck{
			RequestID:     open.RequestID,
			BoundAddrType: addrType,
			BoundAddr:     boundAddr,
			BoundPort:     uint16(localAddr.Port),
		}

		if err := h.writer.WriteUDPOpenAck(peerID, streamID, ack); err != nil {
			h.removeAssociation(streamID)
			return fmt.Errorf("send UDP_OPEN_ACK: %w", err)
		}

		assoc.SetOpen()

		// Start read loop
		h.wg.Add(1)
		go h.readLoop(assoc)
	}

	return nil
}

// HandleUDPDatagram processes a UDP_DATAGRAM frame.
// Decrypts and forwards the datagram to the destination.
func (h *Handler) HandleUDPDatagram(peerID identity.AgentID, streamID uint64, datagram *protocol.UDPDatagram) error {
	h.mu.RLock()
	assoc := h.associations[streamID]
	h.mu.RUnlock()

	if assoc == nil {
		return fmt.Errorf("unknown stream ID: %d", streamID)
	}

	if assoc.IsClosed() {
		return fmt.Errorf("association closed")
	}

	assoc.UpdateActivity()

	// Decrypt payload
	plaintext, err := assoc.Decrypt(datagram.Data)
	if err != nil {
		return fmt.Errorf("decrypt: %w", err)
	}

	// Check size
	if len(plaintext) > h.config.MaxDatagramSize {
		return fmt.Errorf("datagram too large: %d > %d", len(plaintext), h.config.MaxDatagramSize)
	}

	// Resolve destination address
	var destAddr *net.UDPAddr
	switch datagram.AddressType {
	case protocol.AddrTypeIPv4:
		destAddr = &net.UDPAddr{
			IP:   net.IP(datagram.Address),
			Port: int(datagram.Port),
		}
	case protocol.AddrTypeIPv6:
		destAddr = &net.UDPAddr{
			IP:   net.IP(datagram.Address),
			Port: int(datagram.Port),
		}
	case protocol.AddrTypeDomain:
		if len(datagram.Address) < 2 {
			return fmt.Errorf("invalid domain address")
		}
		domain := string(datagram.Address[1:])
		ips, err := net.LookupIP(domain)
		if err != nil {
			return fmt.Errorf("DNS lookup failed: %w", err)
		}
		if len(ips) == 0 {
			return fmt.Errorf("no IP addresses for domain")
		}
		destAddr = &net.UDPAddr{
			IP:   ips[0],
			Port: int(datagram.Port),
		}
	default:
		return fmt.Errorf("unknown address type: %d", datagram.AddressType)
	}

	// Send to destination
	assoc.mu.RLock()
	conn := assoc.UDPConn
	assoc.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("UDP connection closed")
	}

	_, err = conn.WriteToUDP(plaintext, destAddr)
	if err != nil {
		return fmt.Errorf("send: %w", err)
	}

	return nil
}

// HandleUDPClose processes a UDP_CLOSE frame.
func (h *Handler) HandleUDPClose(peerID identity.AgentID, streamID uint64) error {
	h.mu.RLock()
	assoc := h.associations[streamID]
	h.mu.RUnlock()

	if assoc == nil {
		return nil // Already closed
	}

	h.removeAssociation(streamID)
	return nil
}

// GetAssociation returns an association by stream ID.
func (h *Handler) GetAssociation(streamID uint64) *Association {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return h.associations[streamID]
}

// GetAssociationByRequestID returns an association by request ID.
func (h *Handler) GetAssociationByRequestID(requestID uint64) *Association {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return h.byRequestID[requestID]
}

// ActiveCount returns the number of active associations.
func (h *Handler) ActiveCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return len(h.associations)
}

// Close shuts down the handler and all associations.
func (h *Handler) Close() error {
	h.cancel()

	// Close all associations
	h.mu.Lock()
	for _, assoc := range h.associations {
		assoc.Close()
	}
	h.associations = make(map[uint64]*Association)
	h.byRequestID = make(map[uint64]*Association)
	h.mu.Unlock()

	// Wait for goroutines
	h.wg.Wait()

	return nil
}

// removeAssociation removes an association and cleans up resources.
func (h *Handler) removeAssociation(streamID uint64) {
	h.mu.Lock()
	assoc := h.associations[streamID]
	if assoc != nil {
		delete(h.associations, streamID)
		delete(h.byRequestID, assoc.RequestID)
	}
	h.mu.Unlock()

	if assoc != nil {
		assoc.Close()
	}
}

// readLoop reads datagrams from the UDP socket and sends them back through the mesh.
func (h *Handler) readLoop(assoc *Association) {
	defer h.wg.Done()

	buf := make([]byte, h.config.MaxDatagramSize+128) // Extra for encryption overhead

	for {
		select {
		case <-assoc.Context().Done():
			return
		case <-h.ctx.Done():
			return
		default:
		}

		assoc.mu.RLock()
		conn := assoc.UDPConn
		assoc.mu.RUnlock()

		if conn == nil {
			return
		}

		// Set read deadline for responsiveness to cancellation
		conn.SetReadDeadline(time.Now().Add(1 * time.Second))

		n, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue // Timeout, check for cancellation
			}
			if assoc.IsClosed() {
				return
			}
			continue
		}

		assoc.UpdateActivity()

		// Encrypt payload
		plaintext := buf[:n]
		ciphertext, err := assoc.Encrypt(plaintext)
		if err != nil {
			continue
		}

		// Build datagram
		var addrType uint8
		var addr []byte
		if ip4 := remoteAddr.IP.To4(); ip4 != nil {
			addrType = protocol.AddrTypeIPv4
			addr = ip4
		} else {
			addrType = protocol.AddrTypeIPv6
			addr = remoteAddr.IP.To16()
		}

		datagram := &protocol.UDPDatagram{
			AddressType: addrType,
			Address:     addr,
			Port:        uint16(remoteAddr.Port),
			Data:        ciphertext,
		}

		// Send back through mesh
		h.writer.WriteUDPDatagram(assoc.PeerID, assoc.StreamID, datagram)
	}
}

// cleanupLoop periodically removes expired associations.
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

// cleanupExpired removes associations that have exceeded the idle timeout.
func (h *Handler) cleanupExpired() {
	h.mu.RLock()
	var expired []uint64
	for streamID, assoc := range h.associations {
		if assoc.IsExpired(h.config.IdleTimeout) {
			expired = append(expired, streamID)
		}
	}
	h.mu.RUnlock()

	for _, streamID := range expired {
		h.mu.RLock()
		assoc := h.associations[streamID]
		h.mu.RUnlock()

		if assoc != nil {
			// Send close to peer
			h.writer.WriteUDPClose(assoc.PeerID, streamID, protocol.UDPCloseTimeout)
			h.removeAssociation(streamID)
		}
	}
}
