package flood

import (
	"sync"
	"testing"
	"time"

	"github.com/postalsys/muti-metroo/internal/crypto"
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

// ============================================================================
// Sleep/Wake Command Signature Verification Tests
// ============================================================================

func TestVerifySleepCommand_NoKeyConfigured_AcceptsAll(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peerID, _ := identity.NewAgentID()
	routeMgr := routing.NewManager(localID)
	sender := newMockPeerSender()

	// No signing key configured
	cfg := DefaultFloodConfig()

	f := NewFlooder(cfg, localID, routeMgr, sender)
	defer f.Stop()

	// Unsigned command should be accepted
	cmd := &protocol.SleepCommand{
		OriginAgent: peerID,
		CommandID:   1,
		Timestamp:   uint64(time.Now().Unix()),
		Signature:   [64]byte{}, // Zero signature (unsigned)
		SeenBy:      []identity.AgentID{},
	}

	accepted := f.HandleSleepCommand(peerID, cmd)
	if !accepted {
		t.Error("Unsigned command should be accepted when no signing key configured")
	}
}

func TestVerifySleepCommand_ValidSignature_Accepts(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peerID, _ := identity.NewAgentID()
	routeMgr := routing.NewManager(localID)
	sender := newMockPeerSender()

	// Generate signing keypair
	kp, err := crypto.GenerateSigningKeypair()
	if err != nil {
		t.Fatalf("GenerateSigningKeypair() error = %v", err)
	}

	cfg := DefaultFloodConfig()
	cfg.SigningPublicKey = &kp.PublicKey

	f := NewFlooder(cfg, localID, routeMgr, sender)
	defer f.Stop()

	// Create and sign command
	cmd := &protocol.SleepCommand{
		OriginAgent: peerID,
		CommandID:   1,
		Timestamp:   uint64(time.Now().Unix()),
		SeenBy:      []identity.AgentID{},
	}
	cmd.Signature = crypto.Sign(kp.PrivateKey, cmd.SignableBytes())

	accepted := f.HandleSleepCommand(peerID, cmd)
	if !accepted {
		t.Error("Validly signed command should be accepted")
	}
}

func TestVerifySleepCommand_InvalidSignature_Rejects(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peerID, _ := identity.NewAgentID()
	routeMgr := routing.NewManager(localID)
	sender := newMockPeerSender()

	// Generate signing keypair
	kp, err := crypto.GenerateSigningKeypair()
	if err != nil {
		t.Fatalf("GenerateSigningKeypair() error = %v", err)
	}

	cfg := DefaultFloodConfig()
	cfg.SigningPublicKey = &kp.PublicKey

	f := NewFlooder(cfg, localID, routeMgr, sender)
	defer f.Stop()

	// Create command with garbage signature
	cmd := &protocol.SleepCommand{
		OriginAgent: peerID,
		CommandID:   1,
		Timestamp:   uint64(time.Now().Unix()),
		Signature:   [64]byte{0xFF, 0xDE, 0xAD, 0xBE, 0xEF}, // Invalid signature
		SeenBy:      []identity.AgentID{},
	}

	accepted := f.HandleSleepCommand(peerID, cmd)
	if accepted {
		t.Error("Invalid signature should be rejected")
	}
}

func TestVerifySleepCommand_MissingSignature_Rejects(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peerID, _ := identity.NewAgentID()
	routeMgr := routing.NewManager(localID)
	sender := newMockPeerSender()

	// Generate signing keypair
	kp, err := crypto.GenerateSigningKeypair()
	if err != nil {
		t.Fatalf("GenerateSigningKeypair() error = %v", err)
	}

	cfg := DefaultFloodConfig()
	cfg.SigningPublicKey = &kp.PublicKey

	f := NewFlooder(cfg, localID, routeMgr, sender)
	defer f.Stop()

	// Unsigned command (zero signature) when key is configured
	cmd := &protocol.SleepCommand{
		OriginAgent: peerID,
		CommandID:   1,
		Timestamp:   uint64(time.Now().Unix()),
		Signature:   [64]byte{}, // Zero = unsigned
		SeenBy:      []identity.AgentID{},
	}

	accepted := f.HandleSleepCommand(peerID, cmd)
	if accepted {
		t.Error("Unsigned command should be rejected when signing key is configured")
	}
}

func TestVerifySleepCommand_ExpiredTimestamp_Rejects(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peerID, _ := identity.NewAgentID()
	routeMgr := routing.NewManager(localID)
	sender := newMockPeerSender()

	// Generate signing keypair
	kp, err := crypto.GenerateSigningKeypair()
	if err != nil {
		t.Fatalf("GenerateSigningKeypair() error = %v", err)
	}

	cfg := DefaultFloodConfig()
	cfg.SigningPublicKey = &kp.PublicKey
	cfg.TimestampWindow = 5 * time.Minute

	f := NewFlooder(cfg, localID, routeMgr, sender)
	defer f.Stop()

	// Command with old timestamp (10 minutes ago)
	cmd := &protocol.SleepCommand{
		OriginAgent: peerID,
		CommandID:   1,
		Timestamp:   uint64(time.Now().Add(-10 * time.Minute).Unix()),
		SeenBy:      []identity.AgentID{},
	}
	cmd.Signature = crypto.Sign(kp.PrivateKey, cmd.SignableBytes())

	accepted := f.HandleSleepCommand(peerID, cmd)
	if accepted {
		t.Error("Expired timestamp should be rejected")
	}
}

func TestVerifySleepCommand_FutureTimestamp_Rejects(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peerID, _ := identity.NewAgentID()
	routeMgr := routing.NewManager(localID)
	sender := newMockPeerSender()

	// Generate signing keypair
	kp, err := crypto.GenerateSigningKeypair()
	if err != nil {
		t.Fatalf("GenerateSigningKeypair() error = %v", err)
	}

	cfg := DefaultFloodConfig()
	cfg.SigningPublicKey = &kp.PublicKey
	cfg.TimestampWindow = 5 * time.Minute

	f := NewFlooder(cfg, localID, routeMgr, sender)
	defer f.Stop()

	// Command with future timestamp (10 minutes ahead)
	cmd := &protocol.SleepCommand{
		OriginAgent: peerID,
		CommandID:   1,
		Timestamp:   uint64(time.Now().Add(10 * time.Minute).Unix()),
		SeenBy:      []identity.AgentID{},
	}
	cmd.Signature = crypto.Sign(kp.PrivateKey, cmd.SignableBytes())

	accepted := f.HandleSleepCommand(peerID, cmd)
	if accepted {
		t.Error("Future timestamp should be rejected")
	}
}

func TestVerifyWakeCommand_NoKeyConfigured_AcceptsAll(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peerID, _ := identity.NewAgentID()
	routeMgr := routing.NewManager(localID)
	sender := newMockPeerSender()

	// No signing key configured
	cfg := DefaultFloodConfig()

	f := NewFlooder(cfg, localID, routeMgr, sender)
	defer f.Stop()

	// Unsigned command should be accepted
	cmd := &protocol.WakeCommand{
		OriginAgent: peerID,
		CommandID:   1,
		Timestamp:   uint64(time.Now().Unix()),
		Signature:   [64]byte{}, // Zero signature (unsigned)
		SeenBy:      []identity.AgentID{},
	}

	accepted := f.HandleWakeCommand(peerID, cmd)
	if !accepted {
		t.Error("Unsigned wake command should be accepted when no signing key configured")
	}
}

func TestVerifyWakeCommand_ValidSignature_Accepts(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peerID, _ := identity.NewAgentID()
	routeMgr := routing.NewManager(localID)
	sender := newMockPeerSender()

	// Generate signing keypair
	kp, err := crypto.GenerateSigningKeypair()
	if err != nil {
		t.Fatalf("GenerateSigningKeypair() error = %v", err)
	}

	cfg := DefaultFloodConfig()
	cfg.SigningPublicKey = &kp.PublicKey

	f := NewFlooder(cfg, localID, routeMgr, sender)
	defer f.Stop()

	// Create and sign command
	cmd := &protocol.WakeCommand{
		OriginAgent: peerID,
		CommandID:   1,
		Timestamp:   uint64(time.Now().Unix()),
		SeenBy:      []identity.AgentID{},
	}
	cmd.Signature = crypto.Sign(kp.PrivateKey, cmd.SignableBytes())

	accepted := f.HandleWakeCommand(peerID, cmd)
	if !accepted {
		t.Error("Validly signed wake command should be accepted")
	}
}

func TestVerifyWakeCommand_InvalidSignature_Rejects(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peerID, _ := identity.NewAgentID()
	routeMgr := routing.NewManager(localID)
	sender := newMockPeerSender()

	// Generate signing keypair
	kp, err := crypto.GenerateSigningKeypair()
	if err != nil {
		t.Fatalf("GenerateSigningKeypair() error = %v", err)
	}

	cfg := DefaultFloodConfig()
	cfg.SigningPublicKey = &kp.PublicKey

	f := NewFlooder(cfg, localID, routeMgr, sender)
	defer f.Stop()

	// Create command with garbage signature
	cmd := &protocol.WakeCommand{
		OriginAgent: peerID,
		CommandID:   1,
		Timestamp:   uint64(time.Now().Unix()),
		Signature:   [64]byte{0xCA, 0xFE, 0xBA, 0xBE}, // Invalid signature
		SeenBy:      []identity.AgentID{},
	}

	accepted := f.HandleWakeCommand(peerID, cmd)
	if accepted {
		t.Error("Invalid signature should be rejected")
	}
}

func TestVerifyWakeCommand_MissingSignature_Rejects(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peerID, _ := identity.NewAgentID()
	routeMgr := routing.NewManager(localID)
	sender := newMockPeerSender()

	// Generate signing keypair
	kp, err := crypto.GenerateSigningKeypair()
	if err != nil {
		t.Fatalf("GenerateSigningKeypair() error = %v", err)
	}

	cfg := DefaultFloodConfig()
	cfg.SigningPublicKey = &kp.PublicKey

	f := NewFlooder(cfg, localID, routeMgr, sender)
	defer f.Stop()

	// Unsigned command (zero signature) when key is configured
	cmd := &protocol.WakeCommand{
		OriginAgent: peerID,
		CommandID:   1,
		Timestamp:   uint64(time.Now().Unix()),
		Signature:   [64]byte{}, // Zero = unsigned
		SeenBy:      []identity.AgentID{},
	}

	accepted := f.HandleWakeCommand(peerID, cmd)
	if accepted {
		t.Error("Unsigned wake command should be rejected when signing key is configured")
	}
}

func TestVerifyWakeCommand_ExpiredTimestamp_Rejects(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peerID, _ := identity.NewAgentID()
	routeMgr := routing.NewManager(localID)
	sender := newMockPeerSender()

	// Generate signing keypair
	kp, err := crypto.GenerateSigningKeypair()
	if err != nil {
		t.Fatalf("GenerateSigningKeypair() error = %v", err)
	}

	cfg := DefaultFloodConfig()
	cfg.SigningPublicKey = &kp.PublicKey
	cfg.TimestampWindow = 5 * time.Minute

	f := NewFlooder(cfg, localID, routeMgr, sender)
	defer f.Stop()

	// Command with old timestamp (10 minutes ago)
	cmd := &protocol.WakeCommand{
		OriginAgent: peerID,
		CommandID:   1,
		Timestamp:   uint64(time.Now().Add(-10 * time.Minute).Unix()),
		SeenBy:      []identity.AgentID{},
	}
	cmd.Signature = crypto.Sign(kp.PrivateKey, cmd.SignableBytes())

	accepted := f.HandleWakeCommand(peerID, cmd)
	if accepted {
		t.Error("Wake command with expired timestamp should be rejected")
	}
}

func TestVerifyWakeCommand_FutureTimestamp_Rejects(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peerID, _ := identity.NewAgentID()
	routeMgr := routing.NewManager(localID)
	sender := newMockPeerSender()

	// Generate signing keypair
	kp, err := crypto.GenerateSigningKeypair()
	if err != nil {
		t.Fatalf("GenerateSigningKeypair() error = %v", err)
	}

	cfg := DefaultFloodConfig()
	cfg.SigningPublicKey = &kp.PublicKey
	cfg.TimestampWindow = 5 * time.Minute

	f := NewFlooder(cfg, localID, routeMgr, sender)
	defer f.Stop()

	// Command with future timestamp (10 minutes ahead)
	cmd := &protocol.WakeCommand{
		OriginAgent: peerID,
		CommandID:   1,
		Timestamp:   uint64(time.Now().Add(10 * time.Minute).Unix()),
		SeenBy:      []identity.AgentID{},
	}
	cmd.Signature = crypto.Sign(kp.PrivateKey, cmd.SignableBytes())

	accepted := f.HandleWakeCommand(peerID, cmd)
	if accepted {
		t.Error("Wake command with future timestamp should be rejected")
	}
}

func TestHandleSleepCommand_SignedCommand_Floods(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peer1, _ := identity.NewAgentID()
	peer2, _ := identity.NewAgentID()
	routeMgr := routing.NewManager(localID)
	sender := newMockPeerSender()
	sender.AddPeer(peer1)
	sender.AddPeer(peer2)

	// Generate signing keypair
	kp, err := crypto.GenerateSigningKeypair()
	if err != nil {
		t.Fatalf("GenerateSigningKeypair() error = %v", err)
	}

	cfg := DefaultFloodConfig()
	cfg.SigningPublicKey = &kp.PublicKey

	f := NewFlooder(cfg, localID, routeMgr, sender)
	defer f.Stop()

	// Create signed command
	cmd := &protocol.SleepCommand{
		OriginAgent: peer1,
		CommandID:   1,
		Timestamp:   uint64(time.Now().Unix()),
		SeenBy:      []identity.AgentID{},
	}
	cmd.Signature = crypto.Sign(kp.PrivateKey, cmd.SignableBytes())

	// Handle from peer1
	accepted := f.HandleSleepCommand(peer1, cmd)
	if !accepted {
		t.Error("Signed command should be accepted")
	}

	// Should flood to peer2 (not back to peer1)
	msgs := sender.GetMessages(peer2)
	if len(msgs) != 1 {
		t.Errorf("Should flood to peer2, got %d messages", len(msgs))
	}
	if len(sender.GetMessages(peer1)) != 0 {
		t.Error("Should not flood back to source peer")
	}
}

func TestHandleSleepCommand_UnsignedCommand_NoKey_Floods(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peer1, _ := identity.NewAgentID()
	peer2, _ := identity.NewAgentID()
	routeMgr := routing.NewManager(localID)
	sender := newMockPeerSender()
	sender.AddPeer(peer1)
	sender.AddPeer(peer2)

	// No signing key configured (backward compatible mode)
	cfg := DefaultFloodConfig()

	f := NewFlooder(cfg, localID, routeMgr, sender)
	defer f.Stop()

	// Unsigned command
	cmd := &protocol.SleepCommand{
		OriginAgent: peer1,
		CommandID:   1,
		Timestamp:   uint64(time.Now().Unix()),
		Signature:   [64]byte{}, // Zero = unsigned
		SeenBy:      []identity.AgentID{},
	}

	// Handle from peer1
	accepted := f.HandleSleepCommand(peer1, cmd)
	if !accepted {
		t.Error("Unsigned command should be accepted when no key configured")
	}

	// Should flood to peer2
	if len(sender.GetMessages(peer2)) != 1 {
		t.Error("Should flood unsigned command when no key configured")
	}
}

func TestHandleSleepCommand_UnsignedCommand_WithKey_Rejected(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peer1, _ := identity.NewAgentID()
	peer2, _ := identity.NewAgentID()
	routeMgr := routing.NewManager(localID)
	sender := newMockPeerSender()
	sender.AddPeer(peer1)
	sender.AddPeer(peer2)

	// Generate signing keypair
	kp, err := crypto.GenerateSigningKeypair()
	if err != nil {
		t.Fatalf("GenerateSigningKeypair() error = %v", err)
	}

	cfg := DefaultFloodConfig()
	cfg.SigningPublicKey = &kp.PublicKey

	f := NewFlooder(cfg, localID, routeMgr, sender)
	defer f.Stop()

	// Unsigned command
	cmd := &protocol.SleepCommand{
		OriginAgent: peer1,
		CommandID:   1,
		Timestamp:   uint64(time.Now().Unix()),
		Signature:   [64]byte{}, // Zero = unsigned
		SeenBy:      []identity.AgentID{},
	}

	// Handle from peer1
	accepted := f.HandleSleepCommand(peer1, cmd)
	if accepted {
		t.Error("Unsigned command should be rejected when key is configured")
	}

	// Should NOT flood
	if sender.TotalMessages() != 0 {
		t.Error("Should not flood rejected command")
	}
}

func TestHandleWakeCommand_SignedCommand_Floods(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peer1, _ := identity.NewAgentID()
	peer2, _ := identity.NewAgentID()
	routeMgr := routing.NewManager(localID)
	sender := newMockPeerSender()
	sender.AddPeer(peer1)
	sender.AddPeer(peer2)

	// Generate signing keypair
	kp, err := crypto.GenerateSigningKeypair()
	if err != nil {
		t.Fatalf("GenerateSigningKeypair() error = %v", err)
	}

	cfg := DefaultFloodConfig()
	cfg.SigningPublicKey = &kp.PublicKey

	f := NewFlooder(cfg, localID, routeMgr, sender)
	defer f.Stop()

	// Create signed command
	cmd := &protocol.WakeCommand{
		OriginAgent: peer1,
		CommandID:   1,
		Timestamp:   uint64(time.Now().Unix()),
		SeenBy:      []identity.AgentID{},
	}
	cmd.Signature = crypto.Sign(kp.PrivateKey, cmd.SignableBytes())

	// Handle from peer1
	accepted := f.HandleWakeCommand(peer1, cmd)
	if !accepted {
		t.Error("Signed wake command should be accepted")
	}

	// Should flood to peer2 (not back to peer1)
	msgs := sender.GetMessages(peer2)
	if len(msgs) != 1 {
		t.Errorf("Should flood wake command to peer2, got %d messages", len(msgs))
	}
}

func TestHandleSleepCommand_Deduplication(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peerID, _ := identity.NewAgentID()
	routeMgr := routing.NewManager(localID)
	sender := newMockPeerSender()

	cfg := DefaultFloodConfig()

	f := NewFlooder(cfg, localID, routeMgr, sender)
	defer f.Stop()

	cmd := &protocol.SleepCommand{
		OriginAgent: peerID,
		CommandID:   12345,
		Timestamp:   uint64(time.Now().Unix()),
		Signature:   [64]byte{},
		SeenBy:      []identity.AgentID{},
	}

	// First time should accept
	if !f.HandleSleepCommand(peerID, cmd) {
		t.Error("First sleep command should be accepted")
	}

	// Second time should reject (duplicate)
	if f.HandleSleepCommand(peerID, cmd) {
		t.Error("Duplicate sleep command should be rejected")
	}
}

func TestHandleWakeCommand_Deduplication(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peerID, _ := identity.NewAgentID()
	routeMgr := routing.NewManager(localID)
	sender := newMockPeerSender()

	cfg := DefaultFloodConfig()

	f := NewFlooder(cfg, localID, routeMgr, sender)
	defer f.Stop()

	cmd := &protocol.WakeCommand{
		OriginAgent: peerID,
		CommandID:   54321,
		Timestamp:   uint64(time.Now().Unix()),
		Signature:   [64]byte{},
		SeenBy:      []identity.AgentID{},
	}

	// First time should accept
	if !f.HandleWakeCommand(peerID, cmd) {
		t.Error("First wake command should be accepted")
	}

	// Second time should reject (duplicate)
	if f.HandleWakeCommand(peerID, cmd) {
		t.Error("Duplicate wake command should be rejected")
	}
}

func TestHandleSleepCommand_LoopDetection(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peerID, _ := identity.NewAgentID()
	routeMgr := routing.NewManager(localID)
	sender := newMockPeerSender()

	cfg := DefaultFloodConfig()

	f := NewFlooder(cfg, localID, routeMgr, sender)
	defer f.Stop()

	// Command with our ID in SeenBy (loop)
	cmd := &protocol.SleepCommand{
		OriginAgent: peerID,
		CommandID:   1,
		Timestamp:   uint64(time.Now().Unix()),
		Signature:   [64]byte{},
		SeenBy:      []identity.AgentID{localID}, // Loop!
	}

	accepted := f.HandleSleepCommand(peerID, cmd)
	if accepted {
		t.Error("Sleep command with our ID in SeenBy should be rejected (loop)")
	}
}

func TestHandleWakeCommand_LoopDetection(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peerID, _ := identity.NewAgentID()
	routeMgr := routing.NewManager(localID)
	sender := newMockPeerSender()

	cfg := DefaultFloodConfig()

	f := NewFlooder(cfg, localID, routeMgr, sender)
	defer f.Stop()

	// Command with our ID in SeenBy (loop)
	cmd := &protocol.WakeCommand{
		OriginAgent: peerID,
		CommandID:   1,
		Timestamp:   uint64(time.Now().Unix()),
		Signature:   [64]byte{},
		SeenBy:      []identity.AgentID{localID}, // Loop!
	}

	accepted := f.HandleWakeCommand(peerID, cmd)
	if accepted {
		t.Error("Wake command with our ID in SeenBy should be rejected (loop)")
	}
}

func TestSleepCommandSeenCacheSize(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peerID, _ := identity.NewAgentID()
	routeMgr := routing.NewManager(localID)
	sender := newMockPeerSender()

	cfg := DefaultFloodConfig()

	f := NewFlooder(cfg, localID, routeMgr, sender)
	defer f.Stop()

	// Initially empty
	if f.SleepCommandSeenCacheSize() != 0 {
		t.Errorf("Initial SleepCommandSeenCacheSize = %d, want 0", f.SleepCommandSeenCacheSize())
	}

	// Add some commands
	for i := uint64(1); i <= 5; i++ {
		cmd := &protocol.SleepCommand{
			OriginAgent: peerID,
			CommandID:   i,
			Timestamp:   uint64(time.Now().Unix()),
			Signature:   [64]byte{},
			SeenBy:      []identity.AgentID{},
		}
		f.HandleSleepCommand(peerID, cmd)
	}

	if f.SleepCommandSeenCacheSize() != 5 {
		t.Errorf("SleepCommandSeenCacheSize = %d, want 5", f.SleepCommandSeenCacheSize())
	}
}

func TestFlooder_AnnounceLocalRoutes_AlwaysIncludesAgentPresence(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peer1, _ := identity.NewAgentID()

	routeMgr := routing.NewManager(localID)
	sender := newMockPeerSender()
	sender.AddPeer(peer1)

	cfg := DefaultFloodConfig()
	f := NewFlooder(cfg, localID, routeMgr, sender)
	defer f.Stop()

	// No exit routes configured - AnnounceLocalRoutes should still send
	f.AnnounceLocalRoutes()

	msgs := sender.GetMessages(peer1)
	if len(msgs) != 1 {
		t.Fatalf("Expected 1 message to peer1, got %d", len(msgs))
	}

	// Decode the advertisement
	adv, err := protocol.DecodeRouteAdvertise(msgs[0].Payload)
	if err != nil {
		t.Fatalf("DecodeRouteAdvertise() error = %v", err)
	}

	// Should have exactly 1 route: the agent presence route
	if len(adv.Routes) != 1 {
		t.Fatalf("Expected 1 route, got %d", len(adv.Routes))
	}

	if adv.Routes[0].AddressFamily != protocol.AddrFamilyAgent {
		t.Errorf("Route AddressFamily = %d, want %d (AddrFamilyAgent)", adv.Routes[0].AddressFamily, protocol.AddrFamilyAgent)
	}

	decodedID := protocol.DecodeAgentPrefix(adv.Routes[0].Prefix)
	if decodedID != localID {
		t.Errorf("Agent presence route ID = %s, want %s", decodedID.String(), localID.String())
	}
}

func TestFlooder_HandleRouteAdvertise_AgentPresenceRoute(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peer1, _ := identity.NewAgentID()
	remoteAgent, _ := identity.NewAgentID()

	routeMgr := routing.NewManager(localID)
	sender := newMockPeerSender()
	sender.AddPeer(peer1)

	cfg := DefaultFloodConfig()
	f := NewFlooder(cfg, localID, routeMgr, sender)
	defer f.Stop()

	// Simulate receiving a route advertisement with agent presence route
	path := protocol.EncodePath([]identity.AgentID{peer1, remoteAgent})
	encPath := &protocol.EncryptedData{
		Encrypted: false,
		Data:      path,
	}

	routes := []protocol.Route{
		{
			AddressFamily: protocol.AddrFamilyAgent,
			PrefixLength:  0,
			Prefix:        protocol.EncodeAgentPrefix(remoteAgent),
			Metric:        0,
		},
	}

	handled := f.HandleRouteAdvertise(peer1, remoteAgent, "", 1, routes, encPath, []identity.AgentID{remoteAgent})
	if !handled {
		t.Error("HandleRouteAdvertise should return true for new advertisement")
	}

	// Verify agent is now in the agent table
	agentRoute := routeMgr.LookupAgent(remoteAgent)
	if agentRoute == nil {
		t.Fatal("LookupAgent should find the remote agent after advertisement")
	}
	if agentRoute.NextHop != peer1 {
		t.Errorf("NextHop = %s, want %s", agentRoute.NextHop.ShortString(), peer1.ShortString())
	}
}

func TestFlooder_SendFullTable_IncludesAgentPresenceRoutes(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peer1, _ := identity.NewAgentID()
	peer2, _ := identity.NewAgentID()
	remoteAgent, _ := identity.NewAgentID()

	routeMgr := routing.NewManager(localID)
	sender := newMockPeerSender()
	sender.AddPeer(peer1)
	sender.AddPeer(peer2)

	cfg := DefaultFloodConfig()
	f := NewFlooder(cfg, localID, routeMgr, sender)
	defer f.Stop()

	// Add an agent route learned from peer1
	routeMgr.ProcessAgentRouteAdvertise(
		peer1, remoteAgent, 1, remoteAgent,
		[]identity.AgentID{peer1, remoteAgent}, nil, 1,
	)

	// Send full table to peer2
	f.SendFullTable(peer2)

	msgs := sender.GetMessages(peer2)
	if len(msgs) == 0 {
		t.Fatal("Expected at least 1 message to peer2")
	}

	// Find the agent presence route in sent messages
	found := false
	for _, msg := range msgs {
		adv, err := protocol.DecodeRouteAdvertise(msg.Payload)
		if err != nil {
			continue
		}
		for _, r := range adv.Routes {
			if r.AddressFamily == protocol.AddrFamilyAgent {
				decodedID := protocol.DecodeAgentPrefix(r.Prefix)
				if decodedID == remoteAgent {
					found = true
				}
			}
		}
	}

	if !found {
		t.Error("SendFullTable should include agent presence route for remote agent")
	}
}
