// Package routing implements tunnel-based routing for Muti Metroo mesh network.
package routing

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/postalsys/muti-metroo/internal/identity"
	"github.com/postalsys/muti-metroo/internal/protocol"
)

// TunnelRoute represents a single tunnel route entry.
type TunnelRoute struct {
	// Key is the routing key that identifies this tunnel (e.g., "my-web-server")
	Key string

	// Target is the fixed destination host:port for tunneled connections.
	// This is only set on the origin agent; transit nodes don't know the target.
	Target string

	// NextHop is the peer ID to forward traffic to
	NextHop identity.AgentID

	// OriginAgent is the original advertiser of this route
	OriginAgent identity.AgentID

	// Metric is the route cost (lower is better)
	Metric uint16

	// Path is the AS-path style list of agent IDs (nil if encrypted and can't decrypt)
	Path []identity.AgentID

	// EncPath is the encrypted path data for forwarding (nil if no encryption)
	EncPath *protocol.EncryptedData

	// Sequence is used for route versioning
	Sequence uint64

	// LastUpdate is when this route was last added or refreshed
	LastUpdate time.Time
}

// String returns a human-readable representation of the tunnel route.
func (r *TunnelRoute) String() string {
	return fmt.Sprintf("TunnelRoute{%s via %s, metric=%d, origin=%s}",
		r.Key, r.NextHop.ShortString(), r.Metric, r.OriginAgent.ShortString())
}

// Clone creates a deep copy of the tunnel route.
func (r *TunnelRoute) Clone() *TunnelRoute {
	clone := &TunnelRoute{
		Key:         r.Key,
		Target:      r.Target,
		NextHop:     r.NextHop,
		OriginAgent: r.OriginAgent,
		Metric:      r.Metric,
		Sequence:    r.Sequence,
		LastUpdate:  r.LastUpdate,
	}
	if len(r.Path) > 0 {
		clone.Path = make([]identity.AgentID, len(r.Path))
		copy(clone.Path, r.Path)
	}
	if r.EncPath != nil {
		clone.EncPath = &protocol.EncryptedData{
			Encrypted: r.EncPath.Encrypted,
			Data:      make([]byte, len(r.EncPath.Data)),
		}
		copy(clone.EncPath.Data, r.EncPath.Data)
	}
	return clone
}

// TunnelTable is a thread-safe tunnel routing table.
type TunnelTable struct {
	mu sync.RWMutex

	// routes maps tunnel key to route entries
	// Key: tunnel routing key (e.g., "my-web-server")
	routes map[string][]*TunnelRoute

	// localID is this agent's ID (for loop detection)
	localID identity.AgentID
}

// NewTunnelTable creates a new tunnel routing table.
func NewTunnelTable(localID identity.AgentID) *TunnelTable {
	return &TunnelTable{
		routes:  make(map[string][]*TunnelRoute),
		localID: localID,
	}
}

// AddRoute adds or updates a tunnel route in the table.
// Returns true if the route was added/updated, false if rejected (e.g., loop detected).
func (t *TunnelTable) AddRoute(route *TunnelRoute) bool {
	if route == nil || route.Key == "" {
		return false
	}

	// Check for routing loops (is our ID in the path?)
	for _, id := range route.Path {
		if id == t.localID {
			return false // Loop detected
		}
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	key := route.Key

	// Check if we already have a route from this origin
	for i, r := range t.routes[key] {
		if r.OriginAgent == route.OriginAgent {
			// Update if newer sequence or better metric
			if route.Sequence > r.Sequence ||
				(route.Sequence == r.Sequence && route.Metric < r.Metric) {
				cloned := route.Clone()
				cloned.LastUpdate = time.Now()
				t.routes[key][i] = cloned
				t.sortRoutes(key)
				return true
			}
			return false // Older/worse route
		}
	}

	// New route from this origin
	cloned := route.Clone()
	cloned.LastUpdate = time.Now()
	t.routes[key] = append(t.routes[key], cloned)
	t.sortRoutes(key)
	return true
}

// sortRoutes sorts routes for a key by metric (lowest first).
func (t *TunnelTable) sortRoutes(key string) {
	routes := t.routes[key]
	sort.Slice(routes, func(i, j int) bool {
		return routes[i].Metric < routes[j].Metric
	})
}

// RemoveRoute removes a tunnel route from a specific origin.
func (t *TunnelTable) RemoveRoute(key string, originAgent identity.AgentID) bool {
	if key == "" {
		return false
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	routes := t.routes[key]
	for i, r := range routes {
		if r.OriginAgent == originAgent {
			t.routes[key] = append(routes[:i], routes[i+1:]...)
			if len(t.routes[key]) == 0 {
				delete(t.routes, key)
			}
			return true
		}
	}
	return false
}

// RemoveRoutesFromPeer removes all tunnel routes learned from a specific peer.
func (t *TunnelTable) RemoveRoutesFromPeer(peerID identity.AgentID) int {
	t.mu.Lock()
	defer t.mu.Unlock()

	count := 0
	for key, routes := range t.routes {
		filtered := routes[:0]
		for _, r := range routes {
			if r.NextHop != peerID {
				filtered = append(filtered, r)
			} else {
				count++
			}
		}
		if len(filtered) == 0 {
			delete(t.routes, key)
		} else {
			t.routes[key] = filtered
		}
	}
	return count
}

// Lookup finds the best tunnel route for a routing key.
func (t *TunnelTable) Lookup(key string) *TunnelRoute {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if routes, ok := t.routes[key]; ok && len(routes) > 0 {
		return routes[0].Clone() // First is best due to sorting by metric
	}
	return nil
}

// GetAllRoutes returns all tunnel routes in the table.
func (t *TunnelTable) GetAllRoutes() []*TunnelRoute {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var all []*TunnelRoute
	for _, routes := range t.routes {
		for _, r := range routes {
			all = append(all, r.Clone())
		}
	}
	return all
}

// GetRoutesFromAgent returns all tunnel routes originating from a specific agent.
func (t *TunnelTable) GetRoutesFromAgent(agentID identity.AgentID) []*TunnelRoute {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var matching []*TunnelRoute
	for _, routes := range t.routes {
		for _, r := range routes {
			if r.OriginAgent == agentID {
				matching = append(matching, r.Clone())
			}
		}
	}
	return matching
}

// Size returns the number of unique tunnel keys in the table.
func (t *TunnelTable) Size() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.routes)
}

// TotalRoutes returns the total number of tunnel route entries.
func (t *TunnelTable) TotalRoutes() int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	count := 0
	for _, routes := range t.routes {
		count += len(routes)
	}
	return count
}

// Clear removes all tunnel routes from the table.
func (t *TunnelTable) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.routes = make(map[string][]*TunnelRoute)
}

// HasRoute checks if a tunnel route exists for the given key and origin.
func (t *TunnelTable) HasRoute(key string, originAgent identity.AgentID) bool {
	if key == "" {
		return false
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	for _, r := range t.routes[key] {
		if r.OriginAgent == originAgent {
			return true
		}
	}
	return false
}

// CleanupStaleRoutes removes tunnel routes that haven't been updated within maxAge.
// Local routes (where OriginAgent == localID) are never removed.
// Returns the number of routes removed.
func (t *TunnelTable) CleanupStaleRoutes(maxAge time.Duration) int {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	removed := 0

	for key, routes := range t.routes {
		var kept []*TunnelRoute
		for _, r := range routes {
			if r.OriginAgent == t.localID || now.Sub(r.LastUpdate) <= maxAge {
				kept = append(kept, r)
			} else {
				removed++
			}
		}
		if len(kept) > 0 {
			t.routes[key] = kept
		} else {
			delete(t.routes, key)
		}
	}
	return removed
}

// LocalTunnelRoute represents a locally-announced tunnel route.
type LocalTunnelRoute struct {
	Key    string // Routing key
	Target string // Fixed target host:port
	Metric uint16
}

// TunnelRouteChange represents a tunnel route update.
type TunnelRouteChange struct {
	Type  RouteChangeType
	Route *TunnelRoute
}
