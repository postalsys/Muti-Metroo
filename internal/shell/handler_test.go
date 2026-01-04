package shell

import (
	"log/slog"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/postalsys/muti-metroo/internal/identity"
)

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
	errCode := handler.HandleStreamOpen(peerID, streamID, requestID, false)
	if errCode == 0 {
		t.Error("HandleStreamOpen() should have returned error code for disabled shell")
	}
}

func TestHandler_HandleStreamOpen_StreamingDisabled(t *testing.T) {
	writer := newMockDataWriter()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	exec := NewExecutor(Config{
		Enabled: true,
		Streaming: StreamingConfig{
			Enabled: false, // Streaming disabled
		},
	})

	handler := NewHandler(exec, writer, logger)

	peerID := mustNewAgentID(t)
	streamID := uint64(1)
	requestID := uint64(1)

	// Open streaming mode should fail
	errCode := handler.HandleStreamOpen(peerID, streamID, requestID, false)
	if errCode == 0 {
		t.Error("HandleStreamOpen() should have returned error for disabled streaming")
	}
}

func TestHandler_HandleStreamOpen_InteractiveDisabled(t *testing.T) {
	writer := newMockDataWriter()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	exec := NewExecutor(Config{
		Enabled: true,
		Interactive: InteractiveConfig{
			Enabled: false, // Interactive disabled
		},
	})

	handler := NewHandler(exec, writer, logger)

	peerID := mustNewAgentID(t)
	streamID := uint64(1)
	requestID := uint64(1)

	// Open interactive mode should fail
	errCode := handler.HandleStreamOpen(peerID, streamID, requestID, true)
	if errCode == 0 {
		t.Error("HandleStreamOpen() should have returned error for disabled interactive")
	}
}

func TestHandler_HandleStreamOpen_Success(t *testing.T) {
	writer := newMockDataWriter()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	exec := NewExecutor(Config{
		Enabled:     true,
		MaxSessions: 10,
		Streaming: StreamingConfig{
			Enabled: true,
		},
	})

	handler := NewHandler(exec, writer, logger)

	peerID := mustNewAgentID(t)
	streamID := uint64(1)
	requestID := uint64(1)

	// Open should succeed
	errCode := handler.HandleStreamOpen(peerID, streamID, requestID, false)
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
		Streaming: StreamingConfig{
			Enabled:     true,
			MaxDuration: 10 * time.Second,
		},
	})

	handler := NewHandler(exec, writer, logger)

	peerID := mustNewAgentID(t)
	streamID := uint64(1)
	requestID := uint64(1)

	// Open stream
	errCode := handler.HandleStreamOpen(peerID, streamID, requestID, false)
	if errCode != 0 {
		t.Fatalf("HandleStreamOpen() returned error code %d", errCode)
	}

	// Send metadata - execute echo command
	meta := &ShellMeta{
		Command: "echo",
		Args:    []string{"hello world"},
	}
	metaMsg, _ := EncodeMeta(meta)
	handler.HandleStreamData(peerID, streamID, metaMsg, 0)

	// Wait for output
	time.Sleep(500 * time.Millisecond)

	// Check for messages
	messages := writer.getMessages()
	if len(messages) == 0 {
		t.Fatal("Expected messages from handler")
	}

	// First message should be ACK
	firstMsg := messages[0]
	msgType, _, err := DecodeMessage(firstMsg.data)
	if err != nil {
		t.Fatalf("DecodeMessage() error = %v", err)
	}
	if msgType != MsgAck {
		t.Errorf("First message type = %d, want MsgAck (%d)", msgType, MsgAck)
	}

	// Wait for command to complete
	time.Sleep(500 * time.Millisecond)

	// Check for stdout and exit messages
	messages = writer.getMessages()
	hasStdout := false
	hasExit := false
	for _, msg := range messages {
		msgType, _, _ := DecodeMessage(msg.data)
		if msgType == MsgStdout {
			hasStdout = true
		}
		if msgType == MsgExit {
			hasExit = true
		}
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
		Streaming: StreamingConfig{
			Enabled: true,
		},
	})

	handler := NewHandler(exec, writer, logger)

	peerID := mustNewAgentID(t)
	streamID := uint64(1)
	requestID := uint64(1)

	// Open stream
	errCode := handler.HandleStreamOpen(peerID, streamID, requestID, false)
	if errCode != 0 {
		t.Fatalf("HandleStreamOpen() returned error code %d", errCode)
	}

	// Send metadata with non-whitelisted command
	meta := &ShellMeta{
		Command: "echo", // Not in whitelist
		Args:    []string{"hello"},
	}
	metaMsg, _ := EncodeMeta(meta)
	handler.HandleStreamData(peerID, streamID, metaMsg, 0)

	// Wait for response
	time.Sleep(100 * time.Millisecond)

	// Check for error message
	messages := writer.getMessages()
	hasError := false
	for _, msg := range messages {
		msgType, _, _ := DecodeMessage(msg.data)
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
		Streaming: StreamingConfig{
			Enabled: true,
		},
	})

	handler := NewHandler(exec, writer, logger)

	// Open multiple streams
	peerID := mustNewAgentID(t)
	for i := uint64(1); i <= 3; i++ {
		handler.HandleStreamOpen(peerID, i, i, false)
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
