//go:build linux

package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateSystemdUnit(t *testing.T) {
	cfg := ServiceConfig{
		Name:        "muti-metroo",
		DisplayName: "Muti Metroo Mesh Agent",
		Description: "Userspace mesh networking agent",
		ConfigPath:  "/etc/muti-metroo/config.yaml",
		WorkingDir:  "/etc/muti-metroo",
	}
	execPath := "/usr/local/bin/muti-metroo"

	unit := generateSystemdUnit(cfg, execPath, false)

	// Check that required sections exist
	if !strings.Contains(unit, "[Unit]") {
		t.Error("Unit file missing [Unit] section")
	}
	if !strings.Contains(unit, "[Service]") {
		t.Error("Unit file missing [Service] section")
	}
	if !strings.Contains(unit, "[Install]") {
		t.Error("Unit file missing [Install] section")
	}

	// Check description
	if !strings.Contains(unit, "Description=Userspace mesh networking agent") {
		t.Error("Unit file missing description")
	}

	// Check ExecStart
	expectedExec := "ExecStart=/usr/local/bin/muti-metroo run -c /etc/muti-metroo/config.yaml"
	if !strings.Contains(unit, expectedExec) {
		t.Errorf("Unit file missing ExecStart, expected: %s", expectedExec)
	}

	// Check working directory
	if !strings.Contains(unit, "WorkingDirectory=/etc/muti-metroo") {
		t.Error("Unit file missing WorkingDirectory")
	}

	// Check security settings
	if !strings.Contains(unit, "NoNewPrivileges=true") {
		t.Error("Unit file missing NoNewPrivileges security setting")
	}
	if !strings.Contains(unit, "ProtectSystem=strict") {
		t.Error("Unit file missing ProtectSystem security setting")
	}
	if !strings.Contains(unit, "PrivateTmp=true") {
		t.Error("Unit file missing PrivateTmp security setting")
	}

	// Check restart settings
	if !strings.Contains(unit, "Restart=on-failure") {
		t.Error("Unit file missing Restart setting")
	}
	if !strings.Contains(unit, "RestartSec=5") {
		t.Error("Unit file missing RestartSec setting")
	}

	// Check logging
	if !strings.Contains(unit, "StandardOutput=journal") {
		t.Error("Unit file missing StandardOutput setting")
	}
	if !strings.Contains(unit, "SyslogIdentifier=muti-metroo") {
		t.Error("Unit file missing SyslogIdentifier")
	}

	// Check installation target
	if !strings.Contains(unit, "WantedBy=multi-user.target") {
		t.Error("Unit file missing WantedBy setting")
	}

	// Check network dependency
	if !strings.Contains(unit, "After=network-online.target") {
		t.Error("Unit file missing network dependency")
	}
}

func TestGenerateSystemdUnitWithUser(t *testing.T) {
	cfg := ServiceConfig{
		Name:        "muti-metroo",
		Description: "Test service",
		ConfigPath:  "/etc/config.yaml",
		WorkingDir:  "/etc",
		User:        "metroo",
		Group:       "metroo",
	}
	execPath := "/usr/bin/muti-metroo"

	unit := generateSystemdUnit(cfg, execPath, false)

	// Check User setting
	if !strings.Contains(unit, "User=metroo") {
		t.Error("Unit file missing User setting when User is specified")
	}

	// Check Group setting
	if !strings.Contains(unit, "Group=metroo") {
		t.Error("Unit file missing Group setting when Group is specified")
	}
}

func TestGenerateSystemdUnitWithoutUser(t *testing.T) {
	cfg := ServiceConfig{
		Name:        "muti-metroo",
		Description: "Test service",
		ConfigPath:  "/etc/config.yaml",
		WorkingDir:  "/etc",
		// User and Group are empty
	}
	execPath := "/usr/bin/muti-metroo"

	unit := generateSystemdUnit(cfg, execPath, false)

	// Should not contain User= or Group= lines when empty
	if strings.Contains(unit, "User=") {
		t.Error("Unit file should not contain User= when User is empty")
	}
	if strings.Contains(unit, "Group=") {
		t.Error("Unit file should not contain Group= when Group is empty")
	}
}

func TestGenerateSystemdUnitEmbedded(t *testing.T) {
	cfg := ServiceConfig{
		Name:        "muti-metroo",
		Description: "Test service",
		ConfigPath:  "/etc/config.yaml",
		WorkingDir:  "/etc",
	}
	execPath := "/usr/bin/muti-metroo"

	unit := generateSystemdUnit(cfg, execPath, true)

	// ExecStart should NOT include -c flag for embedded mode
	if strings.Contains(unit, "-c") {
		t.Error("Embedded unit file should not contain -c flag")
	}

	// ExecStart should be just "run" without config path
	expectedExec := "ExecStart=/usr/bin/muti-metroo run"
	if !strings.Contains(unit, expectedExec) {
		t.Errorf("Embedded unit file missing ExecStart, expected: %s", expectedExec)
	}
}

func TestIsRootImplLinux(t *testing.T) {
	// Test that isRootImpl returns a consistent value
	result1 := isRootImpl()
	result2 := isRootImpl()

	if result1 != result2 {
		t.Error("isRootImpl() returned inconsistent results")
	}
}

// =============================================================================
// Cron Service Tests
// =============================================================================

func TestCronServiceConstants(t *testing.T) {
	// Verify constants have expected values
	if cronServiceDirName != ".muti-metroo" {
		t.Errorf("cronServiceDirName = %q, want %q", cronServiceDirName, ".muti-metroo")
	}
	if cronScriptName != "muti-metroo.sh" {
		t.Errorf("cronScriptName = %q, want %q", cronScriptName, "muti-metroo.sh")
	}
	if cronPIDFileName != "muti-metroo.pid" {
		t.Errorf("cronPIDFileName = %q, want %q", cronPIDFileName, "muti-metroo.pid")
	}
	if cronLogFileName != "muti-metroo.log" {
		t.Errorf("cronLogFileName = %q, want %q", cronLogFileName, "muti-metroo.log")
	}
	if cronMarker != "# muti-metroo-cron" {
		t.Errorf("cronMarker = %q, want %q", cronMarker, "# muti-metroo-cron")
	}
}

func TestGetCronServiceDir(t *testing.T) {
	serviceDir, err := getCronServiceDir()
	if err != nil {
		t.Fatalf("getCronServiceDir() error: %v", err)
	}

	// Should return path in home directory
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir() error: %v", err)
	}

	expected := filepath.Join(home, ".muti-metroo")
	if serviceDir != expected {
		t.Errorf("getCronServiceDir() = %q, want %q", serviceDir, expected)
	}

	// Path should be absolute
	if !filepath.IsAbs(serviceDir) {
		t.Errorf("getCronServiceDir() returned non-absolute path: %q", serviceDir)
	}
}

func TestGenerateCronScript(t *testing.T) {
	configPath := "/home/testuser/config.yaml"
	binaryPath := "/usr/local/bin/muti-metroo"
	serviceDir := "/home/testuser/.muti-metroo"

	script := generateCronScript(configPath, binaryPath, serviceDir)

	// Check shebang
	if !strings.HasPrefix(script, "#!/bin/bash") {
		t.Error("Script missing bash shebang")
	}

	// Check header comment
	if !strings.Contains(script, "Muti Metroo user service wrapper") {
		t.Error("Script missing header comment")
	}

	// Check PIDFILE variable
	expectedPIDFile := `PIDFILE="/home/testuser/.muti-metroo/muti-metroo.pid"`
	if !strings.Contains(script, expectedPIDFile) {
		t.Errorf("Script missing PIDFILE, expected: %s", expectedPIDFile)
	}

	// Check LOGFILE variable
	expectedLogFile := `LOGFILE="/home/testuser/.muti-metroo/muti-metroo.log"`
	if !strings.Contains(script, expectedLogFile) {
		t.Errorf("Script missing LOGFILE, expected: %s", expectedLogFile)
	}

	// Check CONFIG variable
	expectedConfig := `CONFIG="/home/testuser/config.yaml"`
	if !strings.Contains(script, expectedConfig) {
		t.Errorf("Script missing CONFIG, expected: %s", expectedConfig)
	}

	// Check BINARY variable
	expectedBinary := `BINARY="/usr/local/bin/muti-metroo"`
	if !strings.Contains(script, expectedBinary) {
		t.Errorf("Script missing BINARY, expected: %s", expectedBinary)
	}

	// Check "already running" logic
	if !strings.Contains(script, "kill -0") {
		t.Error("Script missing process check (kill -0)")
	}
	if !strings.Contains(script, "Already running") {
		t.Error("Script missing 'Already running' message")
	}

	// Check nohup start command
	if !strings.Contains(script, "nohup \"$BINARY\" run -c \"$CONFIG\"") {
		t.Error("Script missing nohup start command")
	}

	// Check PID capture
	if !strings.Contains(script, "echo $! > \"$PIDFILE\"") {
		t.Error("Script missing PID capture")
	}

	// Check working directory change
	if !strings.Contains(script, "cd \"$(dirname \"$CONFIG\")\"") {
		t.Error("Script missing directory change")
	}
}

func TestGenerateCronScriptWithSpaces(t *testing.T) {
	// Test with paths containing spaces
	configPath := "/home/test user/my config.yaml"
	binaryPath := "/usr/local/bin/muti metroo"
	serviceDir := "/home/test user/.muti-metroo"

	script := generateCronScript(configPath, binaryPath, serviceDir)

	// Paths should be quoted in the script
	if !strings.Contains(script, `CONFIG="/home/test user/my config.yaml"`) {
		t.Error("Script should handle spaces in config path")
	}
	if !strings.Contains(script, `BINARY="/usr/local/bin/muti metroo"`) {
		t.Error("Script should handle spaces in binary path")
	}
}

func TestHasCrontab(t *testing.T) {
	// Test that hasCrontab returns a boolean without panicking
	result := hasCrontab()
	_ = result // Result depends on system, just verify it doesn't panic
}

func TestErrCrontabNotFound(t *testing.T) {
	if ErrCrontabNotFound == nil {
		t.Error("ErrCrontabNotFound should not be nil")
	}
	if !strings.Contains(ErrCrontabNotFound.Error(), "crontab") {
		t.Error("ErrCrontabNotFound should mention crontab")
	}
}

func TestStatusUserImplNotInstalled(t *testing.T) {
	// Skip if running as root or if cron service is already installed
	if isUserInstalledImpl() {
		t.Skip("Skipping test because user service is installed")
	}

	status, err := statusUserImpl()
	if err != nil {
		t.Fatalf("statusUserImpl() error: %v", err)
	}

	if status != "not installed" {
		t.Errorf("statusUserImpl() = %q, want %q", status, "not installed")
	}
}

func TestIsUserInstalledImplConsistent(t *testing.T) {
	// Test that isUserInstalledImpl returns consistent values
	result1 := isUserInstalledImpl()
	result2 := isUserInstalledImpl()

	if result1 != result2 {
		t.Error("isUserInstalledImpl() returned inconsistent results")
	}
}

// TestCronEntryIntegration tests the full cron entry add/remove cycle.
// This test modifies the user's crontab and should only run in CI or isolated environments.
func TestCronEntryIntegration(t *testing.T) {
	// Skip if crontab is not available
	if !hasCrontab() {
		t.Skip("crontab not available")
	}

	// Skip if already installed (don't interfere with real installation)
	if isUserInstalledImpl() {
		t.Skip("user service already installed")
	}

	// Skip unless explicitly enabled
	if os.Getenv("TEST_CRON_INTEGRATION") != "1" {
		t.Skip("set TEST_CRON_INTEGRATION=1 to run this test")
	}

	// Create a temporary script path for testing
	tempDir := t.TempDir()
	scriptPath := filepath.Join(tempDir, "test-script.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/bash\necho test"), 0755); err != nil {
		t.Fatalf("failed to create test script: %v", err)
	}

	// Test addCronEntry
	if err := addCronEntry(scriptPath); err != nil {
		t.Fatalf("addCronEntry() error: %v", err)
	}

	// Verify entry was added
	if !isUserInstalledImpl() {
		t.Error("isUserInstalledImpl() = false after addCronEntry, want true")
	}

	// Test adding duplicate entry should fail
	if err := addCronEntry(scriptPath); err == nil {
		t.Error("addCronEntry() should fail for duplicate entry")
	}

	// Test removeCronEntry
	if err := removeCronEntry(); err != nil {
		t.Fatalf("removeCronEntry() error: %v", err)
	}

	// Verify entry was removed
	if isUserInstalledImpl() {
		t.Error("isUserInstalledImpl() = true after removeCronEntry, want false")
	}
}

// TestUserServiceLifecycle tests the full user service lifecycle.
// This test modifies the user's crontab and creates files in ~/.muti-metroo.
func TestUserServiceLifecycle(t *testing.T) {
	// Skip if crontab is not available
	if !hasCrontab() {
		t.Skip("crontab not available")
	}

	// Skip if already installed (don't interfere with real installation)
	if isUserInstalledImpl() {
		t.Skip("user service already installed")
	}

	// Skip unless explicitly enabled
	if os.Getenv("TEST_CRON_INTEGRATION") != "1" {
		t.Skip("set TEST_CRON_INTEGRATION=1 to run this test")
	}

	// Create a temporary config file
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("agent:\n  data_dir: "+tempDir), 0644); err != nil {
		t.Fatalf("failed to create test config: %v", err)
	}

	// Create a fake binary that just exits
	binaryPath := filepath.Join(tempDir, "fake-muti-metroo")
	if err := os.WriteFile(binaryPath, []byte("#!/bin/bash\nsleep 1"), 0755); err != nil {
		t.Fatalf("failed to create fake binary: %v", err)
	}

	cfg := ServiceConfig{
		Name:       "muti-metroo-test",
		ConfigPath: configPath,
	}

	// Get service dir before install for cleanup
	serviceDir, err := getCronServiceDir()
	if err != nil {
		t.Fatalf("getCronServiceDir() error: %v", err)
	}

	// Cleanup function
	cleanup := func() {
		removeCronEntry()
		os.RemoveAll(serviceDir)
	}
	defer cleanup()

	// Test install
	if err := installUserImpl(cfg, binaryPath); err != nil {
		t.Fatalf("installUserImpl() error: %v", err)
	}

	// Verify installation
	if !isUserInstalledImpl() {
		t.Error("isUserInstalledImpl() = false after install, want true")
	}

	// Check wrapper script exists
	scriptPath := filepath.Join(serviceDir, cronScriptName)
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		t.Error("wrapper script not created")
	}

	// Test status
	status, err := statusUserImpl()
	if err != nil {
		t.Fatalf("statusUserImpl() error: %v", err)
	}
	// Status should be either "active" or "inactive" (depending on if fake binary ran)
	if !strings.HasPrefix(status, "active") && !strings.HasPrefix(status, "inactive") {
		t.Errorf("statusUserImpl() = %q, want 'active...' or 'inactive...'", status)
	}

	// Test uninstall
	if err := uninstallUserImpl(); err != nil {
		t.Fatalf("uninstallUserImpl() error: %v", err)
	}

	// Verify uninstallation
	if isUserInstalledImpl() {
		t.Error("isUserInstalledImpl() = true after uninstall, want false")
	}

	// Wrapper script should be removed
	if _, err := os.Stat(scriptPath); !os.IsNotExist(err) {
		t.Error("wrapper script still exists after uninstall")
	}
}
