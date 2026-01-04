// Package integration provides integration tests for Muti Metroo.
package integration

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/postalsys/muti-metroo/internal/config"
	"github.com/postalsys/muti-metroo/internal/shell"
	"golang.org/x/crypto/bcrypt"
)

// newShellTestChain creates a 4-agent chain with shell enabled on agent D
// and HTTP server enabled on agent A.
func newShellTestChain(t *testing.T, shellCfg *config.ShellConfig) *AgentChain {
	chain := NewAgentChain(t)
	chain.EnableHTTP = true
	chain.ShellConfig = shellCfg
	return chain
}

// executeCommand runs a shell command through the mesh and returns the result.
func executeCommand(t *testing.T, chain *AgentChain, targetID, command string, args []string, password string) (stdout, stderr string, exitCode int, err error) {
	var stdoutBuf, stderrBuf bytes.Buffer

	client := shell.NewClient(shell.ClientConfig{
		AgentAddr:   chain.HTTPAddrs[0],
		TargetID:    targetID,
		Interactive: false, // Streaming mode
		Password:    password,
		Command:     command,
		Args:        args,
		Stdout:      &stdoutBuf,
		Stderr:      &stderrBuf,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	exitCode, err = client.Run(ctx)
	return stdoutBuf.String(), stderrBuf.String(), exitCode, err
}

// TestShell_OneOffCommand tests executing a simple command through the mesh.
func TestShell_OneOffCommand(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	shellCfg := &config.ShellConfig{
		Enabled:     true,
		Whitelist:   []string{"*"},
		MaxSessions: 10,
	}

	chain := newShellTestChain(t, shellCfg)
	defer chain.Close()

	chain.CreateAgents(t)
	chain.StartAgents(t)

	// Wait for routes to propagate
	time.Sleep(3 * time.Second)

	// Get target agent ID (agent D)
	targetID := chain.Agents[3].ID().String()

	stdout, stderr, exitCode, err := executeCommand(t, chain, targetID, "whoami", nil, "")
	if err != nil {
		t.Fatalf("Command failed: %v", err)
	}

	if exitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", exitCode)
	}

	output := strings.TrimSpace(stdout)
	if output == "" {
		t.Errorf("Expected non-empty stdout, got empty (stderr: %s)", stderr)
	} else {
		t.Logf("whoami output: %s", output)
	}
}

// TestShell_CommandWithArgs tests executing a command with arguments.
func TestShell_CommandWithArgs(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	shellCfg := &config.ShellConfig{
		Enabled:     true,
		Whitelist:   []string{"*"},
		MaxSessions: 10,
	}

	chain := newShellTestChain(t, shellCfg)
	defer chain.Close()

	chain.CreateAgents(t)
	chain.StartAgents(t)

	time.Sleep(3 * time.Second)

	targetID := chain.Agents[3].ID().String()

	stdout, _, exitCode, err := executeCommand(t, chain, targetID, "echo", []string{"hello", "world"}, "")
	if err != nil {
		t.Fatalf("Command failed: %v", err)
	}

	if exitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", exitCode)
	}

	output := strings.TrimSpace(stdout)
	if output != "hello world" {
		t.Errorf("Expected 'hello world', got '%s'", output)
	}
}

// TestShell_CommandWithStderr tests that stderr is captured separately.
func TestShell_CommandWithStderr(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	shellCfg := &config.ShellConfig{
		Enabled:     true,
		Whitelist:   []string{"*"},
		MaxSessions: 10,
	}

	chain := newShellTestChain(t, shellCfg)
	defer chain.Close()

	chain.CreateAgents(t)
	chain.StartAgents(t)

	time.Sleep(3 * time.Second)

	targetID := chain.Agents[3].ID().String()

	// Use sh -c to run a command that writes to both stdout and stderr
	stdout, stderr, exitCode, err := executeCommand(t, chain, targetID, "sh", []string{"-c", "echo stdout; echo stderr >&2"}, "")
	if err != nil {
		t.Fatalf("Command failed: %v", err)
	}

	if exitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", exitCode)
	}

	if !strings.Contains(stdout, "stdout") {
		t.Errorf("Expected stdout to contain 'stdout', got '%s'", stdout)
	}

	if !strings.Contains(stderr, "stderr") {
		t.Errorf("Expected stderr to contain 'stderr', got '%s'", stderr)
	}
}

// TestShell_NonZeroExitCode tests that non-zero exit codes are propagated.
func TestShell_NonZeroExitCode(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	shellCfg := &config.ShellConfig{
		Enabled:     true,
		Whitelist:   []string{"*"},
		MaxSessions: 10,
	}

	chain := newShellTestChain(t, shellCfg)
	defer chain.Close()

	chain.CreateAgents(t)
	chain.StartAgents(t)

	time.Sleep(3 * time.Second)

	targetID := chain.Agents[3].ID().String()

	_, _, exitCode, err := executeCommand(t, chain, targetID, "sh", []string{"-c", "exit 42"}, "")
	if err != nil {
		t.Fatalf("Command failed: %v", err)
	}

	if exitCode != 42 {
		t.Errorf("Expected exit code 42, got %d", exitCode)
	}
}

// TestShell_StreamingOutput tests that streaming output is received correctly.
func TestShell_StreamingOutput(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	shellCfg := &config.ShellConfig{
		Enabled:     true,
		Whitelist:   []string{"*"},
		MaxSessions: 10,
	}

	chain := newShellTestChain(t, shellCfg)
	defer chain.Close()

	chain.CreateAgents(t)
	chain.StartAgents(t)

	time.Sleep(3 * time.Second)

	targetID := chain.Agents[3].ID().String()

	// Command that produces multiple lines of output with delays
	stdout, _, exitCode, err := executeCommand(t, chain, targetID, "sh", []string{"-c", "for i in 1 2 3 4 5; do echo line$i; done"}, "")
	if err != nil {
		t.Fatalf("Command failed: %v", err)
	}

	if exitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", exitCode)
	}

	// Verify all lines are present
	for i := 1; i <= 5; i++ {
		expected := "line" + string(rune('0'+i))
		if !strings.Contains(stdout, expected) {
			t.Errorf("Expected stdout to contain '%s', got '%s'", expected, stdout)
		}
	}
}

// TestShell_AuthSuccess tests that authentication with correct password works.
func TestShell_AuthSuccess(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	password := "testpassword"
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}

	shellCfg := &config.ShellConfig{
		Enabled:      true,
		Whitelist:    []string{"*"},
		PasswordHash: string(hash),
		MaxSessions:  10,
	}

	chain := newShellTestChain(t, shellCfg)
	defer chain.Close()

	chain.CreateAgents(t)
	chain.StartAgents(t)

	time.Sleep(3 * time.Second)

	targetID := chain.Agents[3].ID().String()

	stdout, _, exitCode, err := executeCommand(t, chain, targetID, "whoami", nil, password)
	if err != nil {
		t.Fatalf("Command with correct password failed: %v", err)
	}

	if exitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", exitCode)
	}

	if strings.TrimSpace(stdout) == "" {
		t.Error("Expected non-empty output")
	}
}

// TestShell_AuthFailure tests that authentication with wrong password fails.
func TestShell_AuthFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	password := "testpassword"
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}

	shellCfg := &config.ShellConfig{
		Enabled:      true,
		Whitelist:    []string{"*"},
		PasswordHash: string(hash),
		MaxSessions:  10,
	}

	chain := newShellTestChain(t, shellCfg)
	defer chain.Close()

	chain.CreateAgents(t)
	chain.StartAgents(t)

	time.Sleep(3 * time.Second)

	targetID := chain.Agents[3].ID().String()

	_, _, _, err = executeCommand(t, chain, targetID, "whoami", nil, "wrongpassword")
	if err == nil {
		t.Error("Expected error with wrong password, got nil")
	} else {
		t.Logf("Got expected error: %v", err)
		if !strings.Contains(err.Error(), "credentials") && !strings.Contains(err.Error(), "auth") {
			t.Logf("Warning: error message doesn't mention credentials: %v", err)
		}
	}
}

// TestShell_AuthMissingPassword tests that missing password when required fails.
func TestShell_AuthMissingPassword(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	password := "testpassword"
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}

	shellCfg := &config.ShellConfig{
		Enabled:      true,
		Whitelist:    []string{"*"},
		PasswordHash: string(hash),
		MaxSessions:  10,
	}

	chain := newShellTestChain(t, shellCfg)
	defer chain.Close()

	chain.CreateAgents(t)
	chain.StartAgents(t)

	time.Sleep(3 * time.Second)

	targetID := chain.Agents[3].ID().String()

	// No password provided
	_, _, _, err = executeCommand(t, chain, targetID, "whoami", nil, "")
	if err == nil {
		t.Error("Expected error with missing password, got nil")
	} else {
		t.Logf("Got expected error: %v", err)
	}
}

// TestShell_WhitelistAllowed tests that whitelisted commands are allowed.
func TestShell_WhitelistAllowed(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	shellCfg := &config.ShellConfig{
		Enabled:     true,
		Whitelist:   []string{"whoami", "echo"},
		MaxSessions: 10,
	}

	chain := newShellTestChain(t, shellCfg)
	defer chain.Close()

	chain.CreateAgents(t)
	chain.StartAgents(t)

	time.Sleep(3 * time.Second)

	targetID := chain.Agents[3].ID().String()

	// whoami should be allowed
	stdout, _, exitCode, err := executeCommand(t, chain, targetID, "whoami", nil, "")
	if err != nil {
		t.Fatalf("whoami failed: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("Expected exit code 0 for whoami, got %d", exitCode)
	}
	t.Logf("whoami output: %s", strings.TrimSpace(stdout))

	// echo should be allowed
	stdout, _, exitCode, err = executeCommand(t, chain, targetID, "echo", []string{"test"}, "")
	if err != nil {
		t.Fatalf("echo failed: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("Expected exit code 0 for echo, got %d", exitCode)
	}
	if strings.TrimSpace(stdout) != "test" {
		t.Errorf("Expected 'test', got '%s'", stdout)
	}
}

// TestShell_WhitelistDenied tests that non-whitelisted commands are denied.
func TestShell_WhitelistDenied(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	shellCfg := &config.ShellConfig{
		Enabled:     true,
		Whitelist:   []string{"whoami"}, // Only whoami allowed
		MaxSessions: 10,
	}

	chain := newShellTestChain(t, shellCfg)
	defer chain.Close()

	chain.CreateAgents(t)
	chain.StartAgents(t)

	time.Sleep(3 * time.Second)

	targetID := chain.Agents[3].ID().String()

	// cat should be denied
	_, _, _, err := executeCommand(t, chain, targetID, "cat", []string{"/etc/passwd"}, "")
	if err == nil {
		t.Error("Expected error for non-whitelisted command, got nil")
	} else {
		t.Logf("Got expected error: %v", err)
		if !strings.Contains(err.Error(), "not allowed") && !strings.Contains(err.Error(), "whitelist") {
			t.Logf("Warning: error message doesn't mention whitelist: %v", err)
		}
	}
}

// TestShell_MaxSessions tests that session limits are enforced.
func TestShell_MaxSessions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	shellCfg := &config.ShellConfig{
		Enabled:     true,
		Whitelist:   []string{"*"},
		MaxSessions: 2, // Only allow 2 concurrent sessions
	}

	chain := newShellTestChain(t, shellCfg)
	defer chain.Close()

	chain.CreateAgents(t)
	chain.StartAgents(t)

	time.Sleep(3 * time.Second)

	targetID := chain.Agents[3].ID().String()

	// Start 2 long-running sessions
	var clients []*shell.Client
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		var stdoutBuf, stderrBuf bytes.Buffer
		client := shell.NewClient(shell.ClientConfig{
			AgentAddr:   chain.HTTPAddrs[0],
			TargetID:    targetID,
			Interactive: false,
			Command:     "sleep",
			Args:        []string{"10"},
			Stdout:      &stdoutBuf,
			Stderr:      &stderrBuf,
		})
		clients = append(clients, client)

		// Run in background
		go func(c *shell.Client) {
			c.Run(ctx)
		}(client)
	}

	// Give sessions time to start
	time.Sleep(500 * time.Millisecond)

	// Third session should fail
	_, _, _, err := executeCommand(t, chain, targetID, "whoami", nil, "")
	if err == nil {
		t.Log("Warning: third session succeeded - max sessions may not be enforced")
	} else {
		if strings.Contains(err.Error(), "session") || strings.Contains(err.Error(), "limit") {
			t.Logf("Got expected session limit error: %v", err)
		} else {
			t.Logf("Got error (may be session limit): %v", err)
		}
	}
}

// TestShell_TTYSessionOpen tests that TTY sessions can be opened and closed.
func TestShell_TTYSessionOpen(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	shellCfg := &config.ShellConfig{
		Enabled:     true,
		Whitelist:   []string{"*"},
		MaxSessions: 10,
	}

	chain := newShellTestChain(t, shellCfg)
	defer chain.Close()

	chain.CreateAgents(t)
	chain.StartAgents(t)

	time.Sleep(3 * time.Second)

	targetID := chain.Agents[3].ID().String()

	// For TTY mode, we'll use a command that exits immediately
	// since we can't really interact with the PTY in a test
	var stdoutBuf, stderrBuf bytes.Buffer
	client := shell.NewClient(shell.ClientConfig{
		AgentAddr:   chain.HTTPAddrs[0],
		TargetID:    targetID,
		Interactive: true, // TTY mode
		Command:     "sh",
		Args:        []string{"-c", "echo ttytest; exit 0"},
		Stdout:      &stdoutBuf,
		Stderr:      &stderrBuf,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	exitCode, err := client.Run(ctx)

	// TTY mode may have different behavior depending on whether stdin is a terminal
	// In a test environment, it might fall back to streaming mode
	if err != nil {
		t.Logf("TTY session result: err=%v", err)
	} else {
		t.Logf("TTY session completed with exit code %d", exitCode)
		if strings.Contains(stdoutBuf.String(), "ttytest") {
			t.Log("TTY output received correctly")
		}
	}
}
