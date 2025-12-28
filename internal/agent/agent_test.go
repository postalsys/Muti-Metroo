package agent

import (
	"os"
	"testing"
	"time"

	"github.com/coinstash/muti-metroo/internal/config"
	"github.com/coinstash/muti-metroo/internal/identity"
	"github.com/coinstash/muti-metroo/internal/protocol"
)

func TestNew(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "agent-test")
	if err != nil {
		t.Fatalf("Create temp dir error: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := config.Default()
	cfg.Agent.DataDir = tmpDir

	agent, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if agent == nil {
		t.Fatal("New() returned nil")
	}

	if agent.ID().IsZero() {
		t.Error("Agent ID should not be zero")
	}

	if agent.IsRunning() {
		t.Error("New agent should not be running")
	}
}

func TestAgent_StartStop(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test")
	if err != nil {
		t.Fatalf("Create temp dir error: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := config.Default()
	cfg.Agent.DataDir = tmpDir

	agent, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	err = agent.Start()
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if !agent.IsRunning() {
		t.Error("Agent should be running after Start()")
	}

	// Double start should fail
	err = agent.Start()
	if err == nil {
		t.Error("Double Start() should fail")
	}

	err = agent.Stop()
	if err != nil {
		t.Errorf("Stop() error = %v", err)
	}

	if agent.IsRunning() {
		t.Error("Agent should not be running after Stop()")
	}

	// Double stop should be safe
	err = agent.Stop()
	if err != nil {
		t.Errorf("Double Stop() error = %v", err)
	}
}

func TestAgent_WithSOCKS5(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test")
	if err != nil {
		t.Fatalf("Create temp dir error: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := config.Default()
	cfg.Agent.DataDir = tmpDir
	cfg.SOCKS5.Enabled = true
	cfg.SOCKS5.Address = "127.0.0.1:0"

	agent, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	err = agent.Start()
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer agent.Stop()

	stats := agent.Stats()
	if !stats.SOCKS5Running {
		t.Error("SOCKS5 should be running")
	}
}

func TestAgent_WithExit(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test")
	if err != nil {
		t.Fatalf("Create temp dir error: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := config.Default()
	cfg.Agent.DataDir = tmpDir
	cfg.Exit.Enabled = true
	cfg.Exit.Routes = []string{"10.0.0.0/8"}

	agent, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	err = agent.Start()
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer agent.Stop()

	stats := agent.Stats()
	if !stats.ExitHandlerRun {
		t.Error("Exit handler should be running")
	}
}

func TestAgent_WithSOCKS5Auth(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test")
	if err != nil {
		t.Fatalf("Create temp dir error: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := config.Default()
	cfg.Agent.DataDir = tmpDir
	cfg.SOCKS5.Enabled = true
	cfg.SOCKS5.Address = "127.0.0.1:0"
	cfg.SOCKS5.Auth.Enabled = true
	cfg.SOCKS5.Auth.Users = []config.SOCKS5UserConfig{
		{Username: "user1", Password: "pass1"},
	}

	agent, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	err = agent.Start()
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer agent.Stop()
}

func TestAgent_Stats(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test")
	if err != nil {
		t.Fatalf("Create temp dir error: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := config.Default()
	cfg.Agent.DataDir = tmpDir

	agent, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	stats := agent.Stats()
	if stats.PeerCount != 0 {
		t.Errorf("PeerCount = %d, want 0", stats.PeerCount)
	}
	if stats.StreamCount != 0 {
		t.Errorf("StreamCount = %d, want 0", stats.StreamCount)
	}
}

func TestAgent_ID(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test")
	if err != nil {
		t.Fatalf("Create temp dir error: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := config.Default()
	cfg.Agent.DataDir = tmpDir

	agent, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	id := agent.ID()
	if id.IsZero() {
		t.Error("ID() should not return zero ID")
	}
}

func TestAgent_ProcessFrame_Keepalive(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test")
	if err != nil {
		t.Fatalf("Create temp dir error: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := config.Default()
	cfg.Agent.DataDir = tmpDir

	agent, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	peerID, _ := identity.NewAgentID()

	// Process a keepalive frame
	ka := &protocol.Keepalive{Timestamp: uint64(time.Now().UnixNano())}
	frame := &protocol.Frame{
		Type:     protocol.FrameKeepalive,
		StreamID: protocol.ControlStreamID,
		Payload:  ka.Encode(),
	}

	// Should not panic
	agent.processFrame(peerID, frame)
}

func TestAgent_ProcessFrame_RouteAdvertise(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test")
	if err != nil {
		t.Fatalf("Create temp dir error: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := config.Default()
	cfg.Agent.DataDir = tmpDir

	agent, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	peerID, _ := identity.NewAgentID()

	// Process a route advertise frame
	adv := &protocol.RouteAdvertise{
		OriginAgent: peerID,
		Sequence:    1,
		Routes: []protocol.Route{
			{
				AddressFamily: protocol.AddrFamilyIPv4,
				PrefixLength:  8,
				Prefix:        []byte{10, 0, 0, 0},
				Metric:        10,
			},
		},
		Path:   []identity.AgentID{peerID},
		SeenBy: []identity.AgentID{peerID},
	}

	frame := &protocol.Frame{
		Type:     protocol.FrameRouteAdvertise,
		StreamID: protocol.ControlStreamID,
		Payload:  adv.Encode(),
	}

	// Should not panic
	agent.processFrame(peerID, frame)

	// Route should be added
	if agent.routeMgr.TotalRoutes() != 1 {
		t.Errorf("TotalRoutes = %d, want 1", agent.routeMgr.TotalRoutes())
	}
}

func TestAgent_ProcessFrame_StreamOpen(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test")
	if err != nil {
		t.Fatalf("Create temp dir error: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := config.Default()
	cfg.Agent.DataDir = tmpDir
	cfg.Exit.Enabled = true
	cfg.Exit.Routes = []string{"0.0.0.0/0"}

	agent, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	err = agent.Start()
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer agent.Stop()

	peerID, _ := identity.NewAgentID()

	// Process a stream open frame with empty path (we are exit)
	open := &protocol.StreamOpen{
		RequestID:     1,
		AddressType:   protocol.AddrTypeIPv4,
		Address:       []byte{127, 0, 0, 1},
		Port:          8080,
		TTL:           16,
		RemainingPath: nil, // Empty path means we are exit
	}

	frame := &protocol.Frame{
		Type:     protocol.FrameStreamOpen,
		StreamID: 1,
		Payload:  open.Encode(),
	}

	// Should not panic (even though connection may fail)
	agent.processFrame(peerID, frame)
}

func TestAgent_buildSOCKS5Auth_NoAuth(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test")
	if err != nil {
		t.Fatalf("Create temp dir error: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := config.Default()
	cfg.Agent.DataDir = tmpDir
	cfg.SOCKS5.Auth.Enabled = false

	agent, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	auths := agent.buildSOCKS5Auth()
	if len(auths) != 1 {
		t.Errorf("Authenticators len = %d, want 1", len(auths))
	}
}

func TestAgent_buildSOCKS5Auth_WithUsers(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test")
	if err != nil {
		t.Fatalf("Create temp dir error: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := config.Default()
	cfg.Agent.DataDir = tmpDir
	cfg.SOCKS5.Auth.Enabled = true
	cfg.SOCKS5.Auth.Users = []config.SOCKS5UserConfig{
		{Username: "user1", Password: "pass1"},
		{Username: "user2", Password: "pass2"},
	}

	agent, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	auths := agent.buildSOCKS5Auth()
	if len(auths) != 1 {
		t.Errorf("Authenticators len = %d, want 1", len(auths))
	}
}
