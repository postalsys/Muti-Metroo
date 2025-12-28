//go:build linux

package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const systemdUnitPath = "/etc/systemd/system"

// isRootImpl checks if running as root on Linux.
func isRootImpl() bool {
	return os.Getuid() == 0
}

// installImpl installs the service on Linux using systemd.
func installImpl(cfg ServiceConfig, execPath string) error {
	unitName := cfg.Name + ".service"
	unitPath := filepath.Join(systemdUnitPath, unitName)

	// Check if already installed
	if _, err := os.Stat(unitPath); err == nil {
		return fmt.Errorf("service %s is already installed at %s", cfg.Name, unitPath)
	}

	// Generate systemd unit file
	unit := generateSystemdUnit(cfg, execPath)

	// Write unit file
	if err := os.WriteFile(unitPath, []byte(unit), 0644); err != nil {
		return fmt.Errorf("failed to write systemd unit file: %w", err)
	}

	fmt.Printf("Created systemd unit: %s\n", unitPath)

	// Reload systemd
	if output, err := runCommand("systemctl", "daemon-reload"); err != nil {
		os.Remove(unitPath)
		return fmt.Errorf("failed to reload systemd: %s: %w", output, err)
	}

	// Enable the service
	if output, err := runCommand("systemctl", "enable", cfg.Name); err != nil {
		return fmt.Errorf("failed to enable service: %s: %w", output, err)
	}

	fmt.Printf("Enabled service: %s\n", cfg.Name)

	// Start the service
	if output, err := runCommand("systemctl", "start", cfg.Name); err != nil {
		return fmt.Errorf("failed to start service: %s: %w", output, err)
	}

	fmt.Printf("Started service: %s\n", cfg.Name)

	return nil
}

// uninstallImpl removes the systemd service on Linux.
func uninstallImpl(serviceName string) error {
	unitName := serviceName + ".service"
	unitPath := filepath.Join(systemdUnitPath, unitName)

	// Check if installed
	if _, err := os.Stat(unitPath); os.IsNotExist(err) {
		return fmt.Errorf("service %s is not installed", serviceName)
	}

	// Stop the service (ignore error if not running)
	if output, err := runCommand("systemctl", "stop", serviceName); err != nil {
		// Check if it's just not running
		if !strings.Contains(output, "not loaded") {
			fmt.Printf("Note: could not stop service: %s\n", strings.TrimSpace(output))
		}
	} else {
		fmt.Printf("Stopped service: %s\n", serviceName)
	}

	// Disable the service
	if output, err := runCommand("systemctl", "disable", serviceName); err != nil {
		if !strings.Contains(output, "not loaded") {
			fmt.Printf("Note: could not disable service: %s\n", strings.TrimSpace(output))
		}
	} else {
		fmt.Printf("Disabled service: %s\n", serviceName)
	}

	// Remove the unit file
	if err := os.Remove(unitPath); err != nil {
		return fmt.Errorf("failed to remove systemd unit file: %w", err)
	}

	fmt.Printf("Removed systemd unit: %s\n", unitPath)

	// Reload systemd
	if _, err := runCommand("systemctl", "daemon-reload"); err != nil {
		fmt.Println("Note: failed to reload systemd daemon")
	}

	// Reset failed state
	runCommand("systemctl", "reset-failed", serviceName)

	return nil
}

// statusImpl returns the service status on Linux.
func statusImpl(serviceName string) (string, error) {
	output, err := runCommand("systemctl", "is-active", serviceName)
	status := strings.TrimSpace(output)

	if err != nil {
		if status == "inactive" || status == "unknown" {
			return status, nil
		}
		return "", fmt.Errorf("failed to get service status: %w", err)
	}

	return status, nil
}

// isInstalledImpl checks if the service is installed on Linux.
func isInstalledImpl(serviceName string) bool {
	unitPath := filepath.Join(systemdUnitPath, serviceName+".service")
	_, err := os.Stat(unitPath)
	return err == nil
}

// generateSystemdUnit generates a systemd unit file.
func generateSystemdUnit(cfg ServiceConfig, execPath string) string {
	var user, group string
	if cfg.User != "" {
		user = fmt.Sprintf("User=%s\n", cfg.User)
	}
	if cfg.Group != "" {
		group = fmt.Sprintf("Group=%s\n", cfg.Group)
	}

	return fmt.Sprintf(`[Unit]
Description=%s
Documentation=https://github.com/coinstash/muti-metroo
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s run -c %s
WorkingDirectory=%s
%s%sRestart=on-failure
RestartSec=5
TimeoutStopSec=30

# Security hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=read-only
PrivateTmp=true
ReadWritePaths=%s

# Logging
StandardOutput=journal
StandardError=journal
SyslogIdentifier=%s

[Install]
WantedBy=multi-user.target
`, cfg.Description, execPath, cfg.ConfigPath, cfg.WorkingDir, user, group, cfg.WorkingDir, cfg.Name)
}
