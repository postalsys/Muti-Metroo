package shell

import (
	"bytes"
	"testing"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name        string
		cfg         ClientConfig
		wantURL     string
		wantHasArgs bool
	}{
		{
			name: "basic streaming mode",
			cfg: ClientConfig{
				AgentAddr:   "localhost:8080",
				TargetID:    "abc123def456",
				Interactive: false,
				Command:     "echo",
				Args:        []string{"hello"},
			},
			wantURL:     "ws://localhost:8080/agents/abc123def456/shell?mode=stream",
			wantHasArgs: true,
		},
		{
			name: "interactive mode (tty)",
			cfg: ClientConfig{
				AgentAddr:   "192.168.1.10:8080",
				TargetID:    "target-agent-id",
				Interactive: true,
				Command:     "bash",
			},
			wantURL:     "ws://192.168.1.10:8080/agents/target-agent-id/shell?mode=tty",
			wantHasArgs: false,
		},
		{
			name: "with password",
			cfg: ClientConfig{
				AgentAddr:   "localhost:8080",
				TargetID:    "abcd1234",
				Interactive: false,
				Password:    "secretpass",
				Command:     "whoami",
			},
			wantURL:     "ws://localhost:8080/agents/abcd1234/shell?mode=stream",
			wantHasArgs: false,
		},
		{
			name: "with environment",
			cfg: ClientConfig{
				AgentAddr:   "localhost:8080",
				TargetID:    "env-test-id",
				Interactive: false,
				Command:     "env",
				Env:         map[string]string{"FOO": "bar", "BAZ": "qux"},
			},
			wantURL:     "ws://localhost:8080/agents/env-test-id/shell?mode=stream",
			wantHasArgs: false,
		},
		{
			name: "with work directory",
			cfg: ClientConfig{
				AgentAddr:   "localhost:8080",
				TargetID:    "workdir-id",
				Interactive: false,
				Command:     "pwd",
				WorkDir:     "/tmp/test",
			},
			wantURL: "ws://localhost:8080/agents/workdir-id/shell?mode=stream",
		},
		{
			name: "with timeout",
			cfg: ClientConfig{
				AgentAddr:   "localhost:8080",
				TargetID:    "timeout-id",
				Interactive: false,
				Command:     "sleep",
				Args:        []string{"10"},
				Timeout:     5,
			},
			wantURL: "ws://localhost:8080/agents/timeout-id/shell?mode=stream",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient(tt.cfg)
			if client == nil {
				t.Fatal("NewClient() returned nil")
			}

			if client.url != tt.wantURL {
				t.Errorf("url = %q, want %q", client.url, tt.wantURL)
			}

			if client.interactive != tt.cfg.Interactive {
				t.Errorf("interactive = %v, want %v", client.interactive, tt.cfg.Interactive)
			}

			if client.command != tt.cfg.Command {
				t.Errorf("command = %q, want %q", client.command, tt.cfg.Command)
			}

			if client.password != tt.cfg.Password {
				t.Errorf("password = %q, want %q", client.password, tt.cfg.Password)
			}

			if client.workDir != tt.cfg.WorkDir {
				t.Errorf("workDir = %q, want %q", client.workDir, tt.cfg.WorkDir)
			}

			if client.timeout != tt.cfg.Timeout {
				t.Errorf("timeout = %d, want %d", client.timeout, tt.cfg.Timeout)
			}
		})
	}
}

func TestNewClient_CustomOutputWriters(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	cfg := ClientConfig{
		AgentAddr: "localhost:8080",
		TargetID:  "test-id-1234",
		Command:   "echo",
		Args:      []string{"test"},
		Stdout:    stdout,
		Stderr:    stderr,
	}

	client := NewClient(cfg)
	if client == nil {
		t.Fatal("NewClient() returned nil")
	}

	if client.stdout != stdout {
		t.Error("stdout writer not set correctly")
	}
	if client.stderr != stderr {
		t.Error("stderr writer not set correctly")
	}
}

func TestNewClient_DefaultOutputWriters(t *testing.T) {
	cfg := ClientConfig{
		AgentAddr: "localhost:8080",
		TargetID:  "test-id-5678",
		Command:   "echo",
		// Stdout and Stderr not set - should use defaults
	}

	client := NewClient(cfg)
	if client == nil {
		t.Fatal("NewClient() returned nil")
	}

	// Should have non-nil writers (os.Stdout/os.Stderr)
	if client.stdout == nil {
		t.Error("stdout should not be nil")
	}
	if client.stderr == nil {
		t.Error("stderr should not be nil")
	}
}

func TestClient_displayName(t *testing.T) {
	tests := []struct {
		name       string
		agentName  string
		targetID   string
		wantResult string
	}{
		{
			name:       "with agent name",
			agentName:  "my-agent-server",
			targetID:   "abcd1234efgh5678",
			wantResult: "my-agent-server",
		},
		{
			name:       "without agent name - short target ID",
			agentName:  "",
			targetID:   "abcd1234efgh5678",
			wantResult: "abcd1234",
		},
		{
			name:       "without agent name - uses first 8 chars",
			agentName:  "",
			targetID:   "12345678901234567890",
			wantResult: "12345678",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{
				agentName: tt.agentName,
				targetID:  tt.targetID,
			}

			result := client.displayName()
			if result != tt.wantResult {
				t.Errorf("displayName() = %q, want %q", result, tt.wantResult)
			}
		})
	}
}

func TestClient_setError(t *testing.T) {
	client := &Client{
		done: make(chan struct{}),
	}

	// First error should be set
	err1 := &testError{"first error"}
	client.setError(err1)

	if client.exitError != err1 {
		t.Error("first error should be set")
	}

	// Second error should be ignored
	err2 := &testError{"second error"}
	client.setError(err2)

	if client.exitError != err1 {
		t.Error("first error should still be set, second should be ignored")
	}
}

// testError is a simple error implementation for testing.
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
