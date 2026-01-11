package forward

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/postalsys/muti-metroo/internal/crypto"
	"github.com/postalsys/muti-metroo/internal/identity"
	"github.com/postalsys/muti-metroo/internal/protocol"
)

// mustNewAgentID creates a new agent ID or panics.
func mustNewAgentID() identity.AgentID {
	id, err := identity.NewAgentID()
	if err != nil {
		panic(err)
	}
	return id
}

// mockStreamWriter is a mock implementation of StreamWriter for testing.
type mockStreamWriter struct {
	mu           sync.Mutex
	dataWrites   []dataWrite
	acks         []ackWrite
	errors       []errWrite
	closes       []closeWrite
	writeFail    bool
	writeDataErr error
}

type dataWrite struct {
	PeerID   identity.AgentID
	StreamID uint64
	Data     []byte
	Flags    uint8
}

type ackWrite struct {
	PeerID       identity.AgentID
	StreamID     uint64
	RequestID    uint64
	BoundIP      net.IP
	BoundPort    uint16
	EphemeralPub [crypto.KeySize]byte
}

type errWrite struct {
	PeerID    identity.AgentID
	StreamID  uint64
	RequestID uint64
	ErrorCode uint16
	Message   string
}

type closeWrite struct {
	PeerID   identity.AgentID
	StreamID uint64
}

func (m *mockStreamWriter) WriteStreamData(peerID identity.AgentID, streamID uint64, data []byte, flags uint8) error {
	if m.writeDataErr != nil {
		return m.writeDataErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dataWrites = append(m.dataWrites, dataWrite{
		PeerID:   peerID,
		StreamID: streamID,
		Data:     append([]byte(nil), data...),
		Flags:    flags,
	})
	return nil
}

func (m *mockStreamWriter) WriteStreamOpenAck(peerID identity.AgentID, streamID uint64, requestID uint64, boundIP net.IP, boundPort uint16, ephemeralPubKey [crypto.KeySize]byte) error {
	if m.writeFail {
		return net.ErrClosed
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.acks = append(m.acks, ackWrite{
		PeerID:       peerID,
		StreamID:     streamID,
		RequestID:    requestID,
		BoundIP:      boundIP,
		BoundPort:    boundPort,
		EphemeralPub: ephemeralPubKey,
	})
	return nil
}

func (m *mockStreamWriter) WriteStreamOpenErr(peerID identity.AgentID, streamID uint64, requestID uint64, errorCode uint16, message string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errors = append(m.errors, errWrite{
		PeerID:    peerID,
		StreamID:  streamID,
		RequestID: requestID,
		ErrorCode: errorCode,
		Message:   message,
	})
	return nil
}

func (m *mockStreamWriter) WriteStreamClose(peerID identity.AgentID, streamID uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closes = append(m.closes, closeWrite{
		PeerID:   peerID,
		StreamID: streamID,
	})
	return nil
}

func (m *mockStreamWriter) getDataWrites() []dataWrite {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]dataWrite(nil), m.dataWrites...)
}

func (m *mockStreamWriter) getAcks() []ackWrite {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]ackWrite(nil), m.acks...)
}

func (m *mockStreamWriter) getErrors() []errWrite {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]errWrite(nil), m.errors...)
}

func (m *mockStreamWriter) getCloses() []closeWrite {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]closeWrite(nil), m.closes...)
}

func TestDefaultHandlerConfig(t *testing.T) {
	cfg := DefaultHandlerConfig()

	if cfg.ConnectTimeout != 30*time.Second {
		t.Errorf("expected ConnectTimeout 30s, got %v", cfg.ConnectTimeout)
	}

	if cfg.IdleTimeout != 5*time.Minute {
		t.Errorf("expected IdleTimeout 5m, got %v", cfg.IdleTimeout)
	}

	if cfg.MaxConnections != 1000 {
		t.Errorf("expected MaxConnections 1000, got %d", cfg.MaxConnections)
	}
}

func TestNewHandler(t *testing.T) {
	localID := mustNewAgentID()
	writer := &mockStreamWriter{}

	cfg := HandlerConfig{
		Endpoints: []Endpoint{
			{Key: "web", Target: "localhost:8080"},
			{Key: "api", Target: "localhost:9090"},
		},
		ConnectTimeout: 10 * time.Second,
		IdleTimeout:    1 * time.Minute,
		MaxConnections: 100,
	}

	handler := NewHandler(cfg, localID, writer)

	if handler == nil {
		t.Fatal("NewHandler returned nil")
	}

	// Check targets are configured
	target, ok := handler.GetTarget("web")
	if !ok || target != "localhost:8080" {
		t.Errorf("expected target 'localhost:8080' for key 'web', got '%s', ok=%v", target, ok)
	}

	target, ok = handler.GetTarget("api")
	if !ok || target != "localhost:9090" {
		t.Errorf("expected target 'localhost:9090' for key 'api', got '%s', ok=%v", target, ok)
	}

	_, ok = handler.GetTarget("nonexistent")
	if ok {
		t.Error("expected false for nonexistent key")
	}

	// Check keys
	keys := handler.GetKeys()
	if len(keys) != 2 {
		t.Errorf("expected 2 keys, got %d", len(keys))
	}

	// Check initial state
	if handler.IsRunning() {
		t.Error("expected handler not running initially")
	}

	if handler.ConnectionCount() != 0 {
		t.Errorf("expected 0 connections, got %d", handler.ConnectionCount())
	}
}

func TestHandler_StartStop(t *testing.T) {
	handler := NewHandler(HandlerConfig{}, mustNewAgentID(), &mockStreamWriter{})

	if handler.IsRunning() {
		t.Error("expected not running before start")
	}

	handler.Start()

	if !handler.IsRunning() {
		t.Error("expected running after start")
	}

	handler.Stop()

	if handler.IsRunning() {
		t.Error("expected not running after stop")
	}

	// Multiple stops should be safe
	handler.Stop()
	handler.Stop()
}

func TestHandler_HandleStreamOpen_NotRunning(t *testing.T) {
	handler := NewHandler(HandlerConfig{
		Endpoints: []Endpoint{{Key: "test", Target: "localhost:8080"}},
	}, mustNewAgentID(), &mockStreamWriter{})

	// Don't start handler

	var ephPub [crypto.KeySize]byte
	err := handler.HandleStreamOpen(context.Background(), 1, 1, mustNewAgentID(), "test", ephPub)
	if err == nil {
		t.Error("expected error when handler not running")
	}
}

func TestHandler_HandleStreamOpen_UnknownKey(t *testing.T) {
	writer := &mockStreamWriter{}
	handler := NewHandler(HandlerConfig{
		Endpoints: []Endpoint{{Key: "known", Target: "localhost:8080"}},
	}, mustNewAgentID(), writer)

	handler.Start()
	defer handler.Stop()

	var ephPub [crypto.KeySize]byte
	remoteID := mustNewAgentID()
	err := handler.HandleStreamOpen(context.Background(), 1, 100, remoteID, "unknown", ephPub)
	if err == nil {
		t.Error("expected error for unknown key")
	}

	// Wait for async processing
	time.Sleep(50 * time.Millisecond)

	// Check error was sent
	errors := writer.getErrors()
	if len(errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errors))
	}

	if errors[0].ErrorCode != protocol.ErrForwardNotFound {
		t.Errorf("expected error code %d, got %d", protocol.ErrForwardNotFound, errors[0].ErrorCode)
	}

	if errors[0].StreamID != 1 {
		t.Errorf("expected stream ID 1, got %d", errors[0].StreamID)
	}
}

func TestHandler_HandleStreamOpen_ConnectionLimit(t *testing.T) {
	writer := &mockStreamWriter{}
	handler := NewHandler(HandlerConfig{
		Endpoints:      []Endpoint{{Key: "test", Target: "localhost:8080"}},
		MaxConnections: 0, // No limit initially
	}, mustNewAgentID(), writer)

	// Manually set connection count to simulate limit
	handler.connCount.Store(100)
	handler.cfg.MaxConnections = 100

	handler.Start()
	defer handler.Stop()

	var ephPub [crypto.KeySize]byte
	err := handler.HandleStreamOpen(context.Background(), 1, 100, mustNewAgentID(), "test", ephPub)
	if err == nil {
		t.Error("expected error for connection limit")
	}

	// Check error was sent
	errors := writer.getErrors()
	if len(errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errors))
	}

	if errors[0].ErrorCode != protocol.ErrConnectionLimit {
		t.Errorf("expected error code %d, got %d", protocol.ErrConnectionLimit, errors[0].ErrorCode)
	}
}

func TestHandler_HandleStreamOpen_Success(t *testing.T) {
	// Create a target server
	targetListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create target listener: %v", err)
	}
	defer targetListener.Close()

	targetAddr := targetListener.Addr().String()

	// Accept connections on target
	go func() {
		for {
			conn, err := targetListener.Accept()
			if err != nil {
				return
			}
			// Just hold the connection
			go func(c net.Conn) {
				buf := make([]byte, 1024)
				for {
					_, err := c.Read(buf)
					if err != nil {
						c.Close()
						return
					}
				}
			}(conn)
		}
	}()

	writer := &mockStreamWriter{}
	handler := NewHandler(HandlerConfig{
		Endpoints:      []Endpoint{{Key: "test", Target: targetAddr}},
		ConnectTimeout: 5 * time.Second,
		IdleTimeout:    1 * time.Minute,
	}, mustNewAgentID(), writer)

	handler.Start()
	defer handler.Stop()

	// Generate ephemeral keypair for client
	_, ephPub, err := crypto.GenerateEphemeralKeypair()
	if err != nil {
		t.Fatalf("failed to generate ephemeral key: %v", err)
	}

	remoteID := mustNewAgentID()
	err = handler.HandleStreamOpen(context.Background(), 1, 100, remoteID, "test", ephPub)
	if err != nil {
		t.Fatalf("HandleStreamOpen failed: %v", err)
	}

	// Wait for async processing
	time.Sleep(200 * time.Millisecond)

	// Check ACK was sent
	acks := writer.getAcks()
	if len(acks) != 1 {
		t.Fatalf("expected 1 ack, got %d", len(acks))
	}

	if acks[0].StreamID != 1 {
		t.Errorf("expected stream ID 1, got %d", acks[0].StreamID)
	}

	if acks[0].RequestID != 100 {
		t.Errorf("expected request ID 100, got %d", acks[0].RequestID)
	}

	// Check connection count
	if handler.ConnectionCount() != 1 {
		t.Errorf("expected 1 connection, got %d", handler.ConnectionCount())
	}
}

func TestHandler_HandleStreamData_UnknownStream(t *testing.T) {
	handler := NewHandler(HandlerConfig{}, mustNewAgentID(), &mockStreamWriter{})
	handler.Start()
	defer handler.Stop()

	err := handler.HandleStreamData(mustNewAgentID(), 999, []byte("data"), 0)
	if err == nil {
		t.Error("expected error for unknown stream")
	}
}

func TestHandler_HandleStreamClose(t *testing.T) {
	writer := &mockStreamWriter{}
	handler := NewHandler(HandlerConfig{}, mustNewAgentID(), writer)
	handler.Start()
	defer handler.Stop()

	// Add a mock connection
	peerID := mustNewAgentID()
	ac := &ActiveConnection{
		StreamID: 1,
		RemoteID: peerID,
		Key:      "test",
		Target:   "localhost:8080",
	}
	handler.mu.Lock()
	handler.connections[1] = ac
	handler.connCount.Add(1)
	handler.mu.Unlock()

	// Close the stream
	handler.HandleStreamClose(peerID, 1)

	// Wait for processing
	time.Sleep(50 * time.Millisecond)

	// Check connection was removed
	if handler.ConnectionCount() != 0 {
		t.Errorf("expected 0 connections after close, got %d", handler.ConnectionCount())
	}

	// Check close was sent
	closes := writer.getCloses()
	if len(closes) != 1 {
		t.Fatalf("expected 1 close, got %d", len(closes))
	}
}

func TestHandler_HandleStreamReset(t *testing.T) {
	writer := &mockStreamWriter{}
	handler := NewHandler(HandlerConfig{}, mustNewAgentID(), writer)
	handler.Start()
	defer handler.Stop()

	// Add a mock connection
	peerID := mustNewAgentID()
	ac := &ActiveConnection{
		StreamID: 1,
		RemoteID: peerID,
		Key:      "test",
		Target:   "localhost:8080",
	}
	handler.mu.Lock()
	handler.connections[1] = ac
	handler.connCount.Add(1)
	handler.mu.Unlock()

	// Reset the stream
	handler.HandleStreamReset(peerID, 1, protocol.ErrGeneralFailure)

	// Wait for processing
	time.Sleep(50 * time.Millisecond)

	// Check connection was removed
	if handler.ConnectionCount() != 0 {
		t.Errorf("expected 0 connections after reset, got %d", handler.ConnectionCount())
	}
}

func TestActiveConnection_Close(t *testing.T) {
	conn := newMockConn()
	ac := &ActiveConnection{
		StreamID: 1,
		Conn:     conn,
	}

	if ac.IsClosed() {
		t.Error("expected connection not closed initially")
	}

	err := ac.Close()
	if err != nil {
		t.Errorf("Close returned error: %v", err)
	}

	if !ac.IsClosed() {
		t.Error("expected connection closed after Close()")
	}

	// Multiple closes should be safe
	err = ac.Close()
	if err != nil {
		t.Errorf("Second Close returned error: %v", err)
	}
}

func TestActiveConnection_CloseNilConn(t *testing.T) {
	ac := &ActiveConnection{
		StreamID: 1,
		Conn:     nil,
	}

	err := ac.Close()
	if err != nil {
		t.Errorf("Close with nil conn returned error: %v", err)
	}

	if !ac.IsClosed() {
		t.Error("expected connection marked as closed")
	}
}

func TestHandler_GetTarget(t *testing.T) {
	handler := NewHandler(HandlerConfig{
		Endpoints: []Endpoint{
			{Key: "web", Target: "localhost:80"},
			{Key: "api", Target: "api.local:8080"},
		},
	}, mustNewAgentID(), &mockStreamWriter{})

	tests := []struct {
		key      string
		expected string
		ok       bool
	}{
		{"web", "localhost:80", true},
		{"api", "api.local:8080", true},
		{"unknown", "", false},
		{"", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			target, ok := handler.GetTarget(tt.key)
			if ok != tt.ok {
				t.Errorf("GetTarget(%q): expected ok=%v, got %v", tt.key, tt.ok, ok)
			}
			if target != tt.expected {
				t.Errorf("GetTarget(%q): expected %q, got %q", tt.key, tt.expected, target)
			}
		})
	}
}

func TestHandler_GetKeys(t *testing.T) {
	handler := NewHandler(HandlerConfig{
		Endpoints: []Endpoint{
			{Key: "a", Target: "localhost:1"},
			{Key: "b", Target: "localhost:2"},
			{Key: "c", Target: "localhost:3"},
		},
	}, mustNewAgentID(), &mockStreamWriter{})

	keys := handler.GetKeys()
	if len(keys) != 3 {
		t.Errorf("expected 3 keys, got %d", len(keys))
	}

	// Check all keys are present (order may vary)
	keyMap := make(map[string]bool)
	for _, k := range keys {
		keyMap[k] = true
	}

	for _, expected := range []string{"a", "b", "c"} {
		if !keyMap[expected] {
			t.Errorf("expected key %q in keys", expected)
		}
	}
}

func TestHandler_MapDialError(t *testing.T) {
	handler := NewHandler(HandlerConfig{}, mustNewAgentID(), nil)

	tests := []struct {
		name     string
		err      error
		expected uint16
	}{
		{"nil error", nil, 0},
		{"connection refused", &net.OpError{Err: &net.DNSError{Err: "connection refused"}}, protocol.ErrConnectionRefused},
		{"host unreachable", &net.OpError{Err: &net.DNSError{Err: "host unreachable"}}, protocol.ErrHostUnreachable},
		{"timeout in message", &net.OpError{Err: &net.DNSError{Err: "i/o timeout"}}, protocol.ErrConnectionTimeout},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code := handler.mapDialError(tt.err)
			if code != tt.expected {
				t.Errorf("mapDialError(%v): expected %d, got %d", tt.err, tt.expected, code)
			}
		})
	}
}

func TestHandler_StopClosesAllConnections(t *testing.T) {
	handler := NewHandler(HandlerConfig{}, mustNewAgentID(), &mockStreamWriter{})
	handler.Start()

	// Add multiple mock connections
	closedCount := atomic.Int64{}
	for i := uint64(1); i <= 3; i++ {
		conn := newMockConn()
		ac := &ActiveConnection{
			StreamID: i,
			RemoteID: mustNewAgentID(),
			Conn:     conn,
		}
		handler.mu.Lock()
		handler.connections[i] = ac
		handler.connCount.Add(1)
		handler.mu.Unlock()

		// Track when connections are closed
		go func(c *mockConn) {
			for !c.closed.Load() {
				time.Sleep(10 * time.Millisecond)
			}
			closedCount.Add(1)
		}(conn)
	}

	// Stop should close all connections
	handler.Stop()

	// Wait for close tracking
	time.Sleep(100 * time.Millisecond)

	if closedCount.Load() != 3 {
		t.Errorf("expected 3 connections closed, got %d", closedCount.Load())
	}

	if handler.ConnectionCount() != 0 {
		t.Errorf("expected 0 connections after stop, got %d", handler.ConnectionCount())
	}
}

func TestHandler_ConnectionCount(t *testing.T) {
	handler := NewHandler(HandlerConfig{}, mustNewAgentID(), &mockStreamWriter{})

	if handler.ConnectionCount() != 0 {
		t.Errorf("expected 0 initially, got %d", handler.ConnectionCount())
	}

	// Add connections
	handler.connCount.Add(5)
	if handler.ConnectionCount() != 5 {
		t.Errorf("expected 5, got %d", handler.ConnectionCount())
	}

	// Remove some
	handler.connCount.Add(-2)
	if handler.ConnectionCount() != 3 {
		t.Errorf("expected 3, got %d", handler.ConnectionCount())
	}
}

func TestHandler_NoEndpoints(t *testing.T) {
	handler := NewHandler(HandlerConfig{
		Endpoints: nil,
	}, mustNewAgentID(), &mockStreamWriter{})

	keys := handler.GetKeys()
	if len(keys) != 0 {
		t.Errorf("expected 0 keys, got %d", len(keys))
	}

	_, ok := handler.GetTarget("any")
	if ok {
		t.Error("expected false for any key with no endpoints")
	}
}
