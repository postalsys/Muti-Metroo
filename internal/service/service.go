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
	// Check for user service first (Linux only)
	if runtime.GOOS == "linux" && IsUserInstalled() {
		return UninstallUser()
	}

	if !IsRoot() {
		return fmt.Errorf("must run as root/administrator to uninstall service")
	}

	return uninstallImpl(serviceName)
}

// InstallUser installs as a user-level service (cron+nohup on Linux).
// This does not require root privileges.
func InstallUser(cfg ServiceConfig) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("user service installation is only supported on Linux")
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

// UninstallUser removes the user-level service (Linux only).
func UninstallUser() error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("user service is only supported on Linux")
	}
	return uninstallUserImpl()
}

// IsUserInstalled checks if a user-level service is installed (Linux only).
func IsUserInstalled() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	return isUserInstalledImpl()
}

// StatusUser returns the status of the user-level service (Linux only).
func StatusUser() (string, error) {
	if runtime.GOOS != "linux" {
		return "", fmt.Errorf("user service is only supported on Linux")
	}
	return statusUserImpl()
}

// StartUser starts the user-level service (Linux only).
func StartUser() error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("user service is only supported on Linux")
	}
	return startUserImpl()
}

// StopUser stops the user-level service (Linux only).
func StopUser() error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("user service is only supported on Linux")
	}
	return stopUserImpl()
}

// Status returns the current status of the service.
// It auto-detects whether a system or user service is installed.
func Status(serviceName string) (string, error) {
	// Check for user service first (Linux only)
	if runtime.GOOS == "linux" && IsUserInstalled() {
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
	switch runtime.GOOS {
	case "linux":
		return "linux"
	case "windows":
		return "windows"
	case "darwin":
		return "darwin"
	default:
		return "unsupported"
	}
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
