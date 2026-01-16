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

// getPEM returns inline PEM content if set, otherwise reads from file path.
// Returns nil if neither is configured.
func getPEM(inline, filePath string) ([]byte, error) {
	if inline != "" {
		return []byte(inline), nil
	}
	if filePath != "" {
		return os.ReadFile(filePath)
	}
	return nil, nil
}

// isOneOf returns true if value matches any of the allowed values.
func isOneOf(value string, allowed ...string) bool {
	for _, a := range allowed {
		if value == a {
			return true
		}
	}
	return false
}

// parseHexKey decodes a hex-encoded key and validates its length.
func parseHexKey(hexStr, keyName string, expectedSize int) ([KeySize]byte, error) {
	var key [KeySize]byte
	if hexStr == "" {
		return key, fmt.Errorf("%s not configured", keyName)
	}

	decoded, err := hex.DecodeString(hexStr)
	if err != nil {
		return key, fmt.Errorf("invalid %s hex: %w", keyName, err)
	}

	if len(decoded) != expectedSize {
		return key, fmt.Errorf("%s must be %d bytes, got %d", keyName, expectedSize, len(decoded))
	}

	copy(key[:], decoded)
	return key, nil
}

// Config represents the complete agent configuration.
type Config struct {
	// DefaultAction is the action to run when the binary is executed without arguments.
	// Only applies to embedded config binaries. Valid values: "run", "help".
	// When set to "run", the agent starts automatically without requiring "./my-agent run".
	DefaultAction string             `yaml:"default_action,omitempty"`
	Agent         AgentConfig        `yaml:"agent"`
	Protocol     ProtocolConfig     `yaml:"protocol,omitempty"`
	TLS          GlobalTLSConfig    `yaml:"tls,omitempty"`
	Management   ManagementConfig   `yaml:"management,omitempty"`
	Listeners    []ListenerConfig   `yaml:"listeners,omitempty"`
	Peers        []PeerConfig       `yaml:"peers,omitempty"`
	SOCKS5       SOCKS5Config       `yaml:"socks5,omitempty"`
	Exit         ExitConfig         `yaml:"exit,omitempty"`
	Routing      RoutingConfig      `yaml:"routing,omitempty"`
	Connections  ConnectionsConfig  `yaml:"connections,omitempty"`
	Limits       LimitsConfig       `yaml:"limits,omitempty"`
	HTTP         HTTPConfig         `yaml:"http,omitempty"`
	FileTransfer FileTransferConfig `yaml:"file_transfer,omitempty"`
	Shell        ShellConfig        `yaml:"shell,omitempty"`
	UDP          UDPConfig          `yaml:"udp,omitempty"`
	ICMP         ICMPConfig         `yaml:"icmp,omitempty"`
	Forward      ForwardConfig      `yaml:"forward,omitempty"`
}

// ProtocolConfig defines protocol identifiers used for transport negotiation.
// These can be customized to blend with other traffic for OPSEC purposes.
type ProtocolConfig struct {
	// ALPN is the Application-Layer Protocol Negotiation identifier.
	// Used for QUIC and TLS connections. Default: "muti-metroo/1"
	// Set to empty string "" to use no custom ALPN (uses transport defaults like "h2").
	ALPN string `yaml:"alpn,omitempty"`

	// HTTPHeader is the custom header name for HTTP/2 transport protocol identification.
	// Default: "X-Muti-Metroo-Protocol". Set to empty string "" to disable custom header.
	HTTPHeader string `yaml:"http_header,omitempty"`

	// WSSubprotocol is the WebSocket subprotocol identifier.
	// Default: "muti-metroo/1". Set to empty string "" to disable subprotocol negotiation.
	WSSubprotocol string `yaml:"ws_subprotocol,omitempty"`
}

// GlobalTLSConfig defines global TLS settings shared across all connections.
// The CA is used for both verifying peer certificates and client certificate
// verification when mTLS is enabled on listeners.
//
// By default, TLS certificate verification is disabled (Strict: false) because
// Muti Metroo uses an additional E2E encryption layer (X25519 + ChaCha20-Poly1305)
// that provides confidentiality and integrity regardless of transport security.
// Self-signed certificates are auto-generated if none are configured.
type GlobalTLSConfig struct {
	// CA certificate for verifying peer certificates and client certs (mTLS)
	CA    string `yaml:"ca,omitempty"`     // CA certificate file path
	CAPEM string `yaml:"ca_pem,omitempty"` // CA certificate PEM content (takes precedence)

	// Agent's identity certificate used for listeners and peer connections
	// If not configured, a self-signed certificate is auto-generated at startup
	Cert    string `yaml:"cert,omitempty"`     // Certificate file path
	Key     string `yaml:"key,omitempty"`      // Private key file path
	CertPEM string `yaml:"cert_pem,omitempty"` // Certificate PEM content (takes precedence)
	KeyPEM  string `yaml:"key_pem,omitempty"`  // Private key PEM content (takes precedence)

	// MTLS enables mutual TLS on listeners (require client certificates)
	// Requires CA to be configured
	MTLS bool `yaml:"mtls,omitempty"`

	// Strict enables TLS certificate verification (default: false)
	// When false (default), peer certificates are not validated, which is safe
	// because the E2E layer provides security. When true, peer certificates
	// must be signed by the configured CA.
	Strict bool `yaml:"strict,omitempty"`
}

// GetCAPEM returns the CA certificate PEM content, reading from file if necessary.
func (g *GlobalTLSConfig) GetCAPEM() ([]byte, error) {
	return getPEM(g.CAPEM, g.CA)
}

// GetCertPEM returns the certificate PEM content, reading from file if necessary.
func (g *GlobalTLSConfig) GetCertPEM() ([]byte, error) {
	return getPEM(g.CertPEM, g.Cert)
}

// GetKeyPEM returns the private key PEM content, reading from file if necessary.
func (g *GlobalTLSConfig) GetKeyPEM() ([]byte, error) {
	return getPEM(g.KeyPEM, g.Key)
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
	PublicKey string `yaml:"public_key,omitempty"`

	// PrivateKey is the management private key (hex-encoded, 64 characters).
	// Only set on operator/management nodes that need to view topology.
	// NEVER distribute to field agents.
	PrivateKey string `yaml:"private_key,omitempty"`
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
	return parseHexKey(c.Management.PublicKey, "management public key", KeySize)
}

// GetManagementPrivateKey returns the parsed management private key.
// Returns an error if the key is not configured or invalid.
func (c *Config) GetManagementPrivateKey() ([KeySize]byte, error) {
	return parseHexKey(c.Management.PrivateKey, "management private key", KeySize)
}

// CanDecryptManagement returns true if management private key is configured.
func (c *Config) CanDecryptManagement() bool {
	return c.Management.PrivateKey != ""
}

// AgentConfig contains agent identity settings.
type AgentConfig struct {
	ID          string `yaml:"id,omitempty"`           // "auto" or hex string
	DisplayName string `yaml:"display_name,omitempty"` // Human-readable name (Unicode allowed)
	DataDir     string `yaml:"data_dir,omitempty"`     // Directory for persistent state (optional with identity in config)
	LogLevel    string `yaml:"log_level,omitempty"`    // debug, info, warn, error
	LogFormat   string `yaml:"log_format,omitempty"`   // text, json

	// X25519 keypair for E2E encryption (optional - enables single-file deployment)
	// When specified, takes precedence over data_dir files, making data_dir optional.
	// Generate with: muti-metroo init, then copy values from agent_key file.
	PrivateKey string `yaml:"private_key,omitempty"` // 64-char hex string (32 bytes)
	PublicKey  string `yaml:"public_key,omitempty"`  // Optional - derived from private_key if not specified
}

// HasIdentityKeypair returns true if the identity private key is configured in config.
func (a *AgentConfig) HasIdentityKeypair() bool {
	return a.PrivateKey != ""
}

// GetIdentityPrivateKey returns the parsed identity private key.
// Returns an error if the key is not configured or invalid.
func (a *AgentConfig) GetIdentityPrivateKey() ([KeySize]byte, error) {
	return parseHexKey(a.PrivateKey, "agent private key", KeySize)
}

// GetIdentityPublicKey returns the parsed identity public key if configured.
// Returns the key, a boolean indicating if it was configured, and any error.
func (a *AgentConfig) GetIdentityPublicKey() ([KeySize]byte, bool, error) {
	if a.PublicKey == "" {
		return [KeySize]byte{}, false, nil
	}
	key, err := parseHexKey(a.PublicKey, "agent public key", KeySize)
	return key, true, err
}

// ListenerConfig defines a transport listener.
type ListenerConfig struct {
	Transport string    `yaml:"transport"`           // quic, h2, ws (required)
	Address   string    `yaml:"address"`             // listen address (required)
	Path      string    `yaml:"path,omitempty"`      // HTTP path for h2/ws
	PlainText bool      `yaml:"plaintext,omitempty"` // Allow plain WebSocket without TLS (for reverse proxy)
	TLS       TLSConfig `yaml:"tls,omitempty"`
}

// PeerConfig defines a peer connection.
type PeerConfig struct {
	ID        string    `yaml:"id,omitempty"`         // Expected peer AgentID
	Transport string    `yaml:"transport"`            // quic, h2, ws (required)
	Address   string    `yaml:"address"`              // peer address (required)
	Path      string    `yaml:"path,omitempty"`       // HTTP path for h2/ws
	Proxy     string    `yaml:"proxy,omitempty"`      // HTTP proxy for ws
	ProxyAuth ProxyAuth `yaml:"proxy_auth,omitempty"` // Proxy authentication
	TLS       TLSConfig `yaml:"tls,omitempty"`
}

// TLSConfig defines per-connection TLS settings that can override global settings.
// For each certificate/key, you can specify either a file path or inline PEM content.
// If both are provided, inline PEM takes precedence.
type TLSConfig struct {
	// Override global cert/key (optional - uses global if not set)
	Cert    string `yaml:"cert,omitempty"`     // Certificate file path
	Key     string `yaml:"key,omitempty"`      // Private key file path
	CertPEM string `yaml:"cert_pem,omitempty"` // Certificate PEM content
	KeyPEM  string `yaml:"key_pem,omitempty"`  // Private key PEM content

	// Override global CA (optional - peer connections only)
	CA    string `yaml:"ca,omitempty"`     // CA certificate file path
	CAPEM string `yaml:"ca_pem,omitempty"` // CA certificate PEM content

	// mTLS override (optional - listener only, uses global if nil)
	// Use pointer to distinguish "not set" from "false"
	MTLS *bool `yaml:"mtls,omitempty"`

	// Strict override (optional - peer connections only, uses global if nil)
	// When true, peer certificates must be validated against CA
	Strict *bool `yaml:"strict,omitempty"`
}

// GetCertPEM returns the certificate PEM content, reading from file if necessary.
func (t *TLSConfig) GetCertPEM() ([]byte, error) {
	return getPEM(t.CertPEM, t.Cert)
}

// GetKeyPEM returns the private key PEM content, reading from file if necessary.
func (t *TLSConfig) GetKeyPEM() ([]byte, error) {
	return getPEM(t.KeyPEM, t.Key)
}

// GetCAPEM returns the CA certificate PEM content, reading from file if necessary.
func (t *TLSConfig) GetCAPEM() ([]byte, error) {
	return getPEM(t.CAPEM, t.CA)
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

// GetEffectiveStrict returns the effective strict TLS verification setting,
// preferring per-connection override over global config.
// Default is false (no certificate verification) because the E2E layer provides security.
func (c *Config) GetEffectiveStrict(override *TLSConfig) bool {
	// Check per-connection override first
	if override != nil && override.Strict != nil {
		return *override.Strict
	}
	// Fall back to global
	return c.TLS.Strict
}

// ProxyAuth defines proxy authentication.
type ProxyAuth struct {
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`
}

// SOCKS5Config defines SOCKS5 server settings.
type SOCKS5Config struct {
	Enabled        bool                   `yaml:"enabled,omitempty"`
	Address        string                 `yaml:"address,omitempty"`
	Auth           SOCKS5AuthConfig       `yaml:"auth,omitempty"`
	MaxConnections int                    `yaml:"max_connections,omitempty"`
	WebSocket      WebSocketSOCKS5Config  `yaml:"websocket,omitempty"`
}

// WebSocketSOCKS5Config defines WebSocket SOCKS5 listener settings.
// This allows SOCKS5 protocol to be tunneled over WebSocket transport,
// which can pass through firewalls that block raw TCP/SOCKS5 traffic.
type WebSocketSOCKS5Config struct {
	// Enabled controls whether the WebSocket SOCKS5 listener is active.
	Enabled bool `yaml:"enabled,omitempty"`

	// Address is the listen address (e.g., "0.0.0.0:8443" or "127.0.0.1:8081").
	Address string `yaml:"address,omitempty"`

	// Path is the WebSocket upgrade path (default: "/socks5").
	Path string `yaml:"path,omitempty"`

	// PlainText allows running without TLS (for reverse proxy mode).
	// When true, the listener serves plain HTTP/WebSocket.
	// Use this when running behind nginx/Caddy that handles TLS termination.
	PlainText bool `yaml:"plaintext,omitempty"`
}

// SOCKS5AuthConfig defines SOCKS5 authentication settings.
type SOCKS5AuthConfig struct {
	Enabled bool               `yaml:"enabled,omitempty"`
	Users   []SOCKS5UserConfig `yaml:"users,omitempty"`
}

// SOCKS5UserConfig defines a SOCKS5 user.
type SOCKS5UserConfig struct {
	Username string `yaml:"username,omitempty"`
	// Password is the plaintext password (deprecated, use PasswordHash).
	Password string `yaml:"password,omitempty"`
	// PasswordHash is the bcrypt hash of the password (recommended).
	// Generate with: htpasswd -bnBC 10 "" <password> | tr -d ':\n'
	PasswordHash string `yaml:"password_hash,omitempty"`
}

// ExitConfig defines exit node settings.
type ExitConfig struct {
	Enabled      bool      `yaml:"enabled,omitempty"`
	Routes       []string  `yaml:"routes,omitempty"`        // CIDR routes to advertise
	DomainRoutes []string  `yaml:"domain_routes,omitempty"` // Domain patterns to advertise (exact or *.wildcard)
	DNS          DNSConfig `yaml:"dns,omitempty"`
}

// DNSConfig defines DNS settings for exit nodes.
type DNSConfig struct {
	Servers []string      `yaml:"servers,omitempty"`
	Timeout time.Duration `yaml:"timeout,omitempty"`
}

// RoutingConfig defines routing parameters.
type RoutingConfig struct {
	AdvertiseInterval time.Duration `yaml:"advertise_interval,omitempty"`
	NodeInfoInterval  time.Duration `yaml:"node_info_interval,omitempty"` // Defaults to AdvertiseInterval if not set
	RouteTTL          time.Duration `yaml:"route_ttl,omitempty"`
	MaxHops           int           `yaml:"max_hops,omitempty"`
}

// ConnectionsConfig defines connection tuning parameters.
type ConnectionsConfig struct {
	IdleThreshold   time.Duration   `yaml:"idle_threshold,omitempty"`
	Timeout         time.Duration   `yaml:"timeout,omitempty"`
	KeepaliveJitter float64         `yaml:"keepalive_jitter,omitempty"` // Jitter fraction for keepalive timing (0.0-1.0)
	Reconnect       ReconnectConfig `yaml:"reconnect,omitempty"`
}

// ReconnectConfig defines reconnection behavior.
type ReconnectConfig struct {
	InitialDelay time.Duration `yaml:"initial_delay,omitempty"`
	MaxDelay     time.Duration `yaml:"max_delay,omitempty"`
	Multiplier   float64       `yaml:"multiplier,omitempty"`
	Jitter       float64       `yaml:"jitter,omitempty"`
	MaxRetries   int           `yaml:"max_retries,omitempty"` // 0 = infinite
}

// LimitsConfig defines resource limits.
type LimitsConfig struct {
	MaxStreamsPerPeer int           `yaml:"max_streams_per_peer,omitempty"`
	MaxStreamsTotal   int           `yaml:"max_streams_total,omitempty"`
	MaxPendingOpens   int           `yaml:"max_pending_opens,omitempty"`
	StreamOpenTimeout time.Duration `yaml:"stream_open_timeout,omitempty"`
	BufferSize        int           `yaml:"buffer_size,omitempty"`
}

// HTTPConfig defines HTTP API server settings.
type HTTPConfig struct {
	Enabled      bool          `yaml:"enabled,omitempty"`
	Address      string        `yaml:"address,omitempty"`
	ReadTimeout  time.Duration `yaml:"read_timeout,omitempty"`
	WriteTimeout time.Duration `yaml:"write_timeout,omitempty"`

	// Minimal mode - only enable /health, /healthz, /ready endpoints.
	// When true, overrides all other endpoint flags to false.
	Minimal bool `yaml:"minimal,omitempty"`

	// Endpoint group controls. All default to true when http.enabled=true.
	// Use pointer types to distinguish between "not set" (nil = use default) and "explicitly false".
	// Disabled endpoints return 404 and log access attempts.
	Pprof     *bool `yaml:"pprof,omitempty"`      // /debug/pprof/* - Go profiling endpoints
	Dashboard *bool `yaml:"dashboard,omitempty"`  // /ui/*, /api/* - Web dashboard and API
	RemoteAPI *bool `yaml:"remote_api,omitempty"` // /agents/* - Distributed mesh APIs
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
	Enabled bool `yaml:"enabled,omitempty"`

	// MaxFileSize is the maximum allowed file size in bytes.
	// Default is 500MB (500 * 1024 * 1024).
	MaxFileSize int64 `yaml:"max_file_size,omitempty"`

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
	AllowedPaths []string `yaml:"allowed_paths,omitempty"`

	// PasswordHash is the bcrypt hash of the file transfer password.
	// If set, all file transfer requests must include the correct password.
	// Generate with: htpasswd -bnBC 10 "" <password> | tr -d ':\n'
	PasswordHash string `yaml:"password_hash,omitempty"`
}

// ShellConfig defines remote shell settings.
type ShellConfig struct {
	// Enabled controls whether shell is available on this agent.
	Enabled bool `yaml:"enabled,omitempty"`

	// Whitelist contains allowed commands. Empty list = no commands allowed.
	// Use ["*"] to allow all commands (for testing only!).
	// Commands should be base names only (e.g., "whoami", "ls", "bash").
	Whitelist []string `yaml:"whitelist,omitempty"`

	// PasswordHash is the bcrypt hash of the shell password.
	// If set, all shell requests must include the correct password.
	// Generate with: muti-metroo hash
	PasswordHash string `yaml:"password_hash,omitempty"`

	// Timeout is the optional command timeout (0 = no timeout).
	Timeout time.Duration `yaml:"timeout,omitempty"`

	// MaxSessions limits concurrent shell sessions (0 = unlimited).
	MaxSessions int `yaml:"max_sessions,omitempty"`
}

// UDPConfig configures UDP relay support for exit nodes.
// UDP relay enables SOCKS5 UDP ASSOCIATE for tunneling UDP traffic through the mesh.
type UDPConfig struct {
	// Enabled controls whether UDP relay is available on this exit node.
	Enabled bool `yaml:"enabled,omitempty"`

	// MaxAssociations limits concurrent UDP associations (0 = unlimited).
	MaxAssociations int `yaml:"max_associations,omitempty"`

	// IdleTimeout is the timeout for inactive UDP associations.
	IdleTimeout time.Duration `yaml:"idle_timeout,omitempty"`

	// MaxDatagramSize is the maximum UDP payload size in bytes.
	// Default is 1472 (Ethernet MTU minus IP and UDP headers).
	MaxDatagramSize int `yaml:"max_datagram_size,omitempty"`
}

// ICMPConfig configures ICMP echo (ping) support for exit nodes.
// When enabled, agents can send ICMP echo requests to allowed destinations.
type ICMPConfig struct {
	// Enabled controls whether ICMP echo is available on this exit node.
	Enabled bool `yaml:"enabled,omitempty"`

	// MaxSessions limits concurrent ICMP sessions (0 = unlimited).
	MaxSessions int `yaml:"max_sessions,omitempty"`

	// IdleTimeout is the timeout for inactive ICMP sessions.
	IdleTimeout time.Duration `yaml:"idle_timeout,omitempty"`

	// EchoTimeout is the timeout for each individual ICMP echo request.
	EchoTimeout time.Duration `yaml:"echo_timeout,omitempty"`
}

// ForwardConfig configures TCP port forwarding.
// This enables ngrok/localtunnel-style reverse port forwarding where local services
// can be exposed through the mesh network using named routing keys.
type ForwardConfig struct {
	// Endpoints define port forward exit points - where forwarded connections terminate.
	// Each endpoint maps a routing key to a fixed target host:port.
	// The agent advertises these routing keys through the mesh.
	Endpoints []ForwardEndpoint `yaml:"endpoints,omitempty"`

	// Listeners define port forward ingress points - where external connections enter.
	// Each listener binds to a local address and forwards connections to the
	// agent with the matching routing key.
	Listeners []ForwardListener `yaml:"listeners,omitempty"`
}

// ForwardEndpoint defines a port forward exit point configuration.
// Traffic arriving for this routing key will be forwarded to the target.
type ForwardEndpoint struct {
	// Key is the routing key that identifies this port forward endpoint.
	// Must be unique within the mesh. Example: "my-web-server"
	Key string `yaml:"key,omitempty"`

	// Target is the fixed destination host:port for forwarded connections.
	// Example: "localhost:3000" or "192.168.1.10:8080"
	Target string `yaml:"target,omitempty"`
}

// ForwardListener defines a port forward ingress point configuration.
// Connections to this listener are forwarded through the mesh to the
// agent with a matching routing key endpoint.
type ForwardListener struct {
	// Key is the routing key to look up in the mesh.
	// Must match a ForwardEndpoint.Key on some agent.
	Key string `yaml:"key,omitempty"`

	// Address is the local address to listen on.
	// Example: ":8080" or "127.0.0.1:8080"
	Address string `yaml:"address,omitempty"`

	// MaxConnections limits concurrent connections (0 = unlimited).
	MaxConnections int `yaml:"max_connections,omitempty"`
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
				Servers: []string{}, // Empty = use system resolver (supports .local domains)
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
			Enabled:         true,
			MaxAssociations: 1000,            // Default limit
			IdleTimeout:     5 * time.Minute, // Same as connection idle threshold
			MaxDatagramSize: 1472,            // MTU - IP/UDP headers
		},
		ICMP: ICMPConfig{
			Enabled:     true,
			MaxSessions: 100,               // Default limit
			IdleTimeout: 60 * time.Second,  // Session idle timeout
			EchoTimeout: 5 * time.Second,   // Per-echo timeout
		},
		Forward: ForwardConfig{
			Endpoints: []ForwardEndpoint{},
			Listeners: []ForwardListener{},
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

// LoadOrEmbeddedFrom loads configuration from embedded binary data in a specific binary,
// otherwise falls back to loading from the specified config file path.
// This is useful for DLLs where os.Executable() returns the host process path.
// If binaryPath is empty, falls back to LoadOrEmbedded behavior.
func LoadOrEmbeddedFrom(binaryPath, configPath string) (*Config, bool, error) {
	// If no binary path specified, use standard behavior
	if binaryPath == "" {
		return LoadOrEmbedded(configPath)
	}

	// Check for embedded config in the specified binary
	hasEmbedded, err := embed.HasEmbeddedConfig(binaryPath)
	if err == nil && hasEmbedded {
		data, err := embed.ReadEmbeddedConfig(binaryPath)
		if err != nil {
			return nil, false, fmt.Errorf("failed to read embedded config from %s: %w", binaryPath, err)
		}
		cfg, err := Parse(data)
		if err != nil {
			return nil, false, fmt.Errorf("failed to parse embedded config: %w", err)
		}
		return cfg, true, nil
	}

	// Fall back to config file
	if configPath == "" {
		return nil, false, fmt.Errorf("no embedded config found and no config path specified")
	}
	cfg, err := Load(configPath)
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

	// Validate default_action (only for embedded config usage)
	if c.DefaultAction != "" && !isOneOf(c.DefaultAction, "run", "help") {
		errs = append(errs, fmt.Sprintf("invalid default_action: %s (must be 'run' or 'help')", c.DefaultAction))
	}

	// Validate agent config
	// data_dir is required unless identity keypair is fully specified in config
	if c.Agent.DataDir == "" && !c.Agent.HasIdentityKeypair() {
		errs = append(errs, "agent.data_dir is required when agent.private_key is not configured")
	}
	// data_dir is also required if agent.id is "auto" (can't auto-generate without persistence)
	if c.Agent.DataDir == "" && (c.Agent.ID == "" || c.Agent.ID == "auto") {
		errs = append(errs, "agent.data_dir is required when agent.id is 'auto' (cannot auto-generate without persistence)")
	}
	if !isValidLogLevel(c.Agent.LogLevel) {
		errs = append(errs, fmt.Sprintf("invalid log_level: %s (must be debug, info, warn, or error)", c.Agent.LogLevel))
	}
	if !isValidLogFormat(c.Agent.LogFormat) {
		errs = append(errs, fmt.Sprintf("invalid log_format: %s (must be text or json)", c.Agent.LogFormat))
	}

	// Validate identity keypair configuration
	if err := c.validateIdentityKeypair(); err != nil {
		errs = append(errs, err.Error())
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

	// Validate SOCKS5 WebSocket
	if c.SOCKS5.WebSocket.Enabled {
		if c.SOCKS5.WebSocket.Address == "" {
			errs = append(errs, "socks5.websocket.address is required when enabled")
		}
		if c.SOCKS5.WebSocket.Path != "" && !strings.HasPrefix(c.SOCKS5.WebSocket.Path, "/") {
			errs = append(errs, "socks5.websocket.path must start with '/'")
		}
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

	// Validate port forward configuration
	if err := c.validateForward(); err != nil {
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

	// Check for strict mode without CA (warning: peer verification won't work)
	if c.TLS.Strict && !c.TLS.HasCA() {
		return fmt.Errorf("tls.ca is required when tls.strict is enabled (for peer certificate verification)")
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

// validateIdentityKeypair validates the agent identity keypair configuration.
func (c *Config) validateIdentityKeypair() error {
	// If no private key, check that public key is also not set
	if c.Agent.PrivateKey == "" {
		if c.Agent.PublicKey != "" {
			return fmt.Errorf("agent.public_key requires agent.private_key to be set")
		}
		return nil
	}

	// Validate private key format
	if _, err := c.Agent.GetIdentityPrivateKey(); err != nil {
		return fmt.Errorf("agent.private_key: %w", err)
	}

	// Validate public key format if set
	// Note: The derivation check (public matches private) is done in identity.KeypairFromConfig()
	if c.Agent.PublicKey != "" {
		if _, _, err := c.Agent.GetIdentityPublicKey(); err != nil {
			return fmt.Errorf("agent.public_key: %w", err)
		}
	}

	return nil
}

// validateForward validates the port forward configuration.
func (c *Config) validateForward() error {
	var errs []string

	// Validate forward endpoints
	seenKeys := make(map[string]bool)
	for i, ep := range c.Forward.Endpoints {
		if ep.Key == "" {
			errs = append(errs, fmt.Sprintf("forward.endpoints[%d]: key is required", i))
			continue
		}
		if seenKeys[ep.Key] {
			errs = append(errs, fmt.Sprintf("forward.endpoints[%d]: duplicate key %q", i, ep.Key))
		}
		seenKeys[ep.Key] = true

		if ep.Target == "" {
			errs = append(errs, fmt.Sprintf("forward.endpoints[%d]: target is required", i))
		} else if err := isValidHostPort(ep.Target); err != nil {
			errs = append(errs, fmt.Sprintf("forward.endpoints[%d]: invalid target: %v", i, err))
		}
	}

	// Validate forward listeners
	for i, lis := range c.Forward.Listeners {
		if lis.Key == "" {
			errs = append(errs, fmt.Sprintf("forward.listeners[%d]: key is required", i))
		}
		if lis.Address == "" {
			errs = append(errs, fmt.Sprintf("forward.listeners[%d]: address is required", i))
		}
		if lis.MaxConnections < 0 {
			errs = append(errs, fmt.Sprintf("forward.listeners[%d]: max_connections cannot be negative", i))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("forward validation errors:\n    - %s", strings.Join(errs, "\n    - "))
	}

	return nil
}

// isValidHostPort validates a host:port string.
func isValidHostPort(hostPort string) error {
	host, port, err := net.SplitHostPort(hostPort)
	if err != nil {
		return fmt.Errorf("invalid host:port format: %w", err)
	}
	if host == "" {
		return fmt.Errorf("host cannot be empty")
	}
	if port == "" {
		return fmt.Errorf("port cannot be empty")
	}
	return nil
}

func isValidLogLevel(level string) bool {
	return isOneOf(level, "debug", "info", "warn", "error")
}

func isValidLogFormat(format string) bool {
	return isOneOf(format, "text", "json")
}

func isValidTransport(transport string) bool {
	return isOneOf(transport, "quic", "h2", "ws")
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

	// Note: cert/key is no longer required - if not configured, a self-signed
	// certificate will be auto-generated at startup. Only validate if partially configured.
	hasCert := l.TLS.HasCert() || c.TLS.HasCert()
	hasKey := l.TLS.HasKey() || c.TLS.HasKey()
	if hasCert != hasKey {
		return fmt.Errorf("tls certificate and key must both be specified or both be empty (auto-generated if empty)")
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

	// Check for strict mode without CA
	effectiveStrict := c.GetEffectiveStrict(&p.TLS)
	hasCA := p.TLS.HasCA() || c.TLS.HasCA()
	if effectiveStrict && !hasCA {
		return fmt.Errorf("tls.ca is required when strict mode is enabled (for peer certificate verification)")
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

// redact replaces non-empty strings with redactedValue.
func redact(s *string) {
	if *s != "" {
		*s = redactedValue
	}
}

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
	redact(&redacted.TLS.Key)
	redact(&redacted.TLS.KeyPEM)

	// Redact sensitive fields in peers
	for i := range redacted.Peers {
		redact(&redacted.Peers[i].ProxyAuth.Password)
		redact(&redacted.Peers[i].TLS.Key)
		redact(&redacted.Peers[i].TLS.KeyPEM)
	}

	// Redact sensitive fields in listeners
	for i := range redacted.Listeners {
		redact(&redacted.Listeners[i].TLS.Key)
		redact(&redacted.Listeners[i].TLS.KeyPEM)
	}

	// Redact SOCKS5 user passwords and password hashes
	for i := range redacted.SOCKS5.Auth.Users {
		redact(&redacted.SOCKS5.Auth.Users[i].Password)
		redact(&redacted.SOCKS5.Auth.Users[i].PasswordHash)
	}

	// Redact other sensitive fields
	redact(&redacted.Agent.PrivateKey)
	redact(&redacted.FileTransfer.PasswordHash)
	redact(&redacted.Shell.PasswordHash)
	redact(&redacted.Management.PrivateKey)

	return redacted
}

// HasSensitiveData returns true if the config contains any sensitive data.
func (c *Config) HasSensitiveData() bool {
	// Check agent identity private key
	if c.Agent.PrivateKey != "" {
		return true
	}

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
