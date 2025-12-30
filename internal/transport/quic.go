package transport

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
)

// Default QUIC configuration values
const (
	DefaultMaxIdleTimeout     = 60 * time.Second
	DefaultKeepAlivePeriod    = 30 * time.Second
	DefaultMaxIncomingStreams = 10000
)

// QUICTransport implements Transport using QUIC protocol.
type QUICTransport struct {
	mu        sync.Mutex
	listeners []*QUICListener
	closed    bool
}

// NewQUICTransport creates a new QUIC transport.
func NewQUICTransport() *QUICTransport {
	return &QUICTransport{}
}

// Type returns the transport type.
func (t *QUICTransport) Type() TransportType {
	return TransportQUIC
}

// Dial connects to a remote peer using QUIC.
func (t *QUICTransport) Dial(ctx context.Context, addr string, opts DialOptions) (PeerConn, error) {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil, fmt.Errorf("transport closed")
	}
	t.mu.Unlock()

	tlsConfig := opts.TLSConfig
	if tlsConfig == nil {
		if !opts.InsecureSkipVerify {
			return nil, fmt.Errorf("TLS config required; set InsecureSkipVerify=true for development only")
		}
		// Create insecure TLS config only when explicitly requested
		tlsConfig = &tls.Config{
			InsecureSkipVerify: true,
			NextProtos:         []string{ALPNProtocol},
			MinVersion:         tls.VersionTLS13,
		}
	}

	quicConfig := &quic.Config{
		MaxIdleTimeout:        DefaultMaxIdleTimeout,
		KeepAlivePeriod:       DefaultKeepAlivePeriod,
		MaxIncomingStreams:    DefaultMaxIncomingStreams,
		MaxIncomingUniStreams: 0, // We don't use uni streams
	}

	// Apply timeout
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	conn, err := quic.DialAddr(ctx, addr, tlsConfig, quicConfig)
	if err != nil {
		return nil, fmt.Errorf("QUIC dial failed: %w", err)
	}

	return &QUICPeerConn{
		conn:     conn,
		isDialer: true,
	}, nil
}

// Listen creates a QUIC listener.
func (t *QUICTransport) Listen(addr string, opts ListenOptions) (Listener, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil, fmt.Errorf("transport closed")
	}

	tlsConfig := opts.TLSConfig
	if tlsConfig == nil {
		return nil, fmt.Errorf("TLS config required for QUIC listener")
	}

	// Ensure ALPN is set
	if len(tlsConfig.NextProtos) == 0 {
		tlsConfig = tlsConfig.Clone()
		tlsConfig.NextProtos = []string{ALPNProtocol}
	}

	maxStreams := opts.MaxStreams
	if maxStreams <= 0 {
		maxStreams = DefaultMaxIncomingStreams
	}

	quicConfig := &quic.Config{
		MaxIdleTimeout:        DefaultMaxIdleTimeout,
		KeepAlivePeriod:       DefaultKeepAlivePeriod,
		MaxIncomingStreams:    int64(maxStreams),
		MaxIncomingUniStreams: 0,
	}

	listener, err := quic.ListenAddr(addr, tlsConfig, quicConfig)
	if err != nil {
		return nil, fmt.Errorf("QUIC listen failed: %w", err)
	}

	ql := &QUICListener{
		listener: listener,
	}
	t.listeners = append(t.listeners, ql)

	return ql, nil
}

// Close shuts down the transport and all listeners.
func (t *QUICTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil
	}
	t.closed = true

	var lastErr error
	for _, l := range t.listeners {
		if err := l.Close(); err != nil {
			lastErr = err
		}
	}
	t.listeners = nil

	return lastErr
}

// QUICListener implements Listener for QUIC.
type QUICListener struct {
	listener *quic.Listener
	closed   bool
	mu       sync.Mutex
}

// Accept waits for and returns the next QUIC connection.
func (l *QUICListener) Accept(ctx context.Context) (PeerConn, error) {
	conn, err := l.listener.Accept(ctx)
	if err != nil {
		return nil, err
	}

	return &QUICPeerConn{
		conn:     conn,
		isDialer: false,
	}, nil
}

// Addr returns the listener's address.
func (l *QUICListener) Addr() net.Addr {
	return l.listener.Addr()
}

// Close stops the listener.
func (l *QUICListener) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return nil
	}
	l.closed = true

	return l.listener.Close()
}

// QUICPeerConn implements PeerConn for QUIC.
type QUICPeerConn struct {
	conn     quic.Connection
	isDialer bool
}

// OpenStream creates a new outgoing QUIC stream.
func (c *QUICPeerConn) OpenStream(ctx context.Context) (Stream, error) {
	stream, err := c.conn.OpenStreamSync(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to open QUIC stream: %w", err)
	}

	return &QUICStream{stream: stream}, nil
}

// AcceptStream waits for an incoming QUIC stream.
func (c *QUICPeerConn) AcceptStream(ctx context.Context) (Stream, error) {
	stream, err := c.conn.AcceptStream(ctx)
	if err != nil {
		return nil, err
	}

	return &QUICStream{stream: stream}, nil
}

// Close terminates the QUIC connection.
func (c *QUICPeerConn) Close() error {
	return c.conn.CloseWithError(0, "connection closed")
}

// LocalAddr returns the local address.
func (c *QUICPeerConn) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

// RemoteAddr returns the remote address.
func (c *QUICPeerConn) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

// IsDialer returns true if this side initiated the connection.
func (c *QUICPeerConn) IsDialer() bool {
	return c.isDialer
}

// TransportType returns the transport protocol type.
func (c *QUICPeerConn) TransportType() TransportType {
	return TransportQUIC
}

// QUICStream implements Stream for QUIC.
type QUICStream struct {
	stream quic.Stream
}

// StreamID returns the QUIC stream ID.
func (s *QUICStream) StreamID() uint64 {
	return uint64(s.stream.StreamID())
}

// Read reads data from the stream.
func (s *QUICStream) Read(p []byte) (int, error) {
	return s.stream.Read(p)
}

// Write writes data to the stream.
func (s *QUICStream) Write(p []byte) (int, error) {
	return s.stream.Write(p)
}

// CloseWrite sends a half-close (FIN) on the write side.
func (s *QUICStream) CloseWrite() error {
	return s.stream.Close()
}

// Close fully closes the stream.
func (s *QUICStream) Close() error {
	s.stream.CancelRead(0)
	return s.stream.Close()
}

// SetDeadline sets read and write deadlines.
func (s *QUICStream) SetDeadline(t time.Time) error {
	return s.stream.SetDeadline(t)
}

// SetReadDeadline sets the read deadline.
func (s *QUICStream) SetReadDeadline(t time.Time) error {
	return s.stream.SetReadDeadline(t)
}

// SetWriteDeadline sets the write deadline.
func (s *QUICStream) SetWriteDeadline(t time.Time) error {
	return s.stream.SetWriteDeadline(t)
}
