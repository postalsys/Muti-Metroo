package icmp

import (
	"net"
	"time"
)

// Config holds configuration for the ICMP echo handler.
type Config struct {
	// Enabled controls whether ICMP echo is active.
	// When false, ICMP_OPEN requests are rejected with ErrICMPDisabled.
	Enabled bool

	// AllowedCIDRs is a list of CIDRs that can be pinged.
	// Empty list means no destinations allowed.
	// Use ["0.0.0.0/0"] to allow all IPv4 destinations.
	AllowedCIDRs []*net.IPNet

	// MaxSessions limits concurrent ICMP sessions.
	// 0 means unlimited.
	MaxSessions int

	// IdleTimeout is how long a session can be idle before cleanup.
	// 0 means no timeout.
	IdleTimeout time.Duration

	// EchoTimeout is the timeout for each individual ICMP echo.
	// Default is 5 seconds.
	EchoTimeout time.Duration
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Enabled:      false,
		AllowedCIDRs: nil, // No destinations allowed by default
		MaxSessions:  100,
		IdleTimeout:  60 * time.Second,
		EchoTimeout:  5 * time.Second,
	}
}

// IsDestinationAllowed checks if the given IP is within allowed CIDRs.
func (c *Config) IsDestinationAllowed(ip net.IP) bool {
	if len(c.AllowedCIDRs) == 0 {
		return false
	}

	// Ensure we have IPv4
	ip4 := ip.To4()
	if ip4 == nil {
		return false // IPv6 not supported
	}

	for _, cidr := range c.AllowedCIDRs {
		if cidr.Contains(ip4) {
			return true
		}
	}

	return false
}

// ParseCIDRs parses a list of CIDR strings into IPNet structures.
func ParseCIDRs(cidrs []string) ([]*net.IPNet, error) {
	result := make([]*net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, err
		}
		result = append(result, ipNet)
	}
	return result, nil
}
