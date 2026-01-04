//go:build linux

package service

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
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

// isInteractiveImpl always returns true on Linux.
// Linux uses systemd which manages the process lifecycle externally.
func isInteractiveImpl() bool {
	return true
}

// runAsServiceImpl is a no-op on Linux.
// Linux uses systemd which manages the process lifecycle externally.
func runAsServiceImpl(name string, runner ServiceRunner) error {
	// On Linux, systemd manages the service. The 'run' command just runs normally
	// and systemd handles start/stop/restart. No special service handler needed.
	return nil
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
Documentation=https://github.com/postalsys/muti-metroo
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

// =============================================================================
// Cron+Nohup User Service (non-root installation)
// =============================================================================

const (
	cronServiceDirName = ".muti-metroo"
	cronScriptName     = "muti-metroo.sh"
	cronPIDFileName    = "muti-metroo.pid"
	cronLogFileName    = "muti-metroo.log"
	cronMarker         = "# muti-metroo-cron"
)

// getCronServiceDir returns the path to ~/.muti-metroo
func getCronServiceDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, cronServiceDirName), nil
}

// installUserImpl installs the service using cron @reboot and nohup.
func installUserImpl(cfg ServiceConfig, execPath string) error {
	// Get service directory
	serviceDir, err := getCronServiceDir()
	if err != nil {
		return err
	}

	// Create service directory if needed
	if err := os.MkdirAll(serviceDir, 0755); err != nil {
		return fmt.Errorf("failed to create service directory: %w", err)
	}

	// Check if already installed
	if isUserInstalledImpl() {
		return fmt.Errorf("user service is already installed")
	}

	// Resolve config path to absolute
	configPath, err := filepath.Abs(cfg.ConfigPath)
	if err != nil {
		return fmt.Errorf("failed to resolve config path: %w", err)
	}

	// Generate wrapper script
	scriptPath := filepath.Join(serviceDir, cronScriptName)
	script := generateCronScript(configPath, execPath, serviceDir)
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		return fmt.Errorf("failed to write wrapper script: %w", err)
	}
	fmt.Printf("Created wrapper script: %s\n", scriptPath)

	// Add cron entry
	if err := addCronEntry(scriptPath); err != nil {
		os.Remove(scriptPath)
		return fmt.Errorf("failed to add cron entry: %w", err)
	}
	fmt.Println("Added @reboot cron entry")

	// Start the service now
	if err := startUserImpl(); err != nil {
		fmt.Printf("Note: could not start service: %v\n", err)
		fmt.Println("The service will start on next reboot")
	} else {
		fmt.Println("Started service")
	}

	fmt.Printf("\nLog file: %s\n", filepath.Join(serviceDir, cronLogFileName))

	return nil
}

// uninstallUserImpl removes the cron-based user service.
func uninstallUserImpl() error {
	serviceDir, err := getCronServiceDir()
	if err != nil {
		return err
	}

	if !isUserInstalledImpl() {
		return fmt.Errorf("user service is not installed")
	}

	// Stop the service if running
	if err := stopUserImpl(); err != nil {
		// Ignore errors, service might not be running
		fmt.Printf("Note: could not stop service: %v\n", err)
	} else {
		fmt.Println("Stopped service")
	}

	// Remove cron entry
	if err := removeCronEntry(); err != nil {
		fmt.Printf("Note: could not remove cron entry: %v\n", err)
	} else {
		fmt.Println("Removed cron entry")
	}

	// Remove wrapper script
	scriptPath := filepath.Join(serviceDir, cronScriptName)
	if err := os.Remove(scriptPath); err != nil && !os.IsNotExist(err) {
		fmt.Printf("Note: could not remove script: %v\n", err)
	} else {
		fmt.Printf("Removed wrapper script: %s\n", scriptPath)
	}

	// Remove PID file
	pidPath := filepath.Join(serviceDir, cronPIDFileName)
	os.Remove(pidPath)

	// Keep log file and directory for reference
	fmt.Printf("\nNote: Log file preserved at %s\n", filepath.Join(serviceDir, cronLogFileName))

	return nil
}

// statusUserImpl returns the status of the cron-based user service.
func statusUserImpl() (string, error) {
	serviceDir, err := getCronServiceDir()
	if err != nil {
		return "", err
	}

	if !isUserInstalledImpl() {
		return "not installed", nil
	}

	// Check if process is running
	pidPath := filepath.Join(serviceDir, cronPIDFileName)
	pidData, err := os.ReadFile(pidPath)
	if err != nil {
		return "inactive (no pid file)", nil
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
	if err != nil {
		return "inactive (invalid pid)", nil
	}

	// Check if process exists
	process, err := os.FindProcess(pid)
	if err != nil {
		return "inactive", nil
	}

	// On Unix, FindProcess always succeeds. Use signal 0 to check if alive.
	if err := process.Signal(syscall.Signal(0)); err != nil {
		return "inactive", nil
	}

	return fmt.Sprintf("active (pid %d)", pid), nil
}

// startUserImpl starts the cron-based user service.
func startUserImpl() error {
	serviceDir, err := getCronServiceDir()
	if err != nil {
		return err
	}

	scriptPath := filepath.Join(serviceDir, cronScriptName)
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		return fmt.Errorf("service not installed")
	}

	// Check if already running
	pidPath := filepath.Join(serviceDir, cronPIDFileName)
	if pidData, err := os.ReadFile(pidPath); err == nil {
		if pid, err := strconv.Atoi(strings.TrimSpace(string(pidData))); err == nil {
			if process, err := os.FindProcess(pid); err == nil {
				if err := process.Signal(syscall.Signal(0)); err == nil {
					return fmt.Errorf("service already running (pid %d)", pid)
				}
			}
		}
	}

	// Execute the wrapper script
	cmd := exec.Command("/bin/bash", scriptPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to start: %s: %w", strings.TrimSpace(string(output)), err)
	}

	return nil
}

// stopUserImpl stops the cron-based user service.
func stopUserImpl() error {
	serviceDir, err := getCronServiceDir()
	if err != nil {
		return err
	}

	pidPath := filepath.Join(serviceDir, cronPIDFileName)
	pidData, err := os.ReadFile(pidPath)
	if err != nil {
		return fmt.Errorf("service not running (no pid file)")
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
	if err != nil {
		os.Remove(pidPath)
		return fmt.Errorf("invalid pid file")
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		os.Remove(pidPath)
		return fmt.Errorf("process not found")
	}

	// Send SIGTERM
	if err := process.Signal(syscall.SIGTERM); err != nil {
		os.Remove(pidPath)
		return fmt.Errorf("failed to send signal: %w", err)
	}

	os.Remove(pidPath)
	return nil
}

// isUserInstalledImpl checks if the cron-based user service is installed.
func isUserInstalledImpl() bool {
	// Check if cron entry exists
	output, err := exec.Command("crontab", "-l").CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(string(output), cronMarker)
}

// generateCronScript generates the wrapper script for nohup execution.
func generateCronScript(configPath, binaryPath, serviceDir string) string {
	return fmt.Sprintf(`#!/bin/bash
# Muti Metroo user service wrapper
# This script is managed by 'muti-metroo service install --user'

PIDFILE="%s"
LOGFILE="%s"
CONFIG="%s"
BINARY="%s"

# Check if already running
if [ -f "$PIDFILE" ]; then
    PID=$(cat "$PIDFILE")
    if kill -0 "$PID" 2>/dev/null; then
        echo "Already running (PID $PID)"
        exit 0
    fi
    rm -f "$PIDFILE"
fi

# Start with nohup
cd "$(dirname "$CONFIG")"
nohup "$BINARY" run -c "$CONFIG" >> "$LOGFILE" 2>&1 &
echo $! > "$PIDFILE"
echo "Started (PID $!)"
`,
		filepath.Join(serviceDir, cronPIDFileName),
		filepath.Join(serviceDir, cronLogFileName),
		configPath,
		binaryPath,
	)
}

// addCronEntry adds the @reboot entry to user's crontab.
func addCronEntry(scriptPath string) error {
	// Get existing crontab
	output, err := exec.Command("crontab", "-l").CombinedOutput()
	if err != nil {
		// crontab -l fails if no crontab exists, that's OK
		if !strings.Contains(string(output), "no crontab") {
			return fmt.Errorf("failed to read crontab: %s", strings.TrimSpace(string(output)))
		}
		output = nil
	}

	// Check if already has our entry
	if strings.Contains(string(output), cronMarker) {
		return fmt.Errorf("cron entry already exists")
	}

	// Add our entry
	newEntry := fmt.Sprintf("@reboot %s %s\n", scriptPath, cronMarker)
	var newCrontab bytes.Buffer
	if len(output) > 0 {
		newCrontab.Write(output)
		// Ensure newline before our entry
		if !bytes.HasSuffix(output, []byte("\n")) {
			newCrontab.WriteByte('\n')
		}
	}
	newCrontab.WriteString(newEntry)

	// Write new crontab
	cmd := exec.Command("crontab", "-")
	cmd.Stdin = &newCrontab
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to write crontab: %s", strings.TrimSpace(string(output)))
	}

	return nil
}

// removeCronEntry removes the muti-metroo entry from user's crontab.
func removeCronEntry() error {
	// Get existing crontab
	output, err := exec.Command("crontab", "-l").CombinedOutput()
	if err != nil {
		return nil // No crontab, nothing to remove
	}

	// Filter out our entry
	var newCrontab bytes.Buffer
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, cronMarker) {
			continue // Skip our entry
		}
		if line == "" && newCrontab.Len() == 0 {
			continue // Skip leading empty lines
		}
		newCrontab.WriteString(line)
		newCrontab.WriteByte('\n')
	}

	// Write new crontab (or remove if empty)
	content := strings.TrimSpace(newCrontab.String())
	if content == "" {
		// Remove crontab entirely
		if output, err := exec.Command("crontab", "-r").CombinedOutput(); err != nil {
			// Ignore "no crontab" error
			if !strings.Contains(string(output), "no crontab") {
				return fmt.Errorf("failed to remove crontab: %s", strings.TrimSpace(string(output)))
			}
		}
		return nil
	}

	cmd := exec.Command("crontab", "-")
	cmd.Stdin = strings.NewReader(content + "\n")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to write crontab: %s", strings.TrimSpace(string(output)))
	}

	return nil
}

// hasCrontab checks if crontab command is available.
func hasCrontab() bool {
	_, err := exec.LookPath("crontab")
	return err == nil
}

// ErrCrontabNotFound is returned when crontab is not available.
var ErrCrontabNotFound = errors.New("crontab command not found - install cron to use user service")
