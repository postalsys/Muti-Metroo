package shell

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Config contains shell configuration.
type Config struct {
	// Enabled controls whether shell is available
	Enabled bool `yaml:"enabled"`

	// Whitelist contains allowed commands. Empty list = no commands allowed.
	// Use ["*"] to allow all commands (for testing only!).
	// Commands should be base names only (e.g., "whoami", "ls", "bash").
	Whitelist []string `yaml:"whitelist"`

	// PasswordHash is the bcrypt hash of the shell password.
	// If set, all shell requests must include the correct password.
	PasswordHash string `yaml:"password_hash"`

	// Timeout is the optional command timeout (0 = no timeout).
	Timeout time.Duration `yaml:"timeout"`

	// MaxSessions limits concurrent shell sessions (0 = unlimited)
	MaxSessions int `yaml:"max_sessions"`
}

// DefaultConfig returns default shell configuration (disabled).
func DefaultConfig() Config {
	return Config{
		Enabled:     false,
		Whitelist:   []string{},
		MaxSessions: 0, // 0 = unlimited
	}
}

// Executor handles shell command execution with security checks.
type Executor struct {
	config   Config
	mu       sync.Mutex
	sessions int // Active session count
}

// NewExecutor creates a new shell executor.
func NewExecutor(cfg Config) *Executor {
	return &Executor{
		config: cfg,
	}
}

// ValidateAuth checks if the provided password matches the configured bcrypt hash.
func (e *Executor) ValidateAuth(password string) error {
	hash := e.config.PasswordHash
	if hash == "" {
		// No password configured, authentication not required
		return nil
	}

	if password == "" {
		return fmt.Errorf("authentication required")
	}

	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err != nil {
		return fmt.Errorf("invalid credentials")
	}

	return nil
}

// dangerousArgPattern matches shell metacharacters and injection attempts.
var dangerousArgPattern = regexp.MustCompile(`[;&|$` + "`" + `(){}[\]<>\\!*?~]`)

// IsCommandAllowed checks if the command is in the whitelist.
func (e *Executor) IsCommandAllowed(command string) bool {
	whitelist := e.config.Whitelist

	if len(whitelist) == 0 {
		return false
	}

	// Check for wildcard first
	for _, w := range whitelist {
		if w == "*" {
			return true
		}
	}

	// Only allow base command names - no paths allowed
	if strings.ContainsAny(command, "/\\") {
		return false
	}

	// Command must match exactly (case-sensitive)
	for _, allowed := range whitelist {
		if allowed == command {
			return true
		}
	}

	return false
}

// ValidateArgs checks command arguments for dangerous patterns.
func (e *Executor) ValidateArgs(args []string) error {
	whitelist := e.config.Whitelist

	// In wildcard mode, skip argument validation
	for _, w := range whitelist {
		if w == "*" {
			return nil
		}
	}

	for i, arg := range args {
		if dangerousArgPattern.MatchString(arg) {
			return fmt.Errorf("argument %d contains dangerous characters", i)
		}
		if filepath.IsAbs(arg) {
			return fmt.Errorf("argument %d: absolute paths not allowed", i)
		}
	}
	return nil
}

// AcquireSession tries to acquire a session slot.
// Returns an error if max sessions reached.
func (e *Executor) AcquireSession() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.config.MaxSessions > 0 && e.sessions >= e.config.MaxSessions {
		return fmt.Errorf("max sessions (%d) reached", e.config.MaxSessions)
	}

	e.sessions++
	return nil
}

// ReleaseSession releases a session slot.
func (e *Executor) ReleaseSession() {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.sessions > 0 {
		e.sessions--
	}
}

// ActiveSessions returns the current number of active sessions.
func (e *Executor) ActiveSessions() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.sessions
}

// Session represents an active shell session.
type Session struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    io.ReadCloser
	stderr    io.ReadCloser
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}
	exitCode  int32
	err       error
	mu        sync.Mutex
	started   bool
	startTime time.Time
}

// NewSession creates a new streaming shell session (non-PTY).
func (e *Executor) NewSession(ctx context.Context, meta *ShellMeta) (*Session, error) {
	if !e.config.Enabled {
		return nil, fmt.Errorf("shell is disabled")
	}

	// Validate authentication
	if err := e.ValidateAuth(meta.Password); err != nil {
		return nil, err
	}

	// Validate command
	if !e.IsCommandAllowed(meta.Command) {
		return nil, fmt.Errorf("command '%s' is not allowed", meta.Command)
	}

	// Validate arguments
	if err := e.ValidateArgs(meta.Args); err != nil {
		return nil, err
	}

	// Acquire session slot
	if err := e.AcquireSession(); err != nil {
		return nil, err
	}

	// Determine timeout (per-request timeout takes precedence, then config timeout)
	var maxDuration time.Duration
	if meta.Timeout > 0 {
		maxDuration = time.Duration(meta.Timeout) * time.Second
	} else if e.config.Timeout > 0 {
		maxDuration = e.config.Timeout
	}

	// Create context with optional timeout
	var sessionCtx context.Context
	var cancel context.CancelFunc
	if maxDuration > 0 {
		sessionCtx, cancel = context.WithTimeout(ctx, maxDuration)
	} else {
		sessionCtx, cancel = context.WithCancel(ctx)
	}

	// Create command
	cmd := exec.CommandContext(sessionCtx, meta.Command, meta.Args...)

	// Set up environment
	if len(meta.Env) > 0 {
		cmd.Env = os.Environ()
		for k, v := range meta.Env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}

	// Set working directory
	if meta.WorkDir != "" {
		cmd.Dir = meta.WorkDir
	}

	// Set up pipes
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		e.ReleaseSession()
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		cancel()
		e.ReleaseSession()
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		stdin.Close()
		stdout.Close()
		cancel()
		e.ReleaseSession()
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	session := &Session{
		cmd:       cmd,
		stdin:     stdin,
		stdout:    stdout,
		stderr:    stderr,
		ctx:       sessionCtx,
		cancel:    cancel,
		done:      make(chan struct{}),
		exitCode:  -1,
		startTime: time.Now(),
	}

	return session, nil
}

// Start starts the session command.
func (s *Session) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return fmt.Errorf("session already started")
	}

	if err := s.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start command: %w", err)
	}

	s.started = true

	// Wait for command in background
	go func() {
		err := s.cmd.Wait()
		s.mu.Lock()
		s.err = err
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				s.exitCode = int32(exitErr.ExitCode())
			}
		} else {
			s.exitCode = 0
		}
		s.mu.Unlock()
		close(s.done)
	}()

	return nil
}

// Stdin returns the stdin writer for the session.
func (s *Session) Stdin() io.WriteCloser {
	return s.stdin
}

// Stdout returns the stdout reader for the session.
func (s *Session) Stdout() io.ReadCloser {
	return s.stdout
}

// Stderr returns the stderr reader for the session.
func (s *Session) Stderr() io.ReadCloser {
	return s.stderr
}

// Done returns a channel that closes when the session exits.
func (s *Session) Done() <-chan struct{} {
	return s.done
}

// Context returns the session context.
func (s *Session) Context() context.Context {
	return s.ctx
}

// ExitCode returns the exit code after the session ends.
func (s *Session) ExitCode() int32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.exitCode
}

// Error returns any error from the session.
func (s *Session) Error() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// Signal sends a signal to the session process.
func (s *Session) Signal(sig syscall.Signal) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		return fmt.Errorf("session not started")
	}

	if s.cmd.Process == nil {
		return fmt.Errorf("no process")
	}

	return s.cmd.Process.Signal(sig)
}

// Close terminates the session.
func (s *Session) Close() {
	s.cancel()

	// Close stdin to signal EOF to process
	if s.stdin != nil {
		s.stdin.Close()
	}

	// Kill the process if still running
	s.mu.Lock()
	if s.started && s.cmd.Process != nil {
		s.cmd.Process.Kill()
	}
	s.mu.Unlock()

	// Wait for done
	select {
	case <-s.done:
	case <-time.After(5 * time.Second):
		// Force kill if not done
	}
}

// Duration returns how long the session has been running.
func (s *Session) Duration() time.Duration {
	return time.Since(s.startTime)
}
