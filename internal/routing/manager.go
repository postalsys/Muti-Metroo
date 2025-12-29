// Package routing implements CIDR-based routing for Muti Metroo mesh network.
package routing

import (
	"net"
	"sync"

	"github.com/postalsys/muti-metroo/internal/identity"
)

// RouteChange represents a route update.
type RouteChange struct {
	Type    RouteChangeType
	Route   *Route
	OldPath []identity.AgentID // For updates
}

// RouteChangeType indicates the type of route change.
type RouteChangeType int

const (
	RouteAdded RouteChangeType = iota
	RouteRemoved
	RouteUpdated
)

// LocalRoute represents a locally-announced route.
type LocalRoute struct {
	Network *net.IPNet
	Metric  uint16
}

// Manager handles route management including local routes and propagation.
type Manager struct {
	mu sync.RWMutex

	localID     identity.AgentID
	table       *Table
	localRoutes map[string]*LocalRoute
	sequence    uint64

	// Subscribers for route changes
	subscribers []chan<- RouteChange
	subMu       sync.RWMutex
}

// NewManager creates a new route manager.
func NewManager(localID identity.AgentID) *Manager {
	return &Manager{
		localID:     localID,
		table:       NewTable(localID),
		localRoutes: make(map[string]*LocalRoute),
	}
}

// Table returns the underlying routing table.
func (m *Manager) Table() *Table {
	return m.table
}

// AddLocalRoute adds a locally-originated route.
func (m *Manager) AddLocalRoute(network *net.IPNet, metric uint16) bool {
	if network == nil {
		return false
	}

	key := network.String()

	m.mu.Lock()
	m.sequence++
	seq := m.sequence

	m.localRoutes[key] = &LocalRoute{
		Network: network,
		Metric:  metric,
	}
	m.mu.Unlock()

	// Add to table with ourselves as origin
	// Note: Path is empty for local routes to avoid loop detection on our own ID
	route := &Route{
		Network:     network,
		NextHop:     m.localID, // Local route
		OriginAgent: m.localID,
		Metric:      metric,
		Path:        nil, // Empty path for local routes
		Sequence:    seq,
	}

	added := m.table.AddRoute(route)
	if added {
		m.notifyChange(RouteChange{
			Type:  RouteAdded,
			Route: route.Clone(),
		})
	}

	return added
}

// RemoveLocalRoute removes a locally-originated route.
func (m *Manager) RemoveLocalRoute(network *net.IPNet) bool {
	if network == nil {
		return false
	}

	key := network.String()

	m.mu.Lock()
	_, exists := m.localRoutes[key]
	if !exists {
		m.mu.Unlock()
		return false
	}
	delete(m.localRoutes, key)
	m.mu.Unlock()

	removed := m.table.RemoveRoute(network, m.localID)
	if removed {
		m.notifyChange(RouteChange{
			Type: RouteRemoved,
			Route: &Route{
				Network:     network,
				OriginAgent: m.localID,
			},
		})
	}

	return removed
}

// GetLocalRoutes returns all locally-originated routes.
func (m *Manager) GetLocalRoutes() []*LocalRoute {
	m.mu.RLock()
	defer m.mu.RUnlock()

	routes := make([]*LocalRoute, 0, len(m.localRoutes))
	for _, r := range m.localRoutes {
		routes = append(routes, &LocalRoute{
			Network: r.Network,
			Metric:  r.Metric,
		})
	}
	return routes
}

// ProcessRouteAdvertise processes an incoming ROUTE_ADVERTISE.
// Returns the list of routes that were added/updated.
func (m *Manager) ProcessRouteAdvertise(
	fromPeer identity.AgentID,
	originAgent identity.AgentID,
	sequence uint64,
	routes []RouteEntry,
	path []identity.AgentID,
) []*Route {
	var accepted []*Route

	// Path already contains the sender prepended by the flooder, use it directly
	// (the first element of path should be fromPeer, set by floodAdvertisement)
	for _, entry := range routes {
		route := &Route{
			Network:     entry.Network,
			NextHop:     fromPeer,
			OriginAgent: originAgent,
			Metric:      entry.Metric + 1, // Increment metric
			Path:        path,
			Sequence:    sequence,
		}

		if m.table.AddRoute(route) {
			accepted = append(accepted, route.Clone())
			m.notifyChange(RouteChange{
				Type:  RouteAdded,
				Route: route.Clone(),
			})
		}
	}

	return accepted
}

// ProcessRouteWithdraw processes an incoming ROUTE_WITHDRAW.
// Returns true if any routes were removed.
func (m *Manager) ProcessRouteWithdraw(
	originAgent identity.AgentID,
	routes []RouteEntry,
) bool {
	removed := false

	for _, entry := range routes {
		if m.table.RemoveRoute(entry.Network, originAgent) {
			removed = true
			m.notifyChange(RouteChange{
				Type: RouteRemoved,
				Route: &Route{
					Network:     entry.Network,
					OriginAgent: originAgent,
				},
			})
		}
	}

	return removed
}

// HandlePeerDisconnect removes all routes learned from a disconnected peer.
func (m *Manager) HandlePeerDisconnect(peerID identity.AgentID) int {
	return m.table.RemoveRoutesFromPeer(peerID)
}

// Lookup finds the best route for an IP address.
func (m *Manager) Lookup(ip net.IP) *Route {
	return m.table.Lookup(ip)
}

// LookupNextHop returns just the next-hop peer ID for an IP.
func (m *Manager) LookupNextHop(ip net.IP) (identity.AgentID, bool) {
	route := m.table.Lookup(ip)
	if route == nil {
		return identity.AgentID{}, false
	}
	return route.NextHop, true
}

// Subscribe registers a channel to receive route changes.
func (m *Manager) Subscribe(ch chan<- RouteChange) {
	m.subMu.Lock()
	defer m.subMu.Unlock()
	m.subscribers = append(m.subscribers, ch)
}

// Unsubscribe removes a channel from route change notifications.
func (m *Manager) Unsubscribe(ch chan<- RouteChange) {
	m.subMu.Lock()
	defer m.subMu.Unlock()

	for i, sub := range m.subscribers {
		if sub == ch {
			m.subscribers = append(m.subscribers[:i], m.subscribers[i+1:]...)
			return
		}
	}
}

// notifyChange sends a route change to all subscribers.
func (m *Manager) notifyChange(change RouteChange) {
	m.subMu.RLock()
	defer m.subMu.RUnlock()

	for _, ch := range m.subscribers {
		select {
		case ch <- change:
		default:
			// Channel full, skip
		}
	}
}

// GetCurrentSequence returns the current sequence number.
func (m *Manager) GetCurrentSequence() uint64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sequence
}

// IncrementSequence increments and returns the new sequence number.
func (m *Manager) IncrementSequence() uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sequence++
	return m.sequence
}

// RouteEntry is a simplified route for advertisements.
type RouteEntry struct {
	Network *net.IPNet
	Metric  uint16
}

// GetRoutesToAdvertise returns routes that should be advertised to peers.
// This includes local routes and routes learned from other peers.
func (m *Manager) GetRoutesToAdvertise(excludePeer identity.AgentID) []RouteEntry {
	allRoutes := m.table.GetAllRoutes()
	var entries []RouteEntry

	seen := make(map[string]bool)
	for _, route := range allRoutes {
		// Don't advertise routes learned from the peer we're advertising to
		if route.NextHop == excludePeer {
			continue
		}

		key := route.Network.String()
		if seen[key] {
			continue
		}
		seen[key] = true

		entries = append(entries, RouteEntry{
			Network: route.Network,
			Metric:  route.Metric,
		})
	}

	return entries
}

// GetFullRoutesForAdvertise returns full route information for advertising to a peer.
// Routes are filtered to exclude those learned from the peer we're advertising to.
func (m *Manager) GetFullRoutesForAdvertise(excludePeer identity.AgentID) []*Route {
	allRoutes := m.table.GetAllRoutes()
	var result []*Route

	seen := make(map[string]bool)
	for _, route := range allRoutes {
		// Don't advertise routes learned from the peer we're advertising to
		if route.NextHop == excludePeer {
			continue
		}

		// Use a key of network+origin to allow multiple origins for same network
		key := route.Network.String() + ":" + route.OriginAgent.String()
		if seen[key] {
			continue
		}
		seen[key] = true

		result = append(result, route)
	}

	return result
}

// Size returns the number of unique prefixes in the routing table.
func (m *Manager) Size() int {
	return m.table.Size()
}

// TotalRoutes returns the total number of route entries.
func (m *Manager) TotalRoutes() int {
	return m.table.TotalRoutes()
}
