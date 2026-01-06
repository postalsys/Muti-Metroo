package udp

import (
	"strconv"
	"time"
)

// Config holds configuration for the UDP relay handler.
type Config struct {
	// Enabled controls whether UDP relay is active.
	// When false, UDP_OPEN requests are rejected with ErrUDPDisabled.
	Enabled bool

	// AllowedPorts is the whitelist of allowed destination ports.
	// Empty slice means no ports allowed.
	// ["*"] allows all ports.
	// Specific ports: ["53", "123"]
	AllowedPorts []string

	// MaxAssociations limits concurrent UDP associations.
	// 0 means unlimited.
	MaxAssociations int

	// IdleTimeout is how long an association can be idle before cleanup.
	// 0 means no timeout.
	IdleTimeout time.Duration

	// MaxDatagramSize is the maximum UDP payload size.
	// Default is 1472 (typical MTU - IP/UDP headers).
	MaxDatagramSize int
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Enabled:         false,
		AllowedPorts:    []string{},
		MaxAssociations: 1000,
		IdleTimeout:     5 * time.Minute,
		MaxDatagramSize: 1472,
	}
}

// IsPortAllowed checks if a destination port is permitted.
func (c *Config) IsPortAllowed(port uint16) bool {
	if len(c.AllowedPorts) == 0 {
		return false
	}

	portStr := strconv.Itoa(int(port))

	for _, allowed := range c.AllowedPorts {
		if allowed == "*" {
			return true
		}
		if allowed == portStr {
			return true
		}
	}

	return false
}
