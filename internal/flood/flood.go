// Package flood implements route flooding for Muti Metroo mesh network.
package flood

import (
	"log/slog"
	"net"
	"sync"
	"time"

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

// FloodConfig contains configuration for the flood protocol.
type FloodConfig struct {
	// SeenCacheTTL is how long to remember seen advertisements
	SeenCacheTTL time.Duration

	// FloodInterval is the minimum time between flooding the same route
	FloodInterval time.Duration

	// MaxSeenCacheSize limits the seen cache size
	MaxSeenCacheSize int

	// Logger for logging
	Logger *slog.Logger
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
	cfg       FloodConfig
	localID   identity.AgentID
	routeMgr  *routing.Manager
	sender    PeerSender
	logger    *slog.Logger

	mu        sync.RWMutex
	seenCache map[AdvertisementKey]*SeenAdvertisement

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
		cfg:       cfg,
		localID:   localID,
		routeMgr:  routeMgr,
		sender:    sender,
		logger:    logger,
		seenCache: make(map[AdvertisementKey]*SeenAdvertisement),
		stopCh:    make(chan struct{}),
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
	sequence uint64,
	routes []protocol.Route,
	path []identity.AgentID,
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
		return false
	}

	// Mark as seen
	f.seenCache[key] = &SeenAdvertisement{
		Key:      key,
		SeenAt:   time.Now(),
		SeenFrom: fromPeer,
	}
	f.mu.Unlock()

	// Check if we're in the seen-by list (loop detection)
	for _, id := range seenBy {
		if id == f.localID {
			return false // We've already processed this
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

	// Process in routing manager
	f.routeMgr.ProcessRouteAdvertise(fromPeer, originAgent, sequence, entries, path)

	// Flood to other peers
	newSeenBy := append(seenBy, f.localID)
	f.floodAdvertisement(fromPeer, originAgent, sequence, routes, path, newSeenBy)

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

// floodAdvertisement sends a route advertisement to all peers except the source.
func (f *Flooder) floodAdvertisement(
	fromPeer identity.AgentID,
	originAgent identity.AgentID,
	sequence uint64,
	routes []protocol.Route,
	path []identity.AgentID,
	seenBy []identity.AgentID,
) {
	peers := f.sender.GetPeerIDs()

	// Build the advertise payload
	adv := &protocol.RouteAdvertise{
		OriginAgent: originAgent,
		Sequence:    sequence,
		Routes:      routes,
		Path:        append([]identity.AgentID{f.localID}, path...), // Prepend ourselves
		SeenBy:      seenBy,
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

	// Build advertisement
	adv := &protocol.RouteAdvertise{
		OriginAgent: f.localID,
		Sequence:    seq,
		Routes:      routes,
		Path:        []identity.AgentID{f.localID},
		SeenBy:      []identity.AgentID{f.localID},
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

		adv := &protocol.RouteAdvertise{
			OriginAgent: originAgent,
			Sequence:    seq,
			Routes:      routes,
			Path:        path,
			SeenBy:      []identity.AgentID{f.localID},
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

// cleanup removes expired entries from the seen cache.
func (f *Flooder) cleanup() {
	now := time.Now()
	expiry := f.cfg.SeenCacheTTL

	f.mu.Lock()
	defer f.mu.Unlock()

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
