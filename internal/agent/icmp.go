package agent

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync"

	"github.com/postalsys/muti-metroo/internal/crypto"
	"github.com/postalsys/muti-metroo/internal/health"
	"github.com/postalsys/muti-metroo/internal/icmp"
	"github.com/postalsys/muti-metroo/internal/identity"
	"github.com/postalsys/muti-metroo/internal/logging"
	"github.com/postalsys/muti-metroo/internal/protocol"
	"github.com/postalsys/muti-metroo/internal/socks5"
)

// ICMP-related errors
var (
	ErrICMPNoRoute        = errors.New("no ICMP route available")
	ErrICMPStreamNotFound = errors.New("ICMP stream not found")
)

// icmpRelayEntry tracks an ICMP session being relayed through this agent (transit).
type icmpRelayEntry struct {
	UpstreamPeer   identity.AgentID
	UpstreamID     uint64
	DownstreamPeer identity.AgentID
	DownstreamID   uint64
}

// icmpIngressAssociation tracks a SOCKS5 ICMP session from the ingress side.
type icmpIngressAssociation struct {
	StreamID         uint64
	RequestID        uint64
	DestIP           net.IP
	ExitPeerID       identity.AgentID
	NextHop          identity.AgentID
	SessionKey       *crypto.SessionKey
	EphemeralPrivKey [32]byte
	EphemeralPubKey  [32]byte
	PendingOpen      chan struct{}
	OpenErr          error
	SOCKS5Assoc      *socks5.ICMPAssociation
	closeOnce        sync.Once
	mu               sync.RWMutex
}

// closePendingOpen safely closes the PendingOpen channel.
func (a *icmpIngressAssociation) closePendingOpen(err error) {
	a.closeOnce.Do(func() {
		if err != nil {
			a.mu.Lock()
			a.OpenErr = err
			a.mu.Unlock()
		}
		close(a.PendingOpen)
	})
}

// ICMP relay tracking (for transit nodes)
var icmpRelayMu sync.RWMutex
var icmpRelayByUpstream = make(map[uint64]*icmpRelayEntry)
var icmpRelayByDownstream = make(map[uint64]*icmpRelayEntry)

// ICMP ingress tracking (for SOCKS5 ingress)
var icmpIngressMu sync.RWMutex
var icmpIngressByStream = make(map[uint64]*icmpIngressAssociation)

// generateICMPRequestID generates a cryptographically random request ID.
// Using crypto/rand prevents session correlation attacks.
func generateICMPRequestID() uint64 {
	var buf [8]byte
	if _, err := cryptorand.Read(buf[:]); err != nil {
		// Fallback should never happen, but use time-based ID if it does
		panic("crypto/rand failed: " + err.Error())
	}
	return binary.BigEndian.Uint64(buf[:])
}

// deriveICMPSessionKey performs ECDH key exchange and derives a session key for ICMP sessions.
// The ephPrivKey is zeroed after use.
// Returns nil if the remote key is zero (encryption disabled), or an error if ECDH fails.
func deriveICMPSessionKey(
	ephPrivKey *[32]byte,
	ephPubKey [32]byte,
	remotePubKey [32]byte,
	requestID uint64,
) (*crypto.SessionKey, error) {
	var zeroKey [protocol.EphemeralKeySize]byte
	if remotePubKey == zeroKey {
		return nil, nil
	}

	sharedSecret, err := crypto.ComputeECDH(*ephPrivKey, remotePubKey)
	if err != nil {
		return nil, err
	}

	// Zero out private key immediately
	crypto.ZeroKey(ephPrivKey)

	// Derive session key (caller is initiator)
	sessionKey := crypto.DeriveSessionKey(sharedSecret, requestID, ephPubKey, remotePubKey, true)
	crypto.ZeroKey(&sharedSecret)

	return sessionKey, nil
}

// handleICMPOpen processes an ICMP_OPEN frame.
func (a *Agent) handleICMPOpen(peerID identity.AgentID, frame *protocol.Frame) {
	open, err := protocol.DecodeICMPOpen(frame.Payload)
	if err != nil {
		a.logger.Debug("failed to decode ICMP_OPEN frame", "error", err)
		return
	}

	a.logger.Debug("received ICMP_OPEN",
		logging.KeyStreamID, frame.StreamID,
		"from_peer", peerID.ShortString(),
		"remaining_path_len", len(open.RemainingPath))

	// Check if we are the exit node (path is empty)
	if len(open.RemainingPath) == 0 {
		// We are the exit node - handle with ICMP handler
		if a.icmpHandler == nil {
			a.sendICMPOpenErr(peerID, frame.StreamID, open.RequestID, protocol.ErrICMPDisabled, "ICMP echo disabled")
			return
		}

		ctx := context.Background()
		a.icmpHandler.HandleICMPOpen(ctx, peerID, frame.StreamID, open, open.EphemeralPubKey)
		return
	}

	// Relay to next hop
	nextHop := open.RemainingPath[0]

	conn := a.peerMgr.GetPeer(nextHop)
	if conn == nil {
		a.sendICMPOpenErr(peerID, frame.StreamID, open.RequestID, protocol.ErrHostUnreachable, "no route to next hop")
		return
	}

	// Generate new downstream stream ID
	downstreamID := conn.NextStreamID()

	// Create relay entry
	relay := &icmpRelayEntry{
		UpstreamPeer:   peerID,
		UpstreamID:     frame.StreamID,
		DownstreamPeer: nextHop,
		DownstreamID:   downstreamID,
	}

	icmpRelayMu.Lock()
	icmpRelayByUpstream[frame.StreamID] = relay
	icmpRelayByDownstream[downstreamID] = relay
	icmpRelayMu.Unlock()

	// Update remaining path
	newPath := open.RemainingPath[1:]

	// Forward with new stream ID
	fwdOpen := &protocol.ICMPOpen{
		RequestID:       open.RequestID,
		DestIP:          open.DestIP,
		TTL:             open.TTL,
		RemainingPath:   newPath,
		EphemeralPubKey: open.EphemeralPubKey,
	}

	fwdFrame := &protocol.Frame{
		Type:     protocol.FrameICMPOpen,
		StreamID: downstreamID,
		Payload:  fwdOpen.Encode(),
	}

	a.logger.Debug("relaying ICMP_OPEN to next hop",
		logging.KeyStreamID, downstreamID,
		"next_hop", nextHop.ShortString(),
		"remaining_path_len", len(newPath))

	if err := a.peerMgr.SendToPeer(nextHop, fwdFrame); err != nil {
		a.logger.Debug("failed to relay ICMP_OPEN",
			"error", err,
			"next_hop", nextHop.ShortString())

		// Clean up relay entry
		icmpRelayMu.Lock()
		delete(icmpRelayByUpstream, frame.StreamID)
		delete(icmpRelayByDownstream, downstreamID)
		icmpRelayMu.Unlock()

		a.sendICMPOpenErr(peerID, frame.StreamID, open.RequestID, protocol.ErrConnectionRefused, err.Error())
	}
}

// handleICMPOpenAck processes an ICMP_OPEN_ACK frame.
func (a *Agent) handleICMPOpenAck(peerID identity.AgentID, frame *protocol.Frame) {
	a.logger.Debug("received ICMP_OPEN_ACK",
		logging.KeyStreamID, frame.StreamID,
		"from_peer", peerID.ShortString())

	// Check if this is a relay response - copy fields under lock
	icmpRelayMu.RLock()
	relay := icmpRelayByDownstream[frame.StreamID]
	var relayUpstreamID uint64
	var relayUpstreamPeer, relayDownstreamPeer identity.AgentID
	if relay != nil {
		relayUpstreamID = relay.UpstreamID
		relayUpstreamPeer = relay.UpstreamPeer
		relayDownstreamPeer = relay.DownstreamPeer
	}
	icmpRelayMu.RUnlock()

	if relay != nil && peerID == relayDownstreamPeer {
		a.logger.Debug("relaying ICMP_OPEN_ACK upstream",
			logging.KeyStreamID, relayUpstreamID,
			"upstream_peer", relayUpstreamPeer.ShortString())

		// Forward ACK to upstream
		fwdFrame := &protocol.Frame{
			Type:     protocol.FrameICMPOpenAck,
			StreamID: relayUpstreamID,
			Payload:  frame.Payload,
		}
		a.peerMgr.SendToPeer(relayUpstreamPeer, fwdFrame)
		return
	}

	// Check if this is for ingress (SOCKS5 client initiated)
	icmpIngressMu.RLock()
	ingress := icmpIngressByStream[frame.StreamID]
	if ingress == nil {
		icmpIngressMu.RUnlock()
	} else {
		icmpIngressMu.RUnlock()

		ack, err := protocol.DecodeICMPOpenAck(frame.Payload)
		if err != nil {
			ingress.closePendingOpen(err)
			return
		}

		sessionKey, err := deriveICMPSessionKey(&ingress.EphemeralPrivKey, ingress.EphemeralPubKey, ack.EphemeralPubKey, ack.RequestID)
		if err != nil {
			ingress.closePendingOpen(err)
			return
		}

		if sessionKey != nil {
			ingress.mu.Lock()
			ingress.SessionKey = sessionKey
			ingress.mu.Unlock()
		}

		ingress.closePendingOpen(nil)
		return
	}

	// Check if this is for WebSocket session
	icmpWSSessionMu.RLock()
	wsSession := icmpWSSessionByStream[frame.StreamID]
	if wsSession == nil {
		icmpWSSessionMu.RUnlock()
		return
	}
	icmpWSSessionMu.RUnlock()

	ack, err := protocol.DecodeICMPOpenAck(frame.Payload)
	if err != nil {
		wsSession.closePendingOpenWS(err)
		return
	}

	sessionKey, err := deriveICMPSessionKey(&wsSession.EphemeralPrivKey, wsSession.EphemeralPubKey, ack.EphemeralPubKey, ack.RequestID)
	if err != nil {
		wsSession.closePendingOpenWS(err)
		return
	}

	if sessionKey != nil {
		wsSession.mu.Lock()
		wsSession.SessionKey = sessionKey
		wsSession.mu.Unlock()
	}

	wsSession.closePendingOpenWS(nil)
}

// handleICMPOpenErr processes an ICMP_OPEN_ERR frame.
func (a *Agent) handleICMPOpenErr(peerID identity.AgentID, frame *protocol.Frame) {
	// Check if this is a relay response - use write lock to check and delete atomically
	icmpRelayMu.Lock()
	relay := icmpRelayByDownstream[frame.StreamID]
	var relayUpstreamID uint64
	var relayUpstreamPeer, relayDownstreamPeer identity.AgentID
	var isRelay bool
	if relay != nil {
		relayUpstreamID = relay.UpstreamID
		relayUpstreamPeer = relay.UpstreamPeer
		relayDownstreamPeer = relay.DownstreamPeer
		if peerID == relayDownstreamPeer {
			isRelay = true
			// Clean up relay entry while holding lock
			delete(icmpRelayByUpstream, relayUpstreamID)
			delete(icmpRelayByDownstream, frame.StreamID)
		}
	}
	icmpRelayMu.Unlock()

	if isRelay {
		// Forward error to upstream
		fwdFrame := &protocol.Frame{
			Type:     protocol.FrameICMPOpenErr,
			StreamID: relayUpstreamID,
			Payload:  frame.Payload,
		}
		a.peerMgr.SendToPeer(relayUpstreamPeer, fwdFrame)
		return
	}

	// Check if this is for ingress (SOCKS5 client initiated)
	icmpIngressMu.Lock()
	ingress := icmpIngressByStream[frame.StreamID]
	if ingress != nil {
		delete(icmpIngressByStream, frame.StreamID)
	}
	icmpIngressMu.Unlock()

	if ingress != nil {
		errMsg, err := protocol.DecodeICMPOpenErr(frame.Payload)
		if err != nil {
			ingress.closePendingOpen(err)
			return
		}
		ingress.closePendingOpen(fmt.Errorf("ICMP open failed: %s (code %d)", errMsg.Message, errMsg.ErrorCode))
		return
	}

	// Check if this is for WebSocket session
	icmpWSSessionMu.Lock()
	wsSession := icmpWSSessionByStream[frame.StreamID]
	if wsSession != nil {
		delete(icmpWSSessionByStream, frame.StreamID)
	}
	icmpWSSessionMu.Unlock()

	if wsSession == nil {
		return
	}

	errMsg, err := protocol.DecodeICMPOpenErr(frame.Payload)
	if err != nil {
		wsSession.closePendingOpenWS(err)
		return
	}

	wsSession.closePendingOpenWS(fmt.Errorf("ICMP open failed: %s (code %d)", errMsg.Message, errMsg.ErrorCode))
}

// handleICMPEcho processes an ICMP_ECHO frame.
func (a *Agent) handleICMPEcho(peerID identity.AgentID, frame *protocol.Frame) {
	// Check if this is for our ICMP handler (exit node receiving from mesh)
	if a.icmpHandler != nil {
		if session := a.icmpHandler.GetSession(frame.StreamID); session != nil {
			echo, err := protocol.DecodeICMPEcho(frame.Payload)
			if err != nil {
				return
			}
			a.icmpHandler.HandleICMPEcho(peerID, frame.StreamID, echo)
			return
		}
	}

	// Check if this is for ingress (reply coming back to SOCKS5 client)
	// Hold lock while getting ingress and extracting fields needed for processing
	icmpIngressMu.RLock()
	ingress := icmpIngressByStream[frame.StreamID]
	icmpIngressMu.RUnlock()

	if ingress != nil {
		echo, err := protocol.DecodeICMPEcho(frame.Payload)
		if err != nil {
			return
		}

		// Only process replies
		if !echo.IsReply {
			return
		}

		// Decrypt if we have a session key - get fields under fine-grained lock
		ingress.mu.RLock()
		sessionKey := ingress.SessionKey
		socks5Assoc := ingress.SOCKS5Assoc
		ingress.mu.RUnlock()

		var plaintext []byte
		if sessionKey != nil {
			plaintext, err = sessionKey.Decrypt(echo.Data)
			if err != nil {
				return
			}
		} else {
			plaintext = echo.Data
		}

		// Forward to SOCKS5 client
		if socks5Assoc != nil {
			socks5Assoc.WriteToClient(echo.Identifier, echo.Sequence, plaintext)
		}
		return
	}

	// Check if this is for WebSocket session (reply coming back to CLI ping)
	icmpWSSessionMu.RLock()
	wsSession := icmpWSSessionByStream[frame.StreamID]
	icmpWSSessionMu.RUnlock()

	if wsSession != nil {
		echo, err := protocol.DecodeICMPEcho(frame.Payload)
		if err != nil {
			return
		}

		// Only process replies
		if !echo.IsReply {
			return
		}

		// Decrypt if we have a session key - get fields under fine-grained lock
		wsSession.mu.RLock()
		sessionKey := wsSession.SessionKey
		closed := wsSession.closed
		wsSession.mu.RUnlock()

		if closed {
			return
		}

		var plaintext []byte
		var errStr string
		if sessionKey != nil {
			plaintext, err = sessionKey.Decrypt(echo.Data)
			if err != nil {
				errStr = err.Error()
			}
		} else {
			plaintext = echo.Data
		}

		// Send to WebSocket via channel
		select {
		case wsSession.ReceiveEcho <- &health.ICMPEchoResponse{
			Identifier: echo.Identifier,
			Sequence:   echo.Sequence,
			Payload:    plaintext,
			Error:      errStr,
		}:
		default:
			// Channel full, drop the response
		}
		return
	}

	// Check if this is a relay - copy fields under lock
	icmpRelayMu.RLock()
	relayUp := icmpRelayByUpstream[frame.StreamID]
	relayDown := icmpRelayByDownstream[frame.StreamID]
	var upDownstreamID, upUpstreamID uint64
	var upDownstreamPeer, upUpstreamPeer identity.AgentID
	var downUpstreamID, downDownstreamID uint64
	var downUpstreamPeer, downDownstreamPeer identity.AgentID
	if relayUp != nil {
		upDownstreamID = relayUp.DownstreamID
		upDownstreamPeer = relayUp.DownstreamPeer
		upUpstreamPeer = relayUp.UpstreamPeer
	}
	if relayDown != nil {
		downUpstreamID = relayDown.UpstreamID
		downUpstreamPeer = relayDown.UpstreamPeer
		downDownstreamPeer = relayDown.DownstreamPeer
	}
	_ = upUpstreamID       // unused but kept for symmetry
	_ = downDownstreamID   // unused but kept for symmetry
	icmpRelayMu.RUnlock()

	if relayUp != nil && peerID == upUpstreamPeer {
		// Forward downstream
		fwdFrame := &protocol.Frame{
			Type:     protocol.FrameICMPEcho,
			StreamID: upDownstreamID,
			Payload:  frame.Payload,
		}
		a.peerMgr.SendToPeer(upDownstreamPeer, fwdFrame)
		return
	}

	if relayDown != nil && peerID == downDownstreamPeer {
		// Forward upstream
		fwdFrame := &protocol.Frame{
			Type:     protocol.FrameICMPEcho,
			StreamID: downUpstreamID,
			Payload:  frame.Payload,
		}
		a.peerMgr.SendToPeer(downUpstreamPeer, fwdFrame)
	}
}

// handleICMPClose processes an ICMP_CLOSE frame.
func (a *Agent) handleICMPClose(peerID identity.AgentID, frame *protocol.Frame) {
	// Check if this is for our ICMP handler (exit node)
	if a.icmpHandler != nil {
		a.icmpHandler.HandleICMPClose(peerID, frame.StreamID)
	}

	// Check if this is for ingress (SOCKS5 client session)
	icmpIngressMu.Lock()
	ingress := icmpIngressByStream[frame.StreamID]
	if ingress != nil {
		delete(icmpIngressByStream, frame.StreamID)
	}
	icmpIngressMu.Unlock()

	// Note: We don't need to do anything special for ingress close -
	// the SOCKS5 connection will be closed when the TCP connection ends

	// Check if this is for WebSocket session
	icmpWSSessionMu.Lock()
	wsSession := icmpWSSessionByStream[frame.StreamID]
	if wsSession != nil {
		delete(icmpWSSessionByStream, frame.StreamID)
	}
	icmpWSSessionMu.Unlock()

	if wsSession != nil {
		wsSession.close()
	}

	// Check if this is a relay - copy fields and delete atomically under lock
	icmpRelayMu.Lock()
	relayUp := icmpRelayByUpstream[frame.StreamID]
	relayDown := icmpRelayByDownstream[frame.StreamID]

	var upDownstreamID uint64
	var upDownstreamPeer, upUpstreamPeer identity.AgentID
	var downUpstreamID uint64
	var downUpstreamPeer, downDownstreamPeer identity.AgentID

	if relayUp != nil {
		upDownstreamID = relayUp.DownstreamID
		upDownstreamPeer = relayUp.DownstreamPeer
		upUpstreamPeer = relayUp.UpstreamPeer
		delete(icmpRelayByUpstream, frame.StreamID)
		delete(icmpRelayByDownstream, upDownstreamID)
	}
	if relayDown != nil {
		downUpstreamID = relayDown.UpstreamID
		downUpstreamPeer = relayDown.UpstreamPeer
		downDownstreamPeer = relayDown.DownstreamPeer
		delete(icmpRelayByDownstream, frame.StreamID)
		delete(icmpRelayByUpstream, downUpstreamID)
	}
	icmpRelayMu.Unlock()

	if relayUp != nil && peerID == upUpstreamPeer {
		// Forward downstream
		fwdFrame := &protocol.Frame{
			Type:     protocol.FrameICMPClose,
			StreamID: upDownstreamID,
			Payload:  frame.Payload,
		}
		a.peerMgr.SendToPeer(upDownstreamPeer, fwdFrame)
	}

	if relayDown != nil && peerID == downDownstreamPeer {
		// Forward upstream
		fwdFrame := &protocol.Frame{
			Type:     protocol.FrameICMPClose,
			StreamID: downUpstreamID,
			Payload:  frame.Payload,
		}
		a.peerMgr.SendToPeer(downUpstreamPeer, fwdFrame)
	}
}

// sendICMPOpenErr is a helper to send an ICMP_OPEN_ERR frame.
func (a *Agent) sendICMPOpenErr(peerID identity.AgentID, streamID uint64, requestID uint64, errCode uint16, msg string) {
	errPayload := &protocol.ICMPOpenErr{
		RequestID: requestID,
		ErrorCode: errCode,
		Message:   msg,
	}

	frame := &protocol.Frame{
		Type:     protocol.FrameICMPOpenErr,
		StreamID: streamID,
		Payload:  errPayload.Encode(),
	}

	a.peerMgr.SendToPeer(peerID, frame)
}

// WriteICMPOpenAck implements icmp.DataWriter.
func (a *Agent) WriteICMPOpenAck(peerID identity.AgentID, streamID uint64, ack *protocol.ICMPOpenAck) error {
	frame := &protocol.Frame{
		Type:     protocol.FrameICMPOpenAck,
		StreamID: streamID,
		Payload:  ack.Encode(),
	}

	return a.peerMgr.SendToPeer(peerID, frame)
}

// WriteICMPOpenErr implements icmp.DataWriter.
func (a *Agent) WriteICMPOpenErr(peerID identity.AgentID, streamID uint64, errMsg *protocol.ICMPOpenErr) error {
	frame := &protocol.Frame{
		Type:     protocol.FrameICMPOpenErr,
		StreamID: streamID,
		Payload:  errMsg.Encode(),
	}

	return a.peerMgr.SendToPeer(peerID, frame)
}

// WriteICMPEcho implements icmp.DataWriter.
func (a *Agent) WriteICMPEcho(peerID identity.AgentID, streamID uint64, echo *protocol.ICMPEcho) error {
	frame := &protocol.Frame{
		Type:     protocol.FrameICMPEcho,
		StreamID: streamID,
		Payload:  echo.Encode(),
	}

	return a.peerMgr.SendToPeer(peerID, frame)
}

// WriteICMPClose implements icmp.DataWriter.
func (a *Agent) WriteICMPClose(peerID identity.AgentID, streamID uint64, reason uint8) error {
	closeFrame := &protocol.ICMPClose{
		Reason: reason,
	}

	frame := &protocol.Frame{
		Type:     protocol.FrameICMPClose,
		StreamID: streamID,
		Payload:  closeFrame.Encode(),
	}

	return a.peerMgr.SendToPeer(peerID, frame)
}

// CreateICMPSession implements socks5.ICMPHandler.
// Called when a SOCKS5 client requests ICMP ECHO command.
func (a *Agent) CreateICMPSession(ctx context.Context, destIP net.IP) (uint64, error) {
	// Route lookup based on destination IP
	route := a.routeMgr.Lookup(destIP)
	if route == nil {
		return 0, ErrICMPNoRoute
	}

	nextHop := route.NextHop
	conn := a.peerMgr.GetPeer(nextHop)
	if conn == nil {
		return 0, fmt.Errorf("no connection to next hop: %s", nextHop.ShortString())
	}

	// Build remaining path
	var remainingPath []identity.AgentID
	rPath := route.Path
	for i, id := range rPath {
		if id == nextHop && i+1 < len(rPath) {
			remainingPath = make([]identity.AgentID, len(rPath)-i-1)
			copy(remainingPath, rPath[i+1:])
			break
		}
	}

	streamID := conn.NextStreamID()
	requestID := generateICMPRequestID()

	ephPriv, ephPub, err := crypto.GenerateEphemeralKeypair()
	if err != nil {
		return 0, err
	}

	assoc := &icmpIngressAssociation{
		StreamID:         streamID,
		RequestID:        requestID,
		DestIP:           destIP,
		ExitPeerID:       route.OriginAgent,
		NextHop:          nextHop,
		EphemeralPrivKey: ephPriv,
		EphemeralPubKey:  ephPub,
		PendingOpen:      make(chan struct{}),
	}

	// Register in map
	icmpIngressMu.Lock()
	icmpIngressByStream[streamID] = assoc
	icmpIngressMu.Unlock()

	// Build and send ICMP_OPEN
	open := &protocol.ICMPOpen{
		RequestID:       requestID,
		DestIP:          destIP.To4(),
		TTL:             uint8(len(rPath)),
		RemainingPath:   remainingPath,
		EphemeralPubKey: ephPub,
	}

	frame := &protocol.Frame{
		Type:     protocol.FrameICMPOpen,
		StreamID: streamID,
		Payload:  open.Encode(),
	}

	a.logger.Debug("Creating ICMP session",
		logging.KeyStreamID, streamID,
		"dest_ip", destIP.String(),
		"exit", route.OriginAgent.ShortString(),
		"next_hop", nextHop.ShortString())

	if err := a.peerMgr.SendToPeer(nextHop, frame); err != nil {
		// Cleanup
		icmpIngressMu.Lock()
		delete(icmpIngressByStream, streamID)
		icmpIngressMu.Unlock()
		assoc.closePendingOpen(err)
		return 0, err
	}

	// Wait for ACK with timeout
	select {
	case <-ctx.Done():
		assoc.closePendingOpen(ctx.Err())
		return 0, ctx.Err()
	case <-assoc.PendingOpen:
		if assoc.OpenErr != nil {
			return 0, assoc.OpenErr
		}
		return streamID, nil
	}
}

// SetSOCKS5ICMPAssociation links a SOCKS5 ICMP association to an ingress stream.
func (a *Agent) SetSOCKS5ICMPAssociation(streamID uint64, socks5Assoc *socks5.ICMPAssociation) {
	icmpIngressMu.Lock()
	defer icmpIngressMu.Unlock()

	if assoc := icmpIngressByStream[streamID]; assoc != nil {
		assoc.mu.Lock()
		assoc.SOCKS5Assoc = socks5Assoc
		assoc.mu.Unlock()
	}
}

// RelayICMPEcho implements socks5.ICMPHandler.
// Encrypts and sends an ICMP echo request through the mesh.
func (a *Agent) RelayICMPEcho(streamID uint64, identifier, sequence uint16, payload []byte) error {
	// Get association and extract fields under lock
	icmpIngressMu.RLock()
	assoc := icmpIngressByStream[streamID]
	if assoc == nil {
		icmpIngressMu.RUnlock()
		return ErrICMPStreamNotFound
	}
	icmpIngressMu.RUnlock()

	// Encrypt payload if we have a session key - use fine-grained lock
	assoc.mu.RLock()
	sessionKey := assoc.SessionKey
	nextHop := assoc.NextHop
	assoc.mu.RUnlock()

	var ciphertext []byte
	var err error
	if sessionKey != nil {
		ciphertext, err = sessionKey.Encrypt(payload)
		if err != nil {
			return err
		}
	} else {
		ciphertext = payload
	}

	echo := &protocol.ICMPEcho{
		Identifier: identifier,
		Sequence:   sequence,
		IsReply:    false,
		Data:       ciphertext,
	}

	frame := &protocol.Frame{
		Type:     protocol.FrameICMPEcho,
		StreamID: streamID,
		Payload:  echo.Encode(),
	}

	return a.peerMgr.SendToPeer(nextHop, frame)
}

// CloseICMPSession implements socks5.ICMPHandler.
func (a *Agent) CloseICMPSession(streamID uint64) {
	icmpIngressMu.Lock()
	assoc := icmpIngressByStream[streamID]
	if assoc != nil {
		delete(icmpIngressByStream, streamID)
	}
	icmpIngressMu.Unlock()

	if assoc == nil {
		return
	}

	// Send ICMP_CLOSE to the mesh
	closeFrame := &protocol.ICMPClose{Reason: protocol.ICMPCloseNormal}
	frame := &protocol.Frame{
		Type:     protocol.FrameICMPClose,
		StreamID: streamID,
		Payload:  closeFrame.Encode(),
	}

	a.peerMgr.SendToPeer(assoc.NextHop, frame)

	a.logger.Debug("ICMP session closed",
		logging.KeyStreamID, streamID)
}

// IsICMPEnabled implements socks5.ICMPHandler.
// Returns true if this agent can relay ICMP traffic.
func (a *Agent) IsICMPEnabled() bool {
	// ICMP is enabled for ingress if we have any routes
	return a.cfg.SOCKS5.Enabled
}

// icmpWebSocketSession tracks an ICMP session from WebSocket API.
type icmpWebSocketSession struct {
	StreamID         uint64
	RequestID        uint64
	DestIP           net.IP
	ExitPeerID       identity.AgentID
	NextHop          identity.AgentID
	SessionKey       *crypto.SessionKey
	EphemeralPrivKey [32]byte
	EphemeralPubKey  [32]byte
	PendingOpen      chan struct{}
	OpenErr          error
	SendEcho         chan *health.ICMPEchoRequest
	ReceiveEcho      chan *health.ICMPEchoResponse
	Done             chan struct{}
	closeOnce        sync.Once
	mu               sync.RWMutex
	closed           bool
}

// closePendingOpen safely closes the PendingOpen channel.
func (s *icmpWebSocketSession) closePendingOpenWS(err error) {
	s.closeOnce.Do(func() {
		if err != nil {
			s.mu.Lock()
			s.OpenErr = err
			s.mu.Unlock()
		}
		close(s.PendingOpen)
	})
}

// close closes the session and all its channels.
func (s *icmpWebSocketSession) close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.mu.Unlock()

	close(s.Done)
}

// WebSocket ICMP session tracking
var icmpWSSessionMu sync.RWMutex
var icmpWSSessionByStream = make(map[uint64]*icmpWebSocketSession)

// OpenICMPSession implements health.ICMPProvider.
// Opens an ICMP session to a remote agent for the WebSocket ping API.
func (a *Agent) OpenICMPSession(ctx context.Context, targetID identity.AgentID, destIP net.IP) (*health.ICMPSession, error) {
	// Check if target is a direct peer
	var nextHop identity.AgentID
	var rPath []identity.AgentID
	var remainingPath []identity.AgentID

	if conn := a.peerMgr.GetPeer(targetID); conn != nil {
		nextHop = targetID
		rPath = []identity.AgentID{targetID}
	} else {
		// Find route via routing table
		routes := a.routeMgr.Table().GetRoutesFromAgent(targetID)
		if len(routes) == 0 {
			return nil, fmt.Errorf("no route to agent %s", targetID.ShortString())
		}

		route := routes[0]
		nextHop = route.NextHop
		rPath = route.Path

		// Build remaining path
		for i, id := range rPath {
			if id == nextHop && i+1 < len(rPath) {
				remainingPath = make([]identity.AgentID, len(rPath)-i-1)
				copy(remainingPath, rPath[i+1:])
				break
			}
		}
	}

	conn := a.peerMgr.GetPeer(nextHop)
	if conn == nil {
		return nil, fmt.Errorf("no connection to next hop: %s", nextHop.ShortString())
	}

	streamID := conn.NextStreamID()
	requestID := generateICMPRequestID()

	ephPriv, ephPub, err := crypto.GenerateEphemeralKeypair()
	if err != nil {
		return nil, err
	}

	session := &icmpWebSocketSession{
		StreamID:         streamID,
		RequestID:        requestID,
		DestIP:           destIP,
		ExitPeerID:       targetID,
		NextHop:          nextHop,
		EphemeralPrivKey: ephPriv,
		EphemeralPubKey:  ephPub,
		PendingOpen:      make(chan struct{}),
		SendEcho:         make(chan *health.ICMPEchoRequest, 16),
		ReceiveEcho:      make(chan *health.ICMPEchoResponse, 16),
		Done:             make(chan struct{}),
	}

	// Register in map
	icmpWSSessionMu.Lock()
	icmpWSSessionByStream[streamID] = session
	icmpWSSessionMu.Unlock()

	// Build and send ICMP_OPEN
	open := &protocol.ICMPOpen{
		RequestID:       requestID,
		DestIP:          destIP.To4(),
		TTL:             uint8(len(rPath)),
		RemainingPath:   remainingPath,
		EphemeralPubKey: ephPub,
	}

	frame := &protocol.Frame{
		Type:     protocol.FrameICMPOpen,
		StreamID: streamID,
		Payload:  open.Encode(),
	}

	a.logger.Debug("Opening WebSocket ICMP session",
		logging.KeyStreamID, streamID,
		"dest_ip", destIP.String(),
		"target", targetID.ShortString(),
		"next_hop", nextHop.ShortString())

	if err := a.peerMgr.SendToPeer(nextHop, frame); err != nil {
		// Cleanup
		icmpWSSessionMu.Lock()
		delete(icmpWSSessionByStream, streamID)
		icmpWSSessionMu.Unlock()
		session.closePendingOpenWS(err)
		return nil, err
	}

	// Wait for ACK with timeout
	select {
	case <-ctx.Done():
		session.closePendingOpenWS(ctx.Err())
		icmpWSSessionMu.Lock()
		delete(icmpWSSessionByStream, streamID)
		icmpWSSessionMu.Unlock()
		return nil, ctx.Err()
	case <-session.PendingOpen:
		if session.OpenErr != nil {
			icmpWSSessionMu.Lock()
			delete(icmpWSSessionByStream, streamID)
			icmpWSSessionMu.Unlock()
			return nil, session.OpenErr
		}
	}

	// Start goroutine to send echo requests from channel
	go a.runWSICMPSender(session)

	// Return the session interface
	return &health.ICMPSession{
		StreamID:    streamID,
		TargetID:    targetID,
		DestIP:      destIP,
		SendEcho:    session.SendEcho,
		ReceiveEcho: session.ReceiveEcho,
		Done:        session.Done,
		Close: func() {
			a.closeWSICMPSession(streamID)
		},
	}, nil
}

// runWSICMPSender reads from the SendEcho channel and sends to the mesh.
func (a *Agent) runWSICMPSender(session *icmpWebSocketSession) {
	for {
		select {
		case <-session.Done:
			return
		case req, ok := <-session.SendEcho:
			if !ok {
				return
			}

			// Encrypt payload if we have a session key
			session.mu.RLock()
			sessionKey := session.SessionKey
			nextHop := session.NextHop
			streamID := session.StreamID
			session.mu.RUnlock()

			var ciphertext []byte
			var err error
			if sessionKey != nil {
				ciphertext, err = sessionKey.Encrypt(req.Payload)
				if err != nil {
					a.logger.Debug("failed to encrypt ICMP payload", "error", err)
					continue
				}
			} else {
				ciphertext = req.Payload
			}

			echo := &protocol.ICMPEcho{
				Identifier: req.Identifier,
				Sequence:   req.Sequence,
				IsReply:    false,
				Data:       ciphertext,
			}

			frame := &protocol.Frame{
				Type:     protocol.FrameICMPEcho,
				StreamID: streamID,
				Payload:  echo.Encode(),
			}

			if err := a.peerMgr.SendToPeer(nextHop, frame); err != nil {
				a.logger.Debug("failed to send ICMP echo", "error", err)
			}
		}
	}
}

// closeWSICMPSession closes a WebSocket ICMP session.
func (a *Agent) closeWSICMPSession(streamID uint64) {
	icmpWSSessionMu.Lock()
	session := icmpWSSessionByStream[streamID]
	if session != nil {
		delete(icmpWSSessionByStream, streamID)
	}
	icmpWSSessionMu.Unlock()

	if session == nil {
		return
	}

	session.close()

	// Send ICMP_CLOSE to the mesh
	closeFrame := &protocol.ICMPClose{Reason: protocol.ICMPCloseNormal}
	frame := &protocol.Frame{
		Type:     protocol.FrameICMPClose,
		StreamID: streamID,
		Payload:  closeFrame.Encode(),
	}

	a.peerMgr.SendToPeer(session.NextHop, frame)

	a.logger.Debug("WebSocket ICMP session closed",
		logging.KeyStreamID, streamID)
}

// Compile-time interface verification
var _ icmp.DataWriter = (*Agent)(nil)
var _ socks5.ICMPHandler = (*Agent)(nil)
var _ health.ICMPProvider = (*Agent)(nil)
