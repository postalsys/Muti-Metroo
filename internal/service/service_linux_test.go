//go:build linux

package service

import (
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

	unit := generateSystemdUnit(cfg, execPath)

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

	unit := generateSystemdUnit(cfg, execPath)

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

	unit := generateSystemdUnit(cfg, execPath)

	// Should not contain User= or Group= lines when empty
	if strings.Contains(unit, "User=") {
		t.Error("Unit file should not contain User= when User is empty")
	}
	if strings.Contains(unit, "Group=") {
		t.Error("Unit file should not contain Group= when Group is empty")
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
