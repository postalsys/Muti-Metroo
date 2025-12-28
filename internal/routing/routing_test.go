package routing

import (
	"net"
	"testing"

	"github.com/coinstash/muti-metroo/internal/identity"
)

// ============================================================================
// Route Tests
// ============================================================================

func TestRoute_String(t *testing.T) {
	agentID, _ := identity.NewAgentID()
	route := &Route{
		Network:     MustParseCIDR("10.0.0.0/8"),
		NextHop:     agentID,
		OriginAgent: agentID,
		Metric:      10,
	}

	str := route.String()
	if str == "" {
		t.Error("String() should not be empty")
	}
}

func TestRoute_Clone(t *testing.T) {
	agentID, _ := identity.NewAgentID()
	original := &Route{
		Network:     MustParseCIDR("10.0.0.0/8"),
		NextHop:     agentID,
		OriginAgent: agentID,
		Metric:      10,
		Path:        []identity.AgentID{agentID},
		Sequence:    42,
	}

	clone := original.Clone()

	// Verify values are equal
	if clone.Network.String() != original.Network.String() {
		t.Error("Network should be equal")
	}
	if clone.NextHop != original.NextHop {
		t.Error("NextHop should be equal")
	}
	if clone.Metric != original.Metric {
		t.Errorf("Metric = %d, want %d", clone.Metric, original.Metric)
	}

	// Verify it's a deep copy
	clone.Metric = 999
	if original.Metric == 999 {
		t.Error("Clone should not affect original")
	}
}

// ============================================================================
// Table Tests
// ============================================================================

func TestNewTable(t *testing.T) {
	localID, _ := identity.NewAgentID()
	table := NewTable(localID)

	if table == nil {
		t.Fatal("NewTable returned nil")
	}
	if table.Size() != 0 {
		t.Errorf("Initial Size = %d, want 0", table.Size())
	}
}

func TestTable_AddRoute(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peerID, _ := identity.NewAgentID()
	table := NewTable(localID)

	route := &Route{
		Network:     MustParseCIDR("10.0.0.0/8"),
		NextHop:     peerID,
		OriginAgent: peerID,
		Metric:      10,
		Sequence:    1,
	}

	if !table.AddRoute(route) {
		t.Error("AddRoute should return true for new route")
	}
	if table.Size() != 1 {
		t.Errorf("Size = %d, want 1", table.Size())
	}
}

func TestTable_AddRoute_NilRoute(t *testing.T) {
	localID, _ := identity.NewAgentID()
	table := NewTable(localID)

	if table.AddRoute(nil) {
		t.Error("AddRoute(nil) should return false")
	}
}

func TestTable_AddRoute_LoopDetection(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peerID, _ := identity.NewAgentID()
	table := NewTable(localID)

	// Route with our ID in the path (loop)
	route := &Route{
		Network:     MustParseCIDR("10.0.0.0/8"),
		NextHop:     peerID,
		OriginAgent: peerID,
		Metric:      10,
		Path:        []identity.AgentID{peerID, localID}, // Our ID in path!
		Sequence:    1,
	}

	if table.AddRoute(route) {
		t.Error("AddRoute should reject route with loop")
	}
}

func TestTable_AddRoute_UpdateBetterMetric(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peerID, _ := identity.NewAgentID()
	table := NewTable(localID)

	// Add initial route
	route1 := &Route{
		Network:     MustParseCIDR("10.0.0.0/8"),
		NextHop:     peerID,
		OriginAgent: peerID,
		Metric:      10,
		Sequence:    1,
	}
	table.AddRoute(route1)

	// Add better route from same origin
	route2 := &Route{
		Network:     MustParseCIDR("10.0.0.0/8"),
		NextHop:     peerID,
		OriginAgent: peerID,
		Metric:      5, // Better metric
		Sequence:    1,
	}

	if !table.AddRoute(route2) {
		t.Error("AddRoute should accept route with better metric")
	}

	// Verify the metric was updated
	result := table.GetRoute(MustParseCIDR("10.0.0.0/8"))
	if result.Metric != 5 {
		t.Errorf("Metric = %d, want 5", result.Metric)
	}
}

func TestTable_AddRoute_UpdateNewerSequence(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peerID, _ := identity.NewAgentID()
	table := NewTable(localID)

	// Add initial route
	route1 := &Route{
		Network:     MustParseCIDR("10.0.0.0/8"),
		NextHop:     peerID,
		OriginAgent: peerID,
		Metric:      10,
		Sequence:    1,
	}
	table.AddRoute(route1)

	// Add route with newer sequence (even with worse metric)
	route2 := &Route{
		Network:     MustParseCIDR("10.0.0.0/8"),
		NextHop:     peerID,
		OriginAgent: peerID,
		Metric:      20, // Worse metric
		Sequence:    2,  // But newer sequence
	}

	if !table.AddRoute(route2) {
		t.Error("AddRoute should accept route with newer sequence")
	}

	result := table.GetRoute(MustParseCIDR("10.0.0.0/8"))
	if result.Sequence != 2 {
		t.Errorf("Sequence = %d, want 2", result.Sequence)
	}
}

func TestTable_AddRoute_MultipleOrigins(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peer1, _ := identity.NewAgentID()
	peer2, _ := identity.NewAgentID()
	table := NewTable(localID)

	// Add route from peer1
	route1 := &Route{
		Network:     MustParseCIDR("10.0.0.0/8"),
		NextHop:     peer1,
		OriginAgent: peer1,
		Metric:      10,
		Sequence:    1,
	}
	table.AddRoute(route1)

	// Add same network from peer2
	route2 := &Route{
		Network:     MustParseCIDR("10.0.0.0/8"),
		NextHop:     peer2,
		OriginAgent: peer2,
		Metric:      5, // Better metric
		Sequence:    1,
	}
	table.AddRoute(route2)

	// Should have 2 route entries for this prefix
	if table.TotalRoutes() != 2 {
		t.Errorf("TotalRoutes = %d, want 2", table.TotalRoutes())
	}

	// Best route should be from peer2 (lower metric)
	best := table.GetRoute(MustParseCIDR("10.0.0.0/8"))
	if best.NextHop != peer2 {
		t.Error("Best route should be from peer2")
	}
}

func TestTable_RemoveRoute(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peerID, _ := identity.NewAgentID()
	table := NewTable(localID)

	network := MustParseCIDR("10.0.0.0/8")
	route := &Route{
		Network:     network,
		NextHop:     peerID,
		OriginAgent: peerID,
		Metric:      10,
		Sequence:    1,
	}
	table.AddRoute(route)

	if !table.RemoveRoute(network, peerID) {
		t.Error("RemoveRoute should return true")
	}
	if table.Size() != 0 {
		t.Errorf("Size after remove = %d, want 0", table.Size())
	}
}

func TestTable_RemoveRoute_NotFound(t *testing.T) {
	localID, _ := identity.NewAgentID()
	unknownID, _ := identity.NewAgentID()
	table := NewTable(localID)

	if table.RemoveRoute(MustParseCIDR("10.0.0.0/8"), unknownID) {
		t.Error("RemoveRoute should return false for non-existent route")
	}
}

func TestTable_RemoveRoutesFromPeer(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peer1, _ := identity.NewAgentID()
	peer2, _ := identity.NewAgentID()
	table := NewTable(localID)

	// Add routes from peer1
	table.AddRoute(&Route{
		Network:     MustParseCIDR("10.0.0.0/8"),
		NextHop:     peer1,
		OriginAgent: peer1,
		Metric:      10,
	})
	table.AddRoute(&Route{
		Network:     MustParseCIDR("172.16.0.0/12"),
		NextHop:     peer1,
		OriginAgent: peer1,
		Metric:      10,
	})

	// Add route from peer2
	table.AddRoute(&Route{
		Network:     MustParseCIDR("192.168.0.0/16"),
		NextHop:     peer2,
		OriginAgent: peer2,
		Metric:      10,
	})

	// Remove all routes from peer1
	count := table.RemoveRoutesFromPeer(peer1)
	if count != 2 {
		t.Errorf("RemoveRoutesFromPeer removed %d routes, want 2", count)
	}
	if table.Size() != 1 {
		t.Errorf("Size = %d, want 1", table.Size())
	}
}

// ============================================================================
// Lookup Tests (LPM)
// ============================================================================

func TestTable_Lookup_ExactMatch(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peerID, _ := identity.NewAgentID()
	table := NewTable(localID)

	table.AddRoute(&Route{
		Network:     MustParseCIDR("10.1.2.0/24"),
		NextHop:     peerID,
		OriginAgent: peerID,
		Metric:      10,
	})

	result := table.Lookup(net.ParseIP("10.1.2.100"))
	if result == nil {
		t.Fatal("Lookup should find route")
	}
	if result.Network.String() != "10.1.2.0/24" {
		t.Errorf("Network = %s, want 10.1.2.0/24", result.Network.String())
	}
}

func TestTable_Lookup_LongestPrefixMatch(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peer1, _ := identity.NewAgentID()
	peer2, _ := identity.NewAgentID()
	table := NewTable(localID)

	// Add /8 route
	table.AddRoute(&Route{
		Network:     MustParseCIDR("10.0.0.0/8"),
		NextHop:     peer1,
		OriginAgent: peer1,
		Metric:      10,
	})

	// Add more specific /24 route
	table.AddRoute(&Route{
		Network:     MustParseCIDR("10.1.2.0/24"),
		NextHop:     peer2,
		OriginAgent: peer2,
		Metric:      10,
	})

	// Should match /24 (longest prefix)
	result := table.Lookup(net.ParseIP("10.1.2.100"))
	if result == nil {
		t.Fatal("Lookup should find route")
	}
	if result.NextHop != peer2 {
		t.Error("Should match more specific /24 route")
	}

	// Should match /8 for other addresses
	result2 := table.Lookup(net.ParseIP("10.5.5.5"))
	if result2 == nil {
		t.Fatal("Lookup should find route")
	}
	if result2.NextHop != peer1 {
		t.Error("Should match /8 route")
	}
}

func TestTable_Lookup_NoMatch(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peerID, _ := identity.NewAgentID()
	table := NewTable(localID)

	table.AddRoute(&Route{
		Network:     MustParseCIDR("10.0.0.0/8"),
		NextHop:     peerID,
		OriginAgent: peerID,
		Metric:      10,
	})

	result := table.Lookup(net.ParseIP("192.168.1.1"))
	if result != nil {
		t.Error("Lookup should return nil for non-matching IP")
	}
}

func TestTable_Lookup_DefaultRoute(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peerID, _ := identity.NewAgentID()
	table := NewTable(localID)

	// Add default route
	table.AddRoute(&Route{
		Network:     MustParseCIDR("0.0.0.0/0"),
		NextHop:     peerID,
		OriginAgent: peerID,
		Metric:      10,
	})

	// Any IP should match
	result := table.Lookup(net.ParseIP("8.8.8.8"))
	if result == nil {
		t.Fatal("Default route should match any IP")
	}
}

func TestTable_LookupAll(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peer1, _ := identity.NewAgentID()
	peer2, _ := identity.NewAgentID()
	table := NewTable(localID)

	table.AddRoute(&Route{
		Network:     MustParseCIDR("10.0.0.0/8"),
		NextHop:     peer1,
		OriginAgent: peer1,
		Metric:      10,
	})
	table.AddRoute(&Route{
		Network:     MustParseCIDR("10.1.0.0/16"),
		NextHop:     peer2,
		OriginAgent: peer2,
		Metric:      5,
	})

	results := table.LookupAll(net.ParseIP("10.1.2.3"))
	if len(results) != 2 {
		t.Errorf("LookupAll returned %d results, want 2", len(results))
	}

	// First should be /16 (longest prefix)
	if results[0].Network.String() != "10.1.0.0/16" {
		t.Error("First result should be /16")
	}
}

// ============================================================================
// Manager Tests
// ============================================================================

func TestNewManager(t *testing.T) {
	localID, _ := identity.NewAgentID()
	mgr := NewManager(localID)

	if mgr == nil {
		t.Fatal("NewManager returned nil")
	}
	if mgr.Size() != 0 {
		t.Errorf("Initial Size = %d, want 0", mgr.Size())
	}
}

func TestManager_AddLocalRoute(t *testing.T) {
	localID, _ := identity.NewAgentID()
	mgr := NewManager(localID)

	network := MustParseCIDR("10.0.0.0/8")
	if !mgr.AddLocalRoute(network, 10) {
		t.Error("AddLocalRoute should return true")
	}

	routes := mgr.GetLocalRoutes()
	if len(routes) != 1 {
		t.Errorf("GetLocalRoutes = %d routes, want 1", len(routes))
	}
}

func TestManager_RemoveLocalRoute(t *testing.T) {
	localID, _ := identity.NewAgentID()
	mgr := NewManager(localID)

	network := MustParseCIDR("10.0.0.0/8")
	mgr.AddLocalRoute(network, 10)

	if !mgr.RemoveLocalRoute(network) {
		t.Error("RemoveLocalRoute should return true")
	}

	routes := mgr.GetLocalRoutes()
	if len(routes) != 0 {
		t.Errorf("GetLocalRoutes after remove = %d, want 0", len(routes))
	}
}

func TestManager_ProcessRouteAdvertise(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peerID, _ := identity.NewAgentID()
	mgr := NewManager(localID)

	entries := []RouteEntry{
		{Network: MustParseCIDR("10.0.0.0/8"), Metric: 10},
		{Network: MustParseCIDR("172.16.0.0/12"), Metric: 5},
	}

	accepted := mgr.ProcessRouteAdvertise(peerID, peerID, 1, entries, nil)
	if len(accepted) != 2 {
		t.Errorf("ProcessRouteAdvertise accepted %d routes, want 2", len(accepted))
	}

	// Verify routes were added with incremented metric
	route := mgr.Lookup(net.ParseIP("10.0.0.1"))
	if route == nil {
		t.Fatal("Route should exist")
	}
	if route.Metric != 11 { // Original 10 + 1
		t.Errorf("Metric = %d, want 11", route.Metric)
	}
}

func TestManager_ProcessRouteWithdraw(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peerID, _ := identity.NewAgentID()
	mgr := NewManager(localID)

	// Add routes
	entries := []RouteEntry{
		{Network: MustParseCIDR("10.0.0.0/8"), Metric: 10},
	}
	mgr.ProcessRouteAdvertise(peerID, peerID, 1, entries, nil)

	// Withdraw
	removed := mgr.ProcessRouteWithdraw(peerID, entries)
	if !removed {
		t.Error("ProcessRouteWithdraw should return true")
	}

	if mgr.Size() != 0 {
		t.Errorf("Size after withdraw = %d, want 0", mgr.Size())
	}
}

func TestManager_HandlePeerDisconnect(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peerID, _ := identity.NewAgentID()
	mgr := NewManager(localID)

	// Add routes from peer
	entries := []RouteEntry{
		{Network: MustParseCIDR("10.0.0.0/8"), Metric: 10},
		{Network: MustParseCIDR("172.16.0.0/12"), Metric: 5},
	}
	mgr.ProcessRouteAdvertise(peerID, peerID, 1, entries, nil)

	count := mgr.HandlePeerDisconnect(peerID)
	if count != 2 {
		t.Errorf("HandlePeerDisconnect removed %d routes, want 2", count)
	}
}

func TestManager_LookupNextHop(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peerID, _ := identity.NewAgentID()
	mgr := NewManager(localID)

	entries := []RouteEntry{
		{Network: MustParseCIDR("10.0.0.0/8"), Metric: 10},
	}
	mgr.ProcessRouteAdvertise(peerID, peerID, 1, entries, nil)

	nextHop, found := mgr.LookupNextHop(net.ParseIP("10.1.2.3"))
	if !found {
		t.Error("LookupNextHop should find route")
	}
	if nextHop != peerID {
		t.Error("NextHop should be peerID")
	}
}

func TestManager_LookupNextHop_NotFound(t *testing.T) {
	localID, _ := identity.NewAgentID()
	mgr := NewManager(localID)

	_, found := mgr.LookupNextHop(net.ParseIP("10.1.2.3"))
	if found {
		t.Error("LookupNextHop should not find route")
	}
}

func TestManager_Subscribe(t *testing.T) {
	localID, _ := identity.NewAgentID()
	mgr := NewManager(localID)

	ch := make(chan RouteChange, 10)
	mgr.Subscribe(ch)

	// Add a local route
	if !mgr.AddLocalRoute(MustParseCIDR("10.0.0.0/8"), 10) {
		t.Fatal("AddLocalRoute should return true")
	}

	// Should receive notification (allow a small delay)
	select {
	case change := <-ch:
		if change.Type != RouteAdded {
			t.Errorf("Change type = %v, want RouteAdded", change.Type)
		}
	default:
		t.Error("Should receive route change notification")
	}
}

func TestManager_GetRoutesToAdvertise(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peer1, _ := identity.NewAgentID()
	peer2, _ := identity.NewAgentID()
	mgr := NewManager(localID)

	// Add local route
	if !mgr.AddLocalRoute(MustParseCIDR("10.0.0.0/8"), 10) {
		t.Fatal("AddLocalRoute should return true")
	}

	// Add route from peer1
	mgr.ProcessRouteAdvertise(peer1, peer1, 1, []RouteEntry{
		{Network: MustParseCIDR("172.16.0.0/12"), Metric: 5},
	}, nil)

	// Get routes to advertise to peer1 (should exclude route learned from peer1)
	entries := mgr.GetRoutesToAdvertise(peer1)
	if len(entries) != 1 {
		t.Errorf("GetRoutesToAdvertise returned %d entries, want 1", len(entries))
		return
	}
	if entries[0].Network.String() != "10.0.0.0/8" {
		t.Error("Should only include local route, not route from peer1")
	}

	// Get routes to advertise to peer2 (should include both)
	entries2 := mgr.GetRoutesToAdvertise(peer2)
	if len(entries2) != 2 {
		t.Errorf("GetRoutesToAdvertise for peer2 returned %d entries, want 2", len(entries2))
	}
}

// ============================================================================
// CIDR Parse Tests
// ============================================================================

func TestParseCIDR(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
	}{
		{"10.0.0.0/8", false},
		{"192.168.1.0/24", false},
		{"0.0.0.0/0", false},
		{"2001:db8::/32", false},
		{"invalid", true},
		{"10.0.0.0", true},
		{"", true},
	}

	for _, tt := range tests {
		network, err := ParseCIDR(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ParseCIDR(%q) should return error", tt.input)
			}
		} else {
			if err != nil {
				t.Errorf("ParseCIDR(%q) error: %v", tt.input, err)
			}
			if network == nil {
				t.Errorf("ParseCIDR(%q) returned nil network", tt.input)
			}
		}
	}
}

func TestMustParseCIDR_Panic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustParseCIDR should panic on invalid CIDR")
		}
	}()
	MustParseCIDR("invalid")
}

// ============================================================================
// IPv6 Tests
// ============================================================================

func TestTable_Lookup_IPv6(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peerID, _ := identity.NewAgentID()
	table := NewTable(localID)

	table.AddRoute(&Route{
		Network:     MustParseCIDR("2001:db8::/32"),
		NextHop:     peerID,
		OriginAgent: peerID,
		Metric:      10,
	})

	result := table.Lookup(net.ParseIP("2001:db8::1"))
	if result == nil {
		t.Fatal("Lookup should find IPv6 route")
	}
}

// ============================================================================
// Table Utility Tests
// ============================================================================

func TestTable_Clear(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peerID, _ := identity.NewAgentID()
	table := NewTable(localID)

	table.AddRoute(&Route{
		Network:     MustParseCIDR("10.0.0.0/8"),
		NextHop:     peerID,
		OriginAgent: peerID,
		Metric:      10,
	})

	table.Clear()
	if table.Size() != 0 {
		t.Errorf("Size after Clear = %d, want 0", table.Size())
	}
}

func TestTable_HasRoute(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peerID, _ := identity.NewAgentID()
	table := NewTable(localID)

	network := MustParseCIDR("10.0.0.0/8")
	table.AddRoute(&Route{
		Network:     network,
		NextHop:     peerID,
		OriginAgent: peerID,
		Metric:      10,
	})

	if !table.HasRoute(network, peerID) {
		t.Error("HasRoute should return true")
	}

	unknownID, _ := identity.NewAgentID()
	if table.HasRoute(network, unknownID) {
		t.Error("HasRoute should return false for unknown origin")
	}
}

func TestTable_GetAllRoutes(t *testing.T) {
	localID, _ := identity.NewAgentID()
	peerID, _ := identity.NewAgentID()
	table := NewTable(localID)

	table.AddRoute(&Route{
		Network:     MustParseCIDR("10.0.0.0/8"),
		NextHop:     peerID,
		OriginAgent: peerID,
		Metric:      10,
	})
	table.AddRoute(&Route{
		Network:     MustParseCIDR("172.16.0.0/12"),
		NextHop:     peerID,
		OriginAgent: peerID,
		Metric:      5,
	})

	all := table.GetAllRoutes()
	if len(all) != 2 {
		t.Errorf("GetAllRoutes returned %d routes, want 2", len(all))
	}
}
