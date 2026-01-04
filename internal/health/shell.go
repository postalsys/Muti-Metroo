package health

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"

	"nhooyr.io/websocket"

	"github.com/postalsys/muti-metroo/internal/identity"
	"github.com/postalsys/muti-metroo/internal/shell"
)

// ShellProvider provides shell session functionality.
type ShellProvider interface {
	// OpenShellStream opens a shell stream to a remote agent.
	// Returns a ShellSession that can be used to communicate with the remote shell.
	OpenShellStream(ctx context.Context, targetID identity.AgentID, meta *shell.ShellMeta, interactive bool) (*ShellSession, error)
}

// ShellSession represents an active shell session with a remote agent.
type ShellSession struct {
	StreamID uint64
	TargetID identity.AgentID

	// Channels for bidirectional communication
	Send    chan []byte // Send data to remote
	Receive chan []byte // Receive data from remote

	// Done channel closes when session ends
	Done     chan struct{}
	ExitCode int32
	Error    error

	// Cleanup function
	Close func()

	mu     sync.Mutex
	closed bool
}

// handleShellWebSocket handles WebSocket connections for shell sessions.
// GET /agents/{agent-id}/shell?mode=stream|tty
func (s *Server) handleShellWebSocket(w http.ResponseWriter, r *http.Request, targetID identity.AgentID) {
	if s.shellProvider == nil {
		http.Error(w, "shell not available", http.StatusServiceUnavailable)
		return
	}

	// Parse mode from query params
	mode := r.URL.Query().Get("mode")
	interactive := mode != "stream" // Default to interactive (TTY)

	// Accept WebSocket connection
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		Subprotocols: []string{"muti-shell"},
	})
	if err != nil {
		http.Error(w, "failed to accept websocket: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	ctx := r.Context()

	// Read initial metadata from client
	_, metaData, err := conn.Read(ctx)
	if err != nil {
		conn.Close(websocket.StatusProtocolError, "failed to read metadata")
		return
	}

	// Parse metadata
	var meta shell.ShellMeta
	if err := json.Unmarshal(metaData, &meta); err != nil {
		conn.Close(websocket.StatusProtocolError, "invalid metadata: "+err.Error())
		return
	}

	// Open shell stream to target agent
	session, err := s.shellProvider.OpenShellStream(ctx, targetID, &meta, interactive)
	if err != nil {
		// Send error response
		errResp := shell.ShellError{Message: err.Error()}
		errData, _ := json.Marshal(errResp)
		conn.Write(ctx, websocket.MessageBinary, shell.EncodeMessage(shell.MsgError, errData))
		conn.Close(websocket.StatusInternalError, "failed to open shell")
		return
	}
	defer session.Close()

	// Send ACK to client
	ackData, _ := shell.EncodeAck(&shell.ShellAck{Success: true})
	if err := conn.Write(ctx, websocket.MessageBinary, ackData); err != nil {
		return
	}

	// Create context that cancels when either side closes
	sessionCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup

	// Goroutine: WebSocket -> Remote Shell (stdin)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel()
		for {
			select {
			case <-sessionCtx.Done():
				return
			default:
			}

			_, data, err := conn.Read(sessionCtx)
			if err != nil {
				return
			}

			// Forward to shell session
			select {
			case session.Send <- data:
			case <-sessionCtx.Done():
				return
			case <-session.Done:
				return
			}
		}
	}()

	// Goroutine: Remote Shell -> WebSocket (stdout/stderr)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel()
		for {
			select {
			case <-sessionCtx.Done():
				return
			case data, ok := <-session.Receive:
				if !ok {
					return
				}
				if err := conn.Write(sessionCtx, websocket.MessageBinary, data); err != nil {
					return
				}
			case <-session.Done:
				// Send exit code
				exitData := shell.EncodeExit(session.ExitCode)
				conn.Write(sessionCtx, websocket.MessageBinary, exitData)
				return
			}
		}
	}()

	// Wait for session to complete
	select {
	case <-session.Done:
	case <-sessionCtx.Done():
	}

	// Wait for goroutines with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}
}

// SetShellProvider sets the shell session provider.
func (s *Server) SetShellProvider(provider ShellProvider) {
	s.shellProvider = provider
}

// ShellStreamAdapter adapts the mesh shell stream to the ShellSession interface.
// This is implemented by the agent.
type ShellStreamAdapter struct {
	streamID uint64
	targetID identity.AgentID
	send     chan []byte
	receive  chan []byte
	done     chan struct{}
	exitCode int32
	err      error
	closeFunc func()
	mu       sync.Mutex
}

// NewShellStreamAdapter creates a new shell stream adapter.
func NewShellStreamAdapter(streamID uint64, targetID identity.AgentID, closeFunc func()) *ShellStreamAdapter {
	return &ShellStreamAdapter{
		streamID:  streamID,
		targetID:  targetID,
		send:      make(chan []byte, 64),
		receive:   make(chan []byte, 64),
		done:      make(chan struct{}),
		closeFunc: closeFunc,
	}
}

// ToSession converts the adapter to a ShellSession.
func (a *ShellStreamAdapter) ToSession() *ShellSession {
	return &ShellSession{
		StreamID: a.streamID,
		TargetID: a.targetID,
		Send:     a.send,
		Receive:  a.receive,
		Done:     a.done,
		Close: func() {
			a.Close()
		},
	}
}

// PushReceive pushes data to the receive channel (called by stream handler).
func (a *ShellStreamAdapter) PushReceive(data []byte) {
	select {
	case a.receive <- data:
	case <-a.done:
	default:
		// Drop if buffer full
	}
}

// PopSend pops data from the send channel (called by stream handler).
func (a *ShellStreamAdapter) PopSend() ([]byte, bool) {
	select {
	case data := <-a.send:
		return data, true
	case <-a.done:
		return nil, false
	}
}

// SetExitCode sets the exit code when the remote process exits.
func (a *ShellStreamAdapter) SetExitCode(code int32) {
	a.mu.Lock()
	a.exitCode = code
	a.mu.Unlock()
}

// SetError sets an error.
func (a *ShellStreamAdapter) SetError(err error) {
	a.mu.Lock()
	a.err = err
	a.mu.Unlock()
}

// Close closes the adapter.
func (a *ShellStreamAdapter) Close() {
	a.mu.Lock()
	defer a.mu.Unlock()
	select {
	case <-a.done:
		return
	default:
		close(a.done)
		if a.closeFunc != nil {
			a.closeFunc()
		}
	}
}

// Read implements io.Reader for the receive channel.
func (a *ShellStreamAdapter) Read(p []byte) (n int, err error) {
	select {
	case data, ok := <-a.receive:
		if !ok {
			return 0, io.EOF
		}
		n = copy(p, data)
		return n, nil
	case <-a.done:
		return 0, io.EOF
	}
}

// Write implements io.Writer for the send channel.
func (a *ShellStreamAdapter) Write(p []byte) (n int, err error) {
	data := make([]byte, len(p))
	copy(data, p)
	select {
	case a.send <- data:
		return len(p), nil
	case <-a.done:
		return 0, io.ErrClosedPipe
	}
}
