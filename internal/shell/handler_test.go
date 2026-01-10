package shell

import (
	"log/slog"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/postalsys/muti-metroo/internal/crypto"
	"github.com/postalsys/muti-metroo/internal/identity"
)

// testEphemeralKey generates a test ephemeral key pair for testing.
func testEphemeralKey(t *testing.T) [crypto.KeySize]byte {
	t.Helper()
	_, pub, err := crypto.GenerateEphemeralKeypair()
	if err != nil {
		t.Fatalf("failed to generate ephemeral key: %v", err)
	}
	return pub
}

// openStreamWithSessionKey opens a stream and returns the session key for encrypting test data.
func openStreamWithSessionKey(t *testing.T, handler *Handler, peerID identity.AgentID, streamID, requestID uint64, interactive bool) *crypto.SessionKey {
	t.Helper()
	// Generate client ephemeral keypair
	clientPriv, clientPub, err := crypto.GenerateEphemeralKeypair()
	if err != nil {
		t.Fatalf("failed to generate client ephemeral key: %v", err)
	}

	// Open stream and get handler's ephemeral public key
	errCode, handlerPub := handler.HandleStreamOpen(peerID, streamID, requestID, interactive, clientPub)
	if errCode != 0 {
		t.Fatalf("HandleStreamOpen() returned error code %d", errCode)
	}

	// Derive session key (as initiator)
	sharedSecret, err := crypto.ComputeECDH(clientPriv, handlerPub)
	if err != nil {
		t.Fatalf("failed to compute ECDH: %v", err)
	}
	crypto.ZeroKey(&clientPriv)

	sessionKey := crypto.DeriveSessionKey(sharedSecret, requestID, clientPub, handlerPub, true)
	crypto.ZeroKey(&sharedSecret)

	return sessionKey
}

// mockDataWriter is a mock implementation of DataWriter for testing.
type mockDataWriter struct {
	mu       sync.Mutex
	messages []mockMessage
	closed   map[uint64]bool
}

type mockMessage struct {
	peerID   identity.AgentID
	streamID uint64
	data     []byte
	flags    uint8
}

func newMockDataWriter() *mockDataWriter {
	return &mockDataWriter{
		messages: make([]mockMessage, 0),
		closed:   make(map[uint64]bool),
	}
}

func (m *mockDataWriter) WriteStreamData(peerID identity.AgentID, streamID uint64, data []byte, flags uint8) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	m.messages = append(m.messages, mockMessage{peerID, streamID, dataCopy, flags})
	return nil
}

func (m *mockDataWriter) WriteStreamClose(peerID identity.AgentID, streamID uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed[streamID] = true
	return nil
}

func (m *mockDataWriter) getMessages() []mockMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]mockMessage, len(m.messages))
	copy(result, m.messages)
	return result
}

func (m *mockDataWriter) isClosed(streamID uint64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed[streamID]
}

func mustNewAgentID(t *testing.T) identity.AgentID {
	t.Helper()
	id, err := identity.NewAgentID()
	if err != nil {
		t.Fatalf("NewAgentID() error = %v", err)
	}
	return id
}

func TestHandler_HandleStreamOpen_Disabled(t *testing.T) {
	writer := newMockDataWriter()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	exec := NewExecutor(Config{
		Enabled: false, // Shell disabled
	})

	handler := NewHandler(exec, writer, logger)

	peerID := mustNewAgentID(t)
	streamID := uint64(1)
	requestID := uint64(1)

	// Open should fail with shell disabled error
	ephKey := testEphemeralKey(t)
	errCode, _ := handler.HandleStreamOpen(peerID, streamID, requestID, false, ephKey)
	if errCode == 0 {
		t.Error("HandleStreamOpen() should have returned error code for disabled shell")
	}
}

func TestHandler_HandleStreamOpen_Success_Streaming(t *testing.T) {
	writer := newMockDataWriter()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	exec := NewExecutor(Config{
		Enabled:     true,
		MaxSessions: 10,
		Whitelist:   []string{"*"},
	})

	handler := NewHandler(exec, writer, logger)

	peerID := mustNewAgentID(t)
	streamID := uint64(1)
	requestID := uint64(1)

	// Open should succeed (streaming mode)
	ephKey := testEphemeralKey(t)
	errCode, _ := handler.HandleStreamOpen(peerID, streamID, requestID, false, ephKey)
	if errCode != 0 {
		t.Errorf("HandleStreamOpen() returned error code %d, want 0", errCode)
	}

	// Verify stream is tracked
	if handler.ActiveStreams() != 1 {
		t.Errorf("ActiveStreams() = %d, want 1", handler.ActiveStreams())
	}

	// Close the stream
	handler.HandleStreamClose(streamID)

	// Verify stream is removed
	if handler.ActiveStreams() != 0 {
		t.Errorf("ActiveStreams() after close = %d, want 0", handler.ActiveStreams())
	}
}

func TestHandler_HandleStreamOpen_Success_Interactive(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping interactive test on Windows (no PTY support)")
	}

	writer := newMockDataWriter()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	exec := NewExecutor(Config{
		Enabled:     true,
		MaxSessions: 10,
		Whitelist:   []string{"*"},
	})

	handler := NewHandler(exec, writer, logger)

	peerID := mustNewAgentID(t)
	streamID := uint64(1)
	requestID := uint64(1)

	// Open should succeed (interactive mode)
	ephKey := testEphemeralKey(t)
	errCode, _ := handler.HandleStreamOpen(peerID, streamID, requestID, true, ephKey)
	if errCode != 0 {
		t.Errorf("HandleStreamOpen() interactive returned error code %d, want 0", errCode)
	}

	// Verify stream is tracked
	if handler.ActiveStreams() != 1 {
		t.Errorf("ActiveStreams() = %d, want 1", handler.ActiveStreams())
	}

	// Close the stream
	handler.HandleStreamClose(streamID)
}

func TestHandler_StreamSessionFlow(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping streaming session test on Windows")
	}

	writer := newMockDataWriter()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	exec := NewExecutor(Config{
		Enabled:     true,
		MaxSessions: 10,
		Whitelist:   []string{"*"},
		Timeout:     10 * time.Second,
	})

	handler := NewHandler(exec, writer, logger)

	peerID := mustNewAgentID(t)
	streamID := uint64(1)
	requestID := uint64(1)

	// Open stream and get session key for encryption
	sessionKey := openStreamWithSessionKey(t, handler, peerID, streamID, requestID, false)

	// Send metadata - execute echo command (encrypted)
	meta := &ShellMeta{
		Command: "echo",
		Args:    []string{"hello world"},
	}
	metaMsg, _ := EncodeMeta(meta)
	encryptedMeta, err := sessionKey.Encrypt(metaMsg)
	if err != nil {
		t.Fatalf("failed to encrypt metadata: %v", err)
	}
	handler.HandleStreamData(peerID, streamID, encryptedMeta, 0)

	// Wait for ACK message with timeout
	deadline := time.Now().Add(5 * time.Second)
	var messages []mockMessage
	for time.Now().Before(deadline) {
		messages = writer.getMessages()
		if len(messages) > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if len(messages) == 0 {
		t.Fatal("Expected messages from handler")
	}

	// First message should be ACK (decrypt first)
	firstMsg := messages[0]
	decrypted, err := sessionKey.Decrypt(firstMsg.data)
	if err != nil {
		t.Fatalf("Failed to decrypt first message: %v", err)
	}
	msgType, _, err := DecodeMessage(decrypted)
	if err != nil {
		t.Fatalf("DecodeMessage() error = %v", err)
	}
	if msgType != MsgAck {
		t.Errorf("First message type = %d, want MsgAck (%d)", msgType, MsgAck)
	}

	// Wait for stdout and exit messages with timeout
	hasStdout := false
	hasExit := false
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		messages = writer.getMessages()
		for _, msg := range messages {
			decrypted, err := sessionKey.Decrypt(msg.data)
			if err != nil {
				continue // Skip messages we can't decrypt
			}
			msgType, _, _ := DecodeMessage(decrypted)
			if msgType == MsgStdout {
				hasStdout = true
			}
			if msgType == MsgExit {
				hasExit = true
			}
		}
		if hasStdout && hasExit {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if !hasStdout {
		t.Error("Expected STDOUT message")
	}
	if !hasExit {
		t.Error("Expected EXIT message")
	}

	// Close handler
	handler.Close()
}

func TestHandler_HandleMetadataError(t *testing.T) {
	writer := newMockDataWriter()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	exec := NewExecutor(Config{
		Enabled:     true,
		MaxSessions: 10,
		Whitelist:   []string{"ls", "cat"}, // echo not allowed
	})

	handler := NewHandler(exec, writer, logger)

	peerID := mustNewAgentID(t)
	streamID := uint64(1)
	requestID := uint64(1)

	// Open stream and get session key for encryption
	sessionKey := openStreamWithSessionKey(t, handler, peerID, streamID, requestID, false)

	// Send metadata with non-whitelisted command (encrypted)
	meta := &ShellMeta{
		Command: "echo", // Not in whitelist
		Args:    []string{"hello"},
	}
	metaMsg, _ := EncodeMeta(meta)
	encryptedMeta, err := sessionKey.Encrypt(metaMsg)
	if err != nil {
		t.Fatalf("failed to encrypt metadata: %v", err)
	}
	handler.HandleStreamData(peerID, streamID, encryptedMeta, 0)

	// Wait for response
	time.Sleep(100 * time.Millisecond)

	// Check for error message (decrypt first)
	messages := writer.getMessages()
	hasError := false
	for _, msg := range messages {
		decrypted, err := sessionKey.Decrypt(msg.data)
		if err != nil {
			continue // Skip messages we can't decrypt
		}
		msgType, _, _ := DecodeMessage(decrypted)
		if msgType == MsgError {
			hasError = true
		}
	}

	if !hasError {
		t.Error("Expected ERROR message for non-whitelisted command")
	}

	// Stream should be closed
	if !writer.isClosed(streamID) {
		t.Error("Stream should be closed after error")
	}
}

func TestHandler_Close(t *testing.T) {
	writer := newMockDataWriter()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	exec := NewExecutor(Config{
		Enabled:     true,
		MaxSessions: 10,
		Whitelist:   []string{"*"},
	})

	handler := NewHandler(exec, writer, logger)

	// Open multiple streams
	peerID := mustNewAgentID(t)
	ephKey := testEphemeralKey(t)
	for i := uint64(1); i <= 3; i++ {
		handler.HandleStreamOpen(peerID, i, i, false, ephKey)
	}

	if handler.ActiveStreams() != 3 {
		t.Errorf("ActiveStreams() = %d, want 3", handler.ActiveStreams())
	}

	// Close handler - should close all streams
	handler.Close()

	if handler.ActiveStreams() != 0 {
		t.Errorf("ActiveStreams() after Close = %d, want 0", handler.ActiveStreams())
	}
}

func TestHandler_HandleStreamOpen_ZeroEphemeralKey(t *testing.T) {
	writer := newMockDataWriter()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	exec := NewExecutor(Config{
		Enabled:     true,
		MaxSessions: 10,
		Whitelist:   []string{"*"},
	})

	handler := NewHandler(exec, writer, logger)

	peerID := mustNewAgentID(t)
	streamID := uint64(1)
	requestID := uint64(1)

	// Zero ephemeral key should be rejected
	var zeroKey [crypto.KeySize]byte
	errCode, _ := handler.HandleStreamOpen(peerID, streamID, requestID, false, zeroKey)
	if errCode == 0 {
		t.Error("HandleStreamOpen() should reject zero ephemeral key")
	}

	// Stream should not be tracked
	if handler.ActiveStreams() != 0 {
		t.Errorf("ActiveStreams() = %d, want 0", handler.ActiveStreams())
	}
}

func TestHandler_HandleStreamOpen_NilExecutor(t *testing.T) {
	writer := newMockDataWriter()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Handler with nil executor
	handler := NewHandler(nil, writer, logger)

	peerID := mustNewAgentID(t)
	streamID := uint64(1)
	requestID := uint64(1)
	ephKey := testEphemeralKey(t)

	// Should return error because executor is nil
	errCode, _ := handler.HandleStreamOpen(peerID, streamID, requestID, false, ephKey)
	if errCode == 0 {
		t.Error("HandleStreamOpen() should fail with nil executor")
	}
}

func TestHandler_HandleStreamData_NoStream(t *testing.T) {
	writer := newMockDataWriter()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	exec := NewExecutor(Config{
		Enabled:     true,
		MaxSessions: 10,
		Whitelist:   []string{"*"},
	})

	handler := NewHandler(exec, writer, logger)

	peerID := mustNewAgentID(t)
	streamID := uint64(999) // Non-existent stream

	// Should not panic or error, just return silently
	handler.HandleStreamData(peerID, streamID, []byte("test data"), 0)

	// No messages should be written
	if len(writer.getMessages()) != 0 {
		t.Errorf("Expected no messages for non-existent stream, got %d", len(writer.getMessages()))
	}
}

func TestHandler_HandleStreamClose_NonExistent(t *testing.T) {
	writer := newMockDataWriter()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	exec := NewExecutor(Config{
		Enabled:     true,
		MaxSessions: 10,
		Whitelist:   []string{"*"},
	})

	handler := NewHandler(exec, writer, logger)

	// Close non-existent stream should not panic
	handler.HandleStreamClose(uint64(999))

	// ActiveStreams should be 0
	if handler.ActiveStreams() != 0 {
		t.Errorf("ActiveStreams() = %d, want 0", handler.ActiveStreams())
	}
}

func TestHandler_HandleStreamClose_Twice(t *testing.T) {
	writer := newMockDataWriter()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	exec := NewExecutor(Config{
		Enabled:     true,
		MaxSessions: 10,
		Whitelist:   []string{"*"},
	})

	handler := NewHandler(exec, writer, logger)

	peerID := mustNewAgentID(t)
	streamID := uint64(1)
	requestID := uint64(1)
	ephKey := testEphemeralKey(t)

	// Open stream
	handler.HandleStreamOpen(peerID, streamID, requestID, false, ephKey)

	if handler.ActiveStreams() != 1 {
		t.Errorf("ActiveStreams() = %d, want 1", handler.ActiveStreams())
	}

	// Close twice should not panic
	handler.HandleStreamClose(streamID)
	handler.HandleStreamClose(streamID)

	if handler.ActiveStreams() != 0 {
		t.Errorf("ActiveStreams() after double close = %d, want 0", handler.ActiveStreams())
	}
}

func TestHandler_HandleStreamData_InvalidMessage(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping streaming session test on Windows")
	}

	writer := newMockDataWriter()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	exec := NewExecutor(Config{
		Enabled:     true,
		MaxSessions: 10,
		Whitelist:   []string{"*"},
	})

	handler := NewHandler(exec, writer, logger)

	peerID := mustNewAgentID(t)
	streamID := uint64(1)
	requestID := uint64(1)

	// Open stream and get session key
	sessionKey := openStreamWithSessionKey(t, handler, peerID, streamID, requestID, false)

	// Send invalid metadata (not proper JSON)
	invalidMeta := EncodeMessage(MsgMeta, []byte("not valid json"))
	encryptedInvalid, err := sessionKey.Encrypt(invalidMeta)
	if err != nil {
		t.Fatalf("failed to encrypt: %v", err)
	}
	handler.HandleStreamData(peerID, streamID, encryptedInvalid, 0)

	// Wait a bit for processing
	time.Sleep(100 * time.Millisecond)

	// Should have sent an error and closed the stream
	if !writer.isClosed(streamID) {
		t.Error("Stream should be closed after invalid metadata")
	}
}

func TestHandler_HandleStreamData_WrongMessageType(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping streaming session test on Windows")
	}

	writer := newMockDataWriter()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	exec := NewExecutor(Config{
		Enabled:     true,
		MaxSessions: 10,
		Whitelist:   []string{"*"},
	})

	handler := NewHandler(exec, writer, logger)

	peerID := mustNewAgentID(t)
	streamID := uint64(1)
	requestID := uint64(1)

	// Open stream and get session key
	sessionKey := openStreamWithSessionKey(t, handler, peerID, streamID, requestID, false)

	// First message should be META, but send STDIN instead
	stdinMsg := EncodeStdin([]byte("wrong first message"))
	encryptedStdin, err := sessionKey.Encrypt(stdinMsg)
	if err != nil {
		t.Fatalf("failed to encrypt: %v", err)
	}
	handler.HandleStreamData(peerID, streamID, encryptedStdin, 0)

	// Wait a bit for processing
	time.Sleep(100 * time.Millisecond)

	// Should have sent an error and closed the stream
	if !writer.isClosed(streamID) {
		t.Error("Stream should be closed when first message is not META")
	}
}

func TestNewHandler(t *testing.T) {
	writer := newMockDataWriter()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	exec := NewExecutor(Config{
		Enabled:     true,
		MaxSessions: 10,
		Whitelist:   []string{"*"},
	})

	handler := NewHandler(exec, writer, logger)

	if handler == nil {
		t.Fatal("NewHandler() returned nil")
	}

	if handler.executor != exec {
		t.Error("executor not set correctly")
	}

	if handler.writer != writer {
		t.Error("writer not set correctly")
	}

	if handler.logger != logger {
		t.Error("logger not set correctly")
	}

	if handler.ActiveStreams() != 0 {
		t.Errorf("initial ActiveStreams() = %d, want 0", handler.ActiveStreams())
	}
}

func TestHandler_HandleMessage_Stdin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows")
	}

	writer := newMockDataWriter()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	exec := NewExecutor(Config{
		Enabled:     true,
		MaxSessions: 10,
		Whitelist:   []string{"*"},
	})

	handler := NewHandler(exec, writer, logger)

	peerID := mustNewAgentID(t)
	streamID := uint64(1)
	requestID := uint64(1)

	// Open stream and get session key
	sessionKey := openStreamWithSessionKey(t, handler, peerID, streamID, requestID, false)

	// Send valid metadata first
	meta := &ShellMeta{
		Command: "cat",
	}
	metaMsg, _ := EncodeMeta(meta)
	encryptedMeta, err := sessionKey.Encrypt(metaMsg)
	if err != nil {
		t.Fatalf("failed to encrypt: %v", err)
	}
	handler.HandleStreamData(peerID, streamID, encryptedMeta, 0)

	// Wait for ACK
	time.Sleep(100 * time.Millisecond)

	// Now send stdin
	stdinData := []byte("test input data")
	stdinMsg := EncodeStdin(stdinData)
	encryptedStdin, err := sessionKey.Encrypt(stdinMsg)
	if err != nil {
		t.Fatalf("failed to encrypt: %v", err)
	}
	handler.HandleStreamData(peerID, streamID, encryptedStdin, 0)

	// Give it time to process
	time.Sleep(100 * time.Millisecond)

	// Clean up
	handler.HandleStreamClose(streamID)
}

func TestHandler_HandleMessage_Resize(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows")
	}

	writer := newMockDataWriter()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	exec := NewExecutor(Config{
		Enabled:     true,
		MaxSessions: 10,
		Whitelist:   []string{"*"},
	})

	handler := NewHandler(exec, writer, logger)

	peerID := mustNewAgentID(t)
	streamID := uint64(1)
	requestID := uint64(1)

	// Open stream and get session key
	sessionKey := openStreamWithSessionKey(t, handler, peerID, streamID, requestID, false)

	// Send valid metadata first
	meta := &ShellMeta{
		Command: "cat",
	}
	metaMsg, _ := EncodeMeta(meta)
	encryptedMeta, err := sessionKey.Encrypt(metaMsg)
	if err != nil {
		t.Fatalf("failed to encrypt: %v", err)
	}
	handler.HandleStreamData(peerID, streamID, encryptedMeta, 0)

	// Wait for ACK
	time.Sleep(100 * time.Millisecond)

	// Now send resize (won't do anything for non-PTY, but tests the handler path)
	resizeMsg := EncodeResize(50, 120)
	encryptedResize, err := sessionKey.Encrypt(resizeMsg)
	if err != nil {
		t.Fatalf("failed to encrypt: %v", err)
	}
	handler.HandleStreamData(peerID, streamID, encryptedResize, 0)

	// Give it time to process
	time.Sleep(50 * time.Millisecond)

	// Clean up
	handler.HandleStreamClose(streamID)
}

func TestHandler_HandleMessage_Signal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows")
	}

	writer := newMockDataWriter()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	exec := NewExecutor(Config{
		Enabled:     true,
		MaxSessions: 10,
		Whitelist:   []string{"*"},
	})

	handler := NewHandler(exec, writer, logger)

	peerID := mustNewAgentID(t)
	streamID := uint64(1)
	requestID := uint64(1)

	// Open stream and get session key
	sessionKey := openStreamWithSessionKey(t, handler, peerID, streamID, requestID, false)

	// Send valid metadata first - use sleep so we can signal it
	meta := &ShellMeta{
		Command: "sleep",
		Args:    []string{"10"},
	}
	metaMsg, _ := EncodeMeta(meta)
	encryptedMeta, err := sessionKey.Encrypt(metaMsg)
	if err != nil {
		t.Fatalf("failed to encrypt: %v", err)
	}
	handler.HandleStreamData(peerID, streamID, encryptedMeta, 0)

	// Wait for session to start
	time.Sleep(200 * time.Millisecond)

	// Now send signal (SIGTERM = 15)
	signalMsg := EncodeSignal(15)
	encryptedSignal, err := sessionKey.Encrypt(signalMsg)
	if err != nil {
		t.Fatalf("failed to encrypt: %v", err)
	}
	handler.HandleStreamData(peerID, streamID, encryptedSignal, 0)

	// Wait for signal to be processed
	time.Sleep(100 * time.Millisecond)

	// Clean up
	handler.HandleStreamClose(streamID)
}

func TestHandler_HandleMessage_UnknownType(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows")
	}

	writer := newMockDataWriter()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	exec := NewExecutor(Config{
		Enabled:     true,
		MaxSessions: 10,
		Whitelist:   []string{"*"},
	})

	handler := NewHandler(exec, writer, logger)

	peerID := mustNewAgentID(t)
	streamID := uint64(1)
	requestID := uint64(1)

	// Open stream and get session key
	sessionKey := openStreamWithSessionKey(t, handler, peerID, streamID, requestID, false)

	// Send valid metadata first
	meta := &ShellMeta{
		Command: "echo",
		Args:    []string{"hello"},
	}
	metaMsg, _ := EncodeMeta(meta)
	encryptedMeta, err := sessionKey.Encrypt(metaMsg)
	if err != nil {
		t.Fatalf("failed to encrypt: %v", err)
	}
	handler.HandleStreamData(peerID, streamID, encryptedMeta, 0)

	// Wait for ACK
	time.Sleep(200 * time.Millisecond)

	// Send an unknown message type (0xFF)
	unknownMsg := EncodeMessage(0xFF, []byte("unknown data"))
	encryptedUnknown, err := sessionKey.Encrypt(unknownMsg)
	if err != nil {
		t.Fatalf("failed to encrypt: %v", err)
	}
	handler.HandleStreamData(peerID, streamID, encryptedUnknown, 0)

	// Should not crash, just log and continue
	time.Sleep(50 * time.Millisecond)

	// Clean up
	handler.HandleStreamClose(streamID)
}

func TestHandler_HandleStreamData_ClosedStream(t *testing.T) {
	writer := newMockDataWriter()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	exec := NewExecutor(Config{
		Enabled:     true,
		MaxSessions: 10,
		Whitelist:   []string{"*"},
	})

	handler := NewHandler(exec, writer, logger)

	peerID := mustNewAgentID(t)
	streamID := uint64(1)
	requestID := uint64(1)

	// Open stream and get session key
	sessionKey := openStreamWithSessionKey(t, handler, peerID, streamID, requestID, false)

	// Close the stream first
	handler.HandleStreamClose(streamID)

	// Now try to send data to the closed stream
	meta := &ShellMeta{
		Command: "echo",
		Args:    []string{"hello"},
	}
	metaMsg, _ := EncodeMeta(meta)
	encryptedMeta, err := sessionKey.Encrypt(metaMsg)
	if err != nil {
		t.Fatalf("failed to encrypt: %v", err)
	}

	// This should silently return without error
	handler.HandleStreamData(peerID, streamID, encryptedMeta, 0)

	// No crash means success
}

func TestHandler_HandleMessage_InvalidResize(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows")
	}

	writer := newMockDataWriter()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	exec := NewExecutor(Config{
		Enabled:     true,
		MaxSessions: 10,
		Whitelist:   []string{"*"},
	})

	handler := NewHandler(exec, writer, logger)

	peerID := mustNewAgentID(t)
	streamID := uint64(1)
	requestID := uint64(1)

	// Open stream and get session key
	sessionKey := openStreamWithSessionKey(t, handler, peerID, streamID, requestID, false)

	// Send valid metadata first
	meta := &ShellMeta{
		Command: "cat",
	}
	metaMsg, _ := EncodeMeta(meta)
	encryptedMeta, err := sessionKey.Encrypt(metaMsg)
	if err != nil {
		t.Fatalf("failed to encrypt: %v", err)
	}
	handler.HandleStreamData(peerID, streamID, encryptedMeta, 0)

	// Wait for ACK
	time.Sleep(100 * time.Millisecond)

	// Send invalid resize (too short payload)
	invalidResize := EncodeMessage(MsgResize, []byte{0x00, 0x18}) // Only 2 bytes, need 4
	encryptedInvalid, err := sessionKey.Encrypt(invalidResize)
	if err != nil {
		t.Fatalf("failed to encrypt: %v", err)
	}
	handler.HandleStreamData(peerID, streamID, encryptedInvalid, 0)

	// Should not crash
	time.Sleep(50 * time.Millisecond)

	// Clean up
	handler.HandleStreamClose(streamID)
}

func TestHandler_HandleMessage_InvalidSignal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows")
	}

	writer := newMockDataWriter()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	exec := NewExecutor(Config{
		Enabled:     true,
		MaxSessions: 10,
		Whitelist:   []string{"*"},
	})

	handler := NewHandler(exec, writer, logger)

	peerID := mustNewAgentID(t)
	streamID := uint64(1)
	requestID := uint64(1)

	// Open stream and get session key
	sessionKey := openStreamWithSessionKey(t, handler, peerID, streamID, requestID, false)

	// Send valid metadata first
	meta := &ShellMeta{
		Command: "cat",
	}
	metaMsg, _ := EncodeMeta(meta)
	encryptedMeta, err := sessionKey.Encrypt(metaMsg)
	if err != nil {
		t.Fatalf("failed to encrypt: %v", err)
	}
	handler.HandleStreamData(peerID, streamID, encryptedMeta, 0)

	// Wait for ACK
	time.Sleep(100 * time.Millisecond)

	// Send invalid signal (empty payload)
	invalidSignal := EncodeMessage(MsgSignal, []byte{})
	encryptedInvalid, err := sessionKey.Encrypt(invalidSignal)
	if err != nil {
		t.Fatalf("failed to encrypt: %v", err)
	}
	handler.HandleStreamData(peerID, streamID, encryptedInvalid, 0)

	// Should not crash
	time.Sleep(50 * time.Millisecond)

	// Clean up
	handler.HandleStreamClose(streamID)
}

func TestHandler_HandleStreamData_EmptyData(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows")
	}

	writer := newMockDataWriter()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	exec := NewExecutor(Config{
		Enabled:     true,
		MaxSessions: 10,
		Whitelist:   []string{"*"},
	})

	handler := NewHandler(exec, writer, logger)

	peerID := mustNewAgentID(t)
	streamID := uint64(1)
	requestID := uint64(1)

	// Open stream and get session key
	sessionKey := openStreamWithSessionKey(t, handler, peerID, streamID, requestID, false)

	// Send valid metadata first
	meta := &ShellMeta{
		Command: "cat",
	}
	metaMsg, _ := EncodeMeta(meta)
	encryptedMeta, err := sessionKey.Encrypt(metaMsg)
	if err != nil {
		t.Fatalf("failed to encrypt: %v", err)
	}
	handler.HandleStreamData(peerID, streamID, encryptedMeta, 0)

	// Wait for ACK
	time.Sleep(100 * time.Millisecond)

	// Send empty data (after metadata is received)
	emptyMsg := []byte{}
	encryptedEmpty, err := sessionKey.Encrypt(emptyMsg)
	if err != nil {
		t.Fatalf("failed to encrypt: %v", err)
	}
	handler.HandleStreamData(peerID, streamID, encryptedEmpty, 0)

	// Should not crash
	time.Sleep(50 * time.Millisecond)

	// Clean up
	handler.HandleStreamClose(streamID)
}
