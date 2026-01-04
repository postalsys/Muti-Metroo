//go:build windows

package shell

import (
	"context"
	"fmt"
	"syscall"
)

// PTYSessionInterface defines the interface for PTY sessions.
type PTYSessionInterface interface {
	// Read reads from the PTY output.
	Read(p []byte) (n int, err error)
	// Write writes to the PTY input.
	Write(p []byte) (n int, err error)
	// Resize resizes the PTY.
	Resize(rows, cols uint16) error
	// Signal sends a signal to the process.
	Signal(sig syscall.Signal) error
	// Wait waits for the process to exit and returns the exit code.
	Wait() int32
	// Close closes the PTY session.
	Close()
}

// NewPTYSession is not supported on Windows.
func (e *Executor) NewPTYSession(ctx context.Context, meta *ShellMeta) (PTYSessionInterface, error) {
	return nil, fmt.Errorf("PTY sessions are not supported on Windows")
}
