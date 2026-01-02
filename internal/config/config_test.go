package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefault(t *testing.T) {
	cfg := Default()

	// Check essential defaults
	if cfg.Agent.ID != "auto" {
		t.Errorf("Agent.ID = %s, want auto", cfg.Agent.ID)
	}
	if cfg.Agent.DataDir != "./data" {
		t.Errorf("Agent.DataDir = %s, want ./data", cfg.Agent.DataDir)
	}
	if cfg.Agent.LogLevel != "info" {
		t.Errorf("Agent.LogLevel = %s, want info", cfg.Agent.LogLevel)
	}
	if cfg.SOCKS5.Address != "127.0.0.1:1080" {
		t.Errorf("SOCKS5.Address = %s, want 127.0.0.1:1080", cfg.SOCKS5.Address)
	}
	if cfg.Routing.MaxHops != 16 {
		t.Errorf("Routing.MaxHops = %d, want 16", cfg.Routing.MaxHops)
	}
	if cfg.Limits.BufferSize != 262144 {
		t.Errorf("Limits.BufferSize = %d, want 262144", cfg.Limits.BufferSize)
	}
}

func TestParse_ValidConfig(t *testing.T) {
	yamlConfig := `
agent:
  id: "auto"
  data_dir: "./data"
  log_level: "debug"
  log_format: "json"

listeners:
  - transport: quic
    address: "0.0.0.0:4433"
    tls:
      cert: "./certs/agent.crt"
      key: "./certs/agent.key"

peers:
  - id: "abc123def456789012345678901234ab"
    transport: quic
    address: "192.168.1.50:4433"

socks5:
  enabled: true
  address: "127.0.0.1:1080"
  max_connections: 500

exit:
  enabled: true
  routes:
    - "10.0.0.0/8"
    - "192.168.0.0/16"
  dns:
    servers:
      - "8.8.8.8:53"
    timeout: 10s

routing:
  advertise_interval: 3m
  route_ttl: 6m
  max_hops: 20

limits:
  max_streams_per_peer: 500
  max_streams_total: 5000
  buffer_size: 131072
`

	cfg, err := Parse([]byte(yamlConfig))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	// Verify parsed values
	if cfg.Agent.LogLevel != "debug" {
		t.Errorf("Agent.LogLevel = %s, want debug", cfg.Agent.LogLevel)
	}
	if cfg.Agent.LogFormat != "json" {
		t.Errorf("Agent.LogFormat = %s, want json", cfg.Agent.LogFormat)
	}
	if len(cfg.Listeners) != 1 {
		t.Errorf("len(Listeners) = %d, want 1", len(cfg.Listeners))
	}
	if cfg.Listeners[0].Transport != "quic" {
		t.Errorf("Listeners[0].Transport = %s, want quic", cfg.Listeners[0].Transport)
	}
	if len(cfg.Peers) != 1 {
		t.Errorf("len(Peers) = %d, want 1", len(cfg.Peers))
	}
	if cfg.SOCKS5.Enabled != true {
		t.Error("SOCKS5.Enabled = false, want true")
	}
	if cfg.SOCKS5.MaxConnections != 500 {
		t.Errorf("SOCKS5.MaxConnections = %d, want 500", cfg.SOCKS5.MaxConnections)
	}
	if len(cfg.Exit.Routes) != 2 {
		t.Errorf("len(Exit.Routes) = %d, want 2", len(cfg.Exit.Routes))
	}
	if cfg.Routing.MaxHops != 20 {
		t.Errorf("Routing.MaxHops = %d, want 20", cfg.Routing.MaxHops)
	}
	if cfg.Routing.AdvertiseInterval != 3*time.Minute {
		t.Errorf("Routing.AdvertiseInterval = %v, want 3m", cfg.Routing.AdvertiseInterval)
	}
	if cfg.Limits.MaxStreamsPerPeer != 500 {
		t.Errorf("Limits.MaxStreamsPerPeer = %d, want 500", cfg.Limits.MaxStreamsPerPeer)
	}
}

func TestParse_MinimalConfig(t *testing.T) {
	yamlConfig := `
agent:
  data_dir: "./data"
`

	cfg, err := Parse([]byte(yamlConfig))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	// Should use defaults for unspecified fields
	if cfg.Agent.LogLevel != "info" {
		t.Errorf("Agent.LogLevel = %s, want info (default)", cfg.Agent.LogLevel)
	}
	if cfg.Routing.MaxHops != 16 {
		t.Errorf("Routing.MaxHops = %d, want 16 (default)", cfg.Routing.MaxHops)
	}
}

func TestParse_InvalidYAML(t *testing.T) {
	yamlConfig := `
agent:
  data_dir: "./data"
  invalid yaml here [
`

	_, err := Parse([]byte(yamlConfig))
	if err == nil {
		t.Error("Parse() should fail for invalid YAML")
	}
}

func TestParse_ValidationErrors(t *testing.T) {
	tests := []struct {
		name      string
		yaml      string
		wantError string
	}{
		{
			name: "invalid log level",
			yaml: `
agent:
  data_dir: "./data"
  log_level: "invalid"
`,
			wantError: "invalid log_level",
		},
		{
			name: "invalid log format",
			yaml: `
agent:
  data_dir: "./data"
  log_format: "invalid"
`,
			wantError: "invalid log_format",
		},
		{
			name: "listener missing address",
			yaml: `
agent:
  data_dir: "./data"
listeners:
  - transport: quic
    tls:
      cert: "cert.pem"
      key: "key.pem"
`,
			wantError: "address is required",
		},
		{
			name: "listener invalid transport",
			yaml: `
agent:
  data_dir: "./data"
listeners:
  - transport: invalid
    address: "0.0.0.0:4433"
    tls:
      cert: "cert.pem"
      key: "key.pem"
`,
			wantError: "invalid transport",
		},
		{
			name: "listener missing TLS",
			yaml: `
agent:
  data_dir: "./data"
listeners:
  - transport: quic
    address: "0.0.0.0:4433"
`,
			wantError: "tls certificate and key are required",
		},
		{
			name: "h2 listener missing path",
			yaml: `
agent:
  data_dir: "./data"
listeners:
  - transport: h2
    address: "0.0.0.0:8443"
    tls:
      cert: "cert.pem"
      key: "key.pem"
`,
			wantError: "path is required",
		},
		{
			name: "peer missing id",
			yaml: `
agent:
  data_dir: "./data"
peers:
  - transport: quic
    address: "192.168.1.50:4433"
`,
			wantError: "id is required",
		},
		{
			name: "invalid CIDR route",
			yaml: `
agent:
  data_dir: "./data"
exit:
  enabled: true
  routes:
    - "invalid"
`,
			wantError: "invalid CIDR",
		},
		{
			name: "max_hops too low",
			yaml: `
agent:
  data_dir: "./data"
routing:
  max_hops: 0
`,
			wantError: "max_hops must be between 1 and 255",
		},
		{
			name: "max_hops too high",
			yaml: `
agent:
  data_dir: "./data"
routing:
  max_hops: 256
`,
			wantError: "max_hops must be between 1 and 255",
		},
		{
			name: "buffer_size too small",
			yaml: `
agent:
  data_dir: "./data"
limits:
  buffer_size: 512
`,
			wantError: "buffer_size must be at least 1024",
		},
		{
			name: "max_streams_total less than per_peer",
			yaml: `
agent:
  data_dir: "./data"
limits:
  max_streams_per_peer: 1000
  max_streams_total: 500
`,
			wantError: "max_streams_total must be >= max_streams_per_peer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.yaml))
			if err == nil {
				t.Error("Parse() should fail")
				return
			}
			if !strings.Contains(err.Error(), tt.wantError) {
				t.Errorf("Error = %v, want to contain %q", err, tt.wantError)
			}
		})
	}
}

func TestParse_EnvVarSubstitution(t *testing.T) {
	// Set test environment variables
	os.Setenv("TEST_DATA_DIR", "/custom/data")
	os.Setenv("TEST_PEER_ID", "abc123def456789012345678901234ab")
	os.Setenv("TEST_PEER_ADDR", "10.0.0.1:4433")
	defer func() {
		os.Unsetenv("TEST_DATA_DIR")
		os.Unsetenv("TEST_PEER_ID")
		os.Unsetenv("TEST_PEER_ADDR")
	}()

	yamlConfig := `
agent:
  data_dir: "${TEST_DATA_DIR}"

peers:
  - id: "${TEST_PEER_ID}"
    transport: quic
    address: "$TEST_PEER_ADDR"
`

	cfg, err := Parse([]byte(yamlConfig))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if cfg.Agent.DataDir != "/custom/data" {
		t.Errorf("Agent.DataDir = %s, want /custom/data", cfg.Agent.DataDir)
	}
	if cfg.Peers[0].ID != "abc123def456789012345678901234ab" {
		t.Errorf("Peers[0].ID = %s, want abc123def456789012345678901234ab", cfg.Peers[0].ID)
	}
	if cfg.Peers[0].Address != "10.0.0.1:4433" {
		t.Errorf("Peers[0].Address = %s, want 10.0.0.1:4433", cfg.Peers[0].Address)
	}
}

func TestParse_EnvVarDefaultValue(t *testing.T) {
	// Ensure the variable is NOT set
	os.Unsetenv("NONEXISTENT_VAR")

	yamlConfig := `
agent:
  data_dir: "${NONEXISTENT_VAR:-/default/path}"
`

	cfg, err := Parse([]byte(yamlConfig))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if cfg.Agent.DataDir != "/default/path" {
		t.Errorf("Agent.DataDir = %s, want /default/path", cfg.Agent.DataDir)
	}
}

func TestParse_EnvVarNotFound(t *testing.T) {
	os.Unsetenv("NONEXISTENT_VAR")

	yamlConfig := `
agent:
  data_dir: "${NONEXISTENT_VAR}"
`

	cfg, err := Parse([]byte(yamlConfig))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	// Should keep the original placeholder if not found
	if cfg.Agent.DataDir != "${NONEXISTENT_VAR}" {
		t.Errorf("Agent.DataDir = %s, want ${NONEXISTENT_VAR}", cfg.Agent.DataDir)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("Load() should fail for nonexistent file")
	}
}

func TestLoad_ValidFile(t *testing.T) {
	// Create temp config file
	tmpDir, err := os.MkdirTemp("", "muti-metroo-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.yaml")
	configContent := `
agent:
  data_dir: "./data"
  log_level: "debug"
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Agent.LogLevel != "debug" {
		t.Errorf("Agent.LogLevel = %s, want debug", cfg.Agent.LogLevel)
	}
}

func TestConfig_Validate_MissingDataDir(t *testing.T) {
	cfg := Default()
	cfg.Agent.DataDir = ""

	err := cfg.Validate()
	if err == nil {
		t.Error("Validate() should fail with empty data_dir")
	}
}

func TestConfig_Validate_SOCKS5EnabledNoAddress(t *testing.T) {
	cfg := Default()
	cfg.SOCKS5.Enabled = true
	cfg.SOCKS5.Address = ""

	err := cfg.Validate()
	if err == nil {
		t.Error("Validate() should fail when SOCKS5 enabled without address")
	}
}

func TestIsValidCIDR(t *testing.T) {
	tests := []struct {
		cidr  string
		valid bool
	}{
		{"10.0.0.0/8", true},
		{"192.168.0.0/16", true},
		{"172.16.0.0/12", true},
		{"0.0.0.0/0", true},
		{"10.5.3.0/24", true},
		{"2001:db8::/32", true},
		{"invalid", false},
		{"10.0.0.0", false},
		{"/8", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.cidr, func(t *testing.T) {
			if got := isValidCIDR(tt.cidr); got != tt.valid {
				t.Errorf("isValidCIDR(%q) = %v, want %v", tt.cidr, got, tt.valid)
			}
		})
	}
}

func TestConfig_String(t *testing.T) {
	cfg := Default()
	s := cfg.String()

	// Should contain key fields
	if !strings.Contains(s, "agent") {
		t.Error("String() should contain 'agent'")
	}
	if !strings.Contains(s, "socks5") {
		t.Error("String() should contain 'socks5'")
	}
}

func TestDurationParsing(t *testing.T) {
	yamlConfig := `
agent:
  data_dir: "./data"
routing:
  advertise_interval: 120s
  route_ttl: 5m
connections:
  idle_threshold: 30s
  timeout: 1m30s
`

	cfg, err := Parse([]byte(yamlConfig))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if cfg.Routing.AdvertiseInterval != 120*time.Second {
		t.Errorf("AdvertiseInterval = %v, want 2m", cfg.Routing.AdvertiseInterval)
	}
	if cfg.Routing.RouteTTL != 5*time.Minute {
		t.Errorf("RouteTTL = %v, want 5m", cfg.Routing.RouteTTL)
	}
	if cfg.Connections.Timeout != 90*time.Second {
		t.Errorf("Timeout = %v, want 1m30s", cfg.Connections.Timeout)
	}
}

func TestListenerConfig_WebSocket(t *testing.T) {
	yamlConfig := `
agent:
  data_dir: "./data"
listeners:
  - transport: ws
    address: "0.0.0.0:443"
    path: "/mesh"
    tls:
      cert: "./certs/agent.crt"
      key: "./certs/agent.key"
`

	cfg, err := Parse([]byte(yamlConfig))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(cfg.Listeners) != 1 {
		t.Fatalf("len(Listeners) = %d, want 1", len(cfg.Listeners))
	}
	if cfg.Listeners[0].Transport != "ws" {
		t.Errorf("Transport = %s, want ws", cfg.Listeners[0].Transport)
	}
	if cfg.Listeners[0].Path != "/mesh" {
		t.Errorf("Path = %s, want /mesh", cfg.Listeners[0].Path)
	}
}

func TestPeerConfig_WithProxy(t *testing.T) {
	os.Setenv("PROXY_USER", "testuser")
	os.Setenv("PROXY_PASS", "testpass")
	defer func() {
		os.Unsetenv("PROXY_USER")
		os.Unsetenv("PROXY_PASS")
	}()

	yamlConfig := `
agent:
  data_dir: "./data"
peers:
  - id: "abc123def456789012345678901234ab"
    transport: ws
    address: "wss://relay.example.com:443/mesh"
    proxy: "http://proxy.corp.local:8080"
    proxy_auth:
      username: "${PROXY_USER}"
      password: "${PROXY_PASS}"
`

	cfg, err := Parse([]byte(yamlConfig))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	peer := cfg.Peers[0]
	if peer.Proxy != "http://proxy.corp.local:8080" {
		t.Errorf("Proxy = %s, want http://proxy.corp.local:8080", peer.Proxy)
	}
	if peer.ProxyAuth.Username != "testuser" {
		t.Errorf("ProxyAuth.Username = %s, want testuser", peer.ProxyAuth.Username)
	}
	if peer.ProxyAuth.Password != "testpass" {
		t.Errorf("ProxyAuth.Password = %s, want testpass", peer.ProxyAuth.Password)
	}
}

func TestSOCKS5AuthConfig(t *testing.T) {
	yamlConfig := `
agent:
  data_dir: "./data"
socks5:
  enabled: true
  address: "127.0.0.1:1080"
  auth:
    enabled: true
    users:
      - username: "user1"
        password: "pass1"
      - username: "user2"
        password: "pass2"
`

	cfg, err := Parse([]byte(yamlConfig))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if !cfg.SOCKS5.Auth.Enabled {
		t.Error("SOCKS5.Auth.Enabled = false, want true")
	}
	if len(cfg.SOCKS5.Auth.Users) != 2 {
		t.Fatalf("len(SOCKS5.Auth.Users) = %d, want 2", len(cfg.SOCKS5.Auth.Users))
	}
	if cfg.SOCKS5.Auth.Users[0].Username != "user1" {
		t.Errorf("Users[0].Username = %s, want user1", cfg.SOCKS5.Auth.Users[0].Username)
	}
}

func TestTLSConfig_InlinePEM(t *testing.T) {
	// Create a temp directory with cert files for file path testing
	tmpDir := t.TempDir()
	certFile := filepath.Join(tmpDir, "cert.pem")
	keyFile := filepath.Join(tmpDir, "key.pem")

	certContent := "-----BEGIN CERTIFICATE-----\nMIIB...\n-----END CERTIFICATE-----\n"
	keyContent := "-----BEGIN PRIVATE KEY-----\nMIIE...\n-----END PRIVATE KEY-----\n"

	os.WriteFile(certFile, []byte(certContent), 0644)
	os.WriteFile(keyFile, []byte(keyContent), 0600)

	tests := []struct {
		name     string
		tls      TLSConfig
		wantCert string
		wantKey  string
	}{
		{
			name: "inline PEM takes precedence",
			tls: TLSConfig{
				Cert:    certFile,
				Key:     keyFile,
				CertPEM: "inline-cert-pem",
				KeyPEM:  "inline-key-pem",
			},
			wantCert: "inline-cert-pem",
			wantKey:  "inline-key-pem",
		},
		{
			name: "file path fallback",
			tls: TLSConfig{
				Cert: certFile,
				Key:  keyFile,
			},
			wantCert: certContent,
			wantKey:  keyContent,
		},
		{
			name: "inline PEM only",
			tls: TLSConfig{
				CertPEM: "cert-only-inline",
				KeyPEM:  "key-only-inline",
			},
			wantCert: "cert-only-inline",
			wantKey:  "key-only-inline",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			certPEM, err := tt.tls.GetCertPEM()
			if err != nil {
				t.Fatalf("GetCertPEM() error = %v", err)
			}
			if string(certPEM) != tt.wantCert {
				t.Errorf("GetCertPEM() = %q, want %q", string(certPEM), tt.wantCert)
			}

			keyPEM, err := tt.tls.GetKeyPEM()
			if err != nil {
				t.Fatalf("GetKeyPEM() error = %v", err)
			}
			if string(keyPEM) != tt.wantKey {
				t.Errorf("GetKeyPEM() = %q, want %q", string(keyPEM), tt.wantKey)
			}
		})
	}
}

func TestTLSConfig_HasCertAndKey(t *testing.T) {
	tests := []struct {
		name     string
		tls      TLSConfig
		hasCert  bool
		hasKey   bool
	}{
		{
			name:    "empty",
			tls:     TLSConfig{},
			hasCert: false,
			hasKey:  false,
		},
		{
			name:    "file paths only",
			tls:     TLSConfig{Cert: "cert.pem", Key: "key.pem"},
			hasCert: true,
			hasKey:  true,
		},
		{
			name:    "inline PEM only",
			tls:     TLSConfig{CertPEM: "cert", KeyPEM: "key"},
			hasCert: true,
			hasKey:  true,
		},
		{
			name:    "mixed",
			tls:     TLSConfig{Cert: "cert.pem", KeyPEM: "key"},
			hasCert: true,
			hasKey:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.tls.HasCert(); got != tt.hasCert {
				t.Errorf("HasCert() = %v, want %v", got, tt.hasCert)
			}
			if got := tt.tls.HasKey(); got != tt.hasKey {
				t.Errorf("HasKey() = %v, want %v", got, tt.hasKey)
			}
		})
	}
}

func TestParse_InlinePEM(t *testing.T) {
	yamlConfig := `
agent:
  data_dir: "./data"
listeners:
  - transport: quic
    address: "0.0.0.0:4433"
    tls:
      cert_pem: |
        -----BEGIN CERTIFICATE-----
        MIIBtest
        -----END CERTIFICATE-----
      key_pem: |
        -----BEGIN PRIVATE KEY-----
        MIIEtest
        -----END PRIVATE KEY-----
`

	cfg, err := Parse([]byte(yamlConfig))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(cfg.Listeners) != 1 {
		t.Fatalf("len(Listeners) = %d, want 1", len(cfg.Listeners))
	}

	tls := cfg.Listeners[0].TLS
	if !tls.HasCert() {
		t.Error("HasCert() = false, want true")
	}
	if !tls.HasKey() {
		t.Error("HasKey() = false, want true")
	}
	if !strings.Contains(tls.CertPEM, "BEGIN CERTIFICATE") {
		t.Errorf("CertPEM should contain BEGIN CERTIFICATE, got %q", tls.CertPEM)
	}
	if !strings.Contains(tls.KeyPEM, "BEGIN PRIVATE KEY") {
		t.Errorf("KeyPEM should contain BEGIN PRIVATE KEY, got %q", tls.KeyPEM)
	}
}

func TestListenerConfig_PlainTextWebSocket(t *testing.T) {
	// Test valid plaintext WebSocket configuration (for reverse proxy mode)
	yamlConfig := `
agent:
  data_dir: "./data"
listeners:
  - transport: ws
    address: "127.0.0.1:8080"
    path: "/mesh"
    plaintext: true
`

	cfg, err := Parse([]byte(yamlConfig))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(cfg.Listeners) != 1 {
		t.Fatalf("len(Listeners) = %d, want 1", len(cfg.Listeners))
	}
	listener := cfg.Listeners[0]
	if listener.Transport != "ws" {
		t.Errorf("Transport = %s, want ws", listener.Transport)
	}
	if listener.Address != "127.0.0.1:8080" {
		t.Errorf("Address = %s, want 127.0.0.1:8080", listener.Address)
	}
	if listener.Path != "/mesh" {
		t.Errorf("Path = %s, want /mesh", listener.Path)
	}
	if !listener.PlainText {
		t.Error("PlainText = false, want true")
	}
}

func TestListenerConfig_PlainTextOnlyForWS(t *testing.T) {
	// Test that plaintext is only allowed for ws transport
	tests := []struct {
		name      string
		transport string
		wantError bool
	}{
		{"ws plaintext allowed", "ws", false},
		{"quic plaintext not allowed", "quic", true},
		{"h2 plaintext not allowed", "h2", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yamlConfig := `
agent:
  data_dir: "./data"
listeners:
  - transport: ` + tt.transport + `
    address: "127.0.0.1:8080"
    path: "/mesh"
    plaintext: true
`

			_, err := Parse([]byte(yamlConfig))
			if tt.wantError {
				if err == nil {
					t.Errorf("Parse() should fail for %s with plaintext", tt.transport)
				} else if !strings.Contains(err.Error(), "plaintext mode is only supported for ws") {
					t.Errorf("Error = %v, want to contain 'plaintext mode is only supported for ws'", err)
				}
			} else {
				if err != nil {
					t.Errorf("Parse() error = %v, want nil", err)
				}
			}
		})
	}
}

func TestListenerConfig_PlainTextNoTLSRequired(t *testing.T) {
	// Test that plaintext WS does not require TLS config
	yamlConfig := `
agent:
  data_dir: "./data"
listeners:
  - transport: ws
    address: "127.0.0.1:8080"
    path: "/mesh"
    plaintext: true
`

	cfg, err := Parse([]byte(yamlConfig))
	if err != nil {
		t.Fatalf("Parse() error = %v, plaintext ws should not require TLS", err)
	}

	// Verify TLS is not set
	if cfg.Listeners[0].TLS.HasCert() || cfg.Listeners[0].TLS.HasKey() {
		t.Error("Plaintext WS should work without TLS config")
	}
}

func TestListenerConfig_NonPlainTextRequiresTLS(t *testing.T) {
	// Test that non-plaintext WS still requires TLS
	yamlConfig := `
agent:
  data_dir: "./data"
listeners:
  - transport: ws
    address: "127.0.0.1:8080"
    path: "/mesh"
`

	_, err := Parse([]byte(yamlConfig))
	if err == nil {
		t.Error("Parse() should fail for ws without plaintext or TLS")
	}
	if !strings.Contains(err.Error(), "tls certificate and key are required") {
		t.Errorf("Error = %v, want to contain 'tls certificate and key are required'", err)
	}
}

func TestManagementConfig_ValidKeys(t *testing.T) {
	// Valid 64-character hex strings (32 bytes)
	validPublicKey := "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"
	validPrivateKey := "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"

	yamlConfig := `
agent:
  data_dir: "./data"

management:
  public_key: "` + validPublicKey + `"
  private_key: "` + validPrivateKey + `"
`

	cfg, err := Parse([]byte(yamlConfig))
	if err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}

	// Test HasManagementKey
	if !cfg.HasManagementKey() {
		t.Error("HasManagementKey() = false, want true")
	}

	// Test CanDecryptManagement
	if !cfg.CanDecryptManagement() {
		t.Error("CanDecryptManagement() = false, want true")
	}

	// Test GetManagementPublicKey
	pubKey, err := cfg.GetManagementPublicKey()
	if err != nil {
		t.Fatalf("GetManagementPublicKey() failed: %v", err)
	}
	if pubKey[0] != 0xa1 || pubKey[1] != 0xb2 {
		t.Errorf("GetManagementPublicKey() first bytes = %x %x, want a1 b2", pubKey[0], pubKey[1])
	}

	// Test GetManagementPrivateKey
	privKey, err := cfg.GetManagementPrivateKey()
	if err != nil {
		t.Fatalf("GetManagementPrivateKey() failed: %v", err)
	}
	if privKey[0] != 0x12 || privKey[1] != 0x34 {
		t.Errorf("GetManagementPrivateKey() first bytes = %x %x, want 12 34", privKey[0], privKey[1])
	}
}

func TestManagementConfig_PublicKeyOnly(t *testing.T) {
	validPublicKey := "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"

	yamlConfig := `
agent:
  data_dir: "./data"

management:
  public_key: "` + validPublicKey + `"
`

	cfg, err := Parse([]byte(yamlConfig))
	if err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}

	if !cfg.HasManagementKey() {
		t.Error("HasManagementKey() = false, want true")
	}

	if cfg.CanDecryptManagement() {
		t.Error("CanDecryptManagement() = true, want false (no private key)")
	}

	_, err = cfg.GetManagementPrivateKey()
	if err == nil {
		t.Error("GetManagementPrivateKey() should fail without private key")
	}
}

func TestManagementConfig_NoKeys(t *testing.T) {
	yamlConfig := `
agent:
  data_dir: "./data"
`

	cfg, err := Parse([]byte(yamlConfig))
	if err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}

	if cfg.HasManagementKey() {
		t.Error("HasManagementKey() = true, want false")
	}

	if cfg.CanDecryptManagement() {
		t.Error("CanDecryptManagement() = true, want false")
	}
}

func TestManagementConfig_InvalidPublicKey(t *testing.T) {
	tests := []struct {
		name      string
		publicKey string
		wantErr   string
	}{
		{
			name:      "too_short",
			publicKey: "a1b2c3d4",
			wantErr:   "must be 32 bytes",
		},
		{
			name:      "invalid_hex",
			publicKey: "not_valid_hex_string_here_needs_to_be_64_chars_long_for_testing!",
			wantErr:   "invalid management public key hex",
		},
		{
			name:      "odd_length",
			publicKey: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b",
			wantErr:   "invalid management public key hex",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			yamlConfig := `
agent:
  data_dir: "./data"

management:
  public_key: "` + tc.publicKey + `"
`
			_, err := Parse([]byte(yamlConfig))
			if err == nil {
				t.Error("Parse() should fail for invalid public key")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("Error = %v, want to contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestManagementConfig_PrivateKeyWithoutPublicKey(t *testing.T) {
	validPrivateKey := "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"

	yamlConfig := `
agent:
  data_dir: "./data"

management:
  private_key: "` + validPrivateKey + `"
`

	_, err := Parse([]byte(yamlConfig))
	if err == nil {
		t.Error("Parse() should fail when private key is set without public key")
	}
	if !strings.Contains(err.Error(), "requires management.public_key") {
		t.Errorf("Error = %v, want to contain 'requires management.public_key'", err)
	}
}

func TestManagementConfig_Redacted(t *testing.T) {
	validPublicKey := "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"
	validPrivateKey := "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"

	yamlConfig := `
agent:
  data_dir: "./data"

management:
  public_key: "` + validPublicKey + `"
  private_key: "` + validPrivateKey + `"
`

	cfg, err := Parse([]byte(yamlConfig))
	if err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}

	// Get redacted config
	redacted := cfg.Redacted()

	// Public key should NOT be redacted
	if redacted.Management.PublicKey != validPublicKey {
		t.Errorf("Redacted public key = %s, want %s", redacted.Management.PublicKey, validPublicKey)
	}

	// Private key SHOULD be redacted
	if redacted.Management.PrivateKey != "[REDACTED]" {
		t.Errorf("Redacted private key = %s, want [REDACTED]", redacted.Management.PrivateKey)
	}

	// Original should still have the real private key
	if cfg.Management.PrivateKey != validPrivateKey {
		t.Errorf("Original private key was modified")
	}
}

func TestManagementConfig_HasSensitiveData(t *testing.T) {
	validPublicKey := "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"
	validPrivateKey := "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"

	t.Run("with_private_key", func(t *testing.T) {
		yamlConfig := `
agent:
  data_dir: "./data"

management:
  public_key: "` + validPublicKey + `"
  private_key: "` + validPrivateKey + `"
`
		cfg, err := Parse([]byte(yamlConfig))
		if err != nil {
			t.Fatalf("Parse() failed: %v", err)
		}

		if !cfg.HasSensitiveData() {
			t.Error("HasSensitiveData() = false, want true (has private key)")
		}
	})

	t.Run("public_key_only", func(t *testing.T) {
		yamlConfig := `
agent:
  data_dir: "./data"

management:
  public_key: "` + validPublicKey + `"
`
		cfg, err := Parse([]byte(yamlConfig))
		if err != nil {
			t.Fatalf("Parse() failed: %v", err)
		}

		// Public key alone is not sensitive
		if cfg.HasSensitiveData() {
			t.Error("HasSensitiveData() = true, want false (only public key)")
		}
	})
}
