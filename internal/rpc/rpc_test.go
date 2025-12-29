package rpc

import (
	"context"
	"testing"
	"time"
)

func TestHashPassword(t *testing.T) {
	password := "testpassword123"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}

	if hash == "" {
		t.Fatal("HashPassword() returned empty hash")
	}

	// Verify it's a bcrypt hash (starts with $2a$ or $2b$)
	if hash[0] != '$' || hash[1] != '2' {
		t.Errorf("HashPassword() returned invalid bcrypt hash: %s", hash[:10])
	}

	// Verify the hash validates correctly
	if !ValidatePassword(hash, password) {
		t.Error("ValidatePassword() returned false for correct password")
	}

	// Verify wrong password fails
	if ValidatePassword(hash, "wrongpassword") {
		t.Error("ValidatePassword() returned true for wrong password")
	}
}

func TestMustHashPassword(t *testing.T) {
	password := "testpassword"
	hash := MustHashPassword(password)

	if hash == "" {
		t.Fatal("MustHashPassword() returned empty hash")
	}

	if !ValidatePassword(hash, password) {
		t.Error("ValidatePassword() returned false for correct password")
	}
}

func TestExecutor_ValidateAuth(t *testing.T) {
	password := "secretpassword"
	hash := MustHashPassword(password)

	tests := []struct {
		name       string
		configHash string
		password   string
		wantErr    bool
	}{
		{
			name:       "no password configured",
			configHash: "",
			password:   "",
			wantErr:    false,
		},
		{
			name:       "correct password",
			configHash: hash,
			password:   password,
			wantErr:    false,
		},
		{
			name:       "wrong password",
			configHash: hash,
			password:   "wrongpassword",
			wantErr:    true,
		},
		{
			name:       "empty password when required",
			configHash: hash,
			password:   "",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewExecutor(Config{
				Enabled:      true,
				PasswordHash: tt.configHash,
			})

			err := e.ValidateAuth(tt.password)
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
			name:      "empty whitelist",
			whitelist: []string{},
			command:   "whoami",
			want:      false,
		},
		{
			name:      "wildcard allows all",
			whitelist: []string{"*"},
			command:   "anything",
			want:      true,
		},
		{
			name:      "wildcard allows paths",
			whitelist: []string{"*"},
			command:   "/bin/bash",
			want:      true,
		},
		{
			name:      "exact match allowed",
			whitelist: []string{"whoami", "ls", "ip"},
			command:   "whoami",
			want:      true,
		},
		{
			name:      "command not in whitelist",
			whitelist: []string{"whoami", "ls"},
			command:   "rm",
			want:      false,
		},
		{
			name:      "path traversal blocked",
			whitelist: []string{"whoami"},
			command:   "/tmp/evil/whoami",
			want:      false,
		},
		{
			name:      "windows path traversal blocked",
			whitelist: []string{"whoami"},
			command:   "C:\\temp\\whoami",
			want:      false,
		},
		{
			name:      "relative path blocked",
			whitelist: []string{"whoami"},
			command:   "./whoami",
			want:      false,
		},
		{
			name:      "parent path blocked",
			whitelist: []string{"whoami"},
			command:   "../whoami",
			want:      false,
		},
		{
			name:      "case sensitive",
			whitelist: []string{"whoami"},
			command:   "WHOAMI",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewExecutor(Config{
				Enabled:   true,
				Whitelist: tt.whitelist,
			})

			if got := e.IsCommandAllowed(tt.command); got != tt.want {
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
			whitelist: []string{"ls"},
			args:      []string{},
			wantErr:   false,
		},
		{
			name:      "safe args",
			whitelist: []string{"ls"},
			args:      []string{"-la", "myfile.txt"},
			wantErr:   false,
		},
		{
			name:      "relative path allowed",
			whitelist: []string{"cat"},
			args:      []string{"./file.txt"},
			wantErr:   false,
		},
		{
			name:      "semicolon injection blocked",
			whitelist: []string{"echo"},
			args:      []string{"hello; rm -rf /"},
			wantErr:   true,
		},
		{
			name:      "pipe injection blocked",
			whitelist: []string{"cat"},
			args:      []string{"file.txt | rm -rf /"},
			wantErr:   true,
		},
		{
			name:      "ampersand injection blocked",
			whitelist: []string{"echo"},
			args:      []string{"hello & rm -rf /"},
			wantErr:   true,
		},
		{
			name:      "backtick injection blocked",
			whitelist: []string{"echo"},
			args:      []string{"`rm -rf /`"},
			wantErr:   true,
		},
		{
			name:      "dollar sign blocked",
			whitelist: []string{"echo"},
			args:      []string{"$(rm -rf /)"},
			wantErr:   true,
		},
		{
			name:      "absolute path blocked",
			whitelist: []string{"cat"},
			args:      []string{"/etc/passwd"},
			wantErr:   true,
		},
		{
			name:      "wildcard in strict mode skipped",
			whitelist: []string{"*"},
			args:      []string{"/etc/passwd; rm -rf /"},
			wantErr:   false, // Wildcard mode skips validation
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewExecutor(Config{
				Enabled:   true,
				Whitelist: tt.whitelist,
			})

			err := e.ValidateArgs(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateArgs(%v) error = %v, wantErr %v", tt.args, err, tt.wantErr)
			}
		})
	}
}

func TestExecutor_Execute(t *testing.T) {
	tests := []struct {
		name         string
		config       Config
		request      *Request
		wantExitCode int
		wantError    string
	}{
		{
			name: "disabled RPC",
			config: Config{
				Enabled: false,
			},
			request: &Request{
				Command: "whoami",
			},
			wantExitCode: -1,
			wantError:    "RPC is disabled",
		},
		{
			name: "command not in whitelist",
			config: Config{
				Enabled:   true,
				Whitelist: []string{"ls"},
			},
			request: &Request{
				Command: "rm",
			},
			wantExitCode: -1,
			wantError:    "not in whitelist",
		},
		{
			name: "dangerous args rejected",
			config: Config{
				Enabled:   true,
				Whitelist: []string{"echo"},
			},
			request: &Request{
				Command: "echo",
				Args:    []string{"hello; rm -rf /"},
			},
			wantExitCode: -1,
			wantError:    "dangerous characters",
		},
		{
			name: "successful command",
			config: Config{
				Enabled:   true,
				Whitelist: []string{"echo"},
				Timeout:   5 * time.Second,
			},
			request: &Request{
				Command: "echo",
				Args:    []string{"hello"},
			},
			wantExitCode: 0,
			wantError:    "",
		},
		{
			name: "wildcard allows any command",
			config: Config{
				Enabled:   true,
				Whitelist: []string{"*"},
				Timeout:   5 * time.Second,
			},
			request: &Request{
				Command: "echo",
				Args:    []string{"hello"},
			},
			wantExitCode: 0,
			wantError:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewExecutor(tt.config)
			ctx := context.Background()

			resp, err := e.Execute(ctx, tt.request, nil)
			if err != nil {
				t.Fatalf("Execute() returned error: %v", err)
			}

			if resp.ExitCode != tt.wantExitCode {
				t.Errorf("Execute() exit code = %d, want %d", resp.ExitCode, tt.wantExitCode)
			}

			if tt.wantError != "" {
				if resp.Error == "" {
					t.Errorf("Execute() error is empty, want containing %q", tt.wantError)
				} else if !containsSubstring(resp.Error, tt.wantError) {
					t.Errorf("Execute() error = %q, want containing %q", resp.Error, tt.wantError)
				}
			}
		})
	}
}

func TestExecutor_Execute_Timeout(t *testing.T) {
	e := NewExecutor(Config{
		Enabled:   true,
		Whitelist: []string{"*"},
		Timeout:   100 * time.Millisecond,
	})

	ctx := context.Background()
	resp, err := e.Execute(ctx, &Request{
		Command: "sleep",
		Args:    []string{"10"},
	}, nil)

	if err != nil {
		t.Fatalf("Execute() returned error: %v", err)
	}

	if resp.ExitCode != -1 {
		t.Errorf("Execute() exit code = %d, want -1", resp.ExitCode)
	}

	if !containsSubstring(resp.Error, "timed out") {
		t.Errorf("Execute() error = %q, want containing 'timed out'", resp.Error)
	}
}

func TestEncodeDecodeRequest(t *testing.T) {
	req := &Request{
		Command: "echo",
		Args:    []string{"hello", "world"},
		Stdin:   "input data",
		Timeout: 30,
	}

	data, err := EncodeRequest(req)
	if err != nil {
		t.Fatalf("EncodeRequest() error = %v", err)
	}

	decoded, err := DecodeRequest(data)
	if err != nil {
		t.Fatalf("DecodeRequest() error = %v", err)
	}

	if decoded.Command != req.Command {
		t.Errorf("Command = %q, want %q", decoded.Command, req.Command)
	}
	if len(decoded.Args) != len(req.Args) {
		t.Errorf("Args len = %d, want %d", len(decoded.Args), len(req.Args))
	}
	if decoded.Stdin != req.Stdin {
		t.Errorf("Stdin = %q, want %q", decoded.Stdin, req.Stdin)
	}
	if decoded.Timeout != req.Timeout {
		t.Errorf("Timeout = %d, want %d", decoded.Timeout, req.Timeout)
	}
}

func TestEncodeDecodeResponse(t *testing.T) {
	resp := &Response{
		ExitCode: 0,
		Stdout:   "output",
		Stderr:   "errors",
		Error:    "",
	}

	data, err := EncodeResponse(resp)
	if err != nil {
		t.Fatalf("EncodeResponse() error = %v", err)
	}

	decoded, err := DecodeResponse(data)
	if err != nil {
		t.Fatalf("DecodeResponse() error = %v", err)
	}

	if decoded.ExitCode != resp.ExitCode {
		t.Errorf("ExitCode = %d, want %d", decoded.ExitCode, resp.ExitCode)
	}
	if decoded.Stdout != resp.Stdout {
		t.Errorf("Stdout = %q, want %q", decoded.Stdout, resp.Stdout)
	}
	if decoded.Stderr != resp.Stderr {
		t.Errorf("Stderr = %q, want %q", decoded.Stderr, resp.Stderr)
	}
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstringHelper(s, substr))
}

func containsSubstringHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
