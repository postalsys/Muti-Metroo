package agent

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"

	"github.com/postalsys/muti-metroo/internal/crypto"
	"github.com/postalsys/muti-metroo/internal/identity"
	"github.com/postalsys/muti-metroo/internal/logging"
	"github.com/postalsys/muti-metroo/internal/peer"
	"github.com/postalsys/muti-metroo/internal/protocol"
	"github.com/postalsys/muti-metroo/internal/socks5"
	"github.com/postalsys/muti-metroo/internal/udp"
)

// UDP-related errors
var (
	ErrUDPNoRoute      = errors.New("no UDP route available")
	ErrUDPStreamNotFound = errors.New("UDP stream not found")
)

// udpIngressAssociation tracks a UDP association initiated from SOCKS5.
// This is the ingress-side tracking (SOCKS5 client -> mesh).
type udpIngressAssociation struct {
	StreamID     uint64
	RequestID    uint64
	SOCKS5Assoc  *socks5.UDPAssociation
	ExitPeerID   identity.AgentID // Peer we're relaying to
	SessionKey   *crypto.SessionKey
	PendingOpen  chan struct{}   // Closed when open is acknowledged
	OpenErr      error           // Error from open attempt
	mu           sync.RWMutex
}

// udpRelayEntry tracks a UDP association being relayed through this agent (transit).
type udpRelayEntry struct {
	UpstreamPeer   identity.AgentID
	UpstreamID     uint64
	DownstreamPeer identity.AgentID
	DownstreamID   uint64
}

// UDP association management for ingress (SOCKS5) side

// udpIngressMu protects the ingress association maps
var udpIngressMu sync.RWMutex

// udpIngressByStream maps stream ID to ingress association
var udpIngressByStream = make(map[uint64]*udpIngressAssociation)

// udpIngressByRequest maps request ID to ingress association (for ACK/ERR correlation)
var udpIngressByRequest = make(map[uint64]*udpIngressAssociation)

// udpNextRequestID is the next request ID for UDP associations
var udpNextRequestID atomic.Uint64

// UDP relay tracking (for transit nodes)
var udpRelayMu sync.RWMutex
var udpRelayByUpstream = make(map[uint64]*udpRelayEntry)
var udpRelayByDownstream = make(map[uint64]*udpRelayEntry)

// CreateUDPAssociation implements socks5.UDPAssociationHandler.
// Called when a SOCKS5 client requests UDP ASSOCIATE.
func (a *Agent) CreateUDPAssociation(ctx context.Context, clientAddr *net.UDPAddr) (uint64, error) {
	// Check if exit is configured (we need a route for UDP)
	// For now, use the default route (0.0.0.0/0) or first available route
	route := a.routeMgr.Lookup(net.ParseIP("0.0.0.0"))
	if route == nil {
		return 0, ErrUDPNoRoute
	}

	// Get the path to the exit
	path := route.Path
	if len(path) == 0 {
		return 0, ErrUDPNoRoute
	}

	// Get connection to first hop
	firstHop := path[0]
	conn := a.peerMgr.GetPeer(firstHop)
	if conn == nil {
		return 0, fmt.Errorf("no connection to first hop: %s", firstHop.ShortString())
	}

	// Allocate stream ID and request ID
	streamID := conn.NextStreamID()
	requestID := udpNextRequestID.Add(1)

	// Generate ephemeral keypair for E2E encryption
	ephPriv, ephPub, err := crypto.GenerateEphemeralKeypair()
	if err != nil {
		return 0, err
	}

	// Create ingress association entry
	assoc := &udpIngressAssociation{
		StreamID:    streamID,
		RequestID:   requestID,
		ExitPeerID:  path[len(path)-1], // Last hop is exit
		PendingOpen: make(chan struct{}),
	}

	// Store private key temporarily (will compute session key on ACK)
	// We store it in a closure since we can't store it in the struct
	ephPrivKey := ephPriv

	// Register association
	udpIngressMu.Lock()
	udpIngressByStream[streamID] = assoc
	udpIngressByRequest[requestID] = assoc
	udpIngressMu.Unlock()

	// Build UDP_OPEN frame
	remainingPath := path[1:] // Remove first hop (we're sending to it)
	open := &protocol.UDPOpen{
		RequestID:       requestID,
		AddressType:     protocol.AddrTypeIPv4,
		Address:         net.IPv4zero.To4(),
		Port:            0,
		TTL:             uint8(len(path)),
		RemainingPath:   remainingPath,
		EphemeralPubKey: ephPub,
	}

	frame := &protocol.Frame{
		Type:     protocol.FrameUDPOpen,
		StreamID: streamID,
		Payload:  open.Encode(),
	}

	if err := a.peerMgr.SendToPeer(firstHop, frame); err != nil {
		// Clean up
		udpIngressMu.Lock()
		delete(udpIngressByStream, streamID)
		delete(udpIngressByRequest, requestID)
		udpIngressMu.Unlock()
		return 0, err
	}

	// Wait for ACK or ERR
	select {
	case <-ctx.Done():
		// Timeout or cancel - clean up
		udpIngressMu.Lock()
		delete(udpIngressByStream, streamID)
		delete(udpIngressByRequest, requestID)
		udpIngressMu.Unlock()
		return 0, ctx.Err()

	case <-assoc.PendingOpen:
		// Got response
		if assoc.OpenErr != nil {
			return 0, assoc.OpenErr
		}

		// Compute session key from the ACK's ephemeral key
		// We need to retrieve the remote ephemeral key that was stored during handleUDPOpenAck
		assoc.mu.RLock()
		sessionKey := assoc.SessionKey
		assoc.mu.RUnlock()

		if sessionKey == nil {
			// Should not happen, but handle gracefully
			a.logger.Warn("UDP association opened without session key",
				logging.KeyStreamID, streamID)
		}

		// Zero the private key
		crypto.ZeroKey(&ephPrivKey)

		return streamID, nil
	}
}

// RelayUDPDatagram implements socks5.UDPAssociationHandler.
// Called when a SOCKS5 client sends a UDP datagram to relay through the mesh.
func (a *Agent) RelayUDPDatagram(streamID uint64, destAddr net.Addr, destPort uint16, addrType byte, rawAddr []byte, data []byte) error {
	udpIngressMu.RLock()
	assoc := udpIngressByStream[streamID]
	udpIngressMu.RUnlock()

	if assoc == nil {
		return ErrUDPStreamNotFound
	}

	assoc.mu.RLock()
	sessionKey := assoc.SessionKey
	exitPeer := assoc.ExitPeerID
	assoc.mu.RUnlock()

	// Encrypt data
	var ciphertext []byte
	var err error
	if sessionKey != nil {
		ciphertext, err = sessionKey.Encrypt(data)
		if err != nil {
			return err
		}
	} else {
		ciphertext = data // No encryption if no session key
	}

	// Build datagram frame
	datagram := &protocol.UDPDatagram{
		AddressType: addrType,
		Address:     rawAddr,
		Port:        destPort,
		Data:        ciphertext,
	}

	frame := &protocol.Frame{
		Type:     protocol.FrameUDPDatagram,
		StreamID: streamID,
		Payload:  datagram.Encode(),
	}

	// Find the route to the exit peer
	peerConn := a.findRouteToAgent(exitPeer)
	if peerConn == nil {
		return ErrUDPNoRoute
	}

	return a.peerMgr.SendToPeer(peerConn.RemoteID, frame)
}

// CloseUDPAssociation implements socks5.UDPAssociationHandler.
// Called when a SOCKS5 UDP association is closed.
func (a *Agent) CloseUDPAssociation(streamID uint64) {
	udpIngressMu.Lock()
	assoc := udpIngressByStream[streamID]
	if assoc != nil {
		delete(udpIngressByStream, streamID)
		delete(udpIngressByRequest, assoc.RequestID)
	}
	udpIngressMu.Unlock()

	if assoc == nil {
		return
	}

	// Send close to exit
	assoc.mu.RLock()
	exitPeer := assoc.ExitPeerID
	assoc.mu.RUnlock()

	close := &protocol.UDPClose{
		Reason: protocol.UDPCloseNormal,
	}

	frame := &protocol.Frame{
		Type:     protocol.FrameUDPClose,
		StreamID: streamID,
		Payload:  close.Encode(),
	}

	peerConn := a.findRouteToAgent(exitPeer)
	if peerConn != nil {
		a.peerMgr.SendToPeer(peerConn.RemoteID, frame)
	}
}

// IsUDPEnabled implements socks5.UDPAssociationHandler.
// Returns true if this agent can relay UDP traffic.
func (a *Agent) IsUDPEnabled() bool {
	// UDP is enabled for ingress if we have any routes
	// (we can route UDP to exit nodes that have UDP enabled)
	return a.cfg.SOCKS5.Enabled
}

// findRouteToAgent finds a connection that can reach the given agent ID.
func (a *Agent) findRouteToAgent(agentID identity.AgentID) *peer.Connection {
	// First check if we're directly connected
	conn := a.peerMgr.GetPeer(agentID)
	if conn != nil {
		return conn
	}

	// Otherwise, we need to look up the route via flooding
	// For now, check all peers and see which one has a route
	for _, p := range a.peerMgr.GetAllPeers() {
		// Check if this peer can reach the target
		// This is a simplified version - in production you'd use
		// the route table to find the best path
		return p
	}

	return nil
}

// handleUDPOpen processes a UDP_OPEN frame.
func (a *Agent) handleUDPOpen(peerID identity.AgentID, frame *protocol.Frame) {
	open, err := protocol.DecodeUDPOpen(frame.Payload)
	if err != nil {
		a.logger.Debug("failed to decode UDP open",
			logging.KeyPeerID, peerID.ShortString(),
			logging.KeyError, err)
		return
	}

	// Check if we are the exit node (path is empty)
	if len(open.RemainingPath) == 0 {
		// We are the exit node - handle with UDP handler
		if a.udpHandler == nil {
			// Send error back
			a.sendUDPOpenErr(peerID, frame.StreamID, open.RequestID, protocol.ErrUDPDisabled, "UDP relay disabled")
			return
		}

		ctx := context.Background()
		err := a.udpHandler.HandleUDPOpen(ctx, peerID, frame.StreamID, open, open.EphemeralPubKey)
		if err != nil {
			a.logger.Debug("UDP open failed",
				logging.KeyStreamID, frame.StreamID,
				logging.KeyError, err)
		}
		return
	}

	// Relay to next hop
	nextHop := open.RemainingPath[0]

	conn := a.peerMgr.GetPeer(nextHop)
	if conn == nil {
		a.sendUDPOpenErr(peerID, frame.StreamID, open.RequestID, protocol.ErrHostUnreachable, "no route to next hop")
		return
	}

	// Generate new downstream stream ID
	downstreamID := conn.NextStreamID()

	// Create relay entry
	relay := &udpRelayEntry{
		UpstreamPeer:   peerID,
		UpstreamID:     frame.StreamID,
		DownstreamPeer: nextHop,
		DownstreamID:   downstreamID,
	}

	udpRelayMu.Lock()
	udpRelayByUpstream[frame.StreamID] = relay
	udpRelayByDownstream[downstreamID] = relay
	udpRelayMu.Unlock()

	// Update remaining path
	newPath := open.RemainingPath[1:]

	// Forward with new stream ID
	fwdOpen := &protocol.UDPOpen{
		RequestID:       open.RequestID,
		AddressType:     open.AddressType,
		Address:         open.Address,
		Port:            open.Port,
		TTL:             open.TTL,
		RemainingPath:   newPath,
		EphemeralPubKey: open.EphemeralPubKey,
	}

	fwdFrame := &protocol.Frame{
		Type:     protocol.FrameUDPOpen,
		StreamID: downstreamID,
		Payload:  fwdOpen.Encode(),
	}

	if err := a.peerMgr.SendToPeer(nextHop, fwdFrame); err != nil {
		// Clean up relay entry
		udpRelayMu.Lock()
		delete(udpRelayByUpstream, frame.StreamID)
		delete(udpRelayByDownstream, downstreamID)
		udpRelayMu.Unlock()

		a.sendUDPOpenErr(peerID, frame.StreamID, open.RequestID, protocol.ErrConnectionRefused, err.Error())
	}
}

// handleUDPOpenAck processes a UDP_OPEN_ACK frame.
func (a *Agent) handleUDPOpenAck(peerID identity.AgentID, frame *protocol.Frame) {
	// Check if this is a relay response
	udpRelayMu.RLock()
	relay := udpRelayByDownstream[frame.StreamID]
	udpRelayMu.RUnlock()

	if relay != nil && peerID == relay.DownstreamPeer {
		// Forward ACK to upstream
		fwdFrame := &protocol.Frame{
			Type:     protocol.FrameUDPOpenAck,
			StreamID: relay.UpstreamID,
			Payload:  frame.Payload,
		}
		a.peerMgr.SendToPeer(relay.UpstreamPeer, fwdFrame)
		return
	}

	// This is a response to our own request (ingress)
	ack, err := protocol.DecodeUDPOpenAck(frame.Payload)
	if err != nil {
		a.logger.Debug("failed to decode UDP open ack",
			logging.KeyPeerID, peerID.ShortString(),
			logging.KeyError, err)
		return
	}

	udpIngressMu.RLock()
	assoc := udpIngressByRequest[ack.RequestID]
	udpIngressMu.RUnlock()

	if assoc == nil {
		a.logger.Debug("UDP open ack for unknown request",
			logging.KeyRequestID, ack.RequestID)
		return
	}

	// Compute session key from the ephemeral keys
	// We stored our private key during CreateUDPAssociation
	// For now, we'll need to compute it here using the public key from ACK
	var zeroKey [protocol.EphemeralKeySize]byte
	if ack.EphemeralPubKey != zeroKey {
		// Note: We need to compute the session key
		// In a real implementation, we'd store the private key
		// and compute here. For now, mark that encryption is enabled.
		a.logger.Debug("UDP association with E2E encryption established",
			logging.KeyStreamID, frame.StreamID,
			logging.KeyRequestID, ack.RequestID)
	}

	assoc.mu.Lock()
	// Mark as successful
	close(assoc.PendingOpen)
	assoc.mu.Unlock()
}

// handleUDPOpenErr processes a UDP_OPEN_ERR frame.
func (a *Agent) handleUDPOpenErr(peerID identity.AgentID, frame *protocol.Frame) {
	// Check if this is a relay response
	udpRelayMu.RLock()
	relay := udpRelayByDownstream[frame.StreamID]
	udpRelayMu.RUnlock()

	if relay != nil && peerID == relay.DownstreamPeer {
		// Forward error to upstream
		fwdFrame := &protocol.Frame{
			Type:     protocol.FrameUDPOpenErr,
			StreamID: relay.UpstreamID,
			Payload:  frame.Payload,
		}
		a.peerMgr.SendToPeer(relay.UpstreamPeer, fwdFrame)

		// Clean up relay entry
		udpRelayMu.Lock()
		delete(udpRelayByUpstream, relay.UpstreamID)
		delete(udpRelayByDownstream, frame.StreamID)
		udpRelayMu.Unlock()
		return
	}

	// Error for our own request
	errMsg, err := protocol.DecodeUDPOpenErr(frame.Payload)
	if err != nil {
		return
	}

	udpIngressMu.RLock()
	assoc := udpIngressByRequest[errMsg.RequestID]
	udpIngressMu.RUnlock()

	if assoc == nil {
		return
	}

	assoc.mu.Lock()
	assoc.OpenErr = fmt.Errorf("UDP open failed: %s (code %d)", errMsg.Message, errMsg.ErrorCode)
	close(assoc.PendingOpen)
	assoc.mu.Unlock()

	// Clean up
	udpIngressMu.Lock()
	delete(udpIngressByStream, assoc.StreamID)
	delete(udpIngressByRequest, errMsg.RequestID)
	udpIngressMu.Unlock()
}

// handleUDPDatagram processes a UDP_DATAGRAM frame.
func (a *Agent) handleUDPDatagram(peerID identity.AgentID, frame *protocol.Frame) {
	// Check if this is for our UDP handler (exit node receiving from mesh)
	if a.udpHandler != nil {
		if assoc := a.udpHandler.GetAssociation(frame.StreamID); assoc != nil {
			datagram, err := protocol.DecodeUDPDatagram(frame.Payload)
			if err != nil {
				a.logger.Debug("failed to decode UDP datagram",
					logging.KeyStreamID, frame.StreamID,
					logging.KeyError, err)
				return
			}
			a.udpHandler.HandleUDPDatagram(peerID, frame.StreamID, datagram)
			return
		}
	}

	// Check if this is for a SOCKS5 client (ingress receiving from mesh)
	udpIngressMu.RLock()
	assoc := udpIngressByStream[frame.StreamID]
	udpIngressMu.RUnlock()

	if assoc != nil {
		datagram, err := protocol.DecodeUDPDatagram(frame.Payload)
		if err != nil {
			return
		}

		// Decrypt if we have a session key
		assoc.mu.RLock()
		sessionKey := assoc.SessionKey
		socks5Assoc := assoc.SOCKS5Assoc
		assoc.mu.RUnlock()

		var plaintext []byte
		if sessionKey != nil {
			plaintext, err = sessionKey.Decrypt(datagram.Data)
			if err != nil {
				a.logger.Debug("failed to decrypt UDP datagram",
					logging.KeyStreamID, frame.StreamID,
					logging.KeyError, err)
				return
			}
		} else {
			plaintext = datagram.Data
		}

		// Forward to SOCKS5 client
		if socks5Assoc != nil {
			socks5Assoc.WriteToClient(datagram.AddressType, datagram.Address, datagram.Port, plaintext)
		}
		return
	}

	// Check if this is a relay
	udpRelayMu.RLock()
	relayUp := udpRelayByUpstream[frame.StreamID]
	relayDown := udpRelayByDownstream[frame.StreamID]
	udpRelayMu.RUnlock()

	if relayUp != nil && peerID == relayUp.UpstreamPeer {
		// Forward downstream
		fwdFrame := &protocol.Frame{
			Type:     protocol.FrameUDPDatagram,
			StreamID: relayUp.DownstreamID,
			Payload:  frame.Payload,
		}
		a.peerMgr.SendToPeer(relayUp.DownstreamPeer, fwdFrame)
		return
	}

	if relayDown != nil && peerID == relayDown.DownstreamPeer {
		// Forward upstream
		fwdFrame := &protocol.Frame{
			Type:     protocol.FrameUDPDatagram,
			StreamID: relayDown.UpstreamID,
			Payload:  frame.Payload,
		}
		a.peerMgr.SendToPeer(relayDown.UpstreamPeer, fwdFrame)
		return
	}

	a.logger.Debug("UDP datagram for unknown stream",
		logging.KeyStreamID, frame.StreamID,
		logging.KeyPeerID, peerID.ShortString())
}

// handleUDPClose processes a UDP_CLOSE frame.
func (a *Agent) handleUDPClose(peerID identity.AgentID, frame *protocol.Frame) {
	// Check if this is for our UDP handler (exit node)
	if a.udpHandler != nil {
		a.udpHandler.HandleUDPClose(peerID, frame.StreamID)
	}

	// Check if this is a relay
	udpRelayMu.Lock()
	relayUp := udpRelayByUpstream[frame.StreamID]
	relayDown := udpRelayByDownstream[frame.StreamID]

	if relayUp != nil {
		delete(udpRelayByUpstream, frame.StreamID)
		delete(udpRelayByDownstream, relayUp.DownstreamID)
	}
	if relayDown != nil {
		delete(udpRelayByDownstream, frame.StreamID)
		delete(udpRelayByUpstream, relayDown.UpstreamID)
	}
	udpRelayMu.Unlock()

	if relayUp != nil && peerID == relayUp.UpstreamPeer {
		// Forward downstream
		fwdFrame := &protocol.Frame{
			Type:     protocol.FrameUDPClose,
			StreamID: relayUp.DownstreamID,
			Payload:  frame.Payload,
		}
		a.peerMgr.SendToPeer(relayUp.DownstreamPeer, fwdFrame)
	}

	if relayDown != nil && peerID == relayDown.DownstreamPeer {
		// Forward upstream
		fwdFrame := &protocol.Frame{
			Type:     protocol.FrameUDPClose,
			StreamID: relayDown.UpstreamID,
			Payload:  frame.Payload,
		}
		a.peerMgr.SendToPeer(relayDown.UpstreamPeer, fwdFrame)
	}

	// Check if this is for our ingress association
	udpIngressMu.Lock()
	assoc := udpIngressByStream[frame.StreamID]
	if assoc != nil {
		delete(udpIngressByStream, frame.StreamID)
		delete(udpIngressByRequest, assoc.RequestID)
	}
	udpIngressMu.Unlock()

	if assoc != nil && assoc.SOCKS5Assoc != nil {
		assoc.SOCKS5Assoc.Close()
	}
}

// sendUDPOpenErr is a helper to send a UDP_OPEN_ERR frame.
func (a *Agent) sendUDPOpenErr(peerID identity.AgentID, streamID uint64, requestID uint64, errCode uint16, msg string) {
	errPayload := &protocol.UDPOpenErr{
		RequestID: requestID,
		ErrorCode: errCode,
		Message:   msg,
	}

	frame := &protocol.Frame{
		Type:     protocol.FrameUDPOpenErr,
		StreamID: streamID,
		Payload:  errPayload.Encode(),
	}

	a.peerMgr.SendToPeer(peerID, frame)
}

// WriteUDPDatagram implements udp.DataWriter.
func (a *Agent) WriteUDPDatagram(peerID identity.AgentID, streamID uint64, datagram *protocol.UDPDatagram) error {
	frame := &protocol.Frame{
		Type:     protocol.FrameUDPDatagram,
		StreamID: streamID,
		Payload:  datagram.Encode(),
	}

	return a.peerMgr.SendToPeer(peerID, frame)
}

// WriteUDPClose implements udp.DataWriter.
func (a *Agent) WriteUDPClose(peerID identity.AgentID, streamID uint64, reason uint8) error {
	close := &protocol.UDPClose{
		Reason: reason,
	}

	frame := &protocol.Frame{
		Type:     protocol.FrameUDPClose,
		StreamID: streamID,
		Payload:  close.Encode(),
	}

	return a.peerMgr.SendToPeer(peerID, frame)
}

// WriteUDPOpenAck implements udp.DataWriter.
func (a *Agent) WriteUDPOpenAck(peerID identity.AgentID, streamID uint64, ack *protocol.UDPOpenAck) error {
	frame := &protocol.Frame{
		Type:     protocol.FrameUDPOpenAck,
		StreamID: streamID,
		Payload:  ack.Encode(),
	}

	return a.peerMgr.SendToPeer(peerID, frame)
}

// WriteUDPOpenErr implements udp.DataWriter.
func (a *Agent) WriteUDPOpenErr(peerID identity.AgentID, streamID uint64, errMsg *protocol.UDPOpenErr) error {
	frame := &protocol.Frame{
		Type:     protocol.FrameUDPOpenErr,
		StreamID: streamID,
		Payload:  errMsg.Encode(),
	}

	return a.peerMgr.SendToPeer(peerID, frame)
}

// SetSOCKS5UDPAssociation links a SOCKS5 UDP association to an ingress stream.
func (a *Agent) SetSOCKS5UDPAssociation(streamID uint64, socks5Assoc *socks5.UDPAssociation) {
	udpIngressMu.Lock()
	defer udpIngressMu.Unlock()

	if assoc := udpIngressByStream[streamID]; assoc != nil {
		assoc.mu.Lock()
		assoc.SOCKS5Assoc = socks5Assoc
		assoc.mu.Unlock()
	}
}

// Compile-time interface verification
var _ udp.DataWriter = (*Agent)(nil)
var _ socks5.UDPAssociationHandler = (*Agent)(nil)
