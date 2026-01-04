// Package flood implements route flooding for Muti Metroo mesh network.
package flood

import (
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/postalsys/muti-metroo/internal/crypto"
	"github.com/postalsys/muti-metroo/internal/identity"
	"github.com/postalsys/muti-metroo/internal/logging"
	"github.com/postalsys/muti-metroo/internal/protocol"
	"github.com/postalsys/muti-metroo/internal/recovery"
	"github.com/postalsys/muti-metroo/internal/routing"
)

// AdvertisementKey uniquely identifies a route advertisement for dedup.
type AdvertisementKey struct {
	OriginAgent identity.AgentID
	Sequence    uint64
}

// SeenAdvertisement tracks a seen advertisement for loop prevention.
type SeenAdvertisement struct {
	Key       AdvertisementKey
	SeenAt    time.Time
	SeenFrom  identity.AgentID
}

// NodeInfoKey uniquely identifies a node info advertisement for dedup.
type NodeInfoKey struct {
	OriginAgent identity.AgentID
	Sequence    uint64
}

// SeenNodeInfo tracks a seen node info advertisement.
type SeenNodeInfo struct {
	Key      NodeInfoKey
	SeenAt   time.Time
	SeenFrom identity.AgentID
}

// FloodConfig contains configuration for the flood protocol.
type FloodConfig struct {
	// SeenCacheTTL is how long to remember seen advertisements
	SeenCacheTTL time.Duration

	// FloodInterval is the minimum time between flooding the same route
	FloodInterval time.Duration

	// MaxSeenCacheSize limits the seen cache size
	MaxSeenCacheSize int

	// LocalDisplayName is the display name to include in route advertisements
	LocalDisplayName string

	// Logger for logging
	Logger *slog.Logger

	// SealedBox for management key encryption (nil if not configured)
	// When set, NodeInfo and route paths are encrypted before flooding
	SealedBox *crypto.SealedBox
}

// DefaultFloodConfig returns sensible defaults.
func DefaultFloodConfig() FloodConfig {
	return FloodConfig{
		SeenCacheTTL:     5 * time.Minute,
		FloodInterval:    1 * time.Second,
		MaxSeenCacheSize: 10000,
	}
}

// PeerSender is an interface for sending frames to peers.
type PeerSender interface {
	// SendToPeer sends a frame to a specific peer.
	SendToPeer(peerID identity.AgentID, frame *protocol.Frame) error

	// GetPeerIDs returns all connected peer IDs.
	GetPeerIDs() []identity.AgentID
}

// Flooder handles route flooding to mesh peers.
type Flooder struct {
	cfg              FloodConfig
	localID          identity.AgentID
	localDisplayName string
	routeMgr         *routing.Manager
	sender           PeerSender
	logger           *slog.Logger
	sealedBox        *crypto.SealedBox // Management key encryption (nil if not configured)

	mu        sync.RWMutex
	seenCache map[AdvertisementKey]*SeenAdvertisement

	// Node info seen cache (separate from route advertisements)
	nodeInfoMu        sync.RWMutex
	nodeInfoSeenCache map[NodeInfoKey]*SeenNodeInfo
	nodeInfoSeq       uint64

	wg       sync.WaitGroup
	stopOnce sync.Once
	stopCh   chan struct{}
}

// NewFlooder creates a new route flooder.
func NewFlooder(cfg FloodConfig, localID identity.AgentID, routeMgr *routing.Manager, sender PeerSender) *Flooder {
	logger := cfg.Logger
	if logger == nil {
		logger = logging.NopLogger()
	}

	f := &Flooder{
		cfg:               cfg,
		localID:           localID,
		localDisplayName:  cfg.LocalDisplayName,
		routeMgr:          routeMgr,
		sender:            sender,
		logger:            logger,
		sealedBox:         cfg.SealedBox,
		seenCache:         make(map[AdvertisementKey]*SeenAdvertisement),
		nodeInfoSeenCache: make(map[NodeInfoKey]*SeenNodeInfo),
		stopCh:            make(chan struct{}),
	}

	// Start cache cleanup goroutine
	f.wg.Add(1)
	go f.cleanupLoop()

	return f
}

// HandleRouteAdvertise processes an incoming ROUTE_ADVERTISE frame.
// Returns true if the advertisement was new and should be processed.
func (f *Flooder) HandleRouteAdvertise(
	fromPeer identity.AgentID,
	originAgent identity.AgentID,
	originDisplayName string,
	sequence uint64,
	routes []protocol.Route,
	encPath *protocol.EncryptedData,
	seenBy []identity.AgentID,
) bool {
	key := AdvertisementKey{
		OriginAgent: originAgent,
		Sequence:    sequence,
	}

	// Check if we've already seen this and mark as seen atomically
	f.mu.Lock()
	if existing, ok := f.seenCache[key]; ok {
		// Already seen - update seen time if from a new peer
		if existing.SeenFrom != fromPeer {
			existing.SeenAt = time.Now()
		}
		f.mu.Unlock()
		f.logger.Debug("route advertisement already seen",
			"origin", originAgent.ShortString(),
			"sequence", sequence,
			"from_peer", fromPeer.ShortString(),
			"original_from", existing.SeenFrom.ShortString())
		return false
	}

	// Mark as seen
	f.seenCache[key] = &SeenAdvertisement{
		Key:      key,
		SeenAt:   time.Now(),
		SeenFrom: fromPeer,
	}
	cacheSize := len(f.seenCache)
	f.mu.Unlock()

	f.logger.Debug("new route advertisement received",
		"origin", originAgent.ShortString(),
		"sequence", sequence,
		"from_peer", fromPeer.ShortString(),
		"routes", len(routes),
		"cache_size", cacheSize)

	// Store display name for origin agent
	if originDisplayName != "" {
		f.routeMgr.SetDisplayName(originAgent, originDisplayName)
	}

	// Check if we're in the seen-by list (loop detection)
	for _, id := range seenBy {
		if id == f.localID {
			return false // We've already processed this
		}
	}

	// Decode path from advertisement
	// Note: Paths are sent as plaintext for routing (not encrypted)
	// because transit agents need the path to forward STREAM_OPEN frames.
	// Path hiding happens at the API layer, not on the wire.
	var path []identity.AgentID
	if encPath != nil {
		if encPath.Encrypted {
			// Legacy: try to decrypt if we have the private key
			// (for backwards compatibility with old encrypted paths)
			if f.sealedBox != nil && f.sealedBox.CanDecrypt() {
				decrypted, err := f.sealedBox.Open(encPath.Data)
				if err == nil {
					path, _ = protocol.DecodePath(decrypted)
				}
			}
			// If we can't decrypt, path remains nil (routing will fail)
		} else {
			// Plaintext - decode directly (normal case)
			path, _ = protocol.DecodePath(encPath.Data)
		}
	}

	// Convert protocol routes to routing entries
	entries := make([]routing.RouteEntry, 0, len(routes))
	for _, r := range routes {
		if len(r.Prefix) == 0 {
			continue
		}

		var ipNet *net.IPNet
		if r.AddressFamily == protocol.AddrFamilyIPv4 {
			ip := make(net.IP, 4)
			copy(ip, r.Prefix)
			mask := net.CIDRMask(int(r.PrefixLength), 32)
			ipNet = &net.IPNet{IP: ip, Mask: mask}
		} else if r.AddressFamily == protocol.AddrFamilyIPv6 {
			ip := make(net.IP, 16)
			copy(ip, r.Prefix)
			mask := net.CIDRMask(int(r.PrefixLength), 128)
			ipNet = &net.IPNet{IP: ip, Mask: mask}
		}

		if ipNet != nil {
			entries = append(entries, routing.RouteEntry{
				Network: ipNet,
				Metric:  r.Metric,
			})
		}
	}

	// Process in routing manager (path may be nil if encrypted and can't decrypt)
	f.routeMgr.ProcessRouteAdvertise(fromPeer, originAgent, sequence, entries, path, encPath)

	// Flood to other peers (forward encrypted path as-is)
	newSeenBy := append(seenBy, f.localID)
	f.floodAdvertisementEncrypted(fromPeer, originAgent, originDisplayName, sequence, routes, encPath, newSeenBy)

	return true
}

// HandleRouteWithdraw processes an incoming ROUTE_WITHDRAW frame.
func (f *Flooder) HandleRouteWithdraw(
	fromPeer identity.AgentID,
	originAgent identity.AgentID,
	sequence uint64,
	routes []protocol.Route,
	seenBy []identity.AgentID,
) bool {
	key := AdvertisementKey{
		OriginAgent: originAgent,
		Sequence:    sequence,
	}

	// Check if we've seen this
	f.mu.Lock()
	if _, ok := f.seenCache[key]; ok {
		f.mu.Unlock()
		return false
	}

	f.seenCache[key] = &SeenAdvertisement{
		Key:      key,
		SeenAt:   time.Now(),
		SeenFrom: fromPeer,
	}
	f.mu.Unlock()

	// Check loop detection
	for _, id := range seenBy {
		if id == f.localID {
			return false
		}
	}

	// Convert to routing entries
	entries := make([]routing.RouteEntry, 0, len(routes))
	for _, r := range routes {
		if len(r.Prefix) == 0 {
			continue
		}

		var ipNet *net.IPNet
		if r.AddressFamily == protocol.AddrFamilyIPv4 {
			ip := make(net.IP, 4)
			copy(ip, r.Prefix)
			mask := net.CIDRMask(int(r.PrefixLength), 32)
			ipNet = &net.IPNet{IP: ip, Mask: mask}
		} else if r.AddressFamily == protocol.AddrFamilyIPv6 {
			ip := make(net.IP, 16)
			copy(ip, r.Prefix)
			mask := net.CIDRMask(int(r.PrefixLength), 128)
			ipNet = &net.IPNet{IP: ip, Mask: mask}
		}

		if ipNet != nil {
			entries = append(entries, routing.RouteEntry{
				Network: ipNet,
				Metric:  r.Metric,
			})
		}
	}

	// Process withdrawal
	f.routeMgr.ProcessRouteWithdraw(originAgent, entries)

	// Flood withdrawal to other peers
	newSeenBy := append(seenBy, f.localID)
	f.floodWithdrawal(fromPeer, originAgent, sequence, routes, newSeenBy)

	return true
}

// floodAdvertisementEncrypted sends a route advertisement to all peers except the source.
// For plaintext paths, it prepends the local agent ID to the path before forwarding.
// For encrypted paths (legacy), it forwards as-is since we can't modify encrypted data.
func (f *Flooder) floodAdvertisementEncrypted(
	fromPeer identity.AgentID,
	originAgent identity.AgentID,
	originDisplayName string,
	sequence uint64,
	routes []protocol.Route,
	encPath *protocol.EncryptedData,
	seenBy []identity.AgentID,
) {
	peers := f.sender.GetPeerIDs()

	// Extend the path if it's plaintext (normal case)
	// For encrypted paths (legacy), forward as-is
	fwdEncPath := encPath
	if encPath != nil && !encPath.Encrypted {
		// Decode existing path, prepend our ID, re-encode
		existingPath, _ := protocol.DecodePath(encPath.Data)
		newPath := make([]identity.AgentID, len(existingPath)+1)
		newPath[0] = f.localID
		copy(newPath[1:], existingPath)
		fwdEncPath = &protocol.EncryptedData{
			Encrypted: false,
			Data:      protocol.EncodePath(newPath),
		}
	}

	// Build the advertise payload with extended path
	adv := &protocol.RouteAdvertise{
		OriginAgent:       originAgent,
		OriginDisplayName: originDisplayName,
		Sequence:          sequence,
		Routes:            routes,
		EncPath:           fwdEncPath,
		SeenBy:            seenBy,
	}

	frame := &protocol.Frame{
		Type:     protocol.FrameRouteAdvertise,
		StreamID: protocol.ControlStreamID,
		Payload:  adv.Encode(),
	}

	for _, peerID := range peers {
		// Don't send back to source
		if peerID == fromPeer {
			continue
		}

		// Don't send to peers in the seen-by list
		skip := false
		for _, id := range seenBy {
			if id == peerID {
				skip = true
				break
			}
		}
		if skip {
			continue
		}

		if err := f.sender.SendToPeer(peerID, frame); err != nil {
			f.logger.Debug("failed to send route advertisement",
				logging.KeyPeerID, peerID.ShortString(),
				logging.KeyError, err)
		}
	}
}

// floodWithdrawal sends a route withdrawal to all peers except the source.
func (f *Flooder) floodWithdrawal(
	fromPeer identity.AgentID,
	originAgent identity.AgentID,
	sequence uint64,
	routes []protocol.Route,
	seenBy []identity.AgentID,
) {
	peers := f.sender.GetPeerIDs()

	// Build the withdraw payload
	withdraw := &protocol.RouteWithdraw{
		OriginAgent: originAgent,
		Sequence:    sequence,
		Routes:      routes,
		SeenBy:      seenBy,
	}

	frame := &protocol.Frame{
		Type:     protocol.FrameRouteWithdraw,
		StreamID: protocol.ControlStreamID,
		Payload:  withdraw.Encode(),
	}

	for _, peerID := range peers {
		if peerID == fromPeer {
			continue
		}

		skip := false
		for _, id := range seenBy {
			if id == peerID {
				skip = true
				break
			}
		}
		if skip {
			continue
		}

		if err := f.sender.SendToPeer(peerID, frame); err != nil {
			f.logger.Debug("failed to send route withdrawal",
				logging.KeyPeerID, peerID.ShortString(),
				logging.KeyError, err)
		}
	}
}

// AnnounceLocalRoutes floods all local routes to all peers.
func (f *Flooder) AnnounceLocalRoutes() {
	localRoutes := f.routeMgr.GetLocalRoutes()
	if len(localRoutes) == 0 {
		return
	}

	seq := f.routeMgr.IncrementSequence()

	// Convert to protocol routes
	routes := make([]protocol.Route, 0, len(localRoutes))
	for _, lr := range localRoutes {
		ones, bits := lr.Network.Mask.Size()
		family := protocol.AddrFamilyIPv4
		if bits == 128 {
			family = protocol.AddrFamilyIPv6
		}

		routes = append(routes, protocol.Route{
			AddressFamily: family,
			PrefixLength:  uint8(ones),
			Prefix:        []byte(lr.Network.IP),
			Metric:        lr.Metric,
		})
	}

	// Build path data (always plaintext - needed for multi-hop routing)
	// Note: Path encryption was removed because transit agents need the path
	// to forward STREAM_OPEN frames. Path hiding happens at the API layer.
	path := []identity.AgentID{f.localID}
	pathBytes := protocol.EncodePath(path)
	encPath := &protocol.EncryptedData{
		Encrypted: false,
		Data:      pathBytes,
	}

	// Build advertisement
	adv := &protocol.RouteAdvertise{
		OriginAgent:       f.localID,
		OriginDisplayName: f.localDisplayName,
		Sequence:          seq,
		Routes:            routes,
		Path:              path,    // Keep for backwards compat
		EncPath:           encPath, // Encrypted path for wire format
		SeenBy:            []identity.AgentID{f.localID},
	}

	frame := &protocol.Frame{
		Type:     protocol.FrameRouteAdvertise,
		StreamID: protocol.ControlStreamID,
		Payload:  adv.Encode(),
	}

	// Send to all peers
	for _, peerID := range f.sender.GetPeerIDs() {
		if err := f.sender.SendToPeer(peerID, frame); err != nil {
			f.logger.Debug("failed to announce local routes",
				logging.KeyPeerID, peerID.ShortString(),
				logging.KeyError, err)
		}
	}
}

// WithdrawLocalRoutes floods withdrawal of all local routes.
func (f *Flooder) WithdrawLocalRoutes() {
	localRoutes := f.routeMgr.GetLocalRoutes()
	if len(localRoutes) == 0 {
		return
	}

	seq := f.routeMgr.IncrementSequence()

	routes := make([]protocol.Route, 0, len(localRoutes))
	for _, lr := range localRoutes {
		ones, bits := lr.Network.Mask.Size()
		family := protocol.AddrFamilyIPv4
		if bits == 128 {
			family = protocol.AddrFamilyIPv6
		}

		routes = append(routes, protocol.Route{
			AddressFamily: family,
			PrefixLength:  uint8(ones),
			Prefix:        []byte(lr.Network.IP),
			Metric:        lr.Metric,
		})
	}

	withdraw := &protocol.RouteWithdraw{
		OriginAgent: f.localID,
		Sequence:    seq,
		Routes:      routes,
		SeenBy:      []identity.AgentID{f.localID},
	}

	frame := &protocol.Frame{
		Type:     protocol.FrameRouteWithdraw,
		StreamID: protocol.ControlStreamID,
		Payload:  withdraw.Encode(),
	}

	for _, peerID := range f.sender.GetPeerIDs() {
		if err := f.sender.SendToPeer(peerID, frame); err != nil {
			f.logger.Debug("failed to withdraw local routes",
				logging.KeyPeerID, peerID.ShortString(),
				logging.KeyError, err)
		}
	}
}

// SendFullTable sends the full routing table to a newly connected peer.
// Routes are grouped by origin agent and sent with their original path preserved.
func (f *Flooder) SendFullTable(peerID identity.AgentID) {
	fullRoutes := f.routeMgr.GetFullRoutesForAdvertise(peerID)
	if len(fullRoutes) == 0 {
		return
	}

	// Group routes by origin agent
	byOrigin := make(map[identity.AgentID][]*routing.Route)
	for _, route := range fullRoutes {
		byOrigin[route.OriginAgent] = append(byOrigin[route.OriginAgent], route)
	}

	// Send a separate advertisement for each origin
	for originAgent, originRoutes := range byOrigin {
		seq := f.routeMgr.IncrementSequence()

		routes := make([]protocol.Route, 0, len(originRoutes))
		for _, r := range originRoutes {
			ones, bits := r.Network.Mask.Size()
			family := protocol.AddrFamilyIPv4
			if bits == 128 {
				family = protocol.AddrFamilyIPv6
			}

			routes = append(routes, protocol.Route{
				AddressFamily: family,
				PrefixLength:  uint8(ones),
				Prefix:        []byte(r.Network.IP),
				Metric:        r.Metric,
			})
		}

		// Use the path from the first route (they should be the same for same origin)
		// Prepend ourselves to the path
		var path []identity.AgentID
		if len(originRoutes) > 0 && len(originRoutes[0].Path) > 0 {
			path = append([]identity.AgentID{f.localID}, originRoutes[0].Path...)
		} else {
			path = []identity.AgentID{f.localID}
		}

		// Get display name for origin agent
		var originDisplayName string
		if originAgent == f.localID {
			originDisplayName = f.localDisplayName
		} else {
			originDisplayName = f.routeMgr.GetDisplayName(originAgent)
		}

		adv := &protocol.RouteAdvertise{
			OriginAgent:       originAgent,
			OriginDisplayName: originDisplayName,
			Sequence:          seq,
			Routes:            routes,
			Path:              path,
			SeenBy:            []identity.AgentID{f.localID},
		}

		frame := &protocol.Frame{
			Type:     protocol.FrameRouteAdvertise,
			StreamID: protocol.ControlStreamID,
			Payload:  adv.Encode(),
		}

		if err := f.sender.SendToPeer(peerID, frame); err != nil {
			f.logger.Debug("failed to send full routing table",
				logging.KeyPeerID, peerID.ShortString(),
				logging.KeyError, err)
		}
	}
}

// cleanupLoop periodically cleans up expired seen entries.
func (f *Flooder) cleanupLoop() {
	defer f.wg.Done()
	defer recovery.RecoverWithLog(f.logger, "flood.cleanupLoop")

	ticker := time.NewTicker(f.cfg.SeenCacheTTL / 2)
	defer ticker.Stop()

	for {
		select {
		case <-f.stopCh:
			return
		case <-ticker.C:
			f.cleanup()
		}
	}
}

// cleanup removes expired entries from the seen caches.
func (f *Flooder) cleanup() {
	now := time.Now()
	expiry := f.cfg.SeenCacheTTL

	// Cleanup route advertisement cache
	f.mu.Lock()
	for key, entry := range f.seenCache {
		if now.Sub(entry.SeenAt) > expiry {
			delete(f.seenCache, key)
		}
	}

	// If still too large, remove oldest entries
	if len(f.seenCache) > f.cfg.MaxSeenCacheSize {
		// Find oldest entries and remove them
		excess := len(f.seenCache) - f.cfg.MaxSeenCacheSize
		var oldest []AdvertisementKey
		for key, entry := range f.seenCache {
			oldest = append(oldest, key)
			if len(oldest) >= excess {
				// Just remove any excess entries
				break
			}
			_ = entry // Used for future sorting if needed
		}
		for _, key := range oldest {
			delete(f.seenCache, key)
		}
	}
	f.mu.Unlock()

	// Cleanup node info cache
	f.nodeInfoMu.Lock()
	for key, entry := range f.nodeInfoSeenCache {
		if now.Sub(entry.SeenAt) > expiry {
			delete(f.nodeInfoSeenCache, key)
		}
	}

	// If still too large, remove oldest entries
	if len(f.nodeInfoSeenCache) > f.cfg.MaxSeenCacheSize {
		excess := len(f.nodeInfoSeenCache) - f.cfg.MaxSeenCacheSize
		var oldest []NodeInfoKey
		for key, entry := range f.nodeInfoSeenCache {
			oldest = append(oldest, key)
			if len(oldest) >= excess {
				break
			}
			_ = entry
		}
		for _, key := range oldest {
			delete(f.nodeInfoSeenCache, key)
		}
	}
	f.nodeInfoMu.Unlock()
}

// SeenCacheSize returns the current size of the seen cache.
func (f *Flooder) SeenCacheSize() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.seenCache)
}

// Stop stops the flooder.
func (f *Flooder) Stop() {
	f.stopOnce.Do(func() {
		close(f.stopCh)
	})
	f.wg.Wait()
}

// HasSeen checks if an advertisement has been seen.
func (f *Flooder) HasSeen(originAgent identity.AgentID, sequence uint64) bool {
	key := AdvertisementKey{
		OriginAgent: originAgent,
		Sequence:    sequence,
	}

	f.mu.RLock()
	defer f.mu.RUnlock()

	_, ok := f.seenCache[key]
	return ok
}

// ClearSeenCache clears the seen cache (for testing).
func (f *Flooder) ClearSeenCache() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seenCache = make(map[AdvertisementKey]*SeenAdvertisement)
}

// HandleNodeInfoAdvertise processes an incoming NODE_INFO_ADVERTISE frame.
// Returns true if the node info was new and should be processed.
func (f *Flooder) HandleNodeInfoAdvertise(
	fromPeer identity.AgentID,
	originAgent identity.AgentID,
	sequence uint64,
	encInfo *protocol.EncryptedData,
	seenBy []identity.AgentID,
) bool {
	key := NodeInfoKey{
		OriginAgent: originAgent,
		Sequence:    sequence,
	}

	// Check if we've already seen this and mark as seen atomically
	f.nodeInfoMu.Lock()
	if existing, ok := f.nodeInfoSeenCache[key]; ok {
		// Already seen - update seen time if from a new peer
		if existing.SeenFrom != fromPeer {
			existing.SeenAt = time.Now()
		}
		f.nodeInfoMu.Unlock()
		return false
	}

	// Mark as seen
	f.nodeInfoSeenCache[key] = &SeenNodeInfo{
		Key:      key,
		SeenAt:   time.Now(),
		SeenFrom: fromPeer,
	}
	f.nodeInfoMu.Unlock()

	// Check if we're in the seen-by list (loop detection)
	for _, id := range seenBy {
		if id == f.localID {
			return false // We've already processed this
		}
	}

	// Store the node info in the routing manager (handles decryption if possible)
	if f.routeMgr.SetNodeInfoEncrypted(originAgent, encInfo, sequence) {
		f.logger.Debug("stored node info",
			"origin", originAgent.ShortString(),
			"encrypted", encInfo.Encrypted)
	}

	// Flood to other peers (forward encrypted data as-is)
	newSeenBy := append(seenBy, f.localID)
	f.floodNodeInfoEncrypted(fromPeer, originAgent, sequence, encInfo, newSeenBy)

	return true
}

// floodNodeInfoEncrypted sends encrypted node info advertisement to all peers except the source.
func (f *Flooder) floodNodeInfoEncrypted(
	fromPeer identity.AgentID,
	originAgent identity.AgentID,
	sequence uint64,
	encInfo *protocol.EncryptedData,
	seenBy []identity.AgentID,
) {
	peers := f.sender.GetPeerIDs()

	// Build the node info advertise payload with encrypted data
	adv := &protocol.NodeInfoAdvertise{
		OriginAgent: originAgent,
		Sequence:    sequence,
		EncInfo:     encInfo,
		SeenBy:      seenBy,
	}

	frame := &protocol.Frame{
		Type:     protocol.FrameNodeInfoAdvertise,
		StreamID: protocol.ControlStreamID,
		Payload:  adv.Encode(),
	}

	for _, peerID := range peers {
		// Don't send back to source
		if peerID == fromPeer {
			continue
		}

		// Don't send to peers in the seen-by list
		skip := false
		for _, id := range seenBy {
			if id == peerID {
				skip = true
				break
			}
		}
		if skip {
			continue
		}

		if err := f.sender.SendToPeer(peerID, frame); err != nil {
			f.logger.Debug("failed to send node info advertisement",
				logging.KeyPeerID, peerID.ShortString(),
				logging.KeyError, err)
		}
	}
}

// AnnounceLocalNodeInfo floods local node info to all peers.
func (f *Flooder) AnnounceLocalNodeInfo(info *protocol.NodeInfo) {
	if info == nil {
		return
	}

	// Increment sequence
	f.nodeInfoMu.Lock()
	f.nodeInfoSeq++
	seq := f.nodeInfoSeq
	f.nodeInfoMu.Unlock()

	// Store our own info in the routing manager (plaintext for local access)
	f.routeMgr.SetNodeInfo(f.localID, info, seq)

	// Build encrypted data for flooding
	var encInfo *protocol.EncryptedData
	infoBytes := protocol.EncodeNodeInfo(info)

	if f.sealedBox != nil {
		// Encrypt NodeInfo for flooding
		encrypted, err := f.sealedBox.Seal(infoBytes)
		if err != nil {
			f.logger.Debug("failed to encrypt node info",
				logging.KeyError, err)
			return
		}
		encInfo = &protocol.EncryptedData{
			Encrypted: true,
			Data:      encrypted,
		}
	} else {
		// No encryption - send plaintext
		encInfo = &protocol.EncryptedData{
			Encrypted: false,
			Data:      infoBytes,
		}
	}

	// Build the advertisement
	adv := &protocol.NodeInfoAdvertise{
		OriginAgent: f.localID,
		Sequence:    seq,
		Info:        *info, // Keep for backwards compat in Encode()
		EncInfo:     encInfo,
		SeenBy:      []identity.AgentID{f.localID},
	}

	frame := &protocol.Frame{
		Type:     protocol.FrameNodeInfoAdvertise,
		StreamID: protocol.ControlStreamID,
		Payload:  adv.Encode(),
	}

	// Send to all peers
	for _, peerID := range f.sender.GetPeerIDs() {
		if err := f.sender.SendToPeer(peerID, frame); err != nil {
			f.logger.Debug("failed to announce local node info",
				logging.KeyPeerID, peerID.ShortString(),
				logging.KeyError, err)
		}
	}

	f.logger.Debug("announced local node info",
		"display_name", info.DisplayName,
		"hostname", info.Hostname,
		"sequence", seq,
		"encrypted", encInfo.Encrypted)
}

// SendNodeInfoToNewPeer sends all known node info to a newly connected peer.
func (f *Flooder) SendNodeInfoToNewPeer(peerID identity.AgentID) {
	allEntries := f.routeMgr.GetAllNodeInfoEntries()
	if len(allEntries) == 0 {
		return
	}

	for agentID, entry := range allEntries {
		if entry == nil || entry.EncInfo == nil {
			continue
		}

		adv := &protocol.NodeInfoAdvertise{
			OriginAgent: agentID,
			Sequence:    entry.Sequence,
			EncInfo:     entry.EncInfo, // Forward encrypted data as-is
			SeenBy:      []identity.AgentID{f.localID},
		}

		frame := &protocol.Frame{
			Type:     protocol.FrameNodeInfoAdvertise,
			StreamID: protocol.ControlStreamID,
			Payload:  adv.Encode(),
		}

		if err := f.sender.SendToPeer(peerID, frame); err != nil {
			f.logger.Debug("failed to send node info to new peer",
				logging.KeyPeerID, peerID.ShortString(),
				"origin", agentID.ShortString(),
				logging.KeyError, err)
		}
	}

	f.logger.Debug("sent node info table to new peer",
		logging.KeyPeerID, peerID.ShortString(),
		"count", len(allEntries))
}

// NodeInfoSeenCacheSize returns the current size of the node info seen cache.
func (f *Flooder) NodeInfoSeenCacheSize() int {
	f.nodeInfoMu.RLock()
	defer f.nodeInfoMu.RUnlock()
	return len(f.nodeInfoSeenCache)
}
