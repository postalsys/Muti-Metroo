package flood

import (
	"sync"
	"testing"
	"time"

	"github.com/postalsys/muti-metroo/internal/identity"
	"github.com/postalsys/muti-metroo/internal/protocol"
	"github.com/postalsys/muti-metroo/internal/routing"
)

// ============================================================================
// Mock PeerSender
// ============================================================================

type mockPeerSender struct {
	mu       sync.Mutex
	peers    []identity.AgentID
	messages map[identity.AgentID][]*protocol.Frame
}

func newMockPeerSender() *mockPeerSender {
	return &mockPeerSender{
		messages: make(map[identity.AgentID][]*protocol.Frame),
	}
}

func (m *mockPeerSender) SendToPeer(peerID identity.AgentID, frame *protocol.Frame) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages[peerID] = append(m.messages[peerID], frame)
	return nil
}

func (m *mockPeerSender) GetPeerIDs() []identity.AgentID {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]identity.AgentID, len(m.peers))
	copy(result, m.peers)
	return result
}

func (m *mockPeerSender) AddPeer(id identity.AgentID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.peers = append(m.peers, id)
}

func (m *mockPeerSender) GetMessages(peerID identity.AgentID) []*protocol.Frame {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.messages[peerID]
}

func (m *mockPeerSender) TotalMessages() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, msgs := range m.messages {
		count += len(msgs)
	}
	return count
}

// ============================================================================
// Config Tests
// ============================================================================

func TestDefaultFloodConfig(t *testing.T) {
	cfg := DefaultFloodConfig()

	if cfg.SeenCacheTTL != 5*time.Minute {
		t.Errorf("SeenCacheTTL = %v, want 5m", cfg.SeenCacheTTL)
	}
	if cfg.FloodInterval != 1*time.Second {
		t.Errorf("FloodInterval = %v, want 1s", cfg.FloodInterval)
	}
	if cfg.MaxSeenCacheSize != 10000 {
		t.Errorf("MaxSeenCacheSize = %d, want 10000", cfg.MaxSeenCacheSize)
	}
}

// ============================================================================
// Flooder Tests
// ============================================================================

func TestNewFlooder(t *testing.T) {
	localID, _ := identity.NewAgentID()
	routeMgr := routing.NewManager(localID)
	sender := newMockPeerSender()
	cfg := DefaultFloodConfig()

	f := NewFlooder(cfg, localID, routeMgr, sender)
	defer f.Stop()

	if f == nil {
		t.Fatal("NewFlooder returned nil")
	}
	if f.SeenCacheSize() != 0 {
		t.Errorf("Initial SeenCacheSize = %d, want 0", f.SeenCacheSize())
	}
}

func TestFlooder_HandleRouteAdvertise_NewRoute(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peerID, _ := identity.NewAgentID()
	routeMgr := routing.NewManager(localID)
	sender := newMockPeerSender()
	cfg := DefaultFloodConfig()

	f := NewFlooder(cfg, localID, routeMgr, sender)
	defer f.Stop()

	routes := []protocol.Route{
		{
			AddressFamily: protocol.AddrFamilyIPv4,
			PrefixLength:  8,
			Prefix:        []byte{10, 0, 0, 0},
			Metric:        10,
		},
	}

	accepted := f.HandleRouteAdvertise(peerID, peerID, "", 1, routes, nil, nil)
	if !accepted {
		t.Error("First advertisement should be accepted")
	}

	// Should now be seen
	if !f.HasSeen(peerID, 1) {
		t.Error("Advertisement should be in seen cache")
	}

	// Route should be in routing table
	if routeMgr.TotalRoutes() != 1 {
		t.Errorf("TotalRoutes = %d, want 1", routeMgr.TotalRoutes())
	}
}

func TestFlooder_HandleRouteAdvertise_Duplicate(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peerID, _ := identity.NewAgentID()
	routeMgr := routing.NewManager(localID)
	sender := newMockPeerSender()
	cfg := DefaultFloodConfig()

	f := NewFlooder(cfg, localID, routeMgr, sender)
	defer f.Stop()

	routes := []protocol.Route{
		{
			AddressFamily: protocol.AddrFamilyIPv4,
			PrefixLength:  8,
			Prefix:        []byte{10, 0, 0, 0},
			Metric:        10,
		},
	}

	// First advertisement
	f.HandleRouteAdvertise(peerID, peerID, "", 1, routes, nil, nil)

	// Duplicate
	accepted := f.HandleRouteAdvertise(peerID, peerID, "", 1, routes, nil, nil)
	if accepted {
		t.Error("Duplicate advertisement should be rejected")
	}
}

func TestFlooder_HandleRouteAdvertise_LoopDetection(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peerID, _ := identity.NewAgentID()
	routeMgr := routing.NewManager(localID)
	sender := newMockPeerSender()
	cfg := DefaultFloodConfig()

	f := NewFlooder(cfg, localID, routeMgr, sender)
	defer f.Stop()

	routes := []protocol.Route{
		{
			AddressFamily: protocol.AddrFamilyIPv4,
			PrefixLength:  8,
			Prefix:        []byte{10, 0, 0, 0},
			Metric:        10,
		},
	}

	// Advertisement with our ID in seen-by list (loop)
	seenBy := []identity.AgentID{localID}
	accepted := f.HandleRouteAdvertise(peerID, peerID, "", 1, routes, nil, seenBy)
	if accepted {
		t.Error("Advertisement with our ID in seen-by should be rejected")
	}
}

func TestFlooder_HandleRouteAdvertise_Flood(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peer1, _ := identity.NewAgentID()
	peer2, _ := identity.NewAgentID()
	peer3, _ := identity.NewAgentID()
	routeMgr := routing.NewManager(localID)
	sender := newMockPeerSender()
	sender.AddPeer(peer1)
	sender.AddPeer(peer2)
	sender.AddPeer(peer3)
	cfg := DefaultFloodConfig()

	f := NewFlooder(cfg, localID, routeMgr, sender)
	defer f.Stop()

	routes := []protocol.Route{
		{
			AddressFamily: protocol.AddrFamilyIPv4,
			PrefixLength:  8,
			Prefix:        []byte{10, 0, 0, 0},
			Metric:        10,
		},
	}

	// Receive from peer1
	f.HandleRouteAdvertise(peer1, peer1, "", 1, routes, nil, nil)

	// Should flood to peer2 and peer3, but not back to peer1
	if len(sender.GetMessages(peer1)) != 0 {
		t.Error("Should not send back to source peer")
	}
	if len(sender.GetMessages(peer2)) != 1 {
		t.Errorf("Should send to peer2, got %d messages", len(sender.GetMessages(peer2)))
	}
	if len(sender.GetMessages(peer3)) != 1 {
		t.Errorf("Should send to peer3, got %d messages", len(sender.GetMessages(peer3)))
	}
}

func TestFlooder_HandleRouteWithdraw(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peerID, _ := identity.NewAgentID()
	routeMgr := routing.NewManager(localID)
	sender := newMockPeerSender()
	cfg := DefaultFloodConfig()

	f := NewFlooder(cfg, localID, routeMgr, sender)
	defer f.Stop()

	routes := []protocol.Route{
		{
			AddressFamily: protocol.AddrFamilyIPv4,
			PrefixLength:  8,
			Prefix:        []byte{10, 0, 0, 0},
			Metric:        10,
		},
	}

	// First add the route
	f.HandleRouteAdvertise(peerID, peerID, "", 1, routes, nil, nil)

	// Then withdraw
	accepted := f.HandleRouteWithdraw(peerID, peerID, 2, routes, nil)
	if !accepted {
		t.Error("Withdrawal should be accepted")
	}
}

func TestFlooder_HandleRouteWithdraw_Duplicate(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peerID, _ := identity.NewAgentID()
	routeMgr := routing.NewManager(localID)
	sender := newMockPeerSender()
	cfg := DefaultFloodConfig()

	f := NewFlooder(cfg, localID, routeMgr, sender)
	defer f.Stop()

	routes := []protocol.Route{
		{
			AddressFamily: protocol.AddrFamilyIPv4,
			PrefixLength:  8,
			Prefix:        []byte{10, 0, 0, 0},
			Metric:        10,
		},
	}

	// First withdrawal
	f.HandleRouteWithdraw(peerID, peerID, 1, routes, nil)

	// Duplicate
	accepted := f.HandleRouteWithdraw(peerID, peerID, 1, routes, nil)
	if accepted {
		t.Error("Duplicate withdrawal should be rejected")
	}
}

func TestFlooder_AnnounceLocalRoutes(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peerID, _ := identity.NewAgentID()
	routeMgr := routing.NewManager(localID)
	sender := newMockPeerSender()
	sender.AddPeer(peerID)
	cfg := DefaultFloodConfig()

	f := NewFlooder(cfg, localID, routeMgr, sender)
	defer f.Stop()

	// Add a local route
	routeMgr.AddLocalRoute(routing.MustParseCIDR("10.0.0.0/8"), 10)

	// Announce
	f.AnnounceLocalRoutes()

	// Should have sent to peer
	msgs := sender.GetMessages(peerID)
	if len(msgs) != 1 {
		t.Errorf("Should send 1 message, got %d", len(msgs))
	}
	if msgs[0].Type != protocol.FrameRouteAdvertise {
		t.Errorf("Frame type = 0x%02x, want ROUTE_ADVERTISE", msgs[0].Type)
	}
}

func TestFlooder_WithdrawLocalRoutes(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peerID, _ := identity.NewAgentID()
	routeMgr := routing.NewManager(localID)
	sender := newMockPeerSender()
	sender.AddPeer(peerID)
	cfg := DefaultFloodConfig()

	f := NewFlooder(cfg, localID, routeMgr, sender)
	defer f.Stop()

	// Add a local route
	routeMgr.AddLocalRoute(routing.MustParseCIDR("10.0.0.0/8"), 10)

	// Withdraw
	f.WithdrawLocalRoutes()

	msgs := sender.GetMessages(peerID)
	if len(msgs) != 1 {
		t.Errorf("Should send 1 message, got %d", len(msgs))
	}
	if msgs[0].Type != protocol.FrameRouteWithdraw {
		t.Errorf("Frame type = 0x%02x, want ROUTE_WITHDRAW", msgs[0].Type)
	}
}

func TestFlooder_SendFullTable(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peer1, _ := identity.NewAgentID()
	peer2, _ := identity.NewAgentID()
	routeMgr := routing.NewManager(localID)
	sender := newMockPeerSender()
	sender.AddPeer(peer1)
	sender.AddPeer(peer2)
	cfg := DefaultFloodConfig()

	f := NewFlooder(cfg, localID, routeMgr, sender)
	defer f.Stop()

	// Add a local route
	routeMgr.AddLocalRoute(routing.MustParseCIDR("10.0.0.0/8"), 10)

	// Send full table to peer1 only
	f.SendFullTable(peer1)

	// Only peer1 should receive
	if len(sender.GetMessages(peer1)) != 1 {
		t.Error("peer1 should receive full table")
	}
	if len(sender.GetMessages(peer2)) != 0 {
		t.Error("peer2 should not receive full table")
	}
}

func TestFlooder_ClearSeenCache(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peerID, _ := identity.NewAgentID()
	routeMgr := routing.NewManager(localID)
	sender := newMockPeerSender()
	cfg := DefaultFloodConfig()

	f := NewFlooder(cfg, localID, routeMgr, sender)
	defer f.Stop()

	routes := []protocol.Route{
		{
			AddressFamily: protocol.AddrFamilyIPv4,
			PrefixLength:  8,
			Prefix:        []byte{10, 0, 0, 0},
			Metric:        10,
		},
	}

	// Add some entries
	f.HandleRouteAdvertise(peerID, peerID, "", 1, routes, nil, nil)
	f.HandleRouteAdvertise(peerID, peerID, "", 2, routes, nil, nil)

	if f.SeenCacheSize() != 2 {
		t.Errorf("SeenCacheSize = %d, want 2", f.SeenCacheSize())
	}

	// Clear
	f.ClearSeenCache()

	if f.SeenCacheSize() != 0 {
		t.Errorf("SeenCacheSize after clear = %d, want 0", f.SeenCacheSize())
	}
}

func TestFlooder_HasSeen(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peerID, _ := identity.NewAgentID()
	routeMgr := routing.NewManager(localID)
	sender := newMockPeerSender()
	cfg := DefaultFloodConfig()

	f := NewFlooder(cfg, localID, routeMgr, sender)
	defer f.Stop()

	if f.HasSeen(peerID, 1) {
		t.Error("Should not have seen yet")
	}

	routes := []protocol.Route{
		{
			AddressFamily: protocol.AddrFamilyIPv4,
			PrefixLength:  8,
			Prefix:        []byte{10, 0, 0, 0},
			Metric:        10,
		},
	}

	f.HandleRouteAdvertise(peerID, peerID, "", 1, routes, nil, nil)

	if !f.HasSeen(peerID, 1) {
		t.Error("Should have seen after handling")
	}
}

func TestFlooder_IPv6Routes(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peerID, _ := identity.NewAgentID()
	routeMgr := routing.NewManager(localID)
	sender := newMockPeerSender()
	cfg := DefaultFloodConfig()

	f := NewFlooder(cfg, localID, routeMgr, sender)
	defer f.Stop()

	routes := []protocol.Route{
		{
			AddressFamily: protocol.AddrFamilyIPv6,
			PrefixLength:  32,
			Prefix:        []byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			Metric:        10,
		},
	}

	accepted := f.HandleRouteAdvertise(peerID, peerID, "", 1, routes, nil, nil)
	if !accepted {
		t.Error("IPv6 route should be accepted")
	}

	if routeMgr.TotalRoutes() != 1 {
		t.Errorf("TotalRoutes = %d, want 1", routeMgr.TotalRoutes())
	}
}

func TestFlooder_Stop(t *testing.T) {
	localID, _ := identity.NewAgentID()
	routeMgr := routing.NewManager(localID)
	sender := newMockPeerSender()
	cfg := DefaultFloodConfig()
	cfg.SeenCacheTTL = 100 * time.Millisecond

	f := NewFlooder(cfg, localID, routeMgr, sender)

	// Stop multiple times should not panic
	f.Stop()
	f.Stop()
}

// ============================================================================
// Advertisement Key Tests
// ============================================================================

func TestAdvertisementKey_Uniqueness(t *testing.T) {
	id1, _ := identity.NewAgentID()
	id2, _ := identity.NewAgentID()

	key1 := AdvertisementKey{OriginAgent: id1, Sequence: 1}
	key2 := AdvertisementKey{OriginAgent: id1, Sequence: 2}
	key3 := AdvertisementKey{OriginAgent: id2, Sequence: 1}

	if key1 == key2 {
		t.Error("Different sequences should be different keys")
	}
	if key1 == key3 {
		t.Error("Different origins should be different keys")
	}
}
