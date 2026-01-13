package icmp

import (
	"net"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Enabled {
		t.Error("Enabled should be false by default")
	}
	if cfg.AllowedCIDRs != nil {
		t.Error("AllowedCIDRs should be nil by default")
	}
	if cfg.MaxSessions != 100 {
		t.Errorf("MaxSessions = %d, want 100", cfg.MaxSessions)
	}
	if cfg.IdleTimeout.Seconds() != 60 {
		t.Errorf("IdleTimeout = %v, want 60s", cfg.IdleTimeout)
	}
	if cfg.EchoTimeout.Seconds() != 5 {
		t.Errorf("EchoTimeout = %v, want 5s", cfg.EchoTimeout)
	}
}

func TestParseCIDRs(t *testing.T) {
	tests := []struct {
		name    string
		cidrs   []string
		want    int
		wantErr bool
	}{
		{
			name:  "single CIDR",
			cidrs: []string{"10.0.0.0/8"},
			want:  1,
		},
		{
			name:  "multiple CIDRs",
			cidrs: []string{"10.0.0.0/8", "192.168.0.0/16", "172.16.0.0/12"},
			want:  3,
		},
		{
			name:  "all IPv4",
			cidrs: []string{"0.0.0.0/0"},
			want:  1,
		},
		{
			name:  "empty list",
			cidrs: []string{},
			want:  0,
		},
		{
			name:    "invalid CIDR",
			cidrs:   []string{"not-a-cidr"},
			wantErr: true,
		},
		{
			name:    "missing prefix",
			cidrs:   []string{"10.0.0.0"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseCIDRs(tt.cidrs)
			if tt.wantErr {
				if err == nil {
					t.Error("ParseCIDRs() should fail")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseCIDRs() error = %v", err)
			}
			if len(result) != tt.want {
				t.Errorf("ParseCIDRs() returned %d CIDRs, want %d", len(result), tt.want)
			}
		})
	}
}

func TestIsDestinationAllowed(t *testing.T) {
	// Parse some test CIDRs
	cidrs, _ := ParseCIDRs([]string{"10.0.0.0/8", "192.168.0.0/16"})

	tests := []struct {
		name   string
		cidrs  []*net.IPNet
		ip     string
		want   bool
	}{
		{
			name:  "IP in first CIDR",
			cidrs: cidrs,
			ip:    "10.0.0.1",
			want:  true,
		},
		{
			name:  "IP in second CIDR",
			cidrs: cidrs,
			ip:    "192.168.1.100",
			want:  true,
		},
		{
			name:  "IP not in any CIDR",
			cidrs: cidrs,
			ip:    "8.8.8.8",
			want:  false,
		},
		{
			name:  "empty CIDR list",
			cidrs: nil,
			ip:    "10.0.0.1",
			want:  false,
		},
		{
			name:  "IPv6 address",
			cidrs: cidrs,
			ip:    "::1",
			want:  false, // IPv6 not supported
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{AllowedCIDRs: tt.cidrs}
			ip := net.ParseIP(tt.ip)
			got := cfg.IsDestinationAllowed(ip)
			if got != tt.want {
				t.Errorf("IsDestinationAllowed(%s) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestIsDestinationAllowed_AllIPv4(t *testing.T) {
	cidrs, _ := ParseCIDRs([]string{"0.0.0.0/0"})
	cfg := Config{AllowedCIDRs: cidrs}

	tests := []string{
		"8.8.8.8",
		"1.1.1.1",
		"192.168.1.1",
		"10.0.0.1",
		"172.16.0.1",
		"255.255.255.255",
		"0.0.0.1",
	}

	for _, ip := range tests {
		if !cfg.IsDestinationAllowed(net.ParseIP(ip)) {
			t.Errorf("IsDestinationAllowed(%s) = false, want true for 0.0.0.0/0", ip)
		}
	}
}
