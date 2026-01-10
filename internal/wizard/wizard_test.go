package wizard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/postalsys/muti-metroo/internal/config"
)

func TestNew(t *testing.T) {
	w := New()
	if w == nil {
		t.Fatal("New() returned nil")
	}
	// Wizard struct now only has existingCfg field which starts as nil
	if w.existingCfg != nil {
		t.Error("New() returned wizard with non-nil existingCfg")
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		item     string
		expected bool
	}{
		{
			name:     "item exists",
			slice:    []string{"ingress", "transit", "exit"},
			item:     "transit",
			expected: true,
		},
		{
			name:     "item does not exist",
			slice:    []string{"ingress", "transit", "exit"},
			item:     "other",
			expected: false,
		},
		{
			name:     "empty slice",
			slice:    []string{},
			item:     "test",
			expected: false,
		},
		{
			name:     "single item match",
			slice:    []string{"only"},
			item:     "only",
			expected: true,
		},
		{
			name:     "single item no match",
			slice:    []string{"only"},
			item:     "other",
			expected: false,
		},
		{
			name:     "empty item",
			slice:    []string{"a", "", "b"},
			item:     "",
			expected: true,
		},
		{
			name:     "case sensitive",
			slice:    []string{"Ingress", "Transit"},
			item:     "ingress",
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := contains(tc.slice, tc.item)
			if result != tc.expected {
				t.Errorf("contains(%v, %q) = %v, want %v", tc.slice, tc.item, result, tc.expected)
			}
		})
	}
}

func TestBuildConfig(t *testing.T) {
	w := New()

	tests := []struct {
		name           string
		dataDir        string
		transport      string
		listenAddr     string
		listenPath     string
		tlsConfig      config.GlobalTLSConfig
		peers          []config.PeerConfig
		socks5Config  config.SOCKS5Config
		exitConfig    config.ExitConfig
		healthEnabled bool
		logLevel      string
		validate       func(*testing.T, *config.Config)
	}{
		{
			name:       "basic QUIC config",
			dataDir:    "/data",
			transport:  "quic",
			listenAddr: "0.0.0.0:4433",
			listenPath: "",
			tlsConfig: config.GlobalTLSConfig{
				Cert: "/certs/server.crt",
				Key:  "/certs/server.key",
			},
			peers:         nil,
			socks5Config:  config.SOCKS5Config{Enabled: false},
			exitConfig:    config.ExitConfig{Enabled: false},
			healthEnabled: true,
			logLevel:      "info",
			validate: func(t *testing.T, cfg *config.Config) {
				if cfg.Agent.DataDir != "/data" {
					t.Errorf("DataDir = %q, want %q", cfg.Agent.DataDir, "/data")
				}
				if cfg.Agent.LogLevel != "info" {
					t.Errorf("LogLevel = %q, want %q", cfg.Agent.LogLevel, "info")
				}
				if len(cfg.Listeners) != 1 {
					t.Fatalf("Listeners count = %d, want 1", len(cfg.Listeners))
				}
				if cfg.Listeners[0].Transport != "quic" {
					t.Errorf("Transport = %q, want %q", cfg.Listeners[0].Transport, "quic")
				}
				if cfg.Listeners[0].Address != "0.0.0.0:4433" {
					t.Errorf("Address = %q, want %q", cfg.Listeners[0].Address, "0.0.0.0:4433")
				}
				if cfg.Listeners[0].Path != "" {
					t.Errorf("Path = %q, want empty", cfg.Listeners[0].Path)
				}
				if !cfg.HTTP.Enabled {
					t.Error("HTTP.Enabled = false, want true")
				}
				if cfg.HTTP.Address != ":8080" {
					t.Errorf("HTTP.Address = %q, want %q", cfg.HTTP.Address, ":8080")
				}
			},
		},
		{
			name:       "HTTP/2 with path",
			dataDir:    "./mydata",
			transport:  "h2",
			listenAddr: "0.0.0.0:443",
			listenPath: "/mesh",
			tlsConfig: config.GlobalTLSConfig{
				Cert: "cert.pem",
				Key:  "key.pem",
			},
			peers:         nil,
			socks5Config:  config.SOCKS5Config{Enabled: false},
			exitConfig:    config.ExitConfig{Enabled: false},
			healthEnabled: false,
			logLevel:      "debug",
			validate: func(t *testing.T, cfg *config.Config) {
				if cfg.Listeners[0].Transport != "h2" {
					t.Errorf("Transport = %q, want %q", cfg.Listeners[0].Transport, "h2")
				}
				if cfg.Listeners[0].Path != "/mesh" {
					t.Errorf("Path = %q, want %q", cfg.Listeners[0].Path, "/mesh")
				}
				if cfg.Agent.LogLevel != "debug" {
					t.Errorf("LogLevel = %q, want %q", cfg.Agent.LogLevel, "debug")
				}
				if cfg.HTTP.Enabled {
					t.Error("HTTP.Enabled = true, want false")
				}
			},
		},
		{
			name:       "WebSocket with path",
			dataDir:    "/opt/muti",
			transport:  "ws",
			listenAddr: "0.0.0.0:8080",
			listenPath: "/ws",
			tlsConfig: config.GlobalTLSConfig{
				Cert: "ws.crt",
				Key:  "ws.key",
			},
			peers:         nil,
			socks5Config:  config.SOCKS5Config{Enabled: false},
			exitConfig:    config.ExitConfig{Enabled: false},
			healthEnabled: true,
			logLevel:      "warn",
			validate: func(t *testing.T, cfg *config.Config) {
				if cfg.Listeners[0].Transport != "ws" {
					t.Errorf("Transport = %q, want %q", cfg.Listeners[0].Transport, "ws")
				}
				if cfg.Listeners[0].Path != "/ws" {
					t.Errorf("Path = %q, want %q", cfg.Listeners[0].Path, "/ws")
				}
			},
		},
		{
			name:       "with SOCKS5 enabled",
			dataDir:    "/data",
			transport:  "quic",
			listenAddr: "0.0.0.0:4433",
			listenPath: "",
			tlsConfig:  config.GlobalTLSConfig{Cert: "c", Key: "k"},
			peers:      nil,
			socks5Config: config.SOCKS5Config{
				Enabled:        true,
				Address:        "127.0.0.1:1080",
				MaxConnections: 500,
				Auth: config.SOCKS5AuthConfig{
					Enabled: true,
					Users: []config.SOCKS5UserConfig{
						{Username: "user1", Password: "pass1"},
					},
				},
			},
			exitConfig:    config.ExitConfig{Enabled: false},
			healthEnabled: false,
			logLevel:      "info",
			validate: func(t *testing.T, cfg *config.Config) {
				if !cfg.SOCKS5.Enabled {
					t.Error("SOCKS5.Enabled = false, want true")
				}
				if cfg.SOCKS5.Address != "127.0.0.1:1080" {
					t.Errorf("SOCKS5.Address = %q, want %q", cfg.SOCKS5.Address, "127.0.0.1:1080")
				}
				if cfg.SOCKS5.MaxConnections != 500 {
					t.Errorf("SOCKS5.MaxConnections = %d, want 500", cfg.SOCKS5.MaxConnections)
				}
				if !cfg.SOCKS5.Auth.Enabled {
					t.Error("SOCKS5.Auth.Enabled = false, want true")
				}
				if len(cfg.SOCKS5.Auth.Users) != 1 {
					t.Fatalf("SOCKS5.Auth.Users count = %d, want 1", len(cfg.SOCKS5.Auth.Users))
				}
				if cfg.SOCKS5.Auth.Users[0].Username != "user1" {
					t.Errorf("Username = %q, want %q", cfg.SOCKS5.Auth.Users[0].Username, "user1")
				}
			},
		},
		{
			name:       "with exit enabled",
			dataDir:    "/data",
			transport:  "quic",
			listenAddr: "0.0.0.0:4433",
			listenPath: "",
			tlsConfig:  config.GlobalTLSConfig{Cert: "c", Key: "k"},
			peers:      nil,
			socks5Config: config.SOCKS5Config{Enabled: false},
			exitConfig: config.ExitConfig{
				Enabled: true,
				Routes:  []string{"0.0.0.0/0", "10.0.0.0/8"},
				DNS: config.DNSConfig{
					Servers: []string{"8.8.8.8:53"},
					Timeout: 10 * time.Second,
				},
			},
			healthEnabled: false,
			logLevel:      "info",
			validate: func(t *testing.T, cfg *config.Config) {
				if !cfg.Exit.Enabled {
					t.Error("Exit.Enabled = false, want true")
				}
				if len(cfg.Exit.Routes) != 2 {
					t.Fatalf("Exit.Routes count = %d, want 2", len(cfg.Exit.Routes))
				}
				if cfg.Exit.Routes[0] != "0.0.0.0/0" {
					t.Errorf("Exit.Routes[0] = %q, want %q", cfg.Exit.Routes[0], "0.0.0.0/0")
				}
				if len(cfg.Exit.DNS.Servers) != 1 {
					t.Fatalf("Exit.DNS.Servers count = %d, want 1", len(cfg.Exit.DNS.Servers))
				}
			},
		},
		{
			name:       "with peers",
			dataDir:    "/data",
			transport:  "quic",
			listenAddr: "0.0.0.0:4433",
			listenPath: "",
			tlsConfig:  config.GlobalTLSConfig{Cert: "c", Key: "k"},
			peers: []config.PeerConfig{
				{
					ID:        "peer1",
					Transport: "quic",
					Address:   "peer1.example.com:4433",
					TLS:       config.TLSConfig{InsecureSkipVerify: true},
				},
				{
					ID:        "peer2",
					Transport: "h2",
					Address:   "peer2.example.com:443",
					Path:      "/mesh",
				},
			},
			socks5Config:  config.SOCKS5Config{Enabled: false},
			exitConfig:    config.ExitConfig{Enabled: false},
			healthEnabled: false,
			logLevel:      "info",
			validate: func(t *testing.T, cfg *config.Config) {
				if len(cfg.Peers) != 2 {
					t.Fatalf("Peers count = %d, want 2", len(cfg.Peers))
				}
				if cfg.Peers[0].ID != "peer1" {
					t.Errorf("Peers[0].ID = %q, want %q", cfg.Peers[0].ID, "peer1")
				}
				if cfg.Peers[0].Address != "peer1.example.com:4433" {
					t.Errorf("Peers[0].Address = %q, want %q", cfg.Peers[0].Address, "peer1.example.com:4433")
				}
				if !cfg.Peers[0].TLS.InsecureSkipVerify {
					t.Error("Peers[0].TLS.InsecureSkipVerify = false, want true")
				}
				if cfg.Peers[1].Path != "/mesh" {
					t.Errorf("Peers[1].Path = %q, want %q", cfg.Peers[1].Path, "/mesh")
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := w.buildConfig(
				tc.dataDir, "", tc.transport, tc.listenAddr, tc.listenPath,
				tc.tlsConfig, tc.peers, tc.socks5Config, tc.exitConfig,
				tc.healthEnabled, tc.logLevel,
				config.ShellConfig{}, config.FileTransferConfig{}, config.ManagementConfig{},
			)

			if cfg == nil {
				t.Fatal("buildConfig returned nil")
			}

			tc.validate(t, cfg)
		})
	}
}

func TestWriteConfig(t *testing.T) {
	w := New()

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "wizard_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := config.Default()
	cfg.Agent.DataDir = "/data"
	cfg.Agent.LogLevel = "debug"
	cfg.SOCKS5.Enabled = true
	cfg.SOCKS5.Address = "127.0.0.1:1080"

	configPath := filepath.Join(tmpDir, "config.yaml")

	if err := w.writeConfig(cfg, configPath); err != nil {
		t.Fatalf("writeConfig failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("Config file was not created")
	}

	// Read and verify content
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}

	content := string(data)

	// Check header comment
	if !strings.HasPrefix(content, "# Muti Metroo Configuration") {
		t.Error("Config file missing header comment")
	}

	// Check key values are present
	if !strings.Contains(content, "data_dir: /data") {
		t.Error("Config file missing data_dir value")
	}
	if !strings.Contains(content, "log_level: debug") {
		t.Error("Config file missing log_level value")
	}
	if !strings.Contains(content, "enabled: true") {
		t.Error("Config file missing enabled value")
	}
	if !strings.Contains(content, "address: 127.0.0.1:1080") {
		t.Error("Config file missing SOCKS5 address")
	}
}

func TestWriteConfigCreatesDirectory(t *testing.T) {
	w := New()

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "wizard_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Path with non-existent subdirectory
	configPath := filepath.Join(tmpDir, "subdir", "nested", "config.yaml")

	cfg := config.Default()

	if err := w.writeConfig(cfg, configPath); err != nil {
		t.Fatalf("writeConfig failed: %v", err)
	}

	// Verify directory was created
	dir := filepath.Dir(configPath)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("writeConfig did not create parent directories")
	}

	// Verify file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("Config file was not created")
	}
}

func TestResultStruct(t *testing.T) {
	// Test that Result struct fields are properly initialized
	result := &Result{
		Config:           config.Default(),
		ConfigPath:       "/path/to/config.yaml",
		DataDir:          "/data",
		CertsDir:         "/data/certs",
		ServiceInstalled: true,
	}

	if result.Config == nil {
		t.Error("Result.Config is nil")
	}
	if result.ConfigPath != "/path/to/config.yaml" {
		t.Errorf("Result.ConfigPath = %q, want %q", result.ConfigPath, "/path/to/config.yaml")
	}
	if result.DataDir != "/data" {
		t.Errorf("Result.DataDir = %q, want %q", result.DataDir, "/data")
	}
	if result.CertsDir != "/data/certs" {
		t.Errorf("Result.CertsDir = %q, want %q", result.CertsDir, "/data/certs")
	}
	if !result.ServiceInstalled {
		t.Error("Result.ServiceInstalled = false, want true")
	}
}

func TestBuildConfigLogFormat(t *testing.T) {
	w := New()

	cfg := w.buildConfig(
		"/data", "", "quic", "0.0.0.0:4433", "",
		config.GlobalTLSConfig{Cert: "c", Key: "k"},
		nil, config.SOCKS5Config{}, config.ExitConfig{},
		false, "info",
		config.ShellConfig{}, config.FileTransferConfig{}, config.ManagementConfig{},
	)

	// Verify LogFormat is always set to "text"
	if cfg.Agent.LogFormat != "text" {
		t.Errorf("Agent.LogFormat = %q, want %q", cfg.Agent.LogFormat, "text")
	}
}

func TestBuildConfigDefaults(t *testing.T) {
	w := New()

	cfg := w.buildConfig(
		"/data", "", "quic", "0.0.0.0:4433", "",
		config.GlobalTLSConfig{Cert: "c", Key: "k"},
		nil, config.SOCKS5Config{}, config.ExitConfig{},
		false, "info",
		config.ShellConfig{}, config.FileTransferConfig{}, config.ManagementConfig{},
	)

	// Verify default values from config.Default() are preserved
	if cfg.Routing.MaxHops == 0 {
		t.Error("Routing.MaxHops should have default value")
	}
	if cfg.Limits.BufferSize == 0 {
		t.Error("Limits.BufferSize should have default value")
	}
	if cfg.Connections.Timeout == 0 {
		t.Error("Connections.Timeout should have default value")
	}
}

func TestTestPeerConnectivity(t *testing.T) {
	w := New()

	tests := []struct {
		name        string
		peer        config.PeerConfig
		expectError bool
		errorContains string
	}{
		{
			name: "connection refused",
			peer: config.PeerConfig{
				Transport: "quic",
				Address:   "127.0.0.1:59999", // Non-existent port
				TLS:       config.TLSConfig{InsecureSkipVerify: true},
			},
			expectError: true,
			errorContains: "", // Can be timeout or refused
		},
		{
			name: "invalid address",
			peer: config.PeerConfig{
				Transport: "quic",
				Address:   "invalid-host-that-does-not-exist.local:4433",
				TLS:       config.TLSConfig{InsecureSkipVerify: true},
			},
			expectError: true,
			errorContains: "", // DNS or connection error
		},
		{
			name: "h2 without path uses default",
			peer: config.PeerConfig{
				Transport: "h2",
				Address:   "127.0.0.1:59999",
				Path:      "", // Should default to /mesh
				TLS:       config.TLSConfig{InsecureSkipVerify: true},
			},
			expectError: true,
			errorContains: "", // Connection error expected
		},
		{
			name: "ws without path uses default",
			peer: config.PeerConfig{
				Transport: "ws",
				Address:   "127.0.0.1:59999",
				Path:      "", // Should default to /mesh
				TLS:       config.TLSConfig{InsecureSkipVerify: true},
			},
			expectError: true,
			errorContains: "", // Connection error expected
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := w.testPeerConnectivity(tc.peer)
			if tc.expectError {
				if err == nil {
					t.Error("Expected error but got nil")
				} else if tc.errorContains != "" && !strings.Contains(err.Error(), tc.errorContains) {
					t.Errorf("Error %q does not contain %q", err.Error(), tc.errorContains)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

// TestTestPeerConnectivityLive tests against real listeners.
// Skip if testbed is not running.
func TestTestPeerConnectivityLive(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping live connectivity test in short mode")
	}

	// Check if Agent B (Docker) is reachable
	w := New()
	peer := config.PeerConfig{
		Transport: "quic",
		Address:   "127.0.0.1:4433",
		TLS: config.TLSConfig{
			CA:   "../../testbed/certs/ca.crt",
			Cert: "../../testbed/certs/agent-a.crt",
			Key:  "../../testbed/certs/agent-a.key",
		},
	}

	err := w.testPeerConnectivity(peer)
	if err != nil {
		t.Skipf("Agent B not available (testbed not running?): %v", err)
	}

	// If we get here, Agent B is running and the test passed
	t.Log("Successfully connected to Agent B")
}

func TestTransportIndex(t *testing.T) {
	tests := []struct {
		name      string
		transport string
		expected  int
	}{
		{"quic", "quic", 0},
		{"h2", "h2", 1},
		{"ws", "ws", 2},
		{"unknown defaults to 0", "unknown", 0},
		{"empty defaults to 0", "", 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := transportIndex(tc.transport)
			if result != tc.expected {
				t.Errorf("transportIndex(%q) = %d, want %d", tc.transport, result, tc.expected)
			}
		})
	}
}

func TestNormalizeHexKey(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"no prefix", "abcd1234", "abcd1234"},
		{"0x prefix", "0xabcd1234", "abcd1234"},
		{"0X prefix", "0Xabcd1234", "abcd1234"},
		{"whitespace", "  abcd1234  ", "abcd1234"},
		{"prefix and whitespace", "  0xabcd1234  ", "abcd1234"},
		{"empty string", "", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := normalizeHexKey(tc.input)
			if result != tc.expected {
				t.Errorf("normalizeHexKey(%q) = %q, want %q", tc.input, result, tc.expected)
			}
		})
	}
}
