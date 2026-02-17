// Package sysinfo collects system information for node info advertisements.
package sysinfo

import (
	"net"
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"
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

	// cachedShells holds the detected shells, computed once at init time.
	cachedShells []string
)

func init() {
	startTimeOnce.Do(func() {
		startTime = time.Now()
	})

	// Enhance "dev" version with VCS info from Go build system
	if Version == "dev" {
		Version = enhanceDevVersion()
	}

	// Detect available shells once at startup
	cachedShells = detectShells()
}

// enhanceDevVersion adds git commit info to dev version using Go's build info.
// Returns formats like: "dev-a1b2c3d", "dev-a1b2c3d-dirty", or "dev-<timestamp>" as fallback.
func enhanceDevVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		// Fallback to build timestamp if no build info available
		return "dev-" + startTime.UTC().Format("20060102-150405")
	}

	var revision string
	var dirty bool

	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			dirty = setting.Value == "true"
		}
	}

	if revision == "" {
		// No VCS info, use timestamp fallback
		return "dev-" + startTime.UTC().Format("20060102-150405")
	}

	// Shorten commit hash to 7 characters (standard git short hash)
	if len(revision) > 7 {
		revision = revision[:7]
	}

	if dirty {
		return "dev-" + revision + "-dirty"
	}
	return "dev-" + revision
}

// detectShells probes the system for known shells using exec.LookPath.
// Returns base names of found shells in preference order.
func detectShells() []string {
	var candidates []string
	if runtime.GOOS == "windows" {
		candidates = []string{"powershell.exe", "pwsh.exe", "cmd.exe"}
	} else {
		candidates = []string{"bash", "sh", "zsh", "fish", "ash", "dash", "ksh"}
	}

	var found []string
	for _, shell := range candidates {
		if _, err := exec.LookPath(shell); err == nil {
			found = append(found, shell)
		}
	}
	return found
}

// DetectShells returns the list of available shells on the system.
// Results are cached at init time since shells don't change during runtime.
func DetectShells() []string {
	return cachedShells
}

// Collect gathers local system information and returns a NodeInfo struct.
// UDPConfig contains UDP relay configuration for node info advertisements.
type UDPConfig struct {
	Enabled bool
}

// ForwardConfig contains port forward listener configuration for node info advertisements.
type ForwardConfig struct {
	Listeners []protocol.ForwardListenerInfo
}

// The peers parameter contains current peer connection details to include in the advertisement.
// The publicKey parameter is the agent's X25519 public key for E2E encryption.
// The udpConfig parameter is optional and can be nil if UDP is not configured.
// The forwardConfig parameter is optional and can be nil if no forward listeners are configured.
func Collect(displayName string, peers []protocol.PeerConnectionInfo, publicKey [protocol.EphemeralKeySize]byte, udpConfig *UDPConfig, forwardConfig *ForwardConfig) *protocol.NodeInfo {
	hostname, _ := os.Hostname()

	info := &protocol.NodeInfo{
		DisplayName: displayName,
		Hostname:    hostname,
		OS:          runtime.GOOS,
		Arch:        runtime.GOARCH,
		Version:     Version,
		StartTime:   startTime.Unix(),
		IPAddresses: GetLocalIPs(),
		Peers:       peers,
		PublicKey:   publicKey,
		Shells:      cachedShells,
	}

	// Add UDP config if provided
	if udpConfig != nil {
		info.UDPEnabled = udpConfig.Enabled
	}

	// Add forward listeners if provided
	if forwardConfig != nil {
		info.ForwardListeners = forwardConfig.Listeners
	}

	return info
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
