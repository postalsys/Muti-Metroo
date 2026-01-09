// Package config provides configuration parsing and validation for Muti Metroo.
package config

import (
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/postalsys/muti-metroo/internal/embed"
	"gopkg.in/yaml.v3"
)

// Config represents the complete agent configuration.
type Config struct {
	Agent        AgentConfig        `yaml:"agent"`
	Protocol     ProtocolConfig     `yaml:"protocol"`
	TLS          GlobalTLSConfig    `yaml:"tls"`
	Management   ManagementConfig   `yaml:"management"`
	Listeners    []ListenerConfig   `yaml:"listeners"`
	Peers        []PeerConfig       `yaml:"peers"`
	SOCKS5       SOCKS5Config       `yaml:"socks5"`
	Exit         ExitConfig         `yaml:"exit"`
	Routing      RoutingConfig      `yaml:"routing"`
	Connections  ConnectionsConfig  `yaml:"connections"`
	Limits       LimitsConfig       `yaml:"limits"`
	HTTP         HTTPConfig         `yaml:"http"`
	FileTransfer FileTransferConfig `yaml:"file_transfer"`
	Shell        ShellConfig        `yaml:"shell"`
	UDP          UDPConfig          `yaml:"udp"`
}

// ProtocolConfig defines protocol identifiers used for transport negotiation.
// These can be customized to blend with other traffic for OPSEC purposes.
type ProtocolConfig struct {
	// ALPN is the Application-Layer Protocol Negotiation identifier.
	// Used for QUIC and TLS connections. Default: "muti-metroo/1"
	// Set to empty string "" to use no custom ALPN (uses transport defaults like "h2").
	ALPN string `yaml:"alpn"`

	// HTTPHeader is the custom header name for HTTP/2 transport protocol identification.
	// Default: "X-Muti-Metroo-Protocol". Set to empty string "" to disable custom header.
	HTTPHeader string `yaml:"http_header"`

	// WSSubprotocol is the WebSocket subprotocol identifier.
	// Default: "muti-metroo/1". Set to empty string "" to disable subprotocol negotiation.
	WSSubprotocol string `yaml:"ws_subprotocol"`
}

// GlobalTLSConfig defines global TLS settings shared across all connections.
// The CA is used for both verifying peer certificates and client certificate
// verification when mTLS is enabled on listeners.
type GlobalTLSConfig struct {
	// CA certificate for verifying peer certificates and client certs (mTLS)
	CA    string `yaml:"ca"`     // CA certificate file path
	CAPEM string `yaml:"ca_pem"` // CA certificate PEM content (takes precedence)

	// Agent's identity certificate used for listeners and peer connections
	Cert    string `yaml:"cert"`     // Certificate file path
	Key     string `yaml:"key"`      // Private key file path
	CertPEM string `yaml:"cert_pem"` // Certificate PEM content (takes precedence)
	KeyPEM  string `yaml:"key_pem"`  // Private key PEM content (takes precedence)

	// MTLS enables mutual TLS on listeners (require client certificates)
	MTLS bool `yaml:"mtls"`
}

// GetCAPEM returns the CA certificate PEM content, reading from file if necessary.
func (g *GlobalTLSConfig) GetCAPEM() ([]byte, error) {
	if g.CAPEM != "" {
		return []byte(g.CAPEM), nil
	}
	if g.CA != "" {
		return os.ReadFile(g.CA)
	}
	return nil, nil
}

// GetCertPEM returns the certificate PEM content, reading from file if necessary.
func (g *GlobalTLSConfig) GetCertPEM() ([]byte, error) {
	if g.CertPEM != "" {
		return []byte(g.CertPEM), nil
	}
	if g.Cert != "" {
		return os.ReadFile(g.Cert)
	}
	return nil, nil
}

// GetKeyPEM returns the private key PEM content, reading from file if necessary.
func (g *GlobalTLSConfig) GetKeyPEM() ([]byte, error) {
	if g.KeyPEM != "" {
		return []byte(g.KeyPEM), nil
	}
	if g.Key != "" {
		return os.ReadFile(g.Key)
	}
	return nil, nil
}

// HasCA returns true if CA certificate is configured (either file or PEM).
func (g *GlobalTLSConfig) HasCA() bool {
	return g.CA != "" || g.CAPEM != ""
}

// HasCert returns true if certificate is configured (either file or PEM).
func (g *GlobalTLSConfig) HasCert() bool {
	return g.Cert != "" || g.CertPEM != ""
}

// HasKey returns true if private key is configured (either file or PEM).
func (g *GlobalTLSConfig) HasKey() bool {
	return g.Key != "" || g.KeyPEM != ""
}

// ManagementConfig configures management key encryption for mesh metadata.
// When enabled, NodeInfo and route paths are encrypted so only operators
// with the private key can view topology details. This protects against
// blue team discovery of mesh topology from compromised agents.
type ManagementConfig struct {
	// PublicKey is the management public key (hex-encoded, 64 characters).
	// When set, NodeInfo and route paths are encrypted before flooding.
	// All agents in the mesh should have the same public key.
	PublicKey string `yaml:"public_key"`

	// PrivateKey is the management private key (hex-encoded, 64 characters).
	// Only set on operator/management nodes that need to view topology.
	// NEVER distribute to field agents.
	PrivateKey string `yaml:"private_key"`
}

// KeySize is the size of X25519 keys in bytes.
const KeySize = 32

// HasManagementKey returns true if management encryption is configured.
func (c *Config) HasManagementKey() bool {
	return c.Management.PublicKey != ""
}

// GetManagementPublicKey returns the parsed management public key.
// Returns an error if the key is not configured or invalid.
func (c *Config) GetManagementPublicKey() ([KeySize]byte, error) {
	var key [KeySize]byte
	if c.Management.PublicKey == "" {
		return key, fmt.Errorf("management public key not configured")
	}

	decoded, err := hex.DecodeString(c.Management.PublicKey)
	if err != nil {
		return key, fmt.Errorf("invalid management public key hex: %w", err)
	}

	if len(decoded) != KeySize {
		return key, fmt.Errorf("management public key must be %d bytes, got %d", KeySize, len(decoded))
	}

	copy(key[:], decoded)
	return key, nil
}

// GetManagementPrivateKey returns the parsed management private key.
// Returns an error if the key is not configured or invalid.
func (c *Config) GetManagementPrivateKey() ([KeySize]byte, error) {
	var key [KeySize]byte
	if c.Management.PrivateKey == "" {
		return key, fmt.Errorf("management private key not configured")
	}

	decoded, err := hex.DecodeString(c.Management.PrivateKey)
	if err != nil {
		return key, fmt.Errorf("invalid management private key hex: %w", err)
	}

	if len(decoded) != KeySize {
		return key, fmt.Errorf("management private key must be %d bytes, got %d", KeySize, len(decoded))
	}

	copy(key[:], decoded)
	return key, nil
}

// CanDecryptManagement returns true if management private key is configured.
func (c *Config) CanDecryptManagement() bool {
	return c.Management.PrivateKey != ""
}

// AgentConfig contains agent identity settings.
type AgentConfig struct {
	ID          string `yaml:"id"`           // "auto" or hex string
	DisplayName string `yaml:"display_name"` // Human-readable name (Unicode allowed)
	DataDir     string `yaml:"data_dir"`     // Directory for persistent state
	LogLevel    string `yaml:"log_level"`    // debug, info, warn, error
	LogFormat   string `yaml:"log_format"`   // text, json
}

// ListenerConfig defines a transport listener.
type ListenerConfig struct {
	Transport string    `yaml:"transport"` // quic, h2, ws
	Address   string    `yaml:"address"`   // listen address
	Path      string    `yaml:"path"`      // HTTP path for h2/ws
	PlainText bool      `yaml:"plaintext"` // Allow plain WebSocket without TLS (for reverse proxy)
	TLS       TLSConfig `yaml:"tls"`
}

// PeerConfig defines a peer connection.
type PeerConfig struct {
	ID        string    `yaml:"id"`         // Expected peer AgentID
	Transport string    `yaml:"transport"`  // quic, h2, ws
	Address   string    `yaml:"address"`    // peer address
	Path      string    `yaml:"path"`       // HTTP path for h2/ws
	Proxy     string    `yaml:"proxy"`      // HTTP proxy for ws
	ProxyAuth ProxyAuth `yaml:"proxy_auth"` // Proxy authentication
	TLS       TLSConfig `yaml:"tls"`
}

// TLSConfig defines per-connection TLS settings that can override global settings.
// For each certificate/key, you can specify either a file path or inline PEM content.
// If both are provided, inline PEM takes precedence.
type TLSConfig struct {
	// Override global cert/key (optional - uses global if not set)
	Cert    string `yaml:"cert"`     // Certificate file path
	Key     string `yaml:"key"`      // Private key file path
	CertPEM string `yaml:"cert_pem"` // Certificate PEM content
	KeyPEM  string `yaml:"key_pem"`  // Private key PEM content

	// Override global CA (optional - peer connections only)
	CA    string `yaml:"ca"`     // CA certificate file path
	CAPEM string `yaml:"ca_pem"` // CA certificate PEM content

	// mTLS override (optional - listener only, uses global if nil)
	// Use pointer to distinguish "not set" from "false"
	MTLS *bool `yaml:"mtls,omitempty"`

	// Other options
	Fingerprint        string `yaml:"fingerprint"`          // Certificate fingerprint for pinning
	InsecureSkipVerify bool   `yaml:"insecure_skip_verify"` // Skip verification (dev only)
}

// GetCertPEM returns the certificate PEM content, reading from file if necessary.
func (t *TLSConfig) GetCertPEM() ([]byte, error) {
	if t.CertPEM != "" {
		return []byte(t.CertPEM), nil
	}
	if t.Cert != "" {
		return os.ReadFile(t.Cert)
	}
	return nil, nil
}

// GetKeyPEM returns the private key PEM content, reading from file if necessary.
func (t *TLSConfig) GetKeyPEM() ([]byte, error) {
	if t.KeyPEM != "" {
		return []byte(t.KeyPEM), nil
	}
	if t.Key != "" {
		return os.ReadFile(t.Key)
	}
	return nil, nil
}

// GetCAPEM returns the CA certificate PEM content, reading from file if necessary.
func (t *TLSConfig) GetCAPEM() ([]byte, error) {
	if t.CAPEM != "" {
		return []byte(t.CAPEM), nil
	}
	if t.CA != "" {
		return os.ReadFile(t.CA)
	}
	return nil, nil
}

// HasCert returns true if certificate is configured (either file or PEM).
func (t *TLSConfig) HasCert() bool {
	return t.Cert != "" || t.CertPEM != ""
}

// HasKey returns true if private key is configured (either file or PEM).
func (t *TLSConfig) HasKey() bool {
	return t.Key != "" || t.KeyPEM != ""
}

// HasCA returns true if CA certificate is configured (either file or PEM).
func (t *TLSConfig) HasCA() bool {
	return t.CA != "" || t.CAPEM != ""
}

// GetEffectiveCertPEM returns the effective certificate PEM, preferring per-connection
// override over global config.
func (c *Config) GetEffectiveCertPEM(override *TLSConfig) ([]byte, error) {
	// Check per-connection override first
	if override != nil && override.HasCert() {
		return override.GetCertPEM()
	}
	// Fall back to global
	return c.TLS.GetCertPEM()
}

// GetEffectiveKeyPEM returns the effective private key PEM, preferring per-connection
// override over global config.
func (c *Config) GetEffectiveKeyPEM(override *TLSConfig) ([]byte, error) {
	// Check per-connection override first
	if override != nil && override.HasKey() {
		return override.GetKeyPEM()
	}
	// Fall back to global
	return c.TLS.GetKeyPEM()
}

// GetEffectiveCAPEM returns the effective CA certificate PEM, preferring per-connection
// override over global config.
func (c *Config) GetEffectiveCAPEM(override *TLSConfig) ([]byte, error) {
	// Check per-connection override first
	if override != nil && override.HasCA() {
		return override.GetCAPEM()
	}
	// Fall back to global
	return c.TLS.GetCAPEM()
}

// ProxyAuth defines proxy authentication.
type ProxyAuth struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// SOCKS5Config defines SOCKS5 server settings.
type SOCKS5Config struct {
	Enabled        bool            `yaml:"enabled"`
	Address        string          `yaml:"address"`
	Auth           SOCKS5AuthConfig `yaml:"auth"`
	MaxConnections int             `yaml:"max_connections"`
}

// SOCKS5AuthConfig defines SOCKS5 authentication settings.
type SOCKS5AuthConfig struct {
	Enabled bool              `yaml:"enabled"`
	Users   []SOCKS5UserConfig `yaml:"users"`
}

// SOCKS5UserConfig defines a SOCKS5 user.
type SOCKS5UserConfig struct {
	Username string `yaml:"username"`
	// Password is the plaintext password (deprecated, use PasswordHash).
	Password string `yaml:"password,omitempty"`
	// PasswordHash is the bcrypt hash of the password (recommended).
	// Generate with: htpasswd -bnBC 10 "" <password> | tr -d ':\n'
	PasswordHash string `yaml:"password_hash,omitempty"`
}

// ExitConfig defines exit node settings.
type ExitConfig struct {
	Enabled      bool      `yaml:"enabled"`
	Routes       []string  `yaml:"routes"`        // CIDR routes to advertise
	DomainRoutes []string  `yaml:"domain_routes"` // Domain patterns to advertise (exact or *.wildcard)
	DNS          DNSConfig `yaml:"dns"`
}

// DNSConfig defines DNS settings for exit nodes.
type DNSConfig struct {
	Servers []string      `yaml:"servers"`
	Timeout time.Duration `yaml:"timeout"`
}

// RoutingConfig defines routing parameters.
type RoutingConfig struct {
	AdvertiseInterval  time.Duration `yaml:"advertise_interval"`
	NodeInfoInterval   time.Duration `yaml:"node_info_interval"` // Defaults to AdvertiseInterval if not set
	RouteTTL           time.Duration `yaml:"route_ttl"`
	MaxHops            int           `yaml:"max_hops"`
}

// ConnectionsConfig defines connection tuning parameters.
type ConnectionsConfig struct {
	IdleThreshold    time.Duration   `yaml:"idle_threshold"`
	Timeout          time.Duration   `yaml:"timeout"`
	KeepaliveJitter  float64         `yaml:"keepalive_jitter"` // Jitter fraction for keepalive timing (0.0-1.0)
	Reconnect        ReconnectConfig `yaml:"reconnect"`
}

// ReconnectConfig defines reconnection behavior.
type ReconnectConfig struct {
	InitialDelay time.Duration `yaml:"initial_delay"`
	MaxDelay     time.Duration `yaml:"max_delay"`
	Multiplier   float64       `yaml:"multiplier"`
	Jitter       float64       `yaml:"jitter"`
	MaxRetries   int           `yaml:"max_retries"` // 0 = infinite
}

// LimitsConfig defines resource limits.
type LimitsConfig struct {
	MaxStreamsPerPeer int           `yaml:"max_streams_per_peer"`
	MaxStreamsTotal   int           `yaml:"max_streams_total"`
	MaxPendingOpens   int           `yaml:"max_pending_opens"`
	StreamOpenTimeout time.Duration `yaml:"stream_open_timeout"`
	BufferSize        int           `yaml:"buffer_size"`
}

// HTTPConfig defines HTTP API server settings.
type HTTPConfig struct {
	Enabled      bool          `yaml:"enabled"`
	Address      string        `yaml:"address"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`

	// Minimal mode - only enable /health, /healthz, /ready endpoints.
	// When true, overrides all other endpoint flags to false.
	Minimal bool `yaml:"minimal"`

	// Endpoint group controls. All default to true when http.enabled=true.
	// Use pointer types to distinguish between "not set" (nil = use default) and "explicitly false".
	// Disabled endpoints return 404 and log access attempts.
	Pprof     *bool `yaml:"pprof"`      // /debug/pprof/* - Go profiling endpoints
	Dashboard *bool `yaml:"dashboard"`  // /ui/*, /api/* - Web dashboard and API
	RemoteAPI *bool `yaml:"remote_api"` // /agents/* - Distributed mesh APIs
}

// PprofEnabled returns whether the /debug/pprof/* endpoints are enabled.
func (h HTTPConfig) PprofEnabled() bool {
	if h.Minimal {
		return false
	}
	return h.Pprof == nil || *h.Pprof
}

// DashboardEnabled returns whether the /ui/* and /api/* endpoints are enabled.
func (h HTTPConfig) DashboardEnabled() bool {
	if h.Minimal {
		return false
	}
	return h.Dashboard == nil || *h.Dashboard
}

// RemoteAPIEnabled returns whether the /agents/* endpoints are enabled.
func (h HTTPConfig) RemoteAPIEnabled() bool {
	if h.Minimal {
		return false
	}
	return h.RemoteAPI == nil || *h.RemoteAPI
}

// FileTransferConfig defines file transfer settings.
type FileTransferConfig struct {
	// Enabled controls whether file transfer is available on this agent.
	Enabled bool `yaml:"enabled"`

	// MaxFileSize is the maximum allowed file size in bytes.
	// Default is 500MB (500 * 1024 * 1024).
	MaxFileSize int64 `yaml:"max_file_size"`

	// AllowedPaths controls which paths are allowed for file operations.
	// This works consistently with RPC whitelist:
	//   - Empty list []: No paths are allowed (feature is effectively disabled)
	//   - ["*"]: All absolute paths are allowed
	//   - Specific paths: Only listed paths/patterns are allowed
	//
	// Supports glob patterns:
	//   - "/tmp": Exact prefix - allows /tmp and any path under /tmp
	//   - "/tmp/*": Single-level glob - allows /tmp/file.txt, /tmp/subdir
	//   - "/tmp/**": Recursive glob - allows any path under /tmp
	//   - "/home/*/uploads": Pattern matching - allows /home/alice/uploads
	//
	// Example: ["/tmp", "/home/*/uploads"]
	AllowedPaths []string `yaml:"allowed_paths"`

	// PasswordHash is the bcrypt hash of the file transfer password.
	// If set, all file transfer requests must include the correct password.
	// Generate with: htpasswd -bnBC 10 "" <password> | tr -d ':\n'
	PasswordHash string `yaml:"password_hash"`
}

// ShellConfig defines remote shell settings.
type ShellConfig struct {
	// Enabled controls whether shell is available on this agent.
	Enabled bool `yaml:"enabled"`

	// Whitelist contains allowed commands. Empty list = no commands allowed.
	// Use ["*"] to allow all commands (for testing only!).
	// Commands should be base names only (e.g., "whoami", "ls", "bash").
	Whitelist []string `yaml:"whitelist"`

	// PasswordHash is the bcrypt hash of the shell password.
	// If set, all shell requests must include the correct password.
	// Generate with: muti-metroo hash
	PasswordHash string `yaml:"password_hash"`

	// Timeout is the optional command timeout (0 = no timeout).
	Timeout time.Duration `yaml:"timeout"`

	// MaxSessions limits concurrent shell sessions (0 = unlimited).
	MaxSessions int `yaml:"max_sessions"`
}

// UDPConfig configures UDP relay support for exit nodes.
// UDP relay enables SOCKS5 UDP ASSOCIATE for tunneling UDP traffic through the mesh.
type UDPConfig struct {
	// Enabled controls whether UDP relay is available on this exit node.
	Enabled bool `yaml:"enabled"`

	// MaxAssociations limits concurrent UDP associations (0 = unlimited).
	MaxAssociations int `yaml:"max_associations"`

	// IdleTimeout is the timeout for inactive UDP associations.
	IdleTimeout time.Duration `yaml:"idle_timeout"`

	// MaxDatagramSize is the maximum UDP payload size in bytes.
	// Default is 1472 (Ethernet MTU minus IP and UDP headers).
	MaxDatagramSize int `yaml:"max_datagram_size"`
}

// Default returns a Config with default values.
func Default() *Config {
	return &Config{
		Agent: AgentConfig{
			ID:        "auto",
			DataDir:   "./data",
			LogLevel:  "info",
			LogFormat: "text",
		},
		Protocol: ProtocolConfig{
			ALPN:          "muti-metroo/1",
			HTTPHeader:    "X-Muti-Metroo-Protocol",
			WSSubprotocol: "muti-metroo/1",
		},
		Listeners: []ListenerConfig{},
		Peers:     []PeerConfig{},
		SOCKS5: SOCKS5Config{
			Enabled:        false,
			Address:        "127.0.0.1:1080",
			MaxConnections: 1000,
		},
		Exit: ExitConfig{
			Enabled: false,
			Routes:  []string{},
			DNS: DNSConfig{
				Servers: []string{"8.8.8.8:53", "1.1.1.1:53"},
				Timeout: 5 * time.Second,
			},
		},
		Routing: RoutingConfig{
			AdvertiseInterval: 2 * time.Minute,
			RouteTTL:          5 * time.Minute,
			MaxHops:           16,
		},
		Connections: ConnectionsConfig{
			IdleThreshold:   5 * time.Minute, // Long-running connections like SSH should stay alive
			Timeout:         90 * time.Second,
			KeepaliveJitter: 0.2, // 20% jitter to avoid detectable beacon patterns
			Reconnect: ReconnectConfig{
				InitialDelay: 1 * time.Second,
				MaxDelay:     60 * time.Second,
				Multiplier:   2.0,
				Jitter:       0.2,
				MaxRetries:   0,
			},
		},
		Limits: LimitsConfig{
			MaxStreamsPerPeer: 1000,
			MaxStreamsTotal:   10000,
			MaxPendingOpens:   100,
			StreamOpenTimeout: 30 * time.Second,
			BufferSize:        262144, // 256 KB
		},
		HTTP: HTTPConfig{
			Enabled:      false,
			Address:      ":8080",
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
		},
		FileTransfer: FileTransferConfig{
			Enabled:      false,
			MaxFileSize:  500 * 1024 * 1024, // 500 MB
			AllowedPaths: []string{},        // Empty = no paths allowed (must configure explicitly)
		},
		Shell: ShellConfig{
			Enabled:     false,      // Disabled by default for security
			Whitelist:   []string{}, // Empty = no commands allowed
			MaxSessions: 0,          // 0 = unlimited (trusted network)
		},
		UDP: UDPConfig{
			Enabled:         false,           // Disabled by default
			MaxAssociations: 1000,            // Default limit
			IdleTimeout:     5 * time.Minute, // Same as connection idle threshold
			MaxDatagramSize: 1472,            // MTU - IP/UDP headers
		},
	}
}

// Load reads and parses a configuration file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	return Parse(data)
}

// LoadOrEmbedded loads configuration from embedded binary data if present,
// otherwise falls back to loading from the specified file path.
// Returns the config, a boolean indicating if it was embedded, and any error.
// When embedded config is present, the path argument is ignored.
func LoadOrEmbedded(path string) (*Config, bool, error) {
	// Check for embedded config first
	if embed.HasEmbeddedConfigSelf() {
		data, err := embed.ReadFromSelf()
		if err != nil {
			return nil, false, fmt.Errorf("failed to read embedded config: %w", err)
		}
		cfg, err := Parse(data)
		if err != nil {
			return nil, false, fmt.Errorf("failed to parse embedded config: %w", err)
		}
		return cfg, true, nil
	}

	// Fall back to file
	cfg, err := Load(path)
	if err != nil {
		return nil, false, err
	}
	return cfg, false, nil
}

// Parse parses configuration from YAML bytes.
func Parse(data []byte) (*Config, error) {
	// Expand environment variables
	expanded := expandEnvVars(string(data))

	// Start with defaults
	cfg := Default()

	// Parse YAML
	if err := yaml.Unmarshal([]byte(expanded), cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Validate
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

// envVarRegex matches ${VAR} or $VAR patterns
var envVarRegex = regexp.MustCompile(`\$\{([^}]+)\}|\$([A-Za-z_][A-Za-z0-9_]*)`)

// expandEnvVars replaces environment variable references with their values.
func expandEnvVars(s string) string {
	return envVarRegex.ReplaceAllStringFunc(s, func(match string) string {
		// Extract variable name
		var name string
		if strings.HasPrefix(match, "${") {
			name = match[2 : len(match)-1]
		} else {
			name = match[1:]
		}

		// Handle default values: ${VAR:-default}
		if idx := strings.Index(name, ":-"); idx != -1 {
			varName := name[:idx]
			defaultVal := name[idx+2:]
			if val, ok := os.LookupEnv(varName); ok {
				return val
			}
			return defaultVal
		}

		// Simple lookup
		if val, ok := os.LookupEnv(name); ok {
			return val
		}
		return match // Keep original if not found
	})
}

// Validate checks the configuration for errors.
func (c *Config) Validate() error {
	var errs []string

	// Validate agent config
	if c.Agent.DataDir == "" {
		errs = append(errs, "agent.data_dir is required")
	}
	if !isValidLogLevel(c.Agent.LogLevel) {
		errs = append(errs, fmt.Sprintf("invalid log_level: %s (must be debug, info, warn, or error)", c.Agent.LogLevel))
	}
	if !isValidLogFormat(c.Agent.LogFormat) {
		errs = append(errs, fmt.Sprintf("invalid log_format: %s (must be text or json)", c.Agent.LogFormat))
	}

	// Validate global TLS config
	if err := c.validateGlobalTLS(); err != nil {
		errs = append(errs, err.Error())
	}

	// Validate listeners
	for i, l := range c.Listeners {
		if err := c.validateListener(l, i); err != nil {
			errs = append(errs, fmt.Sprintf("listeners[%d]: %v", i, err))
		}
	}

	// Validate peers
	for i, p := range c.Peers {
		if err := c.validatePeer(p, i); err != nil {
			errs = append(errs, fmt.Sprintf("peers[%d]: %v", i, err))
		}
	}

	// Validate SOCKS5
	if c.SOCKS5.Enabled && c.SOCKS5.Address == "" {
		errs = append(errs, "socks5.address is required when enabled")
	}

	// Validate exit routes (CIDR)
	for i, route := range c.Exit.Routes {
		if !isValidCIDR(route) {
			errs = append(errs, fmt.Sprintf("exit.routes[%d]: invalid CIDR: %s", i, route))
		}
	}

	// Validate domain routes
	for i, pattern := range c.Exit.DomainRoutes {
		if err := isValidDomainPattern(pattern); err != nil {
			errs = append(errs, fmt.Sprintf("exit.domain_routes[%d]: %v", i, err))
		}
	}

	// Validate routing
	if c.Routing.MaxHops < 1 || c.Routing.MaxHops > 255 {
		errs = append(errs, "routing.max_hops must be between 1 and 255")
	}

	// Validate limits
	if c.Limits.MaxStreamsPerPeer < 1 {
		errs = append(errs, "limits.max_streams_per_peer must be positive")
	}
	if c.Limits.MaxStreamsTotal < c.Limits.MaxStreamsPerPeer {
		errs = append(errs, "limits.max_streams_total must be >= max_streams_per_peer")
	}
	if c.Limits.BufferSize < 1024 {
		errs = append(errs, "limits.buffer_size must be at least 1024")
	}

	// Validate management key configuration
	if err := c.validateManagementKeys(); err != nil {
		errs = append(errs, err.Error())
	}

	if len(errs) > 0 {
		return fmt.Errorf("validation errors:\n  - %s", strings.Join(errs, "\n  - "))
	}

	return nil
}

// validateGlobalTLS validates the global TLS configuration.
func (c *Config) validateGlobalTLS() error {
	// Check for mTLS without CA
	if c.TLS.MTLS && !c.TLS.HasCA() {
		return fmt.Errorf("tls.ca is required when tls.mtls is enabled")
	}

	// Check for partial cert/key configuration
	if c.TLS.HasCert() != c.TLS.HasKey() {
		return fmt.Errorf("tls.cert and tls.key must both be specified or both be empty")
	}

	return nil
}

// validateManagementKeys validates the management key configuration.
func (c *Config) validateManagementKeys() error {
	// If no public key, nothing to validate
	if c.Management.PublicKey == "" {
		// But warn if private key is set without public key
		if c.Management.PrivateKey != "" {
			return fmt.Errorf("management.private_key requires management.public_key to be set")
		}
		return nil
	}

	// Validate public key format
	if _, err := c.GetManagementPublicKey(); err != nil {
		return fmt.Errorf("management.public_key: %w", err)
	}

	// Validate private key format if set
	if c.Management.PrivateKey != "" {
		if _, err := c.GetManagementPrivateKey(); err != nil {
			return fmt.Errorf("management.private_key: %w", err)
		}
	}

	return nil
}

func isValidLogLevel(level string) bool {
	switch level {
	case "debug", "info", "warn", "error":
		return true
	default:
		return false
	}
}

func isValidLogFormat(format string) bool {
	switch format {
	case "text", "json":
		return true
	default:
		return false
	}
}

func isValidTransport(transport string) bool {
	switch transport {
	case "quic", "h2", "ws":
		return true
	default:
		return false
	}
}

// validateListener validates a listener configuration, considering global TLS settings.
func (c *Config) validateListener(l ListenerConfig, index int) error {
	if !isValidTransport(l.Transport) {
		return fmt.Errorf("invalid transport: %s (must be quic, h2, or ws)", l.Transport)
	}
	if l.Address == "" {
		return fmt.Errorf("address is required")
	}
	if (l.Transport == "h2" || l.Transport == "ws") && l.Path == "" {
		return fmt.Errorf("path is required for %s transport", l.Transport)
	}
	// PlainText mode is only supported for WebSocket (for reverse proxy scenarios)
	if l.PlainText {
		if l.Transport != "ws" {
			return fmt.Errorf("plaintext mode is only supported for ws transport (for reverse proxy scenarios)")
		}
		// Skip TLS requirement for plaintext WebSocket
		return nil
	}

	// Check if cert/key is available (from listener override or global)
	hasCert := l.TLS.HasCert() || c.TLS.HasCert()
	hasKey := l.TLS.HasKey() || c.TLS.HasKey()
	if !hasCert || !hasKey {
		return fmt.Errorf("tls certificate and key are required (specify in global tls section or per-listener)")
	}

	// Determine effective mTLS setting
	enableMTLS := c.TLS.MTLS
	if l.TLS.MTLS != nil {
		enableMTLS = *l.TLS.MTLS
	}

	// If mTLS is enabled for this listener, ensure CA is configured
	if enableMTLS && !c.TLS.HasCA() {
		return fmt.Errorf("global tls.ca is required when mTLS is enabled")
	}

	return nil
}

// validatePeer validates a peer configuration, considering global TLS settings.
func (c *Config) validatePeer(p PeerConfig, index int) error {
	if p.ID == "" {
		return fmt.Errorf("id is required")
	}
	if !isValidTransport(p.Transport) {
		return fmt.Errorf("invalid transport: %s (must be quic, h2, or ws)", p.Transport)
	}
	if p.Address == "" {
		return fmt.Errorf("address is required")
	}

	// Check for partial cert/key override
	if p.TLS.HasCert() != p.TLS.HasKey() {
		return fmt.Errorf("tls cert and key must both be specified or both be empty")
	}

	return nil
}

func isValidCIDR(cidr string) bool {
	_, _, err := net.ParseCIDR(cidr)
	return err == nil
}

// isValidDomainPattern validates a domain pattern (exact or *.wildcard).
func isValidDomainPattern(pattern string) error {
	if pattern == "" {
		return fmt.Errorf("empty domain pattern")
	}

	pattern = strings.TrimSpace(pattern)

	// Check for wildcard pattern
	var baseDomain string
	if strings.HasPrefix(pattern, "*.") {
		baseDomain = pattern[2:]
	} else {
		baseDomain = pattern
	}

	if baseDomain == "" {
		return fmt.Errorf("empty domain after wildcard")
	}

	// Basic domain validation
	if strings.HasPrefix(baseDomain, ".") || strings.HasSuffix(baseDomain, ".") {
		return fmt.Errorf("domain cannot start or end with a dot")
	}

	if strings.Contains(baseDomain, "..") {
		return fmt.Errorf("domain cannot contain consecutive dots")
	}

	// Check for valid characters
	for _, r := range baseDomain {
		if !isValidDomainChar(r) {
			return fmt.Errorf("invalid character in domain: %c", r)
		}
	}

	// Must have at least one dot (TLD)
	if !strings.Contains(baseDomain, ".") {
		return fmt.Errorf("domain must have at least one dot (e.g., example.com)")
	}

	return nil
}

// isValidDomainChar checks if a character is valid in a domain name.
func isValidDomainChar(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		r == '-' || r == '.'
}

// String returns a string representation of the config (for debugging).
// WARNING: This method redacts sensitive values. Use StringUnsafe() for full output.
func (c *Config) String() string {
	redacted := c.Redacted()
	data, _ := yaml.Marshal(redacted)
	return string(data)
}

// StringUnsafe returns a string representation including sensitive values.
// Use with caution - do not log the output.
func (c *Config) StringUnsafe() string {
	data, _ := yaml.Marshal(c)
	return string(data)
}

// redactedValue is the placeholder for sensitive values.
const redactedValue = "[REDACTED]"

// Redacted returns a copy of the config with sensitive values redacted.
// This is safe to log or display to users.
func (c *Config) Redacted() *Config {
	// Create a deep copy by marshaling and unmarshaling
	data, err := yaml.Marshal(c)
	if err != nil {
		return c
	}

	redacted := &Config{}
	if err := yaml.Unmarshal(data, redacted); err != nil {
		return c
	}

	// Redact global TLS key
	if redacted.TLS.Key != "" {
		redacted.TLS.Key = redactedValue
	}
	if redacted.TLS.KeyPEM != "" {
		redacted.TLS.KeyPEM = redactedValue
	}

	// Redact sensitive fields in peers
	for i := range redacted.Peers {
		if redacted.Peers[i].ProxyAuth.Password != "" {
			redacted.Peers[i].ProxyAuth.Password = redactedValue
		}
		// Redact TLS key paths and PEM content
		if redacted.Peers[i].TLS.Key != "" {
			redacted.Peers[i].TLS.Key = redactedValue
		}
		if redacted.Peers[i].TLS.KeyPEM != "" {
			redacted.Peers[i].TLS.KeyPEM = redactedValue
		}
	}

	// Redact sensitive fields in listeners
	for i := range redacted.Listeners {
		if redacted.Listeners[i].TLS.Key != "" {
			redacted.Listeners[i].TLS.Key = redactedValue
		}
		if redacted.Listeners[i].TLS.KeyPEM != "" {
			redacted.Listeners[i].TLS.KeyPEM = redactedValue
		}
	}

	// Redact SOCKS5 user passwords and password hashes
	for i := range redacted.SOCKS5.Auth.Users {
		if redacted.SOCKS5.Auth.Users[i].Password != "" {
			redacted.SOCKS5.Auth.Users[i].Password = redactedValue
		}
		if redacted.SOCKS5.Auth.Users[i].PasswordHash != "" {
			redacted.SOCKS5.Auth.Users[i].PasswordHash = redactedValue
		}
	}

	// Redact FileTransfer password hash
	if redacted.FileTransfer.PasswordHash != "" {
		redacted.FileTransfer.PasswordHash = redactedValue
	}

	// Redact Shell password hash
	if redacted.Shell.PasswordHash != "" {
		redacted.Shell.PasswordHash = redactedValue
	}

	// Redact management private key
	if redacted.Management.PrivateKey != "" {
		redacted.Management.PrivateKey = redactedValue
	}

	return redacted
}

// HasSensitiveData returns true if the config contains any sensitive data.
func (c *Config) HasSensitiveData() bool {
	// Check peer proxy passwords
	for _, p := range c.Peers {
		if p.ProxyAuth.Password != "" {
			return true
		}
	}

	// Check SOCKS5 passwords and password hashes
	for _, u := range c.SOCKS5.Auth.Users {
		if u.Password != "" || u.PasswordHash != "" {
			return true
		}
	}

	// Check FileTransfer password hash
	if c.FileTransfer.PasswordHash != "" {
		return true
	}

	// Check Shell password hash
	if c.Shell.PasswordHash != "" {
		return true
	}

	// Check management private key
	if c.Management.PrivateKey != "" {
		return true
	}

	return false
}
