package icmp

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/postalsys/muti-metroo/internal/crypto"
	"github.com/postalsys/muti-metroo/internal/identity"
	"golang.org/x/net/icmp"
)

// SessionState represents the state of an ICMP session.
type SessionState int

const (
	// StateOpening means the session is being established.
	StateOpening SessionState = iota
	// StateOpen means the session is active and can relay echo packets.
	StateOpen
	// StateClosed means the session has been terminated.
	StateClosed
)

// String returns a human-readable name for the state.
func (s SessionState) String() string {
	switch s {
	case StateOpening:
		return "OPENING"
	case StateOpen:
		return "OPEN"
	case StateClosed:
		return "CLOSED"
	default:
		return "UNKNOWN"
	}
}

// Session represents an active ICMP echo tunnel through the mesh.
type Session struct {
	mu sync.RWMutex

	// Identifiers
	StreamID  uint64           // Changes per hop (local to this agent)
	RequestID uint64           // Stable across hops for correlation
	PeerID    identity.AgentID // Direct peer that sent the ICMP_OPEN

	// Target
	DestIP net.IP // Destination IP for ICMP echo

	// State
	State        SessionState
	CreatedAt    time.Time
	LastActivity time.Time

	// ICMP socket (only on exit node)
	Conn *icmp.PacketConn

	// Encryption
	SessionKey *crypto.SessionKey

	// Cleanup
	ctx    context.Context
	cancel context.CancelFunc
	closed bool
}

// NewSession creates a new ICMP session.
func NewSession(streamID, requestID uint64, peerID identity.AgentID, destIP net.IP) *Session {
	ctx, cancel := context.WithCancel(context.Background())
	now := time.Now()

	return &Session{
		StreamID:     streamID,
		RequestID:    requestID,
		PeerID:       peerID,
		DestIP:       destIP,
		State:        StateOpening,
		CreatedAt:    now,
		LastActivity: now,
		ctx:          ctx,
		cancel:       cancel,
	}
}

// SetOpen transitions the session to the open state.
func (s *Session) SetOpen() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.State == StateOpening {
		s.State = StateOpen
		s.LastActivity = time.Now()
	}
}

// UpdateActivity updates the last activity timestamp.
func (s *Session) UpdateActivity() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.LastActivity = time.Now()
}

// IsExpired checks if the session has been idle longer than the timeout.
func (s *Session) IsExpired(timeout time.Duration) bool {
	if timeout == 0 {
		return false
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	return time.Since(s.LastActivity) > timeout
}

// IsClosed returns true if the session has been closed.
func (s *Session) IsClosed() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.closed
}

// GetState returns the current state.
func (s *Session) GetState() SessionState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.State
}

// Context returns the session's context.
func (s *Session) Context() context.Context {
	return s.ctx
}

// Close terminates the session and releases resources.
func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true
	s.State = StateClosed
	s.cancel()

	// Close ICMP socket if present
	if s.Conn != nil {
		if err := s.Conn.Close(); err != nil {
			return err
		}
		s.Conn = nil
	}

	// Clear session key reference
	s.SessionKey = nil

	return nil
}

// SetConn sets the ICMP connection for this session (exit node).
func (s *Session) SetConn(conn *icmp.PacketConn) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Conn = conn
}

// GetConn returns the ICMP connection.
func (s *Session) GetConn() *icmp.PacketConn {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.Conn
}

// SetSessionKey sets the E2E encryption session key.
func (s *Session) SetSessionKey(key *crypto.SessionKey) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.SessionKey = key
}

// GetSessionKey returns the E2E encryption session key.
func (s *Session) GetSessionKey() *crypto.SessionKey {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.SessionKey
}

// Encrypt encrypts data using the session key.
// Returns the original data if no session key is set.
func (s *Session) Encrypt(plaintext []byte) ([]byte, error) {
	s.mu.RLock()
	key := s.SessionKey
	s.mu.RUnlock()

	if key == nil {
		return plaintext, nil
	}

	return key.Encrypt(plaintext)
}

// Decrypt decrypts data using the session key.
// Returns the original data if no session key is set.
func (s *Session) Decrypt(ciphertext []byte) ([]byte, error) {
	s.mu.RLock()
	key := s.SessionKey
	s.mu.RUnlock()

	if key == nil {
		return ciphertext, nil
	}

	return key.Decrypt(ciphertext)
}
