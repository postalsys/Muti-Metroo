//go:build !linux && !windows && !darwin

package service

import "fmt"

// isRootImpl checks if running as root (unsupported platforms).
func isRootImpl() bool {
	return false
}

// installImpl is not supported on this platform.
func installImpl(cfg ServiceConfig, execPath string) error {
	return fmt.Errorf("service installation is not supported on this platform")
}

// uninstallImpl is not supported on this platform.
func uninstallImpl(serviceName string) error {
	return fmt.Errorf("service uninstallation is not supported on this platform")
}

// statusImpl is not supported on this platform.
func statusImpl(serviceName string) (string, error) {
	return "", fmt.Errorf("service status is not supported on this platform")
}

// isInstalledImpl is not supported on this platform.
func isInstalledImpl(serviceName string) bool {
	return false
}

// isInteractiveImpl always returns true on unsupported platforms.
func isInteractiveImpl() bool {
	return true
}

// runAsServiceImpl is not supported on this platform.
func runAsServiceImpl(name string, runner ServiceRunner) error {
	return fmt.Errorf("running as service is not supported on this platform")
}
