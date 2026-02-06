package agent

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/postalsys/muti-metroo/internal/crypto"
	"github.com/postalsys/muti-metroo/internal/identity"
	"github.com/postalsys/muti-metroo/internal/logging"
	"github.com/postalsys/muti-metroo/internal/peer"
	"github.com/postalsys/muti-metroo/internal/protocol"
	"github.com/postalsys/muti-metroo/internal/routing"
	"github.com/postalsys/muti-metroo/internal/socks5"
	"github.com/postalsys/muti-metroo/internal/udp"
)

// UDP-related errors
var (
	ErrUDPNoRoute        = errors.New("no UDP route available")
	ErrUDPStreamNotFound = errors.New("UDP stream not found")
)

// udpDestAssociation tracks a single exit-path for one destination route.
// A SOCKS5 UDP association may have multiple of these (one per exit route).
type udpDestAssociation struct {
	StreamID         uint64
	RequestID        uint64
	ExitPeerID       identity.AgentID
	NextHop          identity.AgentID // First hop peer for sending frames
	SessionKey       *crypto.SessionKey
	EphemeralPrivKey [32]byte
	EphemeralPubKey  [32]byte
	PendingOpen      chan struct{}
	OpenErr          error
	LastActivity     time.Time
	OriginKey        string // Route origin agent ID (cache key)
	closeOnce        sync.Once
	mu               sync.RWMutex
}

// udpDestLookup allows reverse lookup from exit stream to parent association.
type udpDestLookup struct {
	Ingress *udpIngressAssociation
	Dest    *udpDestAssociation
}

// udpIngressAssociation tracks a SOCKS5 UDP association that may route to multiple exits.
// This is the ingress-side tracking (SOCKS5 client -> mesh).
type udpIngressAssociation struct {
	BaseStreamID uint64
	SOCKS5Assoc  *socks5.UDPAssociation

	// Per-destination associations keyed by route.OriginAgent.String()
	destAssocs   map[string]*udpDestAssociation
	streamToDest map[uint64]*udpDestAssociation
	destMu       sync.RWMutex

	idleTimeout time.Duration
	mu          sync.RWMutex
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

// udpIngressByBase maps base stream ID (from SOCKS5) to ingress association
var udpIngressByBase = make(map[uint64]*udpIngressAssociation)

// udpIngressByExitStream maps exit-path stream ID to the parent association + dest
// This is needed to correlate ACKs/datagrams back to the right destination
var udpIngressByExitStream = make(map[uint64]*udpDestLookup)

// udpNextBaseID is the next base stream ID for SOCKS5 UDP associations
var udpNextBaseID atomic.Uint64

// generateUDPRequestID generates a cryptographically random request ID.
// Using crypto/rand prevents session correlation attacks.
func generateUDPRequestID() uint64 {
	var buf [8]byte
	if _, err := cryptorand.Read(buf[:]); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return binary.BigEndian.Uint64(buf[:])
}

// UDP relay tracking (for transit nodes)
var udpRelayMu sync.RWMutex
var udpRelayByUpstream = make(map[uint64]*udpRelayEntry)
var udpRelayByDownstream = make(map[uint64]*udpRelayEntry)

// CreateUDPAssociation implements socks5.UDPAssociationHandler.
// Called when a SOCKS5 client requests UDP ASSOCIATE.
// This creates a lazy association container - actual mesh paths are created on first datagram.
func (a *Agent) CreateUDPAssociation(ctx context.Context, clientAddr *net.UDPAddr) (uint64, error) {
	// Verify at least one route exists (sanity check)
	if a.routeMgr.Lookup(net.IPv4zero) == nil {
		return 0, ErrUDPNoRoute
	}

	baseStreamID := udpNextBaseID.Add(1)

	assoc := &udpIngressAssociation{
		BaseStreamID: baseStreamID,
		destAssocs:   make(map[string]*udpDestAssociation),
		streamToDest: make(map[uint64]*udpDestAssociation),
		idleTimeout:  5 * time.Minute,
	}

	udpIngressMu.Lock()
	udpIngressByBase[baseStreamID] = assoc
	udpIngressMu.Unlock()

	a.logger.Debug("UDP association created (lazy)",
		logging.KeyStreamID, baseStreamID)

	return baseStreamID, nil
}

// getOrCreateDestAssociation finds or creates a destination-specific UDP association.
// It performs route lookup based on actual destination IP and creates mesh paths on demand.
func (a *Agent) getOrCreateDestAssociation(
	ctx context.Context,
	ingress *udpIngressAssociation,
	destIP net.IP,
) (*udpDestAssociation, error) {
	// 1. Route lookup
	route := a.routeMgr.Lookup(destIP)
	if route == nil {
		return nil, ErrUDPNoRoute
	}

	originKey := route.OriginAgent.String()

	// 2. Check cache (read lock)
	ingress.destMu.RLock()
	dest := ingress.destAssocs[originKey]
	ingress.destMu.RUnlock()

	if dest != nil {
		// Wait for the association to complete (or fail)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-dest.PendingOpen:
			// Check result
			if dest.OpenErr != nil {
				// Previous attempt failed, need to retry - remove failed entry
				ingress.destMu.Lock()
				// Double-check it's still the same failed dest
				if ingress.destAssocs[originKey] == dest {
					delete(ingress.destAssocs, originKey)
					delete(ingress.streamToDest, dest.StreamID)
					udpIngressMu.Lock()
					delete(udpIngressByExitStream, dest.StreamID)
					udpIngressMu.Unlock()
				}
				ingress.destMu.Unlock()
				// Fall through to create new
			} else {
				dest.mu.Lock()
				dest.LastActivity = time.Now()
				dest.mu.Unlock()
				return dest, nil
			}
		}
	}

	// 3. Create new (write lock)
	ingress.destMu.Lock()

	// Double-check after acquiring write lock
	if existingDest := ingress.destAssocs[originKey]; existingDest != nil {
		ingress.destMu.Unlock()
		// Another goroutine created it, wait for it
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-existingDest.PendingOpen:
			if existingDest.OpenErr != nil {
				return nil, existingDest.OpenErr
			}
			existingDest.mu.Lock()
			existingDest.LastActivity = time.Now()
			existingDest.mu.Unlock()
			return existingDest, nil
		}
	}

	// 4. Create destination association (releases lock internally while waiting)
	return a.createDestAssociation(ctx, ingress, route, originKey)
}

// createDestAssociation creates a new mesh path to a specific exit for UDP.
// Must be called with ingress.destMu held for writing. Releases lock before returning.
func (a *Agent) createDestAssociation(
	ctx context.Context,
	ingress *udpIngressAssociation,
	route *routing.Route,
	originKey string,
) (*udpDestAssociation, error) {
	// Use route.NextHop for first hop (same as TCP implementation)
	nextHop := route.NextHop
	conn := a.peerMgr.GetPeer(nextHop)
	if conn == nil {
		ingress.destMu.Unlock()
		return nil, fmt.Errorf("no connection to next hop: %s", nextHop.ShortString())
	}

	// Build remaining path: find NextHop in route.Path and take everything after it
	// route.Path is [local, hop1, hop2, ..., origin/target]
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
	requestID := generateUDPRequestID()

	ephPriv, ephPub, err := crypto.GenerateEphemeralKeypair()
	if err != nil {
		ingress.destMu.Unlock()
		return nil, err
	}

	dest := &udpDestAssociation{
		StreamID:         streamID,
		RequestID:        requestID,
		ExitPeerID:       route.OriginAgent,
		NextHop:          nextHop,
		EphemeralPrivKey: ephPriv,
		EphemeralPubKey:  ephPub,
		PendingOpen:      make(chan struct{}),
		LastActivity:     time.Now(),
		OriginKey:        originKey,
	}

	// Register in maps
	ingress.destAssocs[originKey] = dest
	ingress.streamToDest[streamID] = dest

	udpIngressMu.Lock()
	udpIngressByExitStream[streamID] = &udpDestLookup{
		Ingress: ingress,
		Dest:    dest,
	}
	udpIngressMu.Unlock()

	// Build and send UDP_OPEN
	open := &protocol.UDPOpen{
		RequestID:       requestID,
		AddressType:     protocol.AddrTypeIPv4,
		Address:         net.IPv4zero.To4(),
		Port:            0,
		TTL:             uint8(len(rPath)),
		RemainingPath:   remainingPath,
		EphemeralPubKey: ephPub,
	}

	frame := &protocol.Frame{
		Type:     protocol.FrameUDPOpen,
		StreamID: streamID,
		Payload:  open.Encode(),
	}

	a.logger.Debug("Creating UDP destination association",
		logging.KeyStreamID, streamID,
		"exit", route.OriginAgent.ShortString(),
		"next_hop", nextHop.ShortString(),
		"remaining_path_len", len(remainingPath))

	if err := a.peerMgr.SendToPeer(nextHop, frame); err != nil {
		// Cleanup
		delete(ingress.destAssocs, originKey)
		delete(ingress.streamToDest, streamID)
		udpIngressMu.Lock()
		delete(udpIngressByExitStream, streamID)
		udpIngressMu.Unlock()
		// Close PendingOpen to unblock any waiters
		dest.closePendingOpen(err)
		ingress.destMu.Unlock()
		return nil, err
	}

	// Release lock while waiting for ACK
	ingress.destMu.Unlock()

	select {
	case <-ctx.Done():
		dest.closePendingOpen(ctx.Err())
		return nil, ctx.Err()
	case <-dest.PendingOpen:
		if dest.OpenErr != nil {
			return nil, dest.OpenErr
		}

		// Check if session key was established
		dest.mu.RLock()
		hasSessionKey := dest.SessionKey != nil
		dest.mu.RUnlock()

		if !hasSessionKey {
			a.logger.Debug("UDP destination association opened without E2E encryption",
				logging.KeyStreamID, streamID)
		}

		return dest, nil
	}
}

// closePendingOpen safely closes the PendingOpen channel, optionally setting an error.
// Pass nil for successful completion.
func (d *udpDestAssociation) closePendingOpen(err error) {
	d.closeOnce.Do(func() {
		if err != nil {
			d.mu.Lock()
			d.OpenErr = err
			d.mu.Unlock()
		}
		close(d.PendingOpen)
	})
}

// RelayUDPDatagram implements socks5.UDPAssociationHandler.
// Called when a SOCKS5 client sends a UDP datagram to relay through the mesh.
// Routes based on actual destination IP, creating mesh paths on demand.
func (a *Agent) RelayUDPDatagram(streamID uint64, destAddr net.Addr, destPort uint16, addrType byte, rawAddr []byte, data []byte) error {
	// Get ingress and verify it exists under lock
	udpIngressMu.RLock()
	ingress := udpIngressByBase[streamID]
	if ingress == nil {
		udpIngressMu.RUnlock()
		return ErrUDPStreamNotFound
	}
	udpIngressMu.RUnlock()

	// Extract destination IP for routing
	var destIP net.IP
	switch addrType {
	case protocol.AddrTypeIPv4:
		destIP = net.IP(rawAddr)
	case protocol.AddrTypeIPv6:
		destIP = net.IP(rawAddr)
	case protocol.AddrTypeDomain:
		// For domain, resolve at ingress for routing decision
		domain := string(rawAddr[1:]) // Skip length byte
		ips, err := net.LookupIP(domain)
		if err != nil || len(ips) == 0 {
			return fmt.Errorf("DNS lookup failed: %s", domain)
		}
		destIP = ips[0]
	default:
		return fmt.Errorf("unsupported address type: %d", addrType)
	}

	// Get or create destination association
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	dest, err := a.getOrCreateDestAssociation(ctx, ingress, destIP)
	if err != nil {
		return err
	}

	// Encrypt and send
	dest.mu.RLock()
	sessionKey := dest.SessionKey
	exitStreamID := dest.StreamID
	nextHop := dest.NextHop
	dest.mu.RUnlock()

	var ciphertext []byte
	if sessionKey != nil {
		ciphertext, err = sessionKey.Encrypt(data)
		if err != nil {
			return err
		}
	} else {
		ciphertext = data
	}

	datagram := &protocol.UDPDatagram{
		AddressType: addrType,
		Address:     rawAddr,
		Port:        destPort,
		Data:        ciphertext,
	}

	frame := &protocol.Frame{
		Type:     protocol.FrameUDPDatagram,
		StreamID: exitStreamID,
		Payload:  datagram.Encode(),
	}

	return a.peerMgr.SendToPeer(nextHop, frame)
}

// CloseUDPAssociation implements socks5.UDPAssociationHandler.
// Called when a SOCKS5 UDP association is closed.
func (a *Agent) CloseUDPAssociation(streamID uint64) {
	udpIngressMu.Lock()
	ingress := udpIngressByBase[streamID]
	if ingress != nil {
		delete(udpIngressByBase, streamID)
	}
	udpIngressMu.Unlock()

	if ingress == nil {
		return
	}

	// Close all destination associations
	ingress.destMu.Lock()
	for _, dest := range ingress.destAssocs {
		a.closeDestAssociation(dest)

		udpIngressMu.Lock()
		delete(udpIngressByExitStream, dest.StreamID)
		udpIngressMu.Unlock()
	}
	ingress.destAssocs = nil
	ingress.streamToDest = nil
	ingress.destMu.Unlock()

	a.logger.Debug("UDP association closed",
		logging.KeyStreamID, streamID)
}

// closeDestAssociation sends UDP_CLOSE to the exit and cleans up.
func (a *Agent) closeDestAssociation(dest *udpDestAssociation) {
	closeFrame := &protocol.UDPClose{Reason: protocol.UDPCloseNormal}
	frame := &protocol.Frame{
		Type:     protocol.FrameUDPClose,
		StreamID: dest.StreamID,
		Payload:  closeFrame.Encode(),
	}

	// Use stored NextHop for sending close frame
	a.peerMgr.SendToPeer(dest.NextHop, frame)
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
		a.logger.Debug("failed to decode UDP_OPEN frame", "error", err)
		return
	}

	a.logger.Debug("received UDP_OPEN",
		logging.KeyStreamID, frame.StreamID,
		"from_peer", peerID.ShortString(),
		"remaining_path_len", len(open.RemainingPath))

	// Check if we are the exit node (path is empty)
	if len(open.RemainingPath) == 0 {
		// We are the exit node - handle with UDP handler
		if a.udpHandler == nil {
			// Send error back
			a.sendUDPOpenErr(peerID, frame.StreamID, open.RequestID, protocol.ErrUDPDisabled, "UDP relay disabled")
			return
		}

		ctx := context.Background()
		a.udpHandler.HandleUDPOpen(ctx, peerID, frame.StreamID, open, open.EphemeralPubKey)
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

	a.logger.Debug("relaying UDP_OPEN to next hop",
		logging.KeyStreamID, downstreamID,
		"next_hop", nextHop.ShortString(),
		"remaining_path_len", len(newPath))

	if err := a.peerMgr.SendToPeer(nextHop, fwdFrame); err != nil {
		a.logger.Debug("failed to relay UDP_OPEN",
			"error", err,
			"next_hop", nextHop.ShortString())

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
	a.logger.Debug("received UDP_OPEN_ACK",
		logging.KeyStreamID, frame.StreamID,
		"from_peer", peerID.ShortString())

	// Check if this is a relay response - copy fields under lock
	udpRelayMu.RLock()
	relay := udpRelayByDownstream[frame.StreamID]
	var relayUpstreamID uint64
	var relayUpstreamPeer, relayDownstreamPeer identity.AgentID
	if relay != nil {
		relayUpstreamID = relay.UpstreamID
		relayUpstreamPeer = relay.UpstreamPeer
		relayDownstreamPeer = relay.DownstreamPeer
	}
	udpRelayMu.RUnlock()

	if relay != nil && peerID == relayDownstreamPeer {
		a.logger.Debug("relaying UDP_OPEN_ACK upstream",
			logging.KeyStreamID, relayUpstreamID,
			"upstream_peer", relayUpstreamPeer.ShortString())

		// Forward ACK to upstream
		fwdFrame := &protocol.Frame{
			Type:     protocol.FrameUDPOpenAck,
			StreamID: relayUpstreamID,
			Payload:  frame.Payload,
		}
		a.peerMgr.SendToPeer(relayUpstreamPeer, fwdFrame)
		return
	}

	// Look up via exit stream (new structure)
	udpIngressMu.RLock()
	lookup := udpIngressByExitStream[frame.StreamID]
	if lookup == nil {
		udpIngressMu.RUnlock()
		return
	}
	dest := lookup.Dest
	udpIngressMu.RUnlock()

	ack, err := protocol.DecodeUDPOpenAck(frame.Payload)
	if err != nil {
		return
	}

	// Compute session key from the ephemeral keys
	var zeroKey [protocol.EphemeralKeySize]byte
	if ack.EphemeralPubKey != zeroKey {
		// Compute shared secret using our private key and remote public key
		sharedSecret, err := crypto.ComputeECDH(dest.EphemeralPrivKey, ack.EphemeralPubKey)
		if err != nil {
			dest.closePendingOpen(err)
			return
		}

		// Zero out private key immediately
		crypto.ZeroKey(&dest.EphemeralPrivKey)

		// Derive session key (we are initiator, so isResponder=false)
		sessionKey := crypto.DeriveSessionKey(sharedSecret, ack.RequestID, dest.EphemeralPubKey, ack.EphemeralPubKey, true)
		crypto.ZeroKey(&sharedSecret)

		// Store session key
		dest.mu.Lock()
		dest.SessionKey = sessionKey
		dest.mu.Unlock()
	}

	// Mark as successful (nil error)
	dest.closePendingOpen(nil)
}

// handleUDPOpenErr processes a UDP_OPEN_ERR frame.
func (a *Agent) handleUDPOpenErr(peerID identity.AgentID, frame *protocol.Frame) {
	// Check if this is a relay response - use write lock to atomically check and delete
	udpRelayMu.Lock()
	relay := udpRelayByDownstream[frame.StreamID]
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
			delete(udpRelayByUpstream, relayUpstreamID)
			delete(udpRelayByDownstream, frame.StreamID)
		}
	}
	udpRelayMu.Unlock()

	if isRelay {
		// Forward error to upstream
		fwdFrame := &protocol.Frame{
			Type:     protocol.FrameUDPOpenErr,
			StreamID: relayUpstreamID,
			Payload:  frame.Payload,
		}
		a.peerMgr.SendToPeer(relayUpstreamPeer, fwdFrame)
		return
	}

	// Look up via exit stream (new structure)
	udpIngressMu.RLock()
	lookup := udpIngressByExitStream[frame.StreamID]
	if lookup == nil {
		udpIngressMu.RUnlock()
		return
	}
	dest := lookup.Dest
	ingress := lookup.Ingress
	udpIngressMu.RUnlock()

	errMsg, err := protocol.DecodeUDPOpenErr(frame.Payload)
	if err != nil {
		return
	}

	dest.closePendingOpen(fmt.Errorf("UDP open failed: %s (code %d)", errMsg.Message, errMsg.ErrorCode))

	// Clean up
	ingress.destMu.Lock()
	delete(ingress.destAssocs, dest.OriginKey)
	delete(ingress.streamToDest, dest.StreamID)
	ingress.destMu.Unlock()

	udpIngressMu.Lock()
	delete(udpIngressByExitStream, dest.StreamID)
	udpIngressMu.Unlock()
}

// handleUDPDatagram processes a UDP_DATAGRAM frame.
func (a *Agent) handleUDPDatagram(peerID identity.AgentID, frame *protocol.Frame) {
	// Check if this is for our UDP handler (exit node receiving from mesh)
	if a.udpHandler != nil {
		if assoc := a.udpHandler.GetAssociation(frame.StreamID); assoc != nil {
			datagram, err := protocol.DecodeUDPDatagram(frame.Payload)
			if err != nil {
				return
			}
			a.udpHandler.HandleUDPDatagram(peerID, frame.StreamID, datagram)
			return
		}
	}

	// Check if this is for ingress via exit stream lookup (new structure)
	// Hold lock while getting lookup and extracting needed references
	udpIngressMu.RLock()
	lookup := udpIngressByExitStream[frame.StreamID]
	var dest *udpDestAssociation
	var ingress *udpIngressAssociation
	if lookup != nil {
		dest = lookup.Dest
		ingress = lookup.Ingress
	}
	udpIngressMu.RUnlock()

	if lookup != nil {
		datagram, err := protocol.DecodeUDPDatagram(frame.Payload)
		if err != nil {
			return
		}

		// Decrypt if we have a session key
		dest.mu.RLock()
		sessionKey := dest.SessionKey
		dest.mu.RUnlock()

		var plaintext []byte
		if sessionKey != nil {
			plaintext, err = sessionKey.Decrypt(datagram.Data)
			if err != nil {
				return
			}
		} else {
			plaintext = datagram.Data
		}

		// Update activity
		dest.mu.Lock()
		dest.LastActivity = time.Now()
		dest.mu.Unlock()

		// Forward to SOCKS5 client
		ingress.mu.RLock()
		socks5Assoc := ingress.SOCKS5Assoc
		ingress.mu.RUnlock()

		if socks5Assoc != nil {
			socks5Assoc.WriteToClient(datagram.AddressType, datagram.Address, datagram.Port, plaintext)
		}
		return
	}

	// Check if this is a relay - copy fields under lock
	udpRelayMu.RLock()
	relayUp := udpRelayByUpstream[frame.StreamID]
	relayDown := udpRelayByDownstream[frame.StreamID]
	var upDownstreamID uint64
	var upDownstreamPeer, upUpstreamPeer identity.AgentID
	var downUpstreamID uint64
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
	udpRelayMu.RUnlock()

	if relayUp != nil && peerID == upUpstreamPeer {
		// Forward downstream
		fwdFrame := &protocol.Frame{
			Type:     protocol.FrameUDPDatagram,
			StreamID: upDownstreamID,
			Payload:  frame.Payload,
		}
		a.peerMgr.SendToPeer(upDownstreamPeer, fwdFrame)
		return
	}

	if relayDown != nil && peerID == downDownstreamPeer {
		// Forward upstream
		fwdFrame := &protocol.Frame{
			Type:     protocol.FrameUDPDatagram,
			StreamID: downUpstreamID,
			Payload:  frame.Payload,
		}
		a.peerMgr.SendToPeer(downUpstreamPeer, fwdFrame)
	}
}

// handleUDPClose processes a UDP_CLOSE frame.
func (a *Agent) handleUDPClose(peerID identity.AgentID, frame *protocol.Frame) {
	// Check if this is for our UDP handler (exit node)
	if a.udpHandler != nil {
		a.udpHandler.HandleUDPClose(peerID, frame.StreamID)
	}

	// Check if this is a relay - copy fields and delete atomically under lock
	udpRelayMu.Lock()
	relayUp := udpRelayByUpstream[frame.StreamID]
	relayDown := udpRelayByDownstream[frame.StreamID]

	var upDownstreamID uint64
	var upDownstreamPeer, upUpstreamPeer identity.AgentID
	var downUpstreamID uint64
	var downUpstreamPeer, downDownstreamPeer identity.AgentID

	if relayUp != nil {
		upDownstreamID = relayUp.DownstreamID
		upDownstreamPeer = relayUp.DownstreamPeer
		upUpstreamPeer = relayUp.UpstreamPeer
		delete(udpRelayByUpstream, frame.StreamID)
		delete(udpRelayByDownstream, upDownstreamID)
	}
	if relayDown != nil {
		downUpstreamID = relayDown.UpstreamID
		downUpstreamPeer = relayDown.UpstreamPeer
		downDownstreamPeer = relayDown.DownstreamPeer
		delete(udpRelayByDownstream, frame.StreamID)
		delete(udpRelayByUpstream, downUpstreamID)
	}
	udpRelayMu.Unlock()

	if relayUp != nil && peerID == upUpstreamPeer {
		// Forward downstream
		fwdFrame := &protocol.Frame{
			Type:     protocol.FrameUDPClose,
			StreamID: upDownstreamID,
			Payload:  frame.Payload,
		}
		a.peerMgr.SendToPeer(upDownstreamPeer, fwdFrame)
	}

	if relayDown != nil && peerID == downDownstreamPeer {
		// Forward upstream
		fwdFrame := &protocol.Frame{
			Type:     protocol.FrameUDPClose,
			StreamID: downUpstreamID,
			Payload:  frame.Payload,
		}
		a.peerMgr.SendToPeer(downUpstreamPeer, fwdFrame)
	}

	// Check if this is for our ingress via exit stream lookup
	// Use write lock to atomically check and remove
	udpIngressMu.Lock()
	lookup := udpIngressByExitStream[frame.StreamID]
	var dest *udpDestAssociation
	var ingress *udpIngressAssociation
	if lookup != nil {
		dest = lookup.Dest
		ingress = lookup.Ingress
		delete(udpIngressByExitStream, frame.StreamID)
	}
	udpIngressMu.Unlock()

	if lookup != nil {
		// Clean up this destination association
		ingress.destMu.Lock()
		delete(ingress.destAssocs, dest.OriginKey)
		delete(ingress.streamToDest, dest.StreamID)
		ingress.destMu.Unlock()
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
	closeFrame := &protocol.UDPClose{
		Reason: reason,
	}

	frame := &protocol.Frame{
		Type:     protocol.FrameUDPClose,
		StreamID: streamID,
		Payload:  closeFrame.Encode(),
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

	if assoc := udpIngressByBase[streamID]; assoc != nil {
		assoc.mu.Lock()
		assoc.SOCKS5Assoc = socks5Assoc
		assoc.mu.Unlock()
	}
}

// startUDPDestCleanupLoop starts a goroutine that periodically cleans up idle destination associations.
func (a *Agent) startUDPDestCleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		defer ticker.Stop()
		for {
			select {
			case <-a.stopCh:
				return
			case <-ticker.C:
				a.cleanupIdleDestAssociations()
			}
		}
	}()
}

// cleanupIdleDestAssociations removes destination associations that have been idle for too long.
func (a *Agent) cleanupIdleDestAssociations() {
	now := time.Now()

	udpIngressMu.RLock()
	ingressList := make([]*udpIngressAssociation, 0, len(udpIngressByBase))
	for _, ingress := range udpIngressByBase {
		ingressList = append(ingressList, ingress)
	}
	udpIngressMu.RUnlock()

	for _, ingress := range ingressList {
		ingress.destMu.Lock()
		for key, dest := range ingress.destAssocs {
			dest.mu.RLock()
			idle := now.Sub(dest.LastActivity) > ingress.idleTimeout
			streamID := dest.StreamID
			dest.mu.RUnlock()

			if idle {
				a.closeDestAssociation(dest)
				delete(ingress.destAssocs, key)
				delete(ingress.streamToDest, streamID)

				udpIngressMu.Lock()
				delete(udpIngressByExitStream, streamID)
				udpIngressMu.Unlock()

				a.logger.Debug("Cleaned up idle UDP destination association",
					logging.KeyStreamID, streamID)
			}
		}
		ingress.destMu.Unlock()
	}
}

// Compile-time interface verification
var _ udp.DataWriter = (*Agent)(nil)
var _ socks5.UDPAssociationHandler = (*Agent)(nil)
