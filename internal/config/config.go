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
	Agent       AgentConfig       `yaml:"agent"`
	Listeners   []ListenerConfig  `yaml:"listeners"`
	Peers       []PeerConfig      `yaml:"peers"`
	SOCKS5      SOCKS5Config      `yaml:"socks5"`
	Exit        ExitConfig        `yaml:"exit"`
	Routing     RoutingConfig     `yaml:"routing"`
	Connections ConnectionsConfig `yaml:"connections"`
	Limits      LimitsConfig      `yaml:"limits"`
	Health      HealthConfig      `yaml:"health"`
	Control     ControlConfig     `yaml:"control"`
}

// AgentConfig contains agent identity settings.
type AgentConfig struct {
	ID        string `yaml:"id"`         // "auto" or hex string
	DataDir   string `yaml:"data_dir"`   // Directory for persistent state
	LogLevel  string `yaml:"log_level"`  // debug, info, warn, error
	LogFormat string `yaml:"log_format"` // text, json
}

// ListenerConfig defines a transport listener.
type ListenerConfig struct {
	Transport string    `yaml:"transport"` // quic, h2, ws
	Address   string    `yaml:"address"`   // listen address
	Path      string    `yaml:"path"`      // HTTP path for h2/ws
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

// TLSConfig defines TLS settings.
type TLSConfig struct {
	Cert        string `yaml:"cert"`        // Certificate file path
	Key         string `yaml:"key"`         // Private key file path
	CA          string `yaml:"ca"`          // CA certificate file path
	ClientCA    string `yaml:"client_ca"`   // Client CA for mTLS
	Fingerprint string `yaml:"fingerprint"` // Certificate fingerprint for pinning
	InsecureSkipVerify bool `yaml:"insecure_skip_verify"` // Skip verification (dev only)
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
	Password string `yaml:"password"`
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
	AdvertiseInterval time.Duration `yaml:"advertise_interval"`
	RouteTTL          time.Duration `yaml:"route_ttl"`
	MaxHops           int           `yaml:"max_hops"`
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

// HealthConfig defines health check server settings.
type HealthConfig struct {
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
		Health: HealthConfig{
			Enabled:      false,
			Address:      ":8080",
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
		},
		Control: ControlConfig{
			Enabled:    false,
			SocketPath: "./data/control.sock",
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

	// Validate listeners
	for i, l := range c.Listeners {
		if err := validateListener(l); err != nil {
			errs = append(errs, fmt.Sprintf("listeners[%d]: %v", i, err))
		}
	}

	// Validate peers
	for i, p := range c.Peers {
		if err := validatePeer(p); err != nil {
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

func validateListener(l ListenerConfig) error {
	if !isValidTransport(l.Transport) {
		return fmt.Errorf("invalid transport: %s (must be quic, h2, or ws)", l.Transport)
	}
	if l.Address == "" {
		return fmt.Errorf("address is required")
	}
	if (l.Transport == "h2" || l.Transport == "ws") && l.Path == "" {
		return fmt.Errorf("path is required for %s transport", l.Transport)
	}
	if l.TLS.Cert == "" || l.TLS.Key == "" {
		return fmt.Errorf("tls.cert and tls.key are required")
	}
	return nil
}

func validatePeer(p PeerConfig) error {
	if p.ID == "" {
		return fmt.Errorf("id is required")
	}
	if !isValidTransport(p.Transport) {
		return fmt.Errorf("invalid transport: %s (must be quic, h2, or ws)", p.Transport)
	}
	if p.Address == "" {
		return fmt.Errorf("address is required")
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

	// Redact sensitive fields in peers
	for i := range redacted.Peers {
		if redacted.Peers[i].ProxyAuth.Password != "" {
			redacted.Peers[i].ProxyAuth.Password = redactedValue
		}
		// Redact TLS key paths as they point to sensitive files
		if redacted.Peers[i].TLS.Key != "" {
			redacted.Peers[i].TLS.Key = redactedValue
		}
	}

	// Redact sensitive fields in listeners
	for i := range redacted.Listeners {
		if redacted.Listeners[i].TLS.Key != "" {
			redacted.Listeners[i].TLS.Key = redactedValue
		}
	}

	// Redact SOCKS5 user passwords
	for i := range redacted.SOCKS5.Auth.Users {
		if redacted.SOCKS5.Auth.Users[i].Password != "" {
			redacted.SOCKS5.Auth.Users[i].Password = redactedValue
		}
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

	// Check SOCKS5 passwords
	for _, u := range c.SOCKS5.Auth.Users {
		if u.Password != "" {
			return true
		}
	}

	return false
}
