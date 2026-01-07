// Package probe provides connectivity testing for Muti Metroo listeners.
package probe

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/postalsys/muti-metroo/internal/identity"
	"github.com/postalsys/muti-metroo/internal/protocol"
	"github.com/postalsys/muti-metroo/internal/transport"
)

// Options contains configuration for a connectivity probe.
type Options struct {
	// Transport type: "quic", "h2", "ws"
	Transport string

	// Address is the host:port to probe
	Address string

	// Path is the HTTP path for h2/ws transports (default: "/mesh")
	Path string

	// Timeout for the entire probe operation
	Timeout time.Duration

	// InsecureSkipVerify skips TLS certificate verification
	InsecureSkipVerify bool

	// CACert is the path to a CA certificate file for TLS verification
	CACert string

	// ClientCert is the path to a client certificate file for mTLS
	ClientCert string

	// ClientKey is the path to a client key file for mTLS
	ClientKey string

	// Protocol identifiers for OPSEC
	ALPNProtocol  string
	HTTPHeader    string
	WSSubprotocol string
}

// Result contains the outcome of a connectivity probe.
type Result struct {
	// Success indicates whether the probe succeeded
	Success bool

	// Transport type that was tested
	Transport string

	// Address that was probed
	Address string

	// RemoteID is the agent ID from the remote peer (if handshake succeeded)
	RemoteID string

	// RemoteDisplayName is the display name from the remote peer
	RemoteDisplayName string

	// RTT is the round-trip time measured during the probe
	RTT time.Duration

	// Error is the error that occurred (if any)
	Error error

	// ErrorDetail is a human-readable description of the error
	ErrorDetail string
}

// Probe tests connectivity to a Muti Metroo listener.
// It performs:
// 1. Transport-level connection (TCP/TLS)
// 2. Protocol handshake verification (PEER_HELLO exchange)
func Probe(ctx context.Context, opts Options) *Result {
	result := &Result{
		Transport: opts.Transport,
		Address:   opts.Address,
	}

	// Set defaults
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Second
	}
	if opts.Path == "" {
		opts.Path = "/mesh"
	}

	// Create timeout context
	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	// Create TLS config
	tlsConfig, err := buildTLSConfig(opts)
	if err != nil {
		result.Error = err
		result.ErrorDetail = classifyError(err)
		return result
	}

	// Create transport
	tr, err := createTransport(opts.Transport)
	if err != nil {
		result.Error = err
		result.ErrorDetail = classifyError(err)
		return result
	}
	defer tr.Close()

	// Build dial options
	dialOpts := transport.DialOptions{
		TLSConfig:          tlsConfig,
		InsecureSkipVerify: opts.InsecureSkipVerify,
		Timeout:            opts.Timeout,
		ALPNProtocol:       opts.ALPNProtocol,
		HTTPHeader:         opts.HTTPHeader,
		WSSubprotocol:      opts.WSSubprotocol,
	}

	// For HTTP-based transports, format as full URL with path
	addr := opts.Address
	if opts.Transport == "h2" {
		// H2 transport expects https:// or http:// URLs
		if !strings.HasPrefix(addr, "https://") && !strings.HasPrefix(addr, "http://") {
			addr = "https://" + addr
		}
		// Add path if not already in URL (check after scheme)
		if idx := strings.Index(addr, "://"); idx >= 0 {
			rest := addr[idx+3:]
			if !strings.Contains(rest, "/") && opts.Path != "" {
				addr = addr + opts.Path
			}
		}
	} else if opts.Transport == "ws" {
		// WS transport expects wss:// or ws:// URLs
		if !strings.HasPrefix(addr, "wss://") && !strings.HasPrefix(addr, "ws://") {
			addr = "wss://" + addr
		}
		// Add path if not already in URL
		if idx := strings.Index(addr, "://"); idx >= 0 {
			rest := addr[idx+3:]
			if !strings.Contains(rest, "/") && opts.Path != "" {
				addr = addr + opts.Path
			}
		}
	}

	// Dial the listener
	startTime := time.Now()
	conn, err := tr.Dial(ctx, addr, dialOpts)
	if err != nil {
		result.Error = err
		result.ErrorDetail = classifyError(err)
		return result
	}
	defer conn.Close()

	// Perform handshake
	handshakeResult, err := performProbeHandshake(ctx, conn)
	if err != nil {
		result.Error = err
		result.ErrorDetail = classifyError(err)
		return result
	}

	// Success
	result.Success = true
	result.RemoteID = handshakeResult.RemoteID.String()
	result.RemoteDisplayName = handshakeResult.RemoteDisplayName
	result.RTT = time.Since(startTime)

	return result
}

// probeHandshakeResult contains the handshake result for probing.
type probeHandshakeResult struct {
	RemoteID          identity.AgentID
	RemoteDisplayName string
}

// performProbeHandshake performs a minimal handshake to verify the listener.
func performProbeHandshake(ctx context.Context, conn transport.PeerConn) (*probeHandshakeResult, error) {
	// Open a stream for the handshake
	stream, err := conn.OpenStream(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to open stream: %w", err)
	}
	defer stream.Close()

	// Set deadline from context
	if deadline, ok := ctx.Deadline(); ok {
		stream.SetDeadline(deadline)
	}

	// Create frame reader/writer
	reader := protocol.NewFrameReader(stream)
	writer := protocol.NewFrameWriter(stream)

	// Generate a temporary probe identity
	probeID := identity.AgentID{}
	copy(probeID[:], "probe-temporary!")

	// Send PEER_HELLO
	hello := &protocol.PeerHello{
		Version:      protocol.ProtocolVersion,
		AgentID:      probeID,
		Timestamp:    uint64(time.Now().UnixNano()),
		Capabilities: []string{},
		DisplayName:  "probe",
	}

	if err := writer.Write(&protocol.Frame{
		Type:     protocol.FramePeerHello,
		StreamID: protocol.ControlStreamID,
		Payload:  hello.Encode(),
	}); err != nil {
		return nil, fmt.Errorf("failed to send PEER_HELLO: %w", err)
	}

	// Wait for PEER_HELLO_ACK
	frame, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read PEER_HELLO_ACK: %w", err)
	}

	if frame.Type != protocol.FramePeerHelloAck {
		return nil, fmt.Errorf("expected PEER_HELLO_ACK, got frame type 0x%02x", frame.Type)
	}

	// Decode response
	ack, err := protocol.DecodePeerHello(frame.Payload)
	if err != nil {
		return nil, fmt.Errorf("failed to decode PEER_HELLO_ACK: %w", err)
	}

	return &probeHandshakeResult{
		RemoteID:          ack.AgentID,
		RemoteDisplayName: ack.DisplayName,
	}, nil
}

// createTransport creates a transport instance for the given type.
func createTransport(transportType string) (transport.Transport, error) {
	switch transportType {
	case "quic":
		return transport.NewQUICTransport(), nil
	case "h2":
		return transport.NewH2Transport(), nil
	case "ws":
		return transport.NewWebSocketTransport(), nil
	default:
		return nil, fmt.Errorf("unknown transport type: %s", transportType)
	}
}

// buildTLSConfig creates a TLS config from the options.
func buildTLSConfig(opts Options) (*tls.Config, error) {
	config := &tls.Config{
		InsecureSkipVerify: opts.InsecureSkipVerify,
	}

	// Load CA certificate if provided
	if opts.CACert != "" {
		caCert, err := os.ReadFile(opts.CACert)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate: %w", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}

		config.RootCAs = caCertPool
	}

	// Load client certificate if provided (for mTLS)
	if opts.ClientCert != "" && opts.ClientKey != "" {
		cert, err := tls.LoadX509KeyPair(opts.ClientCert, opts.ClientKey)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}
		config.Certificates = []tls.Certificate{cert}
	}

	return config, nil
}

// classifyError returns a human-readable description for common errors.
func classifyError(err error) string {
	if err == nil {
		return ""
	}

	errStr := err.Error()

	// DNS errors
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		if dnsErr.IsNotFound {
			return "Could not resolve hostname - DNS lookup failed"
		}
		return "DNS error: " + dnsErr.Error()
	}

	// Connection errors
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if opErr.Op == "dial" {
			if strings.Contains(errStr, "connection refused") {
				return "Connection refused - listener not running or port blocked"
			}
			if strings.Contains(errStr, "no route to host") {
				return "No route to host - network unreachable"
			}
			if strings.Contains(errStr, "network is unreachable") {
				return "Network unreachable"
			}
		}
	}

	// Timeout errors
	if errors.Is(err, context.DeadlineExceeded) || strings.Contains(errStr, "timeout") || strings.Contains(errStr, "timed out") {
		return "Connection timed out - firewall may be blocking"
	}

	// TLS errors
	if strings.Contains(errStr, "certificate") || strings.Contains(errStr, "tls") || strings.Contains(errStr, "x509") {
		if strings.Contains(errStr, "unknown authority") {
			return "TLS error - certificate signed by unknown authority (try --insecure or --ca)"
		}
		if strings.Contains(errStr, "expired") {
			return "TLS error - certificate has expired"
		}
		return "TLS handshake failed - " + err.Error()
	}

	// Protocol errors
	if strings.Contains(errStr, "PEER_HELLO") || strings.Contains(errStr, "frame type") {
		return "Connected but received invalid response - not a Muti Metroo listener?"
	}

	// Stream/handshake errors
	if strings.Contains(errStr, "failed to open stream") || strings.Contains(errStr, "stream") {
		return "Connected but handshake failed - not a Muti Metroo listener?"
	}

	// Default
	return err.Error()
}
