package shell

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"syscall"
	"time"

	"github.com/postalsys/muti-metroo/internal/identity"
	"github.com/postalsys/muti-metroo/internal/logging"
	"github.com/postalsys/muti-metroo/internal/protocol"
)

// DataWriter is the interface for sending data to a stream.
type DataWriter interface {
	WriteStreamData(peerID identity.AgentID, streamID uint64, data []byte, flags uint8) error
	WriteStreamClose(peerID identity.AgentID, streamID uint64) error
}

// ShellStream tracks an active shell stream.
type ShellStream struct {
	StreamID     uint64
	PeerID       identity.AgentID
	RequestID    uint64
	IsInteractive bool              // true for TTY mode
	Meta         *ShellMeta        // Metadata after first frame
	MetaReceived bool              // true after metadata frame received
	Session      *Session          // Command session (streaming mode)
	PTYSession   PTYSessionInterface // PTY session (interactive mode)
	Closed       bool
	StartTime    time.Time
	mu           sync.Mutex
}

// Handler handles shell stream operations.
type Handler struct {
	executor *Executor
	writer   DataWriter
	logger   *slog.Logger
	streams  map[uint64]*ShellStream
	mu       sync.RWMutex
}

// NewHandler creates a new shell handler.
func NewHandler(executor *Executor, writer DataWriter, logger *slog.Logger) *Handler {
	return &Handler{
		executor: executor,
		writer:   writer,
		logger:   logger,
		streams:  make(map[uint64]*ShellStream),
	}
}

// HandleStreamOpen handles a shell stream open request.
// Returns the error code to send, or 0 if successful.
func (h *Handler) HandleStreamOpen(peerID identity.AgentID, streamID uint64, requestID uint64, interactive bool) uint16 {
	h.logger.Debug("shell stream open",
		logging.KeyPeerID, peerID.ShortString(),
		logging.KeyStreamID, streamID,
		logging.KeyRequestID, requestID,
		"interactive", interactive)

	// Check if shell is enabled
	if h.executor == nil || !h.executor.config.Enabled {
		return protocol.ErrShellDisabled
	}

	// Check if mode is enabled
	if interactive && !h.executor.config.Interactive.Enabled {
		return protocol.ErrShellDisabled
	}
	if !interactive && !h.executor.config.Streaming.Enabled {
		return protocol.ErrShellDisabled
	}

	// Create shell stream entry
	ss := &ShellStream{
		StreamID:      streamID,
		PeerID:        peerID,
		RequestID:     requestID,
		IsInteractive: interactive,
		MetaReceived:  false,
		StartTime:     time.Now(),
	}

	h.mu.Lock()
	h.streams[streamID] = ss
	h.mu.Unlock()

	return 0 // Success
}

// HandleStreamData processes data for a shell stream.
func (h *Handler) HandleStreamData(peerID identity.AgentID, streamID uint64, data []byte, flags uint8) {
	h.mu.RLock()
	ss := h.streams[streamID]
	h.mu.RUnlock()

	if ss == nil || ss.Closed {
		return
	}

	// First data frame contains metadata
	if !ss.MetaReceived {
		h.handleMetadata(ss, data)
		return
	}

	// Subsequent frames are messages
	h.handleMessage(ss, data, flags)
}

// HandleStreamClose handles stream close.
func (h *Handler) HandleStreamClose(streamID uint64) {
	h.mu.Lock()
	ss := h.streams[streamID]
	if ss != nil {
		ss.Closed = true
		delete(h.streams, streamID)
	}
	h.mu.Unlock()

	if ss == nil {
		return
	}

	h.logger.Debug("shell stream closed",
		logging.KeyStreamID, streamID,
		"duration", time.Since(ss.StartTime))

	// Clean up session
	ss.mu.Lock()
	if ss.Session != nil {
		ss.Session.Close()
		h.executor.ReleaseSession()
	}
	if ss.PTYSession != nil {
		ss.PTYSession.Close()
		h.executor.ReleaseSession()
	}
	ss.mu.Unlock()
}

// handleMetadata processes the first data frame containing metadata.
func (h *Handler) handleMetadata(ss *ShellStream, data []byte) {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	// Decode message type and payload
	msgType, payload, err := DecodeMessage(data)
	if err != nil {
		h.sendError(ss, "invalid message format")
		h.closeStream(ss)
		return
	}

	if msgType != MsgMeta {
		h.sendError(ss, "expected META message")
		h.closeStream(ss)
		return
	}

	meta, err := DecodeMeta(payload)
	if err != nil {
		h.sendError(ss, "invalid metadata: "+err.Error())
		h.closeStream(ss)
		return
	}

	ss.Meta = meta
	ss.MetaReceived = true

	// Start the session
	ctx := context.Background()

	if ss.IsInteractive && meta.TTY != nil {
		// Start PTY session
		ptySession, err := h.executor.NewPTYSession(ctx, meta)
		if err != nil {
			h.sendError(ss, "failed to start PTY session: "+err.Error())
			h.closeStream(ss)
			return
		}

		ss.PTYSession = ptySession

		// Send ACK
		h.sendAck(ss, true, "")

		// Start I/O goroutines for PTY
		go h.pumpPTYOutput(ss)
	} else {
		// Start streaming session
		session, err := h.executor.NewSession(ctx, meta)
		if err != nil {
			h.sendError(ss, "failed to start session: "+err.Error())
			h.closeStream(ss)
			return
		}

		if err := session.Start(); err != nil {
			h.executor.ReleaseSession()
			h.sendError(ss, "failed to start command: "+err.Error())
			h.closeStream(ss)
			return
		}

		ss.Session = session

		// Send ACK
		h.sendAck(ss, true, "")

		// Start I/O goroutines for streaming mode
		go h.pumpStdout(ss)
		go h.pumpStderr(ss)
		go h.waitForExit(ss)
	}
}

// handleMessage processes subsequent messages after metadata.
func (h *Handler) handleMessage(ss *ShellStream, data []byte, flags uint8) {
	if len(data) == 0 {
		return
	}

	msgType, payload, err := DecodeMessage(data)
	if err != nil {
		h.logger.Debug("invalid shell message", logging.KeyStreamID, ss.StreamID)
		return
	}

	switch msgType {
	case MsgStdin:
		h.handleStdin(ss, payload)
	case MsgResize:
		h.handleResize(ss, payload)
	case MsgSignal:
		h.handleSignal(ss, payload)
	default:
		h.logger.Debug("unexpected shell message type",
			logging.KeyStreamID, ss.StreamID,
			"type", MsgTypeName(msgType))
	}
}

// handleStdin writes stdin data to the session.
func (h *Handler) handleStdin(ss *ShellStream, data []byte) {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	if ss.PTYSession != nil {
		ss.PTYSession.Write(data)
	} else if ss.Session != nil {
		ss.Session.Stdin().Write(data)
	}
}

// handleResize handles terminal resize.
func (h *Handler) handleResize(ss *ShellStream, data []byte) {
	rows, cols, err := DecodeResize(data)
	if err != nil {
		h.logger.Debug("invalid resize message", logging.KeyStreamID, ss.StreamID)
		return
	}

	ss.mu.Lock()
	defer ss.mu.Unlock()

	if ss.PTYSession != nil {
		ss.PTYSession.Resize(rows, cols)
	}
}

// handleSignal handles signal delivery.
func (h *Handler) handleSignal(ss *ShellStream, data []byte) {
	signum, err := DecodeSignal(data)
	if err != nil {
		h.logger.Debug("invalid signal message", logging.KeyStreamID, ss.StreamID)
		return
	}

	ss.mu.Lock()
	defer ss.mu.Unlock()

	if ss.PTYSession != nil {
		ss.PTYSession.Signal(syscall.Signal(signum))
	} else if ss.Session != nil {
		ss.Session.Signal(syscall.Signal(signum))
	}
}

// pumpStdout reads stdout and sends it to the client.
func (h *Handler) pumpStdout(ss *ShellStream) {
	buf := make([]byte, 16*1024) // 16KB buffer
	for {
		ss.mu.Lock()
		session := ss.Session
		ss.mu.Unlock()

		if session == nil {
			return
		}

		n, err := session.Stdout().Read(buf)
		if n > 0 {
			msg := EncodeStdout(buf[:n])
			h.writer.WriteStreamData(ss.PeerID, ss.StreamID, msg, 0)
		}
		if err != nil {
			return
		}
	}
}

// pumpStderr reads stderr and sends it to the client.
func (h *Handler) pumpStderr(ss *ShellStream) {
	buf := make([]byte, 16*1024) // 16KB buffer
	for {
		ss.mu.Lock()
		session := ss.Session
		ss.mu.Unlock()

		if session == nil {
			return
		}

		n, err := session.Stderr().Read(buf)
		if n > 0 {
			msg := EncodeStderr(buf[:n])
			h.writer.WriteStreamData(ss.PeerID, ss.StreamID, msg, 0)
		}
		if err != nil {
			return
		}
	}
}

// pumpPTYOutput reads PTY output and sends it to the client.
func (h *Handler) pumpPTYOutput(ss *ShellStream) {
	buf := make([]byte, 16*1024) // 16KB buffer
	for {
		ss.mu.Lock()
		ptySession := ss.PTYSession
		ss.mu.Unlock()

		if ptySession == nil {
			return
		}

		n, err := ptySession.Read(buf)
		if n > 0 {
			// PTY output goes to stdout
			msg := EncodeStdout(buf[:n])
			h.writer.WriteStreamData(ss.PeerID, ss.StreamID, msg, 0)
		}
		if err != nil {
			if err != io.EOF {
				h.logger.Debug("PTY read error",
					logging.KeyStreamID, ss.StreamID,
					logging.KeyError, err)
			}
			break
		}
	}

	// Wait for process to exit
	ss.mu.Lock()
	ptySession := ss.PTYSession
	ss.mu.Unlock()

	if ptySession != nil {
		exitCode := ptySession.Wait()
		h.sendExit(ss, exitCode)
	}

	h.closeStream(ss)
}

// waitForExit waits for the session to exit and sends the exit code.
func (h *Handler) waitForExit(ss *ShellStream) {
	ss.mu.Lock()
	session := ss.Session
	ss.mu.Unlock()

	if session == nil {
		return
	}

	<-session.Done()
	exitCode := session.ExitCode()

	h.sendExit(ss, exitCode)
	h.closeStream(ss)
}

// sendAck sends an ACK message.
func (h *Handler) sendAck(ss *ShellStream, success bool, errMsg string) {
	ack := &ShellAck{
		Success: success,
		Error:   errMsg,
	}
	data, err := EncodeAck(ack)
	if err != nil {
		h.logger.Error("failed to encode ack", logging.KeyError, err)
		return
	}
	h.writer.WriteStreamData(ss.PeerID, ss.StreamID, data, 0)
}

// sendError sends an error message.
func (h *Handler) sendError(ss *ShellStream, errMsg string) {
	shellErr := &ShellError{
		Message: errMsg,
	}
	data, err := EncodeError(shellErr)
	if err != nil {
		h.logger.Error("failed to encode error", logging.KeyError, err)
		return
	}
	h.writer.WriteStreamData(ss.PeerID, ss.StreamID, data, 0)
}

// sendExit sends an exit code message.
func (h *Handler) sendExit(ss *ShellStream, exitCode int32) {
	data := EncodeExit(exitCode)
	h.writer.WriteStreamData(ss.PeerID, ss.StreamID, data, 0)
}

// closeStream closes the stream.
func (h *Handler) closeStream(ss *ShellStream) {
	h.mu.Lock()
	if !ss.Closed {
		ss.Closed = true
		delete(h.streams, ss.StreamID)
	}
	h.mu.Unlock()

	h.writer.WriteStreamClose(ss.PeerID, ss.StreamID)
}

// ActiveStreams returns the number of active shell streams.
func (h *Handler) ActiveStreams() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.streams)
}

// Close closes all active streams.
func (h *Handler) Close() {
	h.mu.Lock()
	streams := make([]*ShellStream, 0, len(h.streams))
	for _, ss := range h.streams {
		streams = append(streams, ss)
	}
	h.mu.Unlock()

	for _, ss := range streams {
		h.HandleStreamClose(ss.StreamID)
	}
}
