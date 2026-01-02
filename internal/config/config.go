// Package config provides configuration parsing and validation for Muti Metroo.
package config

import (
	"fmt"
	"net"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the complete agent configuration.
type Config struct {
	Agent        AgentConfig        `yaml:"agent"`
	TLS          GlobalTLSConfig    `yaml:"tls"`
	Listeners    []ListenerConfig   `yaml:"listeners"`
	Peers        []PeerConfig       `yaml:"peers"`
	SOCKS5       SOCKS5Config       `yaml:"socks5"`
	Exit         ExitConfig         `yaml:"exit"`
	Routing      RoutingConfig      `yaml:"routing"`
	Connections  ConnectionsConfig  `yaml:"connections"`
	Limits       LimitsConfig       `yaml:"limits"`
	HTTP         HTTPConfig         `yaml:"http"`
	Control      ControlConfig      `yaml:"control"`
	RPC          RPCConfig          `yaml:"rpc"`
	FileTransfer FileTransferConfig `yaml:"file_transfer"`
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
	Enabled bool        `yaml:"enabled"`
	Routes  []string    `yaml:"routes"` // CIDR routes to advertise
	DNS     DNSConfig   `yaml:"dns"`
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
	IdleThreshold time.Duration   `yaml:"idle_threshold"`
	Timeout       time.Duration   `yaml:"timeout"`
	Reconnect     ReconnectConfig `yaml:"reconnect"`
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
}

// ControlConfig defines control socket settings.
type ControlConfig struct {
	Enabled    bool   `yaml:"enabled"`
	SocketPath string `yaml:"socket_path"`
}

// RPCConfig defines remote procedure call settings.
type RPCConfig struct {
	// Enabled controls whether RPC is available on this agent.
	Enabled bool `yaml:"enabled"`

	// Whitelist contains allowed commands. Empty list = no commands allowed.
	// Use ["*"] to allow all commands (for testing only!).
	// Commands should be base names only (e.g., "whoami", "ls", "ip").
	Whitelist []string `yaml:"whitelist"`

	// PasswordHash is the bcrypt hash of the RPC password.
	// If set, all RPC requests must include the correct password.
	// Generate with: htpasswd -bnBC 10 "" <password> | tr -d ':\n'
	PasswordHash string `yaml:"password_hash"`

	// Timeout is the default command execution timeout.
	Timeout time.Duration `yaml:"timeout"`
}

// FileTransferConfig defines file transfer settings.
type FileTransferConfig struct {
	// Enabled controls whether file transfer is available on this agent.
	Enabled bool `yaml:"enabled"`

	// MaxFileSize is the maximum allowed file size in bytes.
	// Default is 500MB (500 * 1024 * 1024).
	MaxFileSize int64 `yaml:"max_file_size"`

	// AllowedPaths contains path prefixes that are allowed for file operations.
	// Empty list means all absolute paths are allowed.
	// Example: ["/tmp", "/home/user/uploads"]
	AllowedPaths []string `yaml:"allowed_paths"`

	// PasswordHash is the bcrypt hash of the file transfer password.
	// If set, all file transfer requests must include the correct password.
	// Generate with: htpasswd -bnBC 10 "" <password> | tr -d ':\n'
	PasswordHash string `yaml:"password_hash"`
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
			IdleThreshold: 5 * time.Minute, // Long-running connections like SSH should stay alive
			Timeout:       90 * time.Second,
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
		Control: ControlConfig{
			Enabled:    false,
			SocketPath: "./data/control.sock",
		},
		RPC: RPCConfig{
			Enabled:   false,
			Whitelist: []string{}, // Empty = no commands allowed
			Timeout:   60 * time.Second,
		},
		FileTransfer: FileTransferConfig{
			Enabled:      false,
			MaxFileSize:  500 * 1024 * 1024, // 500 MB
			AllowedPaths: []string{},        // Empty = all absolute paths allowed
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

	// Validate exit routes
	for i, route := range c.Exit.Routes {
		if !isValidCIDR(route) {
			errs = append(errs, fmt.Sprintf("exit.routes[%d]: invalid CIDR: %s", i, route))
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

	// Redact RPC password hash
	if redacted.RPC.PasswordHash != "" {
		redacted.RPC.PasswordHash = redactedValue
	}

	// Redact FileTransfer password hash
	if redacted.FileTransfer.PasswordHash != "" {
		redacted.FileTransfer.PasswordHash = redactedValue
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

	// Check RPC password hash
	if c.RPC.PasswordHash != "" {
		return true
	}

	// Check FileTransfer password hash
	if c.FileTransfer.PasswordHash != "" {
		return true
	}

	return false
}
