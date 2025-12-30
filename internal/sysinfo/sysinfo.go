// Package sysinfo collects system information for node info advertisements.
package sysinfo

import (
	"net"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/postalsys/muti-metroo/internal/protocol"
)

var (
	// Version is the agent version, set at build time via ldflags.
	// Example: go build -ldflags="-X github.com/postalsys/muti-metroo/internal/sysinfo.Version=1.0.0"
	Version = "dev"

	// startTime is when the agent started.
	startTime     time.Time
	startTimeOnce sync.Once
)

func init() {
	startTimeOnce.Do(func() {
		startTime = time.Now()
	})
}

// Collect gathers local system information and returns a NodeInfo struct.
func Collect(displayName string) *protocol.NodeInfo {
	hostname, _ := os.Hostname()

	return &protocol.NodeInfo{
		DisplayName: displayName,
		Hostname:    hostname,
		OS:          runtime.GOOS,
		Arch:        runtime.GOARCH,
		Version:     Version,
		StartTime:   startTime.Unix(),
		IPAddresses: GetLocalIPs(),
	}
}

// GetLocalIPs returns non-loopback IPv4 addresses.
func GetLocalIPs() []string {
	var ips []string

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ips
	}

	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}

		// Skip loopback addresses
		if ipNet.IP.IsLoopback() {
			continue
		}

		// Only include IPv4 addresses (limit payload size)
		if ipv4 := ipNet.IP.To4(); ipv4 != nil {
			ips = append(ips, ipv4.String())
		}
	}

	// Limit to first 10 IPs to prevent payload bloat
	if len(ips) > 10 {
		ips = ips[:10]
	}

	return ips
}

// StartTime returns the agent start time.
func StartTime() time.Time {
	return startTime
}

// Uptime returns the agent uptime as a duration.
func Uptime() time.Duration {
	return time.Since(startTime)
}

// UptimeSeconds returns the agent uptime in seconds.
func UptimeSeconds() int64 {
	return int64(Uptime().Seconds())
}
