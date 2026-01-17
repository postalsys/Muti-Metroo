// Package service provides cross-platform service management for Muti Metroo.
// It supports systemd on Linux, launchd on macOS, and Windows Service on Windows.
package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// ServiceRunner is the interface that the agent must implement to run as a service.
type ServiceRunner interface {
	// Start starts the service. Should return quickly after initializing.
	Start() error

	// Stop stops the service gracefully.
	StopWithContext(ctx context.Context) error
}

// ServiceConfig holds configuration for installing the service.
type ServiceConfig struct {
	// Name is the service name (used in systemd/Windows service)
	Name string

	// DisplayName is the human-readable name (Windows only)
	DisplayName string

	// Description is the service description
	Description string

	// ConfigPath is the absolute path to the config file
	ConfigPath string

	// WorkingDir is the working directory for the service
	WorkingDir string

	// DataDir is the data directory that needs write access (Linux systemd only)
	DataDir string

	// User is the user to run the service as (Linux only, empty for root)
	User string

	// Group is the group to run the service as (Linux only, empty for root)
	Group string
}

// DefaultConfig returns a default service configuration.
func DefaultConfig(configPath string) ServiceConfig {
	absPath, _ := filepath.Abs(configPath)
	workDir := filepath.Dir(absPath)

	return ServiceConfig{
		Name:        "muti-metroo",
		DisplayName: "Muti Metroo Mesh Agent",
		Description: "Userspace mesh networking agent for virtual TCP tunnels",
		ConfigPath:  absPath,
		WorkingDir:  workDir,
	}
}

// IsRoot returns true if the current process is running with elevated privileges.
// On Linux, this checks for UID 0 (root).
// On Windows, this checks for Administrator privileges.
func IsRoot() bool {
	return isRootImpl()
}

// Install installs the application as a system service.
// On Linux, this creates and enables a systemd unit.
// On macOS, this creates and loads a launchd plist.
// On Windows, this registers a Windows service.
func Install(cfg ServiceConfig) error {
	if !IsRoot() {
		return fmt.Errorf("must run as root/administrator to install service")
	}

	// Get the executable path
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Resolve symlinks to get the real path
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}

	return installImpl(cfg, execPath)
}

// GetInstallPath returns the standard installation path for a service binary.
// Linux/macOS: /usr/local/bin/<name>
// Windows: C:\Program Files\<name>\<name>.exe
func GetInstallPath(serviceName string) string {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("ProgramFiles"), serviceName, serviceName+".exe")
	default:
		return filepath.Join("/usr/local/bin", serviceName)
	}
}

// InstallWithEmbedded installs a service using a binary with embedded configuration.
// It copies the embedded binary to the standard system location and creates
// a service that runs without the -c config flag.
func InstallWithEmbedded(cfg ServiceConfig, embeddedBinaryPath string) error {
	if !IsRoot() {
		return fmt.Errorf("must run as root/administrator to install service")
	}

	// Determine destination path
	destPath := GetInstallPath(cfg.Name)

	// Create parent directory if needed (Windows)
	if runtime.GOOS == "windows" {
		parentDir := filepath.Dir(destPath)
		if err := os.MkdirAll(parentDir, 0755); err != nil {
			return fmt.Errorf("failed to create program directory: %w", err)
		}
	}

	// Copy the embedded binary to the destination
	if err := copyFile(embeddedBinaryPath, destPath); err != nil {
		return fmt.Errorf("failed to copy binary to %s: %w", destPath, err)
	}

	fmt.Printf("Installed binary: %s\n", destPath)

	// Clear ConfigPath to indicate embedded config mode
	cfg.ConfigPath = ""

	return installImplEmbedded(cfg, destPath)
}

// copyFile copies a file from src to dst, preserving permissions.
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	srcStat, err := srcFile.Stat()
	if err != nil {
		return err
	}

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcStat.Mode())
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = dstFile.ReadFrom(srcFile)
	return err
}

// Uninstall removes the system service.
// On Linux, this stops, disables, and removes the systemd unit.
// On macOS, this unloads and removes the launchd plist.
// On Windows, this stops and removes the Windows service.
func Uninstall(serviceName string) error {
	// Check for user service first (Linux and Windows)
	if (runtime.GOOS == "linux" || runtime.GOOS == "windows") && IsUserInstalled() {
		return UninstallUser()
	}

	if !IsRoot() {
		return fmt.Errorf("must run as root/administrator to uninstall service")
	}

	return uninstallImpl(serviceName)
}

// InstallUser installs as a user-level service (cron+nohup on Linux).
// This does not require root privileges.
// On Windows, use InstallUserWindows instead.
func InstallUser(cfg ServiceConfig) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("user service installation is only supported on Linux (use InstallUserWindows on Windows)")
	}

	// Check for crontab
	if !hasCrontab() {
		return ErrCrontabNotFound
	}

	// Get the executable path
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Resolve symlinks to get the real path
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}

	return installUserImpl(cfg, execPath)
}

// InstallUserWindows installs as a Windows user service using Registry Run key + DLL.
// This does not require Administrator privileges.
// The service runs at user logon via rundll32.exe with the specified DLL.
// The serviceName is used as the Registry value name (visible in startup apps).
func InstallUserWindows(serviceName, dllPath, configPath string) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("InstallUserWindows is only supported on Windows")
	}

	// Check if already installed
	if IsUserInstalled() {
		return fmt.Errorf("user service is already installed")
	}

	return installUserWithDLLImpl(serviceName, dllPath, configPath)
}

// UninstallUser removes the user-level service.
// On Linux: removes cron+nohup service
// On Windows: removes Registry Run key entry
func UninstallUser() error {
	if runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		return fmt.Errorf("user service is only supported on Linux and Windows")
	}
	return uninstallUserImpl()
}

// IsUserInstalled checks if a user-level service is installed.
// On Linux: checks for cron+nohup service
// On Windows: checks for Registry Run key entry
func IsUserInstalled() bool {
	if runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		return false
	}
	return isUserInstalledImpl()
}

// StatusUser returns the status of the user-level service.
// On Linux: checks cron+nohup service status
// On Windows: checks Registry Run key and process status
func StatusUser() (string, error) {
	if runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		return "", fmt.Errorf("user service is only supported on Linux and Windows")
	}
	return statusUserImpl()
}

// StartUser starts the user-level service.
// On Linux: starts the cron+nohup service
// On Windows: starts the DLL via rundll32
func StartUser() error {
	if runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		return fmt.Errorf("user service is only supported on Linux and Windows")
	}
	return startUserImpl()
}

// StopUser stops the user-level service.
// On Linux: stops the cron+nohup service
// On Windows: terminates the rundll32 process
func StopUser() error {
	if runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		return fmt.Errorf("user service is only supported on Linux and Windows")
	}
	return stopUserImpl()
}

// Status returns the current status of the service.
// It auto-detects whether a system or user service is installed.
func Status(serviceName string) (string, error) {
	// Check for user service first (Linux and Windows)
	if (runtime.GOOS == "linux" || runtime.GOOS == "windows") && IsUserInstalled() {
		return StatusUser()
	}
	return statusImpl(serviceName)
}

// IsInstalled checks if the service is already installed.
func IsInstalled(serviceName string) bool {
	return isInstalledImpl(serviceName)
}

// Platform returns the current platform type.
func Platform() string {
	if IsSupported() {
		return runtime.GOOS
	}
	return "unsupported"
}

// IsSupported returns true if service installation is supported on this platform.
func IsSupported() bool {
	return runtime.GOOS == "linux" || runtime.GOOS == "windows" || runtime.GOOS == "darwin"
}

// runCommand executes a command and returns combined output.
func runCommand(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// IsInteractive returns true if the process is running interactively (not as a service).
// On Windows, this detects if running under the Service Control Manager.
// On Linux/macOS, this always returns true (systemd handles service mode differently).
func IsInteractive() bool {
	return isInteractiveImpl()
}

// RunAsService runs the given ServiceRunner as a Windows service.
// This should only be called when IsInteractive() returns false.
// On non-Windows platforms, this is a no-op that returns nil.
func RunAsService(name string, runner ServiceRunner) error {
	return runAsServiceImpl(name, runner)
}

// UserServiceInfo contains information about a user-level service installation.
type UserServiceInfo struct {
	Name       string // Service display name
	DLLPath    string // Path to the DLL (Windows only)
	ConfigPath string // Path to the config file
	LogPath    string // Path to the log file (Linux only)
}

// GetUserServiceInfo returns information about the installed user-level service.
// Returns nil if no user service is installed.
func GetUserServiceInfo() *UserServiceInfo {
	if runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		return nil
	}
	if !IsUserInstalled() {
		return nil
	}
	return getUserServiceInfoImpl()
}
