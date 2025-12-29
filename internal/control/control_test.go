package control

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/postalsys/muti-metroo/internal/identity"
)

// mockAgent implements AgentInfo for testing.
type mockAgent struct {
	id          identity.AgentID
	displayName string
	running     bool
	peers       []identity.AgentID
	routes      []RouteInfo
}

func (m *mockAgent) ID() identity.AgentID {
	return m.id
}

func (m *mockAgent) DisplayName() string {
	if m.displayName != "" {
		return m.displayName
	}
	return m.id.ShortString()
}

func (m *mockAgent) IsRunning() bool {
	return m.running
}

func (m *mockAgent) GetPeerIDs() []identity.AgentID {
	return m.peers
}

func (m *mockAgent) GetRouteInfo() []RouteInfo {
	return m.routes
}

func TestNewServer(t *testing.T) {
	cfg := DefaultServerConfig()
	agent := &mockAgent{running: true}

	s := NewServer(cfg, agent)
	if s == nil {
		t.Fatal("NewServer returned nil")
	}
}

func TestServer_StartStop(t *testing.T) {
	// Use temp directory for socket
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "control.sock")

	cfg := ServerConfig{
		SocketPath:   socketPath,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	id, _ := identity.NewAgentID()
	agent := &mockAgent{
		id:      id,
		running: true,
		peers:   []identity.AgentID{},
		routes:  []RouteInfo{},
	}

	s := NewServer(cfg, agent)

	if err := s.Start(); err != nil {
		t.Fatalf("failed to start: %v", err)
	}

	if !s.IsRunning() {
		t.Error("expected server to be running")
	}

	// Verify socket file exists
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		t.Error("socket file does not exist")
	}

	if err := s.Stop(); err != nil {
		t.Errorf("failed to stop: %v", err)
	}

	if s.IsRunning() {
		t.Error("expected server to be stopped")
	}
}

func TestServer_ClientIntegration(t *testing.T) {
	// Use temp directory for socket
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "control.sock")

	cfg := ServerConfig{
		SocketPath:   socketPath,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	id, _ := identity.NewAgentID()
	peerID, _ := identity.NewAgentID()
	agent := &mockAgent{
		id:      id,
		running: true,
		peers:   []identity.AgentID{peerID},
		routes: []RouteInfo{
			{
				Network:  "10.0.0.0/8",
				NextHop:  peerID.ShortString(),
				Origin:   peerID.ShortString(),
				Metric:   1,
				HopCount: 1,
			},
		},
	}

	s := NewServer(cfg, agent)
	if err := s.Start(); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer s.Stop()

	// Create client
	client := NewClient(socketPath)
	defer client.Close()

	ctx := context.Background()

	// Test status endpoint
	status, err := client.Status(ctx)
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	if status.AgentID != id.ShortString() {
		t.Errorf("expected agent ID %s, got %s", id.ShortString(), status.AgentID)
	}
	if !status.Running {
		t.Error("expected running=true")
	}
	if status.PeerCount != 1 {
		t.Errorf("expected peer count 1, got %d", status.PeerCount)
	}
	if status.RouteCount != 1 {
		t.Errorf("expected route count 1, got %d", status.RouteCount)
	}

	// Test peers endpoint
	peers, err := client.Peers(ctx)
	if err != nil {
		t.Fatalf("peers failed: %v", err)
	}
	if len(peers.Peers) != 1 {
		t.Errorf("expected 1 peer, got %d", len(peers.Peers))
	}
	if peers.Peers[0] != peerID.ShortString() {
		t.Errorf("expected peer %s, got %s", peerID.ShortString(), peers.Peers[0])
	}

	// Test routes endpoint
	routes, err := client.Routes(ctx)
	if err != nil {
		t.Fatalf("routes failed: %v", err)
	}
	if len(routes.Routes) != 1 {
		t.Errorf("expected 1 route, got %d", len(routes.Routes))
	}
	if routes.Routes[0].Network != "10.0.0.0/8" {
		t.Errorf("expected network 10.0.0.0/8, got %s", routes.Routes[0].Network)
	}
}
