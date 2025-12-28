package transport

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/http2"
)

// HTTP/2 transport constants
const (
	h2DefaultPath        = "/mesh"
	h2DefaultIdleTimeout = 60 * time.Second
)

// H2Transport implements Transport using HTTP/2 streaming.
// Unlike QUIC, HTTP/2 doesn't have native stream multiplexing for our use case.
// We use a single HTTP/2 POST request with streaming body in both directions.
// All virtual streams are multiplexed using our frame protocol.
type H2Transport struct {
	mu        sync.Mutex
	listeners []*H2Listener
	closed    bool
}

// NewH2Transport creates a new HTTP/2 transport.
func NewH2Transport() *H2Transport {
	return &H2Transport{}
}

// Type returns the transport type.
func (t *H2Transport) Type() TransportType {
	return TransportHTTP2
}

// Dial connects to a remote peer using HTTP/2 streaming.
func (t *H2Transport) Dial(ctx context.Context, addr string, opts DialOptions) (PeerConn, error) {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil, fmt.Errorf("transport closed")
	}
	t.mu.Unlock()

	// Parse address
	h2URL, path := parseH2Address(addr, opts)

	// Create a connection context that outlives the dial call
	// The request uses a cancellable background context - we cancel it on Close()
	// The dial timeout is handled separately
	connCtx, connCancel := context.WithCancel(context.Background())

	// Apply dial timeout if specified
	var dialCtx context.Context
	var dialCancel context.CancelFunc
	if opts.Timeout > 0 {
		dialCtx, dialCancel = context.WithTimeout(ctx, opts.Timeout)
	} else {
		dialCtx, dialCancel = context.WithCancel(ctx)
	}
	// dialCancel is called after RoundTrip completes (success or failure)

	// Create HTTP/2 client with TLS
	tlsConfig := opts.TLSConfig
	if tlsConfig == nil {
		tlsConfig = &tls.Config{
			InsecureSkipVerify: true,
			NextProtos:         []string{"h2"},
		}
	} else {
		tlsConfig = tlsConfig.Clone()
		// Ensure HTTP/2 is in NextProtos
		hasH2 := false
		for _, proto := range tlsConfig.NextProtos {
			if proto == "h2" {
				hasH2 = true
				break
			}
		}
		if !hasH2 {
			tlsConfig.NextProtos = append([]string{"h2"}, tlsConfig.NextProtos...)
		}
	}

	h2Transport := &http2.Transport{
		TLSClientConfig: tlsConfig,
		AllowHTTP:       false,
	}

	// Create pipe for bidirectional streaming
	// Client writes to pipeWriter, server reads from pipeReader
	pipeReader, pipeWriter := io.Pipe()

	// Create the request with connection context (long-lived, not subject to dial timeout)
	req, err := http.NewRequestWithContext(connCtx, "POST", h2URL+path, pipeReader)
	if err != nil {
		dialCancel()
		connCancel()
		pipeWriter.Close()
		pipeReader.Close()
		return nil, fmt.Errorf("create request failed: %w", err)
	}

	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Muti-Metroo-Protocol", ALPNProtocol)

	// Execute request with dial timeout
	// We use a goroutine to monitor the dial timeout context
	type roundTripResult struct {
		resp *http.Response
		err  error
	}
	resultCh := make(chan roundTripResult, 1)

	go func() {
		resp, err := h2Transport.RoundTrip(req)
		resultCh <- roundTripResult{resp, err}
	}()

	var resp *http.Response
	select {
	case result := <-resultCh:
		dialCancel() // Dial completed, cancel dial timeout
		if result.err != nil {
			connCancel()
			pipeWriter.Close()
			pipeReader.Close()
			return nil, fmt.Errorf("HTTP/2 dial failed: %w", result.err)
		}
		resp = result.resp
	case <-dialCtx.Done():
		// Dial timeout - cancel the connection context to abort the request
		connCancel()
		dialCancel()
		pipeWriter.Close()
		pipeReader.Close()
		return nil, fmt.Errorf("HTTP/2 dial timeout: %w", dialCtx.Err())
	}

	if resp.StatusCode != http.StatusOK {
		connCancel()
		resp.Body.Close()
		pipeWriter.Close()
		pipeReader.Close()
		return nil, fmt.Errorf("HTTP/2 dial failed: status %d", resp.StatusCode)
	}

	return &H2PeerConn{
		reader:       resp.Body,
		writer:       pipeWriter,
		isDialer:     true,
		cancelDialFn: connCancel, // Cancel connection context on Close()
	}, nil
}

// Listen creates an HTTP/2 listener.
func (t *H2Transport) Listen(addr string, opts ListenOptions) (Listener, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil, fmt.Errorf("transport closed")
	}

	tlsConfig := opts.TLSConfig
	if tlsConfig == nil {
		return nil, fmt.Errorf("TLS config required for HTTP/2 listener")
	}

	// Ensure HTTP/2 is in NextProtos
	tlsConfig = tlsConfig.Clone()
	hasH2 := false
	for _, proto := range tlsConfig.NextProtos {
		if proto == "h2" {
			hasH2 = true
			break
		}
	}
	if !hasH2 {
		tlsConfig.NextProtos = append([]string{"h2"}, tlsConfig.NextProtos...)
	}

	path := opts.Path
	if path == "" {
		path = h2DefaultPath
	}

	listener := &H2Listener{
		addr:      addr,
		path:      path,
		tlsConfig: tlsConfig,
		connCh:    make(chan *H2PeerConn, 16),
		closeCh:   make(chan struct{}),
	}

	// Start HTTP/2 server
	if err := listener.start(); err != nil {
		return nil, err
	}

	t.listeners = append(t.listeners, listener)
	return listener, nil
}

// Close shuts down the transport and all listeners.
func (t *H2Transport) Close() error {
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

// H2Listener implements Listener for HTTP/2.
type H2Listener struct {
	addr      string
	path      string
	tlsConfig *tls.Config
	server    *http.Server
	netLn     net.Listener
	connCh    chan *H2PeerConn
	closeCh   chan struct{}
	closed    atomic.Bool
	mu        sync.Mutex
}

// start initializes the HTTP/2 server.
func (l *H2Listener) start() error {
	mux := http.NewServeMux()
	mux.HandleFunc(l.path, l.handleH2Stream)

	l.server = &http.Server{
		Addr:      l.addr,
		Handler:   mux,
		TLSConfig: l.tlsConfig,
	}

	// Configure HTTP/2
	http2.ConfigureServer(l.server, &http2.Server{})

	// Create TCP listener
	ln, err := net.Listen("tcp", l.addr)
	if err != nil {
		return fmt.Errorf("listen failed: %w", err)
	}
	l.netLn = ln

	// Start serving in background with TLS
	go func() {
		tlsLn := tls.NewListener(ln, l.tlsConfig)
		l.server.Serve(tlsLn)
	}()

	return nil
}

// handleH2Stream handles incoming HTTP/2 streaming POST requests.
func (l *H2Listener) handleH2Stream(w http.ResponseWriter, r *http.Request) {
	// Check if we're closed
	if l.closed.Load() {
		http.Error(w, "server closed", http.StatusServiceUnavailable)
		return
	}

	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check protocol header
	proto := r.Header.Get("X-Muti-Metroo-Protocol")
	if proto != "" && proto != ALPNProtocol {
		http.Error(w, "unsupported protocol", http.StatusBadRequest)
		return
	}

	// Enable streaming response
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-Muti-Metroo-Protocol", ALPNProtocol)
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Create pipe for server-to-client writes
	// Server writes to pipeWriter, response body reads from pipeReader
	pipeReader, pipeWriter := io.Pipe()

	// Channel to signal data pump has stopped
	pumpDone := make(chan struct{})

	// Create peer connection with doneCh initialized to avoid race condition
	peerConn := &H2PeerConn{
		reader:     r.Body,
		writer:     pipeWriter,
		isDialer:   false,
		flusher:    flusher,
		respWriter: w,
		doneCh:     make(chan struct{}),
	}

	// Start goroutine to pump from pipe to response
	go func() {
		defer close(pumpDone)
		defer pipeReader.Close()
		buf := make([]byte, 32768)
		for {
			n, err := pipeReader.Read(buf)
			if err != nil {
				return
			}
			if n > 0 {
				_, writeErr := w.Write(buf[:n])
				if writeErr != nil {
					return
				}
				flusher.Flush()
			}
		}
	}()

	// Send to Accept channel
	select {
	case l.connCh <- peerConn:
		// Block until connection is done
		<-peerConn.doneCh
		// Close the pipe writer to stop the pump goroutine
		pipeWriter.Close()
		// Wait for pump to finish before returning (avoids race with http2 handlerDone)
		<-pumpDone
	case <-l.closeCh:
		pipeWriter.Close()
		<-pumpDone
	}
}

// Accept waits for and returns the next HTTP/2 connection.
func (l *H2Listener) Accept(ctx context.Context) (PeerConn, error) {
	select {
	case conn := <-l.connCh:
		// doneCh is already initialized in handleH2Stream to avoid race condition
		return conn, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-l.closeCh:
		return nil, fmt.Errorf("listener closed")
	}
}

// Addr returns the listener's address.
func (l *H2Listener) Addr() net.Addr {
	if l.netLn != nil {
		return l.netLn.Addr()
	}
	return nil
}

// Close stops the listener.
func (l *H2Listener) Close() error {
	if l.closed.Swap(true) {
		return nil
	}

	close(l.closeCh)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if l.server != nil {
		return l.server.Shutdown(ctx)
	}
	return nil
}

// H2PeerConn implements PeerConn for HTTP/2.
// Since HTTP/2 streaming uses a single request/response pair,
// this connection provides a single "stream" that the peer manager
// uses as its control stream.
type H2PeerConn struct {
	reader       io.ReadCloser
	writer       io.WriteCloser
	isDialer     bool
	flusher      http.Flusher
	respWriter   http.ResponseWriter
	streamOnce   sync.Once
	stream       *H2Stream
	closed       atomic.Bool
	doneCh       chan struct{}
	cancelDialFn context.CancelFunc // Cancel function for dial context (client only)
}

// OpenStream returns the single HTTP/2 stream.
func (c *H2PeerConn) OpenStream(ctx context.Context) (Stream, error) {
	c.streamOnce.Do(func() {
		c.stream = &H2Stream{
			reader: c.reader,
			writer: c.writer,
			id:     1,
		}
	})
	return c.stream, nil
}

// AcceptStream returns the single HTTP/2 stream.
func (c *H2PeerConn) AcceptStream(ctx context.Context) (Stream, error) {
	c.streamOnce.Do(func() {
		c.stream = &H2Stream{
			reader: c.reader,
			writer: c.writer,
			id:     1,
		}
	})
	return c.stream, nil
}

// Close terminates the HTTP/2 connection.
func (c *H2PeerConn) Close() error {
	if c.closed.Swap(true) {
		return nil
	}

	if c.doneCh != nil {
		close(c.doneCh)
	}

	// Cancel the dial context to clean up HTTP/2 resources
	if c.cancelDialFn != nil {
		c.cancelDialFn()
	}

	var err error
	if c.writer != nil {
		if closeErr := c.writer.Close(); closeErr != nil {
			err = closeErr
		}
	}
	if c.reader != nil {
		if closeErr := c.reader.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}
	return err
}

// LocalAddr returns the local address (not available for HTTP/2).
func (c *H2PeerConn) LocalAddr() net.Addr {
	return nil
}

// RemoteAddr returns the remote address (not available for HTTP/2).
func (c *H2PeerConn) RemoteAddr() net.Addr {
	return nil
}

// IsDialer returns true if this side initiated the connection.
func (c *H2PeerConn) IsDialer() bool {
	return c.isDialer
}

// H2Stream implements Stream for HTTP/2.
type H2Stream struct {
	reader  io.ReadCloser
	writer  io.WriteCloser
	id      uint64
	writeMu sync.Mutex // Protects concurrent writes
	closed  atomic.Bool
}

// StreamID returns the stream ID.
func (s *H2Stream) StreamID() uint64 {
	return s.id
}

// Read reads data from the HTTP/2 stream.
func (s *H2Stream) Read(p []byte) (int, error) {
	return s.reader.Read(p)
}

// Write writes data to the HTTP/2 stream.
func (s *H2Stream) Write(p []byte) (int, error) {
	if s.closed.Load() {
		return 0, fmt.Errorf("stream closed")
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.writer.Write(p)
}

// CloseWrite signals end of writes.
func (s *H2Stream) CloseWrite() error {
	return nil // HTTP/2 streaming doesn't support half-close in this model
}

// Close fully closes the stream.
func (s *H2Stream) Close() error {
	if s.closed.Swap(true) {
		return nil
	}
	var err error
	if s.writer != nil {
		if closeErr := s.writer.Close(); closeErr != nil {
			err = closeErr
		}
	}
	if s.reader != nil {
		if closeErr := s.reader.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}
	return err
}

// SetDeadline sets read and write deadlines.
func (s *H2Stream) SetDeadline(t time.Time) error {
	return nil
}

// SetReadDeadline sets the read deadline.
func (s *H2Stream) SetReadDeadline(t time.Time) error {
	return nil
}

// SetWriteDeadline sets the write deadline.
func (s *H2Stream) SetWriteDeadline(t time.Time) error {
	return nil
}

// parseH2Address parses the address into HTTP/2 URL components.
func parseH2Address(addr string, opts DialOptions) (baseURL, path string) {
	// If already a URL, extract components
	if len(addr) > 8 && addr[:8] == "https://" {
		// Find path separator
		for i := 8; i < len(addr); i++ {
			if addr[i] == '/' {
				return addr[:i], addr[i:]
			}
		}
		return addr, h2DefaultPath
	}

	if len(addr) > 7 && addr[:7] == "http://" {
		// Find path separator (insecure, for testing)
		for i := 7; i < len(addr); i++ {
			if addr[i] == '/' {
				return addr[:i], addr[i:]
			}
		}
		return addr, h2DefaultPath
	}

	// Build URL from host:port
	return "https://" + addr, h2DefaultPath
}
