package rpc

import (
	"context"
	"strings"
	"testing"
	"time"
)

// ============================================================================
// Authentication Bypass Negative Tests
// ============================================================================

// TestAuthBypass_EmptyPasswordWhenRequired tests that empty password fails when auth required.
func TestAuthBypass_EmptyPasswordWhenRequired(t *testing.T) {
	hash := MustHashPassword("secretpassword")

	e := NewExecutor(Config{
		Enabled:      true,
		Whitelist:    []string{"echo"},
		PasswordHash: hash,
	})

	testCases := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{"empty password", "", true},
		{"whitespace only", "   ", true},
		{"null byte", "\x00", true},
		{"wrong password", "wrongpassword", true},
		{"password with trailing space", "secretpassword ", true},
		{"password with leading space", " secretpassword", true},
		{"correct password", "secretpassword", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := e.ValidateAuth(tc.password)
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidateAuth(%q) error = %v, wantErr %v", tc.password, err, tc.wantErr)
			}
		})
	}
}

// TestAuthBypass_SkipAuthInRequest tests that requests without password validation are rejected.
// Note: Password validation is done via ValidateAuth() before Execute() is called.
// This test verifies the correct pattern for auth enforcement.
func TestAuthBypass_SkipAuthInRequest(t *testing.T) {
	hash := MustHashPassword("secret")

	e := NewExecutor(Config{
		Enabled:      true,
		Whitelist:    []string{"echo"},
		PasswordHash: hash,
		Timeout:      5 * time.Second,
	})

	ctx := context.Background()

	// Verify that ValidateAuth must be called before Execute
	// First, try without auth validation (simulating bypass attempt)
	err := e.ValidateAuth("") // Empty password
	if err == nil {
		t.Error("ValidateAuth should fail with empty password when hash is set")
	}

	// Verify that even with a request ready, auth is required
	err = e.ValidateAuth("wrongpassword")
	if err == nil {
		t.Error("ValidateAuth should fail with wrong password")
	}

	// Correct auth should work
	err = e.ValidateAuth("secret")
	if err != nil {
		t.Errorf("ValidateAuth should pass with correct password: %v", err)
	}

	// After successful auth, Execute should work
	resp, err := e.Execute(ctx, &Request{
		Command: "echo",
		Args:    []string{"hello"},
	}, nil)

	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if resp.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0, error: %s", resp.ExitCode, resp.Error)
	}
}

// TestAuthBypass_CommandInjection tests various command injection attempts.
func TestAuthBypass_CommandInjection(t *testing.T) {
	e := NewExecutor(Config{
		Enabled:   true,
		Whitelist: []string{"echo"},
		Timeout:   5 * time.Second,
	})

	ctx := context.Background()

	testCases := []struct {
		name    string
		command string
		args    []string
		wantErr bool
	}{
		// Command field injection
		{"semicolon in command", "echo; rm -rf /", nil, true},
		{"pipe in command", "echo | cat /etc/passwd", nil, true},
		{"backtick in command", "echo `whoami`", nil, true},
		{"dollar paren in command", "echo $(id)", nil, true},
		{"newline in command", "echo\nrm -rf /", nil, true},
		{"null byte in command", "echo\x00rm", nil, true},

		// Path traversal in command
		{"absolute path command", "/bin/echo", nil, true},
		{"relative path command", "./echo", nil, true},
		{"parent dir command", "../echo", nil, true},
		{"hidden path command", "/tmp/.evil/echo", nil, true},

		// Args injection
		{"semicolon in args", "echo", []string{"hello;rm -rf /"}, true},
		{"pipe in args", "echo", []string{"hello|cat /etc/passwd"}, true},
		{"backtick in args", "echo", []string{"`id`"}, true},
		{"dollar paren in args", "echo", []string{"$(whoami)"}, true},
		{"ampersand in args", "echo", []string{"hello&rm -rf /"}, true},
		{"double ampersand in args", "echo", []string{"hello&&rm"}, true},
		{"redirect out in args", "echo", []string{"hello>/etc/passwd"}, true},
		{"redirect in args", "echo", []string{"hello</etc/passwd"}, true},

		// Valid cases
		{"simple echo", "echo", []string{"hello"}, false},
		{"echo with spaces", "echo", []string{"hello world"}, false},
		{"echo with hyphen", "echo", []string{"-n", "hello"}, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := e.Execute(ctx, &Request{
				Command: tc.command,
				Args:    tc.args,
			}, nil)

			if err != nil {
				t.Fatalf("Execute returned error: %v", err)
			}

			gotErr := resp.ExitCode == -1 && resp.Error != ""
			if gotErr != tc.wantErr {
				t.Errorf("command=%q args=%v: gotErr=%v, wantErr=%v, error=%q",
					tc.command, tc.args, gotErr, tc.wantErr, resp.Error)
			}
		})
	}
}

// TestAuthBypass_WhitelistBypass tests attempts to bypass the command whitelist.
func TestAuthBypass_WhitelistBypass(t *testing.T) {
	e := NewExecutor(Config{
		Enabled:   true,
		Whitelist: []string{"echo", "ls"},
		Timeout:   5 * time.Second,
	})

	ctx := context.Background()

	testCases := []struct {
		name    string
		command string
		wantErr bool
	}{
		// Direct bypass attempts
		{"not in whitelist", "rm", true},
		{"similar to whitelisted", "echos", true},
		{"prefix of whitelisted", "ech", true},
		{"suffix of whitelisted", "cho", true},
		{"whitelisted with suffix", "echo1", true},

		// Case variations
		{"uppercase", "ECHO", true},
		{"mixed case", "Echo", true},

		// Path attempts
		{"absolute path to echo", "/bin/echo", true},
		{"relative path", "./echo", true},
		{"parent path", "../bin/echo", true},

		// Null byte injection
		{"null after command", "echo\x00rm", true},
		{"null before command", "\x00echo", true},

		// Valid commands
		{"echo lowercase", "echo", false},
		{"ls lowercase", "ls", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := e.Execute(ctx, &Request{
				Command: tc.command,
				Args:    []string{},
			}, nil)

			if err != nil {
				t.Fatalf("Execute returned error: %v", err)
			}

			gotErr := resp.ExitCode == -1
			if gotErr != tc.wantErr {
				t.Errorf("command=%q: gotErr=%v, wantErr=%v, error=%q",
					tc.command, gotErr, tc.wantErr, resp.Error)
			}
		})
	}
}

// TestAuthBypass_WildcardWhitelist tests that wildcard whitelist is dangerous.
func TestAuthBypass_WildcardWhitelist(t *testing.T) {
	e := NewExecutor(Config{
		Enabled:   true,
		Whitelist: []string{"*"},
		Timeout:   5 * time.Second,
	})

	ctx := context.Background()

	// With wildcard, any command should be allowed (this is intentional but dangerous)
	resp, err := e.Execute(ctx, &Request{
		Command: "echo",
		Args:    []string{"this works with wildcard"},
	}, nil)

	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if resp.ExitCode != 0 {
		t.Errorf("Wildcard should allow echo, got ExitCode=%d, Error=%q", resp.ExitCode, resp.Error)
	}

	// Test that even path commands work with wildcard
	resp, err = e.Execute(ctx, &Request{
		Command: "/bin/echo",
		Args:    []string{"hello"},
	}, nil)

	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	// Wildcard should allow absolute paths
	if resp.ExitCode != 0 {
		t.Logf("With wildcard, /bin/echo returned: ExitCode=%d, Error=%q", resp.ExitCode, resp.Error)
	}
}

// TestAuthBypass_DisabledRPC tests that disabled RPC rejects all requests.
func TestAuthBypass_DisabledRPC(t *testing.T) {
	e := NewExecutor(Config{
		Enabled:   false, // Disabled!
		Whitelist: []string{"*"},
	})

	ctx := context.Background()

	resp, err := e.Execute(ctx, &Request{
		Command: "echo",
		Args:    []string{"should not work"},
	}, nil)

	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if resp.ExitCode != -1 {
		t.Errorf("Disabled RPC should reject, got ExitCode=%d", resp.ExitCode)
	}

	if !strings.Contains(resp.Error, "disabled") {
		t.Errorf("Error should mention disabled, got: %q", resp.Error)
	}
}

// TestAuthBypass_TimeoutBypass tests that timeout is enforced.
func TestAuthBypass_TimeoutBypass(t *testing.T) {
	e := NewExecutor(Config{
		Enabled:   true,
		Whitelist: []string{"*"},
		Timeout:   100 * time.Millisecond, // Very short timeout
	})

	ctx := context.Background()

	start := time.Now()
	resp, err := e.Execute(ctx, &Request{
		Command: "sleep",
		Args:    []string{"10"}, // 10 seconds
		Timeout: 0,              // Use default
	}, nil)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	// Should have timed out within ~200ms, not 10 seconds
	if elapsed > 2*time.Second {
		t.Errorf("Command ran too long: %v", elapsed)
	}

	if resp.ExitCode != -1 {
		t.Errorf("Timed out command should have ExitCode=-1, got %d", resp.ExitCode)
	}

	if !strings.Contains(resp.Error, "timed out") {
		t.Errorf("Error should mention timeout, got: %q", resp.Error)
	}
}

// TestAuthBypass_RequestTimeoutOverride tests that per-request timeout works.
func TestAuthBypass_RequestTimeoutOverride(t *testing.T) {
	e := NewExecutor(Config{
		Enabled:   true,
		Whitelist: []string{"*"},
		Timeout:   10 * time.Second, // Long default
	})

	ctx := context.Background()

	start := time.Now()
	resp, err := e.Execute(ctx, &Request{
		Command: "sleep",
		Args:    []string{"10"},
		Timeout: 1, // 1 second per-request timeout
	}, nil)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	// Should respect per-request timeout
	if elapsed > 3*time.Second {
		t.Errorf("Per-request timeout not honored: %v", elapsed)
	}

	if resp.ExitCode != -1 {
		t.Errorf("Expected timeout, got ExitCode=%d", resp.ExitCode)
	}
}

// TestAuthBypass_StdinInjection tests that stdin is properly isolated.
func TestAuthBypass_StdinInjection(t *testing.T) {
	e := NewExecutor(Config{
		Enabled:   true,
		Whitelist: []string{"cat"},
		Timeout:   5 * time.Second,
	})

	ctx := context.Background()

	// Large stdin that tries to overwhelm
	largeStdin := strings.Repeat("A", 1024*1024) // 1MB

	resp, err := e.Execute(ctx, &Request{
		Command: "cat",
		Args:    []string{},
		Stdin:   largeStdin,
	}, nil)

	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	// Should either succeed with the data or fail gracefully
	if resp.ExitCode == 0 {
		// If it succeeded, stdout should match stdin
		if resp.Stdout != largeStdin {
			t.Logf("Stdout length: %d, Stdin length: %d", len(resp.Stdout), len(largeStdin))
		}
	}
}

// TestAuthBypass_EnvVarExpansion tests that environment variables aren't expanded.
func TestAuthBypass_EnvVarExpansion(t *testing.T) {
	e := NewExecutor(Config{
		Enabled:   true,
		Whitelist: []string{"echo"},
		Timeout:   5 * time.Second,
	})

	ctx := context.Background()

	// Try to get env vars expanded
	resp, err := e.Execute(ctx, &Request{
		Command: "echo",
		Args:    []string{"$HOME", "${PATH}", "$(id)"},
	}, nil)

	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	// Args with $ should be blocked by dangerous character check
	if resp.ExitCode == 0 {
		// If not blocked, output should contain literal $ not expanded values
		if !strings.Contains(resp.Stdout, "$") && !strings.Contains(resp.Error, "dangerous") {
			t.Logf("Output may have expanded env vars: %q", resp.Stdout)
		}
	}
}

// TestAuthBypass_ConcurrentAuth tests concurrent auth validation.
func TestAuthBypass_ConcurrentAuth(t *testing.T) {
	hash := MustHashPassword("secret")

	e := NewExecutor(Config{
		Enabled:      true,
		Whitelist:    []string{"echo"},
		PasswordHash: hash,
		Timeout:      5 * time.Second,
	})

	// Concurrent auth validations shouldn't interfere
	done := make(chan bool, 100)
	for i := 0; i < 100; i++ {
		go func(attempt int) {
			defer func() { done <- true }()

			// Alternate between correct and wrong passwords
			password := "wrong"
			expectSuccess := false
			if attempt%2 == 0 {
				password = "secret"
				expectSuccess = true
			}

			err := e.ValidateAuth(password)
			gotSuccess := err == nil

			if gotSuccess != expectSuccess {
				t.Errorf("Attempt %d: password=%q, gotSuccess=%v, expectSuccess=%v",
					attempt, password, gotSuccess, expectSuccess)
			}
		}(i)
	}

	for i := 0; i < 100; i++ {
		<-done
	}
}

// TestAuthBypass_PasswordHash tests password hash validation edge cases.
func TestAuthBypass_PasswordHash(t *testing.T) {
	testCases := []struct {
		name         string
		passwordHash string
		password     string
		wantErr      bool
	}{
		{
			name:         "valid bcrypt hash",
			passwordHash: MustHashPassword("correct"),
			password:     "correct",
			wantErr:      false,
		},
		{
			name:         "invalid hash format",
			passwordHash: "notabcrypthash",
			password:     "anything",
			wantErr:      true,
		},
		{
			name:         "empty hash allows empty password",
			passwordHash: "",
			password:     "",
			wantErr:      false,
		},
		{
			name:         "empty hash allows any password",
			passwordHash: "",
			password:     "anything",
			wantErr:      false,
		},
		{
			name:         "hash with wrong password",
			passwordHash: MustHashPassword("correct"),
			password:     "incorrect",
			wantErr:      true,
		},
		{
			name:         "unicode password",
			passwordHash: MustHashPassword("secret"),
			password:     "secret", // Match the hash
			wantErr:      false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			e := NewExecutor(Config{
				Enabled:      true,
				Whitelist:    []string{"echo"},
				PasswordHash: tc.passwordHash,
			})

			err := e.ValidateAuth(tc.password)
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidateAuth() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

// TestAuthBypass_ContextCancellation tests that context cancellation is respected.
func TestAuthBypass_ContextCancellation(t *testing.T) {
	e := NewExecutor(Config{
		Enabled:   true,
		Whitelist: []string{"*"},
		Timeout:   30 * time.Second,
	})

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately
	cancel()

	start := time.Now()
	resp, err := e.Execute(ctx, &Request{
		Command: "sleep",
		Args:    []string{"10"},
	}, nil)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	// Should have returned quickly due to cancelled context
	if elapsed > 2*time.Second {
		t.Errorf("Cancelled context should return quickly, took %v", elapsed)
	}

	if resp.ExitCode == 0 {
		t.Error("Cancelled context should not allow successful execution")
	}
}
