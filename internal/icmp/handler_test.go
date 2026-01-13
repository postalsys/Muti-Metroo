package icmp

import (
	"context"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/postalsys/muti-metroo/internal/identity"
	"github.com/postalsys/muti-metroo/internal/protocol"
)

// mockDataWriter implements DataWriter for testing
type mockDataWriter struct {
	mu         sync.Mutex
	openAcks   []*protocol.ICMPOpenAck
	openErrs   []*protocol.ICMPOpenErr
	echos      []*protocol.ICMPEcho
	closes     []uint8
	ackPeerIDs []identity.AgentID
	errPeerIDs []identity.AgentID
}

func newMockDataWriter() *mockDataWriter {
	return &mockDataWriter{}
}

func (m *mockDataWriter) WriteICMPOpenAck(peerID identity.AgentID, streamID uint64, ack *protocol.ICMPOpenAck) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.openAcks = append(m.openAcks, ack)
	m.ackPeerIDs = append(m.ackPeerIDs, peerID)
	return nil
}

func (m *mockDataWriter) WriteICMPOpenErr(peerID identity.AgentID, streamID uint64, err *protocol.ICMPOpenErr) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.openErrs = append(m.openErrs, err)
	m.errPeerIDs = append(m.errPeerIDs, peerID)
	return nil
}

func (m *mockDataWriter) WriteICMPEcho(peerID identity.AgentID, streamID uint64, echo *protocol.ICMPEcho) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.echos = append(m.echos, echo)
	return nil
}

func (m *mockDataWriter) WriteICMPClose(peerID identity.AgentID, streamID uint64, reason uint8) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closes = append(m.closes, reason)
	return nil
}

func (m *mockDataWriter) getOpenAcks() []*protocol.ICMPOpenAck {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.openAcks
}

func (m *mockDataWriter) getOpenErrs() []*protocol.ICMPOpenErr {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.openErrs
}

func (m *mockDataWriter) getEchos() []*protocol.ICMPEcho {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.echos
}

func (m *mockDataWriter) getCloses() []uint8 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closes
}

func TestNewHandler(t *testing.T) {
	writer := newMockDataWriter()
	logger := slog.Default()

	cfg := Config{
		Enabled:     true,
		MaxSessions: 100,
		IdleTimeout: time.Minute,
		EchoTimeout: 5 * time.Second,
	}

	h := NewHandler(cfg, writer, logger)
	defer h.Close()

	if h == nil {
		t.Fatal("NewHandler() returned nil")
	}
	if h.config.Enabled != true {
		t.Error("config.Enabled should be true")
	}
	if h.config.MaxSessions != 100 {
		t.Error("config.MaxSessions should be 100")
	}
	if h.ActiveCount() != 0 {
		t.Error("ActiveCount() should be 0 initially")
	}
}

func TestNewHandler_NoCleanupWithZeroTimeout(t *testing.T) {
	writer := newMockDataWriter()
	logger := slog.Default()

	cfg := Config{
		Enabled:     true,
		MaxSessions: 100,
		IdleTimeout: 0, // No cleanup
		EchoTimeout: 5 * time.Second,
	}

	h := NewHandler(cfg, writer, logger)
	defer h.Close()

	// Should not panic or have issues when cleanup is disabled
	if h == nil {
		t.Fatal("NewHandler() returned nil")
	}
}

func TestHandler_HandleICMPOpen_Disabled(t *testing.T) {
	writer := newMockDataWriter()
	logger := slog.Default()

	cfg := Config{
		Enabled:     false, // ICMP disabled
		MaxSessions: 100,
		IdleTimeout: time.Minute,
		EchoTimeout: 5 * time.Second,
	}

	h := NewHandler(cfg, writer, logger)
	defer h.Close()

	peerID, _ := identity.NewAgentID()
	destIP := net.ParseIP("8.8.8.8")

	open := &protocol.ICMPOpen{
		RequestID:     12345,
		DestIP:        destIP.To4(), // Must be []byte
		TTL:           64,
		RemainingPath: nil,
	}

	var zeroKey [protocol.EphemeralKeySize]byte
	err := h.HandleICMPOpen(context.Background(), peerID, 1, open, zeroKey)

	if err == nil {
		t.Error("HandleICMPOpen() should return error when ICMP disabled")
	}

	errs := writer.getOpenErrs()
	if len(errs) != 1 {
		t.Fatalf("Expected 1 error, got %d", len(errs))
	}
	if errs[0].ErrorCode != protocol.ErrICMPDisabled {
		t.Errorf("ErrorCode = %d, want %d", errs[0].ErrorCode, protocol.ErrICMPDisabled)
	}
	if errs[0].RequestID != 12345 {
		t.Errorf("RequestID = %d, want 12345", errs[0].RequestID)
	}
}

func TestHandler_HandleICMPOpen_SessionLimit(t *testing.T) {
	writer := newMockDataWriter()
	logger := slog.Default()

	cfg := Config{
		Enabled:     true,
		MaxSessions: 1, // Limit to 1 session
		IdleTimeout: time.Minute,
		EchoTimeout: 5 * time.Second,
	}

	h := NewHandler(cfg, writer, logger)
	defer h.Close()

	peerID, _ := identity.NewAgentID()
	destIP := net.ParseIP("8.8.8.8")

	// Manually add a session to fill the limit
	session := NewSession(999, 999, peerID, destIP)
	h.mu.Lock()
	h.sessions[999] = session
	h.mu.Unlock()

	// Now try to open another session
	open := &protocol.ICMPOpen{
		RequestID:     12345,
		DestIP:        destIP.To4(), // Must be []byte
		TTL:           64,
		RemainingPath: nil,
	}

	var zeroKey [protocol.EphemeralKeySize]byte
	err := h.HandleICMPOpen(context.Background(), peerID, 1, open, zeroKey)

	if err == nil {
		t.Error("HandleICMPOpen() should return error when session limit reached")
	}

	errs := writer.getOpenErrs()
	if len(errs) != 1 {
		t.Fatalf("Expected 1 error, got %d", len(errs))
	}
	if errs[0].ErrorCode != protocol.ErrICMPSessionLimit {
		t.Errorf("ErrorCode = %d, want %d", errs[0].ErrorCode, protocol.ErrICMPSessionLimit)
	}
}

func TestHandler_HandleICMPClose(t *testing.T) {
	writer := newMockDataWriter()
	logger := slog.Default()

	cfg := Config{
		Enabled:     true,
		MaxSessions: 100,
		IdleTimeout: time.Minute,
		EchoTimeout: 5 * time.Second,
	}

	h := NewHandler(cfg, writer, logger)
	defer h.Close()

	peerID, _ := identity.NewAgentID()
	destIP := net.ParseIP("8.8.8.8")

	// Add a session
	session := NewSession(123, 456, peerID, destIP)
	h.mu.Lock()
	h.sessions[123] = session
	h.byRequestID[456] = session
	h.mu.Unlock()

	if h.ActiveCount() != 1 {
		t.Fatalf("ActiveCount() = %d, want 1", h.ActiveCount())
	}

	// Close the session
	err := h.HandleICMPClose(peerID, 123)
	if err != nil {
		t.Errorf("HandleICMPClose() error = %v", err)
	}

	if h.ActiveCount() != 0 {
		t.Errorf("ActiveCount() = %d, want 0 after close", h.ActiveCount())
	}

	// Session should be cleaned up from both maps
	if h.GetSession(123) != nil {
		t.Error("Session should be removed from sessions map")
	}
	if h.GetSessionByRequestID(456) != nil {
		t.Error("Session should be removed from byRequestID map")
	}
}

func TestHandler_HandleICMPClose_NonExistent(t *testing.T) {
	writer := newMockDataWriter()
	logger := slog.Default()

	cfg := DefaultConfig()
	h := NewHandler(cfg, writer, logger)
	defer h.Close()

	peerID, _ := identity.NewAgentID()

	// Close a non-existent session should not error
	err := h.HandleICMPClose(peerID, 999)
	if err != nil {
		t.Errorf("HandleICMPClose() for non-existent session should not error, got %v", err)
	}
}

func TestHandler_HandleICMPEcho_NoSession(t *testing.T) {
	writer := newMockDataWriter()
	logger := slog.Default()

	cfg := DefaultConfig()
	h := NewHandler(cfg, writer, logger)
	defer h.Close()

	peerID, _ := identity.NewAgentID()

	echo := &protocol.ICMPEcho{
		Identifier: 1,
		Sequence:   1,
		IsReply:    false,
		Data:       []byte("test"),
	}

	err := h.HandleICMPEcho(peerID, 999, echo)
	if err == nil {
		t.Error("HandleICMPEcho() should error for unknown stream ID")
	}
}

func TestHandler_HandleICMPEcho_ClosedSession(t *testing.T) {
	writer := newMockDataWriter()
	logger := slog.Default()

	cfg := DefaultConfig()
	h := NewHandler(cfg, writer, logger)
	defer h.Close()

	peerID, _ := identity.NewAgentID()
	destIP := net.ParseIP("8.8.8.8")

	// Add a closed session
	session := NewSession(123, 456, peerID, destIP)
	session.Close()
	h.mu.Lock()
	h.sessions[123] = session
	h.mu.Unlock()

	echo := &protocol.ICMPEcho{
		Identifier: 1,
		Sequence:   1,
		IsReply:    false,
		Data:       []byte("test"),
	}

	err := h.HandleICMPEcho(peerID, 123, echo)
	if err == nil {
		t.Error("HandleICMPEcho() should error for closed session")
	}
}

func TestHandler_HandleICMPEcho_Reply_Ignored(t *testing.T) {
	writer := newMockDataWriter()
	logger := slog.Default()

	cfg := DefaultConfig()
	h := NewHandler(cfg, writer, logger)
	defer h.Close()

	peerID, _ := identity.NewAgentID()
	destIP := net.ParseIP("8.8.8.8")

	// Add an open session
	session := NewSession(123, 456, peerID, destIP)
	session.SetOpen()
	h.mu.Lock()
	h.sessions[123] = session
	h.mu.Unlock()

	// Reply frames from mesh should be ignored
	echo := &protocol.ICMPEcho{
		Identifier: 1,
		Sequence:   1,
		IsReply:    true, // This is a reply
		Data:       []byte("test"),
	}

	err := h.HandleICMPEcho(peerID, 123, echo)
	if err != nil {
		t.Errorf("HandleICMPEcho() for reply should not error, got %v", err)
	}
}

func TestHandler_GetSession(t *testing.T) {
	writer := newMockDataWriter()
	logger := slog.Default()

	cfg := DefaultConfig()
	h := NewHandler(cfg, writer, logger)
	defer h.Close()

	peerID, _ := identity.NewAgentID()
	destIP := net.ParseIP("8.8.8.8")

	// No sessions initially
	if h.GetSession(123) != nil {
		t.Error("GetSession() should return nil for non-existent session")
	}

	// Add a session
	session := NewSession(123, 456, peerID, destIP)
	h.mu.Lock()
	h.sessions[123] = session
	h.mu.Unlock()

	got := h.GetSession(123)
	if got != session {
		t.Error("GetSession() did not return expected session")
	}
}

func TestHandler_GetSessionByRequestID(t *testing.T) {
	writer := newMockDataWriter()
	logger := slog.Default()

	cfg := DefaultConfig()
	h := NewHandler(cfg, writer, logger)
	defer h.Close()

	peerID, _ := identity.NewAgentID()
	destIP := net.ParseIP("8.8.8.8")

	// No sessions initially
	if h.GetSessionByRequestID(456) != nil {
		t.Error("GetSessionByRequestID() should return nil for non-existent session")
	}

	// Add a session
	session := NewSession(123, 456, peerID, destIP)
	h.mu.Lock()
	h.sessions[123] = session
	h.byRequestID[456] = session
	h.mu.Unlock()

	got := h.GetSessionByRequestID(456)
	if got != session {
		t.Error("GetSessionByRequestID() did not return expected session")
	}
}

func TestHandler_CleanupExpired(t *testing.T) {
	writer := newMockDataWriter()
	logger := slog.Default()

	cfg := Config{
		Enabled:     true,
		MaxSessions: 100,
		IdleTimeout: 50 * time.Millisecond, // Short timeout for test
		EchoTimeout: 5 * time.Second,
	}

	h := NewHandler(cfg, writer, logger)
	defer h.Close()

	peerID, _ := identity.NewAgentID()
	destIP := net.ParseIP("8.8.8.8")

	// Add an old session
	session := NewSession(123, 456, peerID, destIP)
	session.SetOpen()
	h.mu.Lock()
	h.sessions[123] = session
	h.byRequestID[456] = session
	h.mu.Unlock()

	if h.ActiveCount() != 1 {
		t.Fatalf("ActiveCount() = %d, want 1", h.ActiveCount())
	}

	// Wait for session to expire and cleanup to run
	time.Sleep(100 * time.Millisecond)

	// Manually trigger cleanup since we can't rely on ticker timing
	h.cleanupExpired()

	if h.ActiveCount() != 0 {
		t.Errorf("ActiveCount() = %d, want 0 after cleanup", h.ActiveCount())
	}

	// Should have sent a close
	closes := writer.getCloses()
	if len(closes) != 1 {
		t.Errorf("Expected 1 close, got %d", len(closes))
	}
	if closes[0] != protocol.ICMPCloseTimeout {
		t.Errorf("Close reason = %d, want %d (timeout)", closes[0], protocol.ICMPCloseTimeout)
	}
}

func TestHandler_Close(t *testing.T) {
	writer := newMockDataWriter()
	logger := slog.Default()

	cfg := Config{
		Enabled:     true,
		MaxSessions: 100,
		IdleTimeout: time.Minute,
		EchoTimeout: 5 * time.Second,
	}

	h := NewHandler(cfg, writer, logger)

	peerID, _ := identity.NewAgentID()
	destIP := net.ParseIP("8.8.8.8")

	// Add sessions
	for i := uint64(1); i <= 5; i++ {
		session := NewSession(i, i*100, peerID, destIP)
		h.mu.Lock()
		h.sessions[i] = session
		h.byRequestID[i*100] = session
		h.mu.Unlock()
	}

	if h.ActiveCount() != 5 {
		t.Fatalf("ActiveCount() = %d, want 5", h.ActiveCount())
	}

	// Close handler
	err := h.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// All sessions should be removed
	if h.ActiveCount() != 0 {
		t.Errorf("ActiveCount() = %d, want 0 after Close()", h.ActiveCount())
	}
}

func TestHandler_HandleICMPOpen_DestinationNotAllowed(t *testing.T) {
	writer := newMockDataWriter()
	logger := slog.Default()

	// Parse allowed CIDRs
	_, cidr1, _ := net.ParseCIDR("10.0.0.0/8")
	_, cidr2, _ := net.ParseCIDR("192.168.0.0/16")

	cfg := Config{
		Enabled:      true,
		MaxSessions:  100,
		IdleTimeout:  time.Minute,
		EchoTimeout:  5 * time.Second,
		AllowedCIDRs: []*net.IPNet{cidr1, cidr2},
	}

	h := NewHandler(cfg, writer, logger)
	defer h.Close()

	peerID, _ := identity.NewAgentID()

	// Test with disallowed destination (8.8.8.8 is not in 10.0.0.0/8 or 192.168.0.0/16)
	destIP := net.ParseIP("8.8.8.8")
	open := &protocol.ICMPOpen{
		RequestID:     12345,
		DestIP:        destIP.To4(),
		TTL:           64,
		RemainingPath: nil,
	}

	var zeroKey [protocol.EphemeralKeySize]byte
	err := h.HandleICMPOpen(context.Background(), peerID, 1, open, zeroKey)

	if err == nil {
		t.Error("HandleICMPOpen() should return error when destination not allowed")
	}

	errs := writer.getOpenErrs()
	if len(errs) != 1 {
		t.Fatalf("Expected 1 error, got %d", len(errs))
	}
	if errs[0].ErrorCode != protocol.ErrICMPDestNotAllowed {
		t.Errorf("ErrorCode = %d, want %d", errs[0].ErrorCode, protocol.ErrICMPDestNotAllowed)
	}
}

func TestHandler_HandleICMPOpen_DestinationAllowed(t *testing.T) {
	writer := newMockDataWriter()
	logger := slog.Default()

	// Parse allowed CIDRs
	_, cidr1, _ := net.ParseCIDR("10.0.0.0/8")
	_, cidr2, _ := net.ParseCIDR("192.168.0.0/16")

	cfg := Config{
		Enabled:      true,
		MaxSessions:  100,
		IdleTimeout:  time.Minute,
		EchoTimeout:  5 * time.Second,
		AllowedCIDRs: []*net.IPNet{cidr1, cidr2},
	}

	h := NewHandler(cfg, writer, logger)
	defer h.Close()

	peerID, _ := identity.NewAgentID()

	// Test with allowed destination (10.1.1.1 is in 10.0.0.0/8)
	destIP := net.ParseIP("10.1.1.1")
	open := &protocol.ICMPOpen{
		RequestID:     12345,
		DestIP:        destIP.To4(),
		TTL:           64,
		RemainingPath: nil,
	}

	var zeroKey [protocol.EphemeralKeySize]byte
	err := h.HandleICMPOpen(context.Background(), peerID, 1, open, zeroKey)

	// May fail due to socket creation, but should NOT fail due to CIDR validation
	if err != nil {
		// Check if it's a CIDR error (which would be wrong)
		errs := writer.getOpenErrs()
		for _, e := range errs {
			if e.ErrorCode == protocol.ErrICMPDestNotAllowed {
				t.Errorf("Should not reject allowed destination 10.1.1.1")
			}
		}
		// Other errors (socket creation) are acceptable in test environment
		t.Logf("HandleICMPOpen() error (acceptable if socket creation): %v", err)
	}
}

func TestHandler_IsDestinationAllowed_EmptyAllowList(t *testing.T) {
	writer := newMockDataWriter()
	logger := slog.Default()

	cfg := Config{
		Enabled:      true,
		MaxSessions:  100,
		IdleTimeout:  time.Minute,
		EchoTimeout:  5 * time.Second,
		AllowedCIDRs: nil, // Empty - all allowed
	}

	h := NewHandler(cfg, writer, logger)
	defer h.Close()

	// All IPs should be allowed with empty list
	testIPs := []string{"8.8.8.8", "10.0.0.1", "192.168.1.1", "127.0.0.1"}
	for _, ip := range testIPs {
		if !h.isDestinationAllowed(net.ParseIP(ip)) {
			t.Errorf("IP %s should be allowed with empty CIDR list", ip)
		}
	}
}

func TestHandler_IsDestinationAllowed_IPv6(t *testing.T) {
	writer := newMockDataWriter()
	logger := slog.Default()

	// Allow IPv6 loopback
	_, cidr, _ := net.ParseCIDR("::1/128")

	cfg := Config{
		Enabled:      true,
		MaxSessions:  100,
		IdleTimeout:  time.Minute,
		EchoTimeout:  5 * time.Second,
		AllowedCIDRs: []*net.IPNet{cidr},
	}

	h := NewHandler(cfg, writer, logger)
	defer h.Close()

	// IPv6 loopback should be allowed
	if !h.isDestinationAllowed(net.ParseIP("::1")) {
		t.Error("::1 should be allowed")
	}

	// Other IPv6 should not be allowed
	if h.isDestinationAllowed(net.ParseIP("2001:db8::1")) {
		t.Error("2001:db8::1 should not be allowed")
	}
}

func TestHandler_ConcurrentAccess(t *testing.T) {
	writer := newMockDataWriter()
	logger := slog.Default()

	cfg := Config{
		Enabled:     true,
		MaxSessions: 1000,
		IdleTimeout: time.Minute,
		EchoTimeout: 5 * time.Second,
	}

	h := NewHandler(cfg, writer, logger)
	defer h.Close()

	peerID, _ := identity.NewAgentID()
	destIP := net.ParseIP("8.8.8.8")

	// Concurrent reads and writes
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(3)

		// Writer goroutine
		go func(id uint64) {
			defer wg.Done()
			session := NewSession(id, id*100, peerID, destIP)
			h.mu.Lock()
			h.sessions[id] = session
			h.byRequestID[id*100] = session
			h.mu.Unlock()
		}(uint64(i))

		// Reader goroutine 1
		go func(id uint64) {
			defer wg.Done()
			_ = h.GetSession(id)
		}(uint64(i))

		// Reader goroutine 2
		go func(id uint64) {
			defer wg.Done()
			_ = h.ActiveCount()
		}(uint64(i))
	}

	wg.Wait()
}
