// Package rpc implements remote procedure call functionality for Muti Metroo.
package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// MaxStdinSize is the maximum allowed stdin size (1MB).
const MaxStdinSize = 1 * 1024 * 1024

// MaxOutputSize is the maximum allowed stdout/stderr size (4MB each).
const MaxOutputSize = 4 * 1024 * 1024

// DefaultTimeout is the default command execution timeout.
const DefaultTimeout = 60 * time.Second

// Request represents an RPC request to execute a command.
type Request struct {
	Command string   `json:"command"`          // Command to execute (e.g., "whoami", "ip")
	Args    []string `json:"args,omitempty"`   // Command arguments
	Stdin   string   `json:"stdin,omitempty"`  // Base64-encoded stdin data
	Timeout int      `json:"timeout,omitempty"` // Timeout in seconds (default: 60)
}

// Response represents the result of an RPC command execution.
type Response struct {
	ExitCode int    `json:"exit_code"`          // Process exit code (0 = success)
	Stdout   string `json:"stdout,omitempty"`   // Base64-encoded stdout
	Stderr   string `json:"stderr,omitempty"`   // Base64-encoded stderr
	Error    string `json:"error,omitempty"`    // Error message if command failed to execute
}

// Config contains RPC configuration.
type Config struct {
	// Enabled controls whether RPC is available
	Enabled bool `yaml:"enabled"`

	// Whitelist contains allowed commands. Empty list = no commands allowed.
	// Use ["*"] to allow all commands (for testing only!).
	// Commands should be base names only (e.g., "whoami", "ls", "ip").
	Whitelist []string `yaml:"whitelist"`

	// PasswordHash is the bcrypt hash of the RPC password.
	// If set, all RPC requests must include the correct password.
	// Generate with: htpasswd -bnBC 10 "" <password> | tr -d ':\n'
	// Or use the HashPassword() function.
	PasswordHash string `yaml:"password_hash"`

	// Timeout is the default command execution timeout.
	Timeout time.Duration `yaml:"timeout"`
}

// DefaultConfig returns default RPC configuration (disabled, empty whitelist).
func DefaultConfig() Config {
	return Config{
		Enabled:   false,
		Whitelist: []string{},
		Timeout:   DefaultTimeout,
	}
}

// Executor handles RPC command execution with security checks.
type Executor struct {
	config Config
}

// NewExecutor creates a new RPC executor.
func NewExecutor(cfg Config) *Executor {
	if cfg.Timeout == 0 {
		cfg.Timeout = DefaultTimeout
	}
	return &Executor{config: cfg}
}

// ValidateAuth checks if the provided password matches the configured bcrypt hash.
// Returns nil if authentication passes, error otherwise.
func (e *Executor) ValidateAuth(password string) error {
	if e.config.PasswordHash == "" {
		// No password configured, authentication not required
		return nil
	}

	if password == "" {
		return fmt.Errorf("authentication required")
	}

	// Compare password against bcrypt hash
	err := bcrypt.CompareHashAndPassword([]byte(e.config.PasswordHash), []byte(password))
	if err != nil {
		return fmt.Errorf("invalid credentials")
	}

	return nil
}

// dangerousArgPattern matches shell metacharacters and injection attempts.
var dangerousArgPattern = regexp.MustCompile(`[;&|$` + "`" + `(){}[\]<>\\!*?~]`)

// IsCommandAllowed checks if the command is in the whitelist.
// The whitelist supports:
//   - "*" to allow all commands (for dev/testing only)
//   - Base command names (e.g., "whoami", "ls", "ip")
//
// Path traversal is blocked - only the base name of the command is checked.
func (e *Executor) IsCommandAllowed(command string) bool {
	if len(e.config.Whitelist) == 0 {
		return false
	}

	// Check for wildcard first
	for _, w := range e.config.Whitelist {
		if w == "*" {
			return true
		}
	}

	// Only allow base command names - no paths allowed in command
	// This prevents bypass via "/tmp/evil/whoami" when "whoami" is whitelisted
	if strings.ContainsAny(command, "/\\") {
		return false
	}

	// Command must match a whitelist entry exactly (case-sensitive)
	for _, allowed := range e.config.Whitelist {
		if allowed == command {
			return true
		}
	}

	return false
}

// ValidateArgs checks command arguments for dangerous patterns.
// Returns an error if any argument contains shell metacharacters.
func (e *Executor) ValidateArgs(args []string) error {
	// In wildcard mode, skip argument validation (for dev/testing)
	for _, w := range e.config.Whitelist {
		if w == "*" {
			return nil
		}
	}

	for i, arg := range args {
		if dangerousArgPattern.MatchString(arg) {
			return fmt.Errorf("argument %d contains dangerous characters", i)
		}
		// Block arguments that look like paths to prevent accessing arbitrary files
		if filepath.IsAbs(arg) {
			return fmt.Errorf("argument %d: absolute paths not allowed", i)
		}
	}
	return nil
}

// Execute runs the RPC command and returns the result.
func (e *Executor) Execute(ctx context.Context, req *Request, stdin []byte) (*Response, error) {
	if !e.config.Enabled {
		return &Response{
			ExitCode: -1,
			Error:    "RPC is disabled on this agent",
		}, nil
	}

	if !e.IsCommandAllowed(req.Command) {
		return &Response{
			ExitCode: -1,
			Error:    fmt.Sprintf("command '%s' is not in whitelist", req.Command),
		}, nil
	}

	// Validate arguments for dangerous patterns
	if err := e.ValidateArgs(req.Args); err != nil {
		return &Response{
			ExitCode: -1,
			Error:    fmt.Sprintf("invalid arguments: %v", err),
		}, nil
	}

	// Determine timeout
	timeout := e.config.Timeout
	if req.Timeout > 0 {
		timeout = time.Duration(req.Timeout) * time.Second
	}

	// Create context with timeout
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Create command
	cmd := exec.CommandContext(execCtx, req.Command, req.Args...)

	// Set up stdin if provided
	if len(stdin) > 0 {
		cmd.Stdin = bytes.NewReader(stdin)
	}

	// Capture stdout and stderr with size limits
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &limitedWriter{w: &stdout, limit: MaxOutputSize}
	cmd.Stderr = &limitedWriter{w: &stderr, limit: MaxOutputSize}

	// Run the command
	err := cmd.Run()

	response := &Response{
		ExitCode: 0,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}

	if err != nil {
		if execCtx.Err() == context.DeadlineExceeded {
			response.ExitCode = -1
			response.Error = fmt.Sprintf("command timed out after %v", timeout)
		} else if exitErr, ok := err.(*exec.ExitError); ok {
			response.ExitCode = exitErr.ExitCode()
		} else {
			response.ExitCode = -1
			response.Error = err.Error()
		}
	}

	return response, nil
}

// limitedWriter wraps a writer with a size limit.
type limitedWriter struct {
	w       io.Writer
	limit   int
	written int
}

func (lw *limitedWriter) Write(p []byte) (int, error) {
	if lw.written >= lw.limit {
		return len(p), nil // Discard but don't error
	}

	remaining := lw.limit - lw.written
	if len(p) > remaining {
		p = p[:remaining]
	}

	n, err := lw.w.Write(p)
	lw.written += n
	return n, err
}

// DefaultBcryptCost is the default cost for bcrypt hashing.
// Cost 10 provides a good balance between security and performance.
const DefaultBcryptCost = 10

// HashPassword creates a bcrypt hash of the password.
// Returns the hash string or an error if hashing fails.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), DefaultBcryptCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}
	return string(hash), nil
}

// MustHashPassword creates a bcrypt hash of the password.
// Panics if hashing fails (for use in tests and initialization).
func MustHashPassword(password string) string {
	hash, err := HashPassword(password)
	if err != nil {
		panic(err)
	}
	return hash
}

// ValidatePassword checks if the password matches the bcrypt hash.
func ValidatePassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// EncodeRequest serializes an RPC request to JSON.
func EncodeRequest(req *Request) ([]byte, error) {
	return json.Marshal(req)
}

// DecodeRequest deserializes an RPC request from JSON.
func DecodeRequest(data []byte) (*Request, error) {
	var req Request
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, err
	}
	return &req, nil
}

// EncodeResponse serializes an RPC response to JSON.
func EncodeResponse(resp *Response) ([]byte, error) {
	return json.Marshal(resp)
}

// DecodeResponse deserializes an RPC response from JSON.
func DecodeResponse(data []byte) (*Response, error) {
	var resp Response
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
