package shell

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"syscall"
	"time"

	"github.com/postalsys/muti-metroo/internal/crypto"
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
	StreamID      uint64
	PeerID        identity.AgentID
	RequestID     uint64
	IsInteractive bool                // true for TTY mode
	Meta          *ShellMeta          // Metadata after first frame
	MetaReceived  bool                // true after metadata frame received
	Session       *Session            // Command session (normal mode)
	PTYSession    PTYSessionInterface // PTY session (interactive mode)
	Closed        bool
	StartTime     time.Time
	sessionKey    *crypto.SessionKey // E2E encryption session key
	mu            sync.Mutex
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

// HandleStreamOpen handles a new shell stream open request.
// Returns error code and local ephemeral public key for E2E encryption.
func (h *Handler) HandleStreamOpen(peerID identity.AgentID, streamID uint64, requestID uint64, interactive bool, remoteEphemeralPub [crypto.KeySize]byte) (uint16, [crypto.KeySize]byte) {
	h.logger.Debug("shell stream open",
		logging.KeyPeerID, peerID.ShortString(),
		logging.KeyStreamID, streamID,
		logging.KeyRequestID, requestID,
		"interactive", interactive)

	var zeroKey [crypto.KeySize]byte

	// Check if shell is enabled
	if h.executor == nil || !h.executor.config.Enabled {
		return protocol.ErrShellDisabled, zeroKey
	}

	// Check for zero key (strict encryption mode - reject unencrypted connections)
	if remoteEphemeralPub == zeroKey {
		h.logger.Warn("rejecting shell stream with zero ephemeral key (encryption required)",
			logging.KeyPeerID, peerID.ShortString(),
			logging.KeyStreamID, streamID)
		return protocol.ErrGeneralFailure, zeroKey
	}

	// Generate ephemeral keypair for E2E encryption
	ephPriv, ephPub, err := crypto.GenerateEphemeralKeypair()
	if err != nil {
		h.logger.Error("failed to generate ephemeral keypair",
			logging.KeyError, err)
		return protocol.ErrGeneralFailure, zeroKey
	}

	// Compute shared secret from ECDH
	sharedSecret, err := crypto.ComputeECDH(ephPriv, remoteEphemeralPub)
	if err != nil {
		crypto.ZeroKey(&ephPriv)
		h.logger.Error("failed to compute ECDH shared secret",
			logging.KeyError, err)
		return protocol.ErrGeneralFailure, zeroKey
	}

	// Zero out ephemeral private key after computing shared secret
	crypto.ZeroKey(&ephPriv)

	// Derive session key - we are the responder (shell target)
	// Use requestID (not streamID) because streamID changes at each relay hop
	sessionKey := crypto.DeriveSessionKey(sharedSecret, requestID, remoteEphemeralPub, ephPub, false)
	crypto.ZeroKey(&sharedSecret)

	// Create shell stream entry with session key
	ss := &ShellStream{
		StreamID:      streamID,
		PeerID:        peerID,
		RequestID:     requestID,
		IsInteractive: interactive,
		MetaReceived:  false,
		StartTime:     time.Now(),
		sessionKey:    sessionKey,
	}

	h.mu.Lock()
	h.streams[streamID] = ss
	h.mu.Unlock()

	return 0, ephPub // Success with our ephemeral public key
}

// HandleStreamData processes data for a shell stream.
func (h *Handler) HandleStreamData(peerID identity.AgentID, streamID uint64, data []byte, flags uint8) {
	h.mu.RLock()
	ss := h.streams[streamID]
	h.mu.RUnlock()

	if ss == nil || ss.Closed {
		return
	}

	// Decrypt data using E2E session key
	if ss.sessionKey == nil {
		h.logger.Error("no session key for shell stream",
			logging.KeyStreamID, streamID)
		return
	}

	plaintext, err := ss.sessionKey.Decrypt(data)
	if err != nil {
		h.logger.Error("failed to decrypt shell data",
			logging.KeyStreamID, streamID,
			logging.KeyError, err)
		return
	}

	// First data frame contains metadata
	if !ss.MetaReceived {
		h.handleMetadata(ss, plaintext)
		return
	}

	// Subsequent frames are messages
	h.handleMessage(ss, plaintext, flags)
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

	// Helper to send error and close stream
	fail := func(msg string) {
		h.sendError(ss, msg)
		h.closeStream(ss)
	}

	msgType, payload, err := DecodeMessage(data)
	if err != nil {
		fail("invalid message format")
		return
	}

	if msgType != MsgMeta {
		fail("expected META message")
		return
	}

	meta, err := DecodeMeta(payload)
	if err != nil {
		fail("invalid metadata: " + err.Error())
		return
	}

	ss.Meta = meta
	ss.MetaReceived = true

	ctx := context.Background()

	if ss.IsInteractive && meta.TTY != nil {
		ptySession, err := h.executor.NewPTYSession(ctx, meta)
		if err != nil {
			fail("failed to start PTY session: " + err.Error())
			return
		}

		ss.PTYSession = ptySession
		h.sendAck(ss, true, "")
		go h.pumpPTYOutput(ss)
		return
	}

	// Streaming session
	session, err := h.executor.NewSession(ctx, meta)
	if err != nil {
		fail("failed to start session: " + err.Error())
		return
	}

	if err := session.Start(); err != nil {
		h.executor.ReleaseSession()
		fail("failed to start command: " + err.Error())
		return
	}

	ss.Session = session
	h.sendAck(ss, true, "")
	go h.pumpStdout(ss)
	go h.pumpStderr(ss)
	go h.waitForExit(ss)
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

// writeEncrypted encrypts data and sends it via the stream.
func (h *Handler) writeEncrypted(ss *ShellStream, data []byte, flags uint8) error {
	if ss.sessionKey == nil {
		return nil // Should not happen, but handle gracefully
	}

	ciphertext, err := ss.sessionKey.Encrypt(data)
	if err != nil {
		h.logger.Error("failed to encrypt shell data",
			logging.KeyStreamID, ss.StreamID,
			logging.KeyError, err)
		return err
	}

	return h.writer.WriteStreamData(ss.PeerID, ss.StreamID, ciphertext, flags)
}

// pumpOutput reads from a reader and sends encoded messages to the client.
// The encoder function determines the message type (stdout or stderr).
func (h *Handler) pumpOutput(ss *ShellStream, getReader func() io.Reader, encode func([]byte) []byte) {
	buf := make([]byte, 16*1024) // 16KB buffer
	for {
		ss.mu.Lock()
		session := ss.Session
		ss.mu.Unlock()

		if session == nil {
			return
		}

		reader := getReader()
		if reader == nil {
			return
		}

		n, err := reader.Read(buf)
		if n > 0 {
			h.writeEncrypted(ss, encode(buf[:n]), 0)
		}
		if err != nil {
			return
		}
	}
}

// pumpStdout reads stdout and sends it to the client.
func (h *Handler) pumpStdout(ss *ShellStream) {
	h.pumpOutput(ss, func() io.Reader {
		if ss.Session != nil {
			return ss.Session.Stdout()
		}
		return nil
	}, EncodeStdout)
}

// pumpStderr reads stderr and sends it to the client.
func (h *Handler) pumpStderr(ss *ShellStream) {
	h.pumpOutput(ss, func() io.Reader {
		if ss.Session != nil {
			return ss.Session.Stderr()
		}
		return nil
	}, EncodeStderr)
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
			h.writeEncrypted(ss, msg, 0)
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
	h.logger.Debug("waitForExit started", logging.KeyStreamID, ss.StreamID)

	ss.mu.Lock()
	session := ss.Session
	ss.mu.Unlock()

	if session == nil {
		h.logger.Debug("waitForExit: session is nil", logging.KeyStreamID, ss.StreamID)
		return
	}

	h.logger.Debug("waitForExit: waiting for session.Done()", logging.KeyStreamID, ss.StreamID)
	<-session.Done()
	exitCode := session.ExitCode()
	h.logger.Debug("waitForExit: session done", logging.KeyStreamID, ss.StreamID, "exit_code", exitCode)

	h.sendExit(ss, exitCode)
	h.closeStream(ss)
	h.logger.Debug("waitForExit: stream closed", logging.KeyStreamID, ss.StreamID)
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
	h.writeEncrypted(ss, data, 0)
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
	h.writeEncrypted(ss, data, 0)
}

// sendExit sends an exit code message.
func (h *Handler) sendExit(ss *ShellStream, exitCode int32) {
	data := EncodeExit(exitCode)
	h.writeEncrypted(ss, data, 0)
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
