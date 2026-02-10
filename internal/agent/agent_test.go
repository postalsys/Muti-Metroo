package agent

import (
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/postalsys/muti-metroo/internal/config"
	"github.com/postalsys/muti-metroo/internal/crypto"
	"github.com/postalsys/muti-metroo/internal/identity"
	"github.com/postalsys/muti-metroo/internal/protocol"
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

// Tests for addressToString helper function
func TestAddressToString(t *testing.T) {
	tests := []struct {
		name     string
		addrType uint8
		addr     []byte
		want     string
	}{
		{
			name:     "IPv4 localhost",
			addrType: protocol.AddrTypeIPv4,
			addr:     []byte{127, 0, 0, 1},
			want:     "127.0.0.1",
		},
		{
			name:     "IPv4 external",
			addrType: protocol.AddrTypeIPv4,
			addr:     []byte{192, 168, 1, 1},
			want:     "192.168.1.1",
		},
		{
			name:     "IPv4 zeros",
			addrType: protocol.AddrTypeIPv4,
			addr:     []byte{0, 0, 0, 0},
			want:     "0.0.0.0",
		},
		{
			name:     "IPv6 localhost",
			addrType: protocol.AddrTypeIPv6,
			addr:     []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1},
			want:     "::1",
		},
		{
			name:     "IPv6 address",
			addrType: protocol.AddrTypeIPv6,
			addr:     []byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1},
			want:     "2001:db8::1",
		},
		{
			name:     "Domain simple",
			addrType: protocol.AddrTypeDomain,
			addr:     []byte{11, 'e', 'x', 'a', 'm', 'p', 'l', 'e', '.', 'c', 'o', 'm'},
			want:     "example.com",
		},
		{
			name:     "Domain with subdomain",
			addrType: protocol.AddrTypeDomain,
			addr:     []byte{15, 'w', 'w', 'w', '.', 'e', 'x', 'a', 'm', 'p', 'l', 'e', '.', 'c', 'o', 'm'},
			want:     "www.example.com",
		},
		{
			name:     "IPv4 too short",
			addrType: protocol.AddrTypeIPv4,
			addr:     []byte{127, 0, 0},
			want:     "",
		},
		{
			name:     "IPv6 too short",
			addrType: protocol.AddrTypeIPv6,
			addr:     []byte{0, 0, 0, 0},
			want:     "",
		},
		{
			name:     "Domain empty",
			addrType: protocol.AddrTypeDomain,
			addr:     []byte{},
			want:     "",
		},
		{
			name:     "Unknown type",
			addrType: 0xFF,
			addr:     []byte{1, 2, 3, 4},
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := addressToString(tt.addrType, tt.addr)
			if got != tt.want {
				t.Errorf("addressToString() = %q, want %q", got, tt.want)
			}
		})
	}
}

// Tests for deriveResponderSessionKey helper function
func TestDeriveResponderSessionKey(t *testing.T) {
	t.Run("valid key exchange", func(t *testing.T) {
		// Generate an initiator keypair to simulate the remote side
		_, remotePub, err := crypto.GenerateEphemeralKeypair()
		if err != nil {
			t.Fatalf("GenerateEphemeralKeypair() error = %v", err)
		}

		requestID := uint64(12345)
		sessionKey, localPub, err := deriveResponderSessionKey(requestID, remotePub)
		if err != nil {
			t.Fatalf("deriveResponderSessionKey() error = %v", err)
		}

		if sessionKey == nil {
			t.Fatal("deriveResponderSessionKey() returned nil session key")
		}

		// Local public key should not be zero
		var zeroKey [crypto.KeySize]byte
		if localPub == zeroKey {
			t.Error("deriveResponderSessionKey() returned zero local public key")
		}

		// Session key should be usable for encryption/decryption
		plaintext := []byte("test message for encryption")
		ciphertext, err := sessionKey.Encrypt(plaintext)
		if err != nil {
			t.Fatalf("SessionKey.Encrypt() error = %v", err)
		}

		if len(ciphertext) <= len(plaintext) {
			t.Error("ciphertext should be larger than plaintext due to overhead")
		}
	})

	t.Run("zero key rejected", func(t *testing.T) {
		var zeroKey [crypto.KeySize]byte
		requestID := uint64(12345)

		_, _, err := deriveResponderSessionKey(requestID, zeroKey)
		if err == nil {
			t.Error("deriveResponderSessionKey() should reject zero key")
		}

		if err.Error() != "encryption required" {
			t.Errorf("deriveResponderSessionKey() error = %q, want %q", err.Error(), "encryption required")
		}
	})

	t.Run("different requests produce different keys", func(t *testing.T) {
		_, remotePub, err := crypto.GenerateEphemeralKeypair()
		if err != nil {
			t.Fatalf("GenerateEphemeralKeypair() error = %v", err)
		}

		sk1, pub1, err := deriveResponderSessionKey(1, remotePub)
		if err != nil {
			t.Fatalf("deriveResponderSessionKey(1) error = %v", err)
		}

		sk2, pub2, err := deriveResponderSessionKey(2, remotePub)
		if err != nil {
			t.Fatalf("deriveResponderSessionKey(2) error = %v", err)
		}

		// Different request IDs should produce different local keys
		// (because we generate new ephemeral keys each time)
		if pub1 == pub2 {
			t.Error("different requests should produce different local public keys")
		}

		// Verify both session keys work
		msg := []byte("test")
		_, err = sk1.Encrypt(msg)
		if err != nil {
			t.Errorf("sk1.Encrypt() error = %v", err)
		}
		_, err = sk2.Encrypt(msg)
		if err != nil {
			t.Errorf("sk2.Encrypt() error = %v", err)
		}
	})
}

// Tests for GetSOCKS5Info accessor method
func TestAgent_GetSOCKS5Info(t *testing.T) {
	tests := []struct {
		name        string
		enabled     bool
		address     string
		wantEnabled bool
		wantAddress string
	}{
		{
			name:        "SOCKS5 enabled",
			enabled:     true,
			address:     "127.0.0.1:1080",
			wantEnabled: true,
			wantAddress: "127.0.0.1:1080",
		},
		{
			name:        "SOCKS5 disabled",
			enabled:     false,
			address:     "",
			wantEnabled: false,
			wantAddress: "",
		},
		{
			name:        "custom address",
			enabled:     true,
			address:     "0.0.0.0:9050",
			wantEnabled: true,
			wantAddress: "0.0.0.0:9050",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "agent-test")
			if err != nil {
				t.Fatalf("Create temp dir error: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			cfg := config.Default()
			cfg.Agent.DataDir = tmpDir
			cfg.SOCKS5.Enabled = tt.enabled
			cfg.SOCKS5.Address = tt.address

			agent, err := New(cfg)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}

			info := agent.GetSOCKS5Info()
			if info.Enabled != tt.wantEnabled {
				t.Errorf("GetSOCKS5Info().Enabled = %v, want %v", info.Enabled, tt.wantEnabled)
			}
			if info.Address != tt.wantAddress {
				t.Errorf("GetSOCKS5Info().Address = %q, want %q", info.Address, tt.wantAddress)
			}
		})
	}
}

// Tests for GetUDPInfo accessor method
func TestAgent_GetUDPInfo(t *testing.T) {
	tests := []struct {
		name        string
		enabled     bool
		wantEnabled bool
	}{
		{
			name:        "UDP enabled",
			enabled:     true,
			wantEnabled: true,
		},
		{
			name:        "UDP disabled",
			enabled:     false,
			wantEnabled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "agent-test")
			if err != nil {
				t.Fatalf("Create temp dir error: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			cfg := config.Default()
			cfg.Agent.DataDir = tmpDir
			cfg.UDP.Enabled = tt.enabled

			agent, err := New(cfg)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}

			info := agent.GetUDPInfo()
			if info.Enabled != tt.wantEnabled {
				t.Errorf("GetUDPInfo().Enabled = %v, want %v", info.Enabled, tt.wantEnabled)
			}
		})
	}
}

// Tests for SOCKS5Address and HealthServerAddress
func TestAgent_AddressMethods(t *testing.T) {
	t.Run("SOCKS5Address without server", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "agent-test")
		if err != nil {
			t.Fatalf("Create temp dir error: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		cfg := config.Default()
		cfg.Agent.DataDir = tmpDir
		cfg.SOCKS5.Enabled = false

		agent, err := New(cfg)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		addr := agent.SOCKS5Address()
		if addr != nil {
			t.Errorf("SOCKS5Address() = %v, want nil", addr)
		}
	})

	t.Run("HealthServerAddress without server", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "agent-test")
		if err != nil {
			t.Fatalf("Create temp dir error: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		cfg := config.Default()
		cfg.Agent.DataDir = tmpDir
		cfg.HTTP.Enabled = false

		agent, err := New(cfg)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		addr := agent.HealthServerAddress()
		if addr != nil {
			t.Errorf("HealthServerAddress() = %v, want nil", addr)
		}
	})

	t.Run("SOCKS5Address with running server", func(t *testing.T) {
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

		addr := agent.SOCKS5Address()
		if addr == nil {
			t.Error("SOCKS5Address() returned nil for running server")
		}
	})
}

// Tests for ProcessFrame with NodeInfoAdvertise
func TestAgent_ProcessFrame_NodeInfoAdvertise(t *testing.T) {
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

	// Process a node info advertise frame
	nodeInfo := &protocol.NodeInfoAdvertise{
		OriginAgent: peerID,
		Sequence:    1,
		Info: protocol.NodeInfo{
			DisplayName: "test-node",
			StartTime:   time.Now().Unix(),
		},
		SeenBy: []identity.AgentID{peerID},
	}

	frame := &protocol.Frame{
		Type:     protocol.FrameNodeInfoAdvertise,
		StreamID: protocol.ControlStreamID,
		Payload:  nodeInfo.Encode(),
	}

	// Should not panic
	agent.processFrame(peerID, frame)
}

// Tests for ProcessFrame with StreamClose
func TestAgent_ProcessFrame_StreamClose(t *testing.T) {
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

	// Process a stream close frame for non-existent stream
	frame := &protocol.Frame{
		Type:     protocol.FrameStreamClose,
		StreamID: 12345,
	}

	// Should not panic (stream doesn't exist)
	agent.processFrame(peerID, frame)
}

// Tests for ProcessFrame with StreamData
func TestAgent_ProcessFrame_StreamData(t *testing.T) {
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

	// Process a stream data frame for non-existent stream
	frame := &protocol.Frame{
		Type:     protocol.FrameStreamData,
		StreamID: 12345,
		Payload:  []byte("test data"),
	}

	// Should not panic (stream doesn't exist, will be logged)
	agent.processFrame(peerID, frame)
}

// Tests for ProcessFrame with StreamReset
func TestAgent_ProcessFrame_StreamReset(t *testing.T) {
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

	// Process a stream reset frame
	reset := &protocol.StreamReset{
		ErrorCode: protocol.ErrConnectionRefused,
	}

	frame := &protocol.Frame{
		Type:     protocol.FrameStreamReset,
		StreamID: 12345,
		Payload:  reset.Encode(),
	}

	// Should not panic
	agent.processFrame(peerID, frame)
}

// Tests for ProcessFrame with KeepaliveAck
func TestAgent_ProcessFrame_KeepaliveAck(t *testing.T) {
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

	// Process a keepalive ack frame
	ka := &protocol.Keepalive{Timestamp: uint64(time.Now().UnixNano())}
	frame := &protocol.Frame{
		Type:     protocol.FrameKeepaliveAck,
		StreamID: protocol.ControlStreamID,
		Payload:  ka.Encode(),
	}

	// Should not panic (peer not tracked, will be logged)
	agent.processFrame(peerID, frame)
}

// Tests for ProcessFrame with StreamOpenAck for non-existent stream
func TestAgent_ProcessFrame_StreamOpenAck_NoStream(t *testing.T) {
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

	// Process a stream open ack for non-existent stream
	ack := &protocol.StreamOpenAck{
		RequestID:     1,
		BoundAddrType: protocol.AddrTypeIPv4,
		BoundAddr:     []byte{127, 0, 0, 1},
		BoundPort:     8080,
	}

	frame := &protocol.Frame{
		Type:     protocol.FrameStreamOpenAck,
		StreamID: 99999,
		Payload:  ack.Encode(),
	}

	// Should not panic
	agent.processFrame(peerID, frame)
}

// Tests for ProcessFrame with StreamOpenErr for non-existent stream
func TestAgent_ProcessFrame_StreamOpenErr_NoStream(t *testing.T) {
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

	// Process a stream open error for non-existent stream
	openErr := &protocol.StreamOpenErr{
		RequestID: 1,
		ErrorCode: protocol.ErrNoRoute,
		Message:   "no route to host",
	}

	frame := &protocol.Frame{
		Type:     protocol.FrameStreamOpenErr,
		StreamID: 99999,
		Payload:  openErr.Encode(),
	}

	// Should not panic
	agent.processFrame(peerID, frame)
}

// Tests for ProcessFrame with RouteWithdraw
func TestAgent_ProcessFrame_RouteWithdraw(t *testing.T) {
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

	// Process a route withdraw frame
	withdraw := &protocol.RouteWithdraw{
		OriginAgent: peerID,
		Sequence:    1,
		Routes: []protocol.Route{
			{
				AddressFamily: protocol.AddrFamilyIPv4,
				PrefixLength:  8,
				Prefix:        []byte{10, 0, 0, 0},
			},
		},
	}

	frame := &protocol.Frame{
		Type:     protocol.FrameRouteWithdraw,
		StreamID: protocol.ControlStreamID,
		Payload:  withdraw.Encode(),
	}

	// Should not panic
	agent.processFrame(peerID, frame)
}

// Tests for TriggerRouteAdvertise
func TestAgent_TriggerRouteAdvertise(t *testing.T) {
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

	// TriggerRouteAdvertise should not block or panic
	agent.TriggerRouteAdvertise()

	// Call it again to test buffered channel behavior
	agent.TriggerRouteAdvertise()
}

// Tests for getLocalStatus method
func TestAgent_getLocalStatus(t *testing.T) {
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

	data, success := agent.getLocalStatus()
	if !success {
		t.Error("getLocalStatus() should succeed")
	}

	if len(data) == 0 {
		t.Error("getLocalStatus() returned empty data")
	}

	// Verify it's valid JSON
	var status map[string]interface{}
	if err := json.Unmarshal(data, &status); err != nil {
		t.Errorf("getLocalStatus() returned invalid JSON: %v", err)
	}

	// Check expected fields
	if _, ok := status["agent_id"]; !ok {
		t.Error("getLocalStatus() missing agent_id field")
	}
	if _, ok := status["running"]; !ok {
		t.Error("getLocalStatus() missing running field")
	}
	if _, ok := status["peer_count"]; !ok {
		t.Error("getLocalStatus() missing peer_count field")
	}
}

// Tests for getLocalPeers method
func TestAgent_getLocalPeers(t *testing.T) {
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

	data, success := agent.getLocalPeers()
	if !success {
		t.Error("getLocalPeers() should succeed")
	}

	// Verify it's valid JSON array
	var peers []string
	if err := json.Unmarshal(data, &peers); err != nil {
		t.Errorf("getLocalPeers() returned invalid JSON: %v", err)
	}

	// No peers connected yet
	if len(peers) != 0 {
		t.Errorf("getLocalPeers() = %d peers, want 0", len(peers))
	}
}

// Tests for getLocalRoutes method
func TestAgent_getLocalRoutes(t *testing.T) {
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

	data, success := agent.getLocalRoutes()
	if !success {
		t.Error("getLocalRoutes() should succeed")
	}

	// Verify it's valid JSON array
	var routes []map[string]interface{}
	if err := json.Unmarshal(data, &routes); err != nil {
		t.Errorf("getLocalRoutes() returned invalid JSON: %v", err)
	}

	// Should have one local route
	if len(routes) != 1 {
		t.Errorf("getLocalRoutes() = %d routes, want 1", len(routes))
	}

	if len(routes) > 0 {
		route := routes[0]
		if _, ok := route["network"]; !ok {
			t.Error("route missing network field")
		}
		if _, ok := route["metric"]; !ok {
			t.Error("route missing metric field")
		}
	}
}

// Tests for cleanupRelaysForPeer method
func TestAgent_cleanupRelaysForPeer(t *testing.T) {
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

	// Create some relay entries
	peerA, _ := identity.NewAgentID()
	peerB, _ := identity.NewAgentID()
	peerC, _ := identity.NewAgentID()

	agent.relayMu.Lock()
	agent.relayByUpstream[1] = &relayStream{
		UpstreamPeer:   peerA,
		UpstreamID:     1,
		DownstreamPeer: peerB,
		DownstreamID:   100,
	}
	agent.relayByDownstream[100] = agent.relayByUpstream[1]

	agent.relayByUpstream[2] = &relayStream{
		UpstreamPeer:   peerB,
		UpstreamID:     2,
		DownstreamPeer: peerC,
		DownstreamID:   200,
	}
	agent.relayByDownstream[200] = agent.relayByUpstream[2]

	agent.relayByUpstream[3] = &relayStream{
		UpstreamPeer:   peerC,
		UpstreamID:     3,
		DownstreamPeer: peerA,
		DownstreamID:   300,
	}
	agent.relayByDownstream[300] = agent.relayByUpstream[3]
	agent.relayMu.Unlock()

	// Cleanup relays for peerA (should remove entries 1 and 3)
	agent.cleanupRelaysForPeer(peerA)

	agent.relayMu.RLock()
	defer agent.relayMu.RUnlock()

	// Entry 2 should remain (only involves peerB and peerC)
	if _, exists := agent.relayByUpstream[2]; !exists {
		t.Error("relay entry 2 should not be removed")
	}

	// Entries 1 and 3 should be removed
	if _, exists := agent.relayByUpstream[1]; exists {
		t.Error("relay entry 1 should be removed (involves peerA)")
	}
	if _, exists := agent.relayByUpstream[3]; exists {
		t.Error("relay entry 3 should be removed (involves peerA)")
	}
}

// Tests for buildSOCKS5Auth with hashed passwords
func TestAgent_buildSOCKS5Auth_WithHashedUsers(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test")
	if err != nil {
		t.Fatalf("Create temp dir error: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := config.Default()
	cfg.Agent.DataDir = tmpDir
	cfg.SOCKS5.Auth.Enabled = true
	cfg.SOCKS5.Auth.Users = []config.SOCKS5UserConfig{
		// bcrypt hash for "password123"
		{Username: "hasheduser", PasswordHash: "$2a$10$N9qo8uLOickgx2ZMRZoMyed0W/OLrKcKqM5Zds6UvDKTVBVVgDg5a"},
		// Mixed: plaintext and hashed
		{Username: "plainuser", Password: "plainpass"},
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

// Tests for getFileTransferStream
func TestAgent_getFileTransferStream(t *testing.T) {
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

	// Non-existent stream should return nil
	fts := agent.getFileTransferStream(12345)
	if fts != nil {
		t.Error("getFileTransferStream() should return nil for non-existent stream")
	}

	// Add a stream and retrieve it
	agent.fileStreamsMu.Lock()
	agent.fileStreams[100] = &fileTransferStream{
		StreamID:  100,
		RequestID: 1,
		IsUpload:  true,
	}
	agent.fileStreamsMu.Unlock()

	fts = agent.getFileTransferStream(100)
	if fts == nil {
		t.Error("getFileTransferStream() should return the stream")
	}
	if fts.StreamID != 100 {
		t.Errorf("getFileTransferStream().StreamID = %d, want 100", fts.StreamID)
	}
}

// Tests for cleanupFileTransferStream
func TestAgent_cleanupFileTransferStream(t *testing.T) {
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

	// Add a stream
	agent.fileStreamsMu.Lock()
	agent.fileStreams[100] = &fileTransferStream{
		StreamID:  100,
		RequestID: 1,
		IsUpload:  true,
	}
	agent.fileStreamsMu.Unlock()

	// Cleanup the stream
	agent.cleanupFileTransferStream(100)

	// Stream should be removed
	agent.fileStreamsMu.RLock()
	_, exists := agent.fileStreams[100]
	agent.fileStreamsMu.RUnlock()

	if exists {
		t.Error("cleanupFileTransferStream() should remove the stream")
	}

	// Cleanup non-existent stream should not panic
	agent.cleanupFileTransferStream(99999)
}

// Tests for ProcessFrame with ControlRequest
func TestAgent_ProcessFrame_ControlRequest(t *testing.T) {
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

	// Process a control request for local agent (status request)
	req := &protocol.ControlRequest{
		RequestID:   1,
		ControlType: protocol.ControlTypeStatus,
		TargetAgent: agent.id, // Request for this agent
		Path:        nil,
	}

	frame := &protocol.Frame{
		Type:     protocol.FrameControlRequest,
		StreamID: protocol.ControlStreamID,
		Payload:  req.Encode(),
	}

	// Should not panic (will try to send response to non-existent peer)
	agent.processFrame(peerID, frame)
}

// Tests for ProcessFrame with ControlResponse
func TestAgent_ProcessFrame_ControlResponse(t *testing.T) {
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

	// Process a control response for non-existent request
	resp := &protocol.ControlResponse{
		RequestID:   99999,
		ControlType: protocol.ControlTypeStatus,
		Success:     true,
		Data:        []byte("{}"),
	}

	frame := &protocol.Frame{
		Type:     protocol.FrameControlResponse,
		StreamID: protocol.ControlStreamID,
		Payload:  resp.Encode(),
	}

	// Should not panic
	agent.processFrame(peerID, frame)
}

// Tests for DisplayName accessor
func TestAgent_DisplayName(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test")
	if err != nil {
		t.Fatalf("Create temp dir error: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := config.Default()
	cfg.Agent.DataDir = tmpDir
	cfg.Agent.DisplayName = "test-agent-name"

	agent, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	name := agent.DisplayName()
	if name != "test-agent-name" {
		t.Errorf("DisplayName() = %q, want %q", name, "test-agent-name")
	}
}

// Tests for DisplayName with fallback to ID
func TestAgent_DisplayName_Fallback(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test")
	if err != nil {
		t.Fatalf("Create temp dir error: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := config.Default()
	cfg.Agent.DataDir = tmpDir
	cfg.Agent.DisplayName = "" // No display name set

	agent, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	name := agent.DisplayName()
	// Should fall back to short agent ID
	if name == "" {
		t.Error("DisplayName() should not be empty when falling back to ID")
	}
	if name != agent.ID().ShortString() {
		t.Errorf("DisplayName() = %q, want %q", name, agent.ID().ShortString())
	}
}

func TestAgent_StopDuringStartupDelay(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test")
	if err != nil {
		t.Fatalf("Create temp dir error: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := config.Default()
	cfg.Agent.DataDir = tmpDir
	cfg.Agent.StartupDelay = 10 * time.Second

	a, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Start in a goroutine (will block on startup delay)
	startErrCh := make(chan error, 1)
	go func() {
		startErrCh <- a.Start()
	}()

	// Give Start() time to enter the delay
	time.Sleep(100 * time.Millisecond)

	// Stop during startup delay
	if err := a.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	// Start() should return ErrInterrupted
	startErr := <-startErrCh
	if !errors.Is(startErr, ErrInterrupted) {
		t.Errorf("Start() returned %v, want ErrInterrupted", startErr)
	}

	if a.IsRunning() {
		t.Error("Agent should not be running after interrupted start")
	}
}
