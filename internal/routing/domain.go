// Package routing implements domain-based routing for Muti Metroo mesh network.
package routing

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/postalsys/muti-metroo/internal/identity"
	"github.com/postalsys/muti-metroo/internal/protocol"
)

// DomainRoute represents a single domain route entry.
type DomainRoute struct {
	// Pattern is the original pattern (e.g., "example.com" or "*.example.com")
	Pattern string

	// IsWildcard indicates if this is a wildcard pattern (*.domain)
	IsWildcard bool

	// BaseDomain is the domain without the wildcard prefix
	// For exact matches, this equals Pattern
	// For wildcards, "*.example.com" -> "example.com"
	BaseDomain string

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

// String returns a human-readable representation of the domain route.
func (r *DomainRoute) String() string {
	return fmt.Sprintf("DomainRoute{%s via %s, metric=%d, origin=%s}",
		r.Pattern, r.NextHop.ShortString(), r.Metric, r.OriginAgent.ShortString())
}

// Clone creates a deep copy of the domain route.
func (r *DomainRoute) Clone() *DomainRoute {
	clone := &DomainRoute{
		Pattern:     r.Pattern,
		IsWildcard:  r.IsWildcard,
		BaseDomain:  r.BaseDomain,
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

// DomainTable is a thread-safe domain routing table.
type DomainTable struct {
	mu sync.RWMutex

	// exactRoutes maps exact domain to route entries
	// Key: lowercase domain (e.g., "api.example.com")
	exactRoutes map[string][]*DomainRoute

	// wildcardBase maps base domain to wildcard route entries
	// Key: lowercase base domain (e.g., "example.com" for "*.example.com")
	wildcardBase map[string][]*DomainRoute

	// localID is this agent's ID (for loop detection)
	localID identity.AgentID
}

// NewDomainTable creates a new domain routing table.
func NewDomainTable(localID identity.AgentID) *DomainTable {
	return &DomainTable{
		exactRoutes:  make(map[string][]*DomainRoute),
		wildcardBase: make(map[string][]*DomainRoute),
		localID:      localID,
	}
}

// AddRoute adds or updates a domain route in the table.
// Returns true if the route was added/updated, false if rejected (e.g., loop detected).
func (t *DomainTable) AddRoute(route *DomainRoute) bool {
	if route == nil || route.Pattern == "" {
		return false
	}

	// Check for routing loops (is our ID in the path?)
	for _, id := range route.Path {
		if id == t.localID {
			return false // Loop detected
		}
	}

	var key string
	var targetMap *map[string][]*DomainRoute

	if route.IsWildcard {
		key = strings.ToLower(route.BaseDomain)
		targetMap = &t.wildcardBase
	} else {
		key = strings.ToLower(route.Pattern)
		targetMap = &t.exactRoutes
	}

	now := time.Now()

	t.mu.Lock()
	defer t.mu.Unlock()

	// Check if we already have a route from this origin
	existing := (*targetMap)[key]
	for i, r := range existing {
		if r.OriginAgent == route.OriginAgent {
			// Update if newer sequence or better metric
			if route.Sequence > r.Sequence ||
				(route.Sequence == r.Sequence && route.Metric < r.Metric) {
				cloned := route.Clone()
				cloned.LastUpdate = now
				(*targetMap)[key][i] = cloned
				t.sortRoutes(targetMap, key)
				return true
			}
			return false // Older/worse route
		}
	}

	// New route from this origin
	cloned := route.Clone()
	cloned.LastUpdate = now
	(*targetMap)[key] = append((*targetMap)[key], cloned)
	t.sortRoutes(targetMap, key)
	return true
}

// sortRoutes sorts routes for a key by metric (lowest first).
func (t *DomainTable) sortRoutes(targetMap *map[string][]*DomainRoute, key string) {
	routes := (*targetMap)[key]
	sort.Slice(routes, func(i, j int) bool {
		return routes[i].Metric < routes[j].Metric
	})
}

// RemoveRoute removes a domain route from a specific origin.
func (t *DomainTable) RemoveRoute(pattern string, originAgent identity.AgentID) bool {
	if pattern == "" {
		return false
	}

	isWildcard, baseDomain := ParseDomainPattern(pattern)

	var key string
	var targetMap *map[string][]*DomainRoute

	if isWildcard {
		key = strings.ToLower(baseDomain)
		targetMap = &t.wildcardBase
	} else {
		key = strings.ToLower(pattern)
		targetMap = &t.exactRoutes
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	routes := (*targetMap)[key]
	for i, r := range routes {
		if r.OriginAgent == originAgent {
			// Remove this route
			(*targetMap)[key] = append(routes[:i], routes[i+1:]...)
			if len((*targetMap)[key]) == 0 {
				delete(*targetMap, key)
			}
			return true
		}
	}
	return false
}

// RemoveRoutesFromPeer removes all domain routes learned from a specific peer.
func (t *DomainTable) RemoveRoutesFromPeer(peerID identity.AgentID) int {
	t.mu.Lock()
	defer t.mu.Unlock()

	count := 0

	// Remove from exact routes
	for key, routes := range t.exactRoutes {
		filtered := routes[:0]
		for _, r := range routes {
			if r.NextHop != peerID {
				filtered = append(filtered, r)
			} else {
				count++
			}
		}
		if len(filtered) == 0 {
			delete(t.exactRoutes, key)
		} else {
			t.exactRoutes[key] = filtered
		}
	}

	// Remove from wildcard routes
	for key, routes := range t.wildcardBase {
		filtered := routes[:0]
		for _, r := range routes {
			if r.NextHop != peerID {
				filtered = append(filtered, r)
			} else {
				count++
			}
		}
		if len(filtered) == 0 {
			delete(t.wildcardBase, key)
		} else {
			t.wildcardBase[key] = filtered
		}
	}

	return count
}

// Lookup finds the best domain route for a domain name.
// First checks exact matches, then single-level wildcards.
func (t *DomainTable) Lookup(domain string) *DomainRoute {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.lookupUnlocked(domain)
}

// lookupUnlocked performs lookup without locking (caller must hold lock).
func (t *DomainTable) lookupUnlocked(domain string) *DomainRoute {
	domain = strings.ToLower(domain)

	// 1. Check exact match first
	if routes, ok := t.exactRoutes[domain]; ok && len(routes) > 0 {
		return routes[0].Clone() // First is best due to sorting by metric
	}

	// 2. Check single-level wildcard
	// Split at first dot to get parent domain
	idx := strings.Index(domain, ".")
	if idx > 0 && idx < len(domain)-1 {
		baseDomain := domain[idx+1:]
		if routes, ok := t.wildcardBase[baseDomain]; ok && len(routes) > 0 {
			return routes[0].Clone()
		}
	}

	return nil
}

// GetAllRoutes returns all domain routes in the table.
func (t *DomainTable) GetAllRoutes() []*DomainRoute {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var all []*DomainRoute
	for _, routes := range t.exactRoutes {
		for _, r := range routes {
			all = append(all, r.Clone())
		}
	}
	for _, routes := range t.wildcardBase {
		for _, r := range routes {
			all = append(all, r.Clone())
		}
	}
	return all
}

// GetRoutesFromAgent returns all domain routes originating from a specific agent.
func (t *DomainTable) GetRoutesFromAgent(agentID identity.AgentID) []*DomainRoute {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var matching []*DomainRoute
	for _, routes := range t.exactRoutes {
		for _, r := range routes {
			if r.OriginAgent == agentID {
				matching = append(matching, r.Clone())
			}
		}
	}
	for _, routes := range t.wildcardBase {
		for _, r := range routes {
			if r.OriginAgent == agentID {
				matching = append(matching, r.Clone())
			}
		}
	}
	return matching
}

// Size returns the number of unique domain patterns in the table.
func (t *DomainTable) Size() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.exactRoutes) + len(t.wildcardBase)
}

// TotalRoutes returns the total number of domain route entries.
func (t *DomainTable) TotalRoutes() int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	count := 0
	for _, routes := range t.exactRoutes {
		count += len(routes)
	}
	for _, routes := range t.wildcardBase {
		count += len(routes)
	}
	return count
}

// Clear removes all domain routes from the table.
func (t *DomainTable) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.exactRoutes = make(map[string][]*DomainRoute)
	t.wildcardBase = make(map[string][]*DomainRoute)
}

// HasRoute checks if a domain route exists for the given pattern and origin.
func (t *DomainTable) HasRoute(pattern string, originAgent identity.AgentID) bool {
	if pattern == "" {
		return false
	}

	isWildcard, baseDomain := ParseDomainPattern(pattern)

	var key string
	var targetMap map[string][]*DomainRoute

	t.mu.RLock()
	defer t.mu.RUnlock()

	if isWildcard {
		key = strings.ToLower(baseDomain)
		targetMap = t.wildcardBase
	} else {
		key = strings.ToLower(pattern)
		targetMap = t.exactRoutes
	}

	for _, r := range targetMap[key] {
		if r.OriginAgent == originAgent {
			return true
		}
	}
	return false
}

// CleanupStaleRoutes removes domain routes that haven't been updated within maxAge.
// Local routes (where OriginAgent == localID) are never removed.
// Returns the number of routes removed.
func (t *DomainTable) CleanupStaleRoutes(maxAge time.Duration) int {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	removed := 0

	// Cleanup exact routes
	for key, routes := range t.exactRoutes {
		var kept []*DomainRoute
		for _, r := range routes {
			// Never remove local routes
			if r.OriginAgent == t.localID {
				kept = append(kept, r)
				continue
			}

			// Keep routes that are still fresh
			if now.Sub(r.LastUpdate) <= maxAge {
				kept = append(kept, r)
			} else {
				removed++
			}
		}

		if len(kept) > 0 {
			t.exactRoutes[key] = kept
		} else {
			delete(t.exactRoutes, key)
		}
	}

	// Cleanup wildcard routes
	for key, routes := range t.wildcardBase {
		var kept []*DomainRoute
		for _, r := range routes {
			// Never remove local routes
			if r.OriginAgent == t.localID {
				kept = append(kept, r)
				continue
			}

			// Keep routes that are still fresh
			if now.Sub(r.LastUpdate) <= maxAge {
				kept = append(kept, r)
			} else {
				removed++
			}
		}

		if len(kept) > 0 {
			t.wildcardBase[key] = kept
		} else {
			delete(t.wildcardBase, key)
		}
	}

	return removed
}

// ParseDomainPattern parses a domain pattern and returns whether it's a wildcard
// and the base domain.
func ParseDomainPattern(pattern string) (isWildcard bool, baseDomain string) {
	pattern = strings.TrimSpace(pattern)
	if strings.HasPrefix(pattern, "*.") {
		return true, pattern[2:]
	}
	return false, pattern
}

// ValidateDomainPattern validates a domain pattern.
// Returns nil if valid, or an error describing the issue.
func ValidateDomainPattern(pattern string) error {
	if pattern == "" {
		return fmt.Errorf("empty domain pattern")
	}

	isWildcard, baseDomain := ParseDomainPattern(pattern)

	// Validate base domain
	domain := baseDomain
	if !isWildcard {
		domain = pattern
	}

	if domain == "" {
		return fmt.Errorf("empty domain after wildcard")
	}

	// Basic domain validation
	if strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return fmt.Errorf("domain cannot start or end with a dot")
	}

	if strings.Contains(domain, "..") {
		return fmt.Errorf("domain cannot contain consecutive dots")
	}

	// Check for valid characters
	for _, r := range domain {
		if !isValidDomainChar(r) {
			return fmt.Errorf("invalid character in domain: %c", r)
		}
	}

	// Must have at least one dot (TLD)
	if !strings.Contains(domain, ".") {
		return fmt.Errorf("domain must have at least one dot (e.g., example.com)")
	}

	return nil
}

// isValidDomainChar checks if a character is valid in a domain name.
func isValidDomainChar(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		r == '-' || r == '.'
}

// LocalDomainRoute represents a locally-announced domain route.
type LocalDomainRoute struct {
	Pattern    string
	IsWildcard bool
	BaseDomain string
	Metric     uint16
}

// DomainRouteChange represents a domain route update.
type DomainRouteChange struct {
	Type  RouteChangeType
	Route *DomainRoute
}
