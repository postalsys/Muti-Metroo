package socks5

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// ServerConfig holds server configuration.
type ServerConfig struct {
	// Address to listen on (e.g., "127.0.0.1:1080")
	Address string

	// MaxConnections limits concurrent connections (0 = unlimited)
	MaxConnections int

	// ConnectTimeout for outbound connections
	ConnectTimeout time.Duration

	// IdleTimeout for idle connections
	IdleTimeout time.Duration

	// Authenticators for authentication
	Authenticators []Authenticator

	// Dialer for making outbound connections
	Dialer Dialer
}

// DefaultServerConfig returns sensible defaults.
func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		Address:        "127.0.0.1:1080",
		MaxConnections: 1000,
		ConnectTimeout: 30 * time.Second,
		IdleTimeout:    5 * time.Minute,
		Authenticators: []Authenticator{&NoAuthAuthenticator{}},
		Dialer:         &DirectDialer{},
	}
}

// Server is a SOCKS5 proxy server.
type Server struct {
	cfg      ServerConfig
	handler  *Handler
	listener net.Listener

	mu          sync.Mutex
	connections map[net.Conn]struct{}
	connCount   atomic.Int64

	running  atomic.Bool
	stopOnce sync.Once
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// NewServer creates a new SOCKS5 server.
func NewServer(cfg ServerConfig) *Server {
	if cfg.Dialer == nil {
		cfg.Dialer = &DirectDialer{}
	}
	if len(cfg.Authenticators) == 0 {
		cfg.Authenticators = []Authenticator{&NoAuthAuthenticator{}}
	}

	return &Server{
		cfg:         cfg,
		handler:     NewHandler(cfg.Authenticators, cfg.Dialer),
		connections: make(map[net.Conn]struct{}),
		stopCh:      make(chan struct{}),
	}
}

// Start starts the SOCKS5 server.
func (s *Server) Start() error {
	if s.running.Load() {
		return fmt.Errorf("server already running")
	}

	listener, err := net.Listen("tcp", s.cfg.Address)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	s.listener = listener
	s.running.Store(true)

	s.wg.Add(1)
	go s.acceptLoop()

	return nil
}

// Stop gracefully stops the server.
func (s *Server) Stop() error {
	var err error
	s.stopOnce.Do(func() {
		s.running.Store(false)
		close(s.stopCh)

		// Close listener
		if s.listener != nil {
			err = s.listener.Close()
		}

		// Close all active connections
		s.mu.Lock()
		for conn := range s.connections {
			conn.Close()
		}
		s.mu.Unlock()
	})

	// Wait for all goroutines to finish
	s.wg.Wait()
	return err
}

// StopWithContext stops with a timeout.
func (s *Server) StopWithContext(ctx context.Context) error {
	done := make(chan error, 1)
	go func() {
		done <- s.Stop()
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Address returns the listening address.
func (s *Server) Address() net.Addr {
	if s.listener == nil {
		return nil
	}
	return s.listener.Addr()
}

// ConnectionCount returns the number of active connections.
func (s *Server) ConnectionCount() int64 {
	return s.connCount.Load()
}

// IsRunning returns true if the server is running.
func (s *Server) IsRunning() bool {
	return s.running.Load()
}

// acceptLoop accepts new connections.
func (s *Server) acceptLoop() {
	defer s.wg.Done()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.stopCh:
				return
			default:
				// Log error and continue
				continue
			}
		}

		// Check connection limit
		if s.cfg.MaxConnections > 0 && s.connCount.Load() >= int64(s.cfg.MaxConnections) {
			conn.Close()
			continue
		}

		s.trackConn(conn, true)
		s.wg.Add(1)
		go s.handleConn(conn)
	}
}

// handleConn handles a single connection.
func (s *Server) handleConn(conn net.Conn) {
	defer s.wg.Done()
	defer s.trackConn(conn, false)
	defer conn.Close()

	// Set timeouts
	if s.cfg.IdleTimeout > 0 {
		conn.SetDeadline(time.Now().Add(s.cfg.IdleTimeout))
	}

	// Handle the connection
	s.handler.Handle(conn)
}

// trackConn tracks active connections.
func (s *Server) trackConn(conn net.Conn, add bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if add {
		s.connections[conn] = struct{}{}
		s.connCount.Add(1)
	} else {
		delete(s.connections, conn)
		s.connCount.Add(-1)
	}
}

// WithAuthenticators returns a new server config with authenticators.
func (cfg ServerConfig) WithAuthenticators(auths ...Authenticator) ServerConfig {
	cfg.Authenticators = auths
	return cfg
}

// WithDialer returns a new server config with a custom dialer.
func (cfg ServerConfig) WithDialer(dialer Dialer) ServerConfig {
	cfg.Dialer = dialer
	return cfg
}

// WithMaxConnections returns a new server config with max connections.
func (cfg ServerConfig) WithMaxConnections(max int) ServerConfig {
	cfg.MaxConnections = max
	return cfg
}
