package udp

import (
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Enabled {
		t.Error("Enabled should be false by default")
	}
	if len(cfg.AllowedPorts) != 0 {
		t.Error("AllowedPorts should be empty by default")
	}
	if cfg.MaxAssociations != 1000 {
		t.Errorf("MaxAssociations = %d, want 1000", cfg.MaxAssociations)
	}
	if cfg.IdleTimeout != 5*time.Minute {
		t.Errorf("IdleTimeout = %v, want 5m", cfg.IdleTimeout)
	}
	if cfg.MaxDatagramSize != 1472 {
		t.Errorf("MaxDatagramSize = %d, want 1472", cfg.MaxDatagramSize)
	}
}

func TestConfig_IsPortAllowed(t *testing.T) {
	tests := []struct {
		name         string
		allowedPorts []string
		port         uint16
		want         bool
	}{
		{
			name:         "empty list denies all",
			allowedPorts: []string{},
			port:         53,
			want:         false,
		},
		{
			name:         "wildcard allows all",
			allowedPorts: []string{"*"},
			port:         53,
			want:         true,
		},
		{
			name:         "wildcard allows high port",
			allowedPorts: []string{"*"},
			port:         65535,
			want:         true,
		},
		{
			name:         "specific port allowed",
			allowedPorts: []string{"53", "123"},
			port:         53,
			want:         true,
		},
		{
			name:         "specific port not in list",
			allowedPorts: []string{"53", "123"},
			port:         80,
			want:         false,
		},
		{
			name:         "NTP port allowed",
			allowedPorts: []string{"53", "123"},
			port:         123,
			want:         true,
		},
		{
			name:         "port 0 with wildcard",
			allowedPorts: []string{"*"},
			port:         0,
			want:         true,
		},
		{
			name:         "port 0 with specific list",
			allowedPorts: []string{"53"},
			port:         0,
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{AllowedPorts: tt.allowedPorts}
			got := cfg.IsPortAllowed(tt.port)
			if got != tt.want {
				t.Errorf("IsPortAllowed(%d) = %v, want %v", tt.port, got, tt.want)
			}
		})
	}
}
