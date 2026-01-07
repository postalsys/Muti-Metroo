package socks5

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"
)

// SOCKS5 protocol constants per RFC 1928.
const (
	SOCKS5Version = 0x05
)

// Command types.
const (
	CmdConnect      = 0x01
	CmdBind         = 0x02
	CmdUDPAssociate = 0x03
)

// Address types.
const (
	AddrTypeIPv4   = 0x01
	AddrTypeDomain = 0x03
	AddrTypeIPv6   = 0x04
)

// Reply codes.
const (
	ReplySucceeded          = 0x00
	ReplyServerFailure      = 0x01
	ReplyNotAllowed         = 0x02
	ReplyNetworkUnreachable = 0x03
	ReplyHostUnreachable    = 0x04
	ReplyConnectionRefused  = 0x05
	ReplyTTLExpired         = 0x06
	ReplyCmdNotSupported    = 0x07
	ReplyAddrNotSupported   = 0x08
)

// halfCloser is implemented by connections that support half-close (TCP, mesh connections).
// This allows signaling that one direction is done while keeping the other open.
type halfCloser interface {
	CloseWrite() error
}

// Request represents a SOCKS5 request.
type Request struct {
	Version  byte
	Command  byte
	AddrType byte
	DestAddr string
	DestPort uint16
	DestIP   net.IP
	RawDest  []byte // Raw destination bytes for forwarding
}

// Handler processes SOCKS5 connections.
type Handler struct {
	authenticators []Authenticator
	dialer         Dialer

	// UDP support
	udpHandler      UDPAssociationHandler
	udpBindIP       net.IP // IP to bind UDP relay sockets (inherited from SOCKS5 listener)
	udpAssocMu      sync.Mutex
	udpAssociations map[uint64]*UDPAssociation
}

// Dialer interface for making outbound connections.
type Dialer interface {
	Dial(network, address string) (net.Conn, error)
}

// DirectDialer connects directly to destinations.
type DirectDialer struct{}

// Dial makes a direct TCP connection.
func (d *DirectDialer) Dial(network, address string) (net.Conn, error) {
	return net.Dial(network, address)
}

// NewHandler creates a new SOCKS5 handler.
func NewHandler(auths []Authenticator, dialer Dialer) *Handler {
	if dialer == nil {
		dialer = &DirectDialer{}
	}
	if len(auths) == 0 {
		auths = []Authenticator{&NoAuthAuthenticator{}}
	}
	return &Handler{
		authenticators:  auths,
		dialer:          dialer,
		udpAssociations: make(map[uint64]*UDPAssociation),
	}
}

// SetUDPHandler sets the UDP association handler.
// This must be called before handling UDP ASSOCIATE requests.
func (h *Handler) SetUDPHandler(handler UDPAssociationHandler) {
	h.udpHandler = handler
}

// SetUDPBindIP sets the IP address for UDP relay sockets.
// This should match the SOCKS5 TCP listener's bind address.
func (h *Handler) SetUDPBindIP(ip net.IP) {
	h.udpBindIP = ip
}

// Handle processes a SOCKS5 connection.
func (h *Handler) Handle(conn net.Conn) error {
	// Perform authentication
	_, err := h.authenticate(conn)
	if err != nil {
		return fmt.Errorf("authentication: %w", err)
	}

	// Read the request
	req, err := h.readRequest(conn)
	if err != nil {
		return fmt.Errorf("read request: %w", err)
	}

	// Dispatch based on command
	switch req.Command {
	case CmdConnect:
		return h.handleConnect(conn, req)
	case CmdUDPAssociate:
		return h.handleUDPAssociate(conn, req)
	default:
		h.sendReply(conn, ReplyCmdNotSupported, nil, 0)
		return fmt.Errorf("unsupported command: %d", req.Command)
	}
}

// handleConnect handles CONNECT commands.
func (h *Handler) handleConnect(conn net.Conn, req *Request) error {
	// Connect to destination
	targetAddr := net.JoinHostPort(req.DestAddr, strconv.Itoa(int(req.DestPort)))
	target, err := h.dialer.Dial("tcp", targetAddr)
	if err != nil {
		// Map error to appropriate reply
		h.sendReplyForError(conn, err)
		return fmt.Errorf("dial %s: %w", targetAddr, err)
	}
	defer target.Close()

	// Get local address for reply
	localAddr := target.LocalAddr().(*net.TCPAddr)
	h.sendReply(conn, ReplySucceeded, localAddr.IP, uint16(localAddr.Port))

	// Clear deadlines before relay - connections should stay open indefinitely
	// The deadline was set for the handshake phase only
	conn.SetDeadline(time.Time{})
	target.SetDeadline(time.Time{})

	// Bidirectional relay
	return relay(conn, target)
}

// handleUDPAssociate handles UDP ASSOCIATE commands (RFC 1928 Section 4).
// Creates a UDP relay socket and manages the association lifetime.
func (h *Handler) handleUDPAssociate(conn net.Conn, req *Request) error {
	// Check if UDP is enabled
	if h.udpHandler == nil || !h.udpHandler.IsUDPEnabled() {
		h.sendReply(conn, ReplyCmdNotSupported, nil, 0)
		return ErrUDPDisabled
	}

	// Parse expected client address from request
	// The client MAY specify the address/port it will use, or 0.0.0.0:0
	var expectedClient *net.UDPAddr
	if req.DestIP != nil && !req.DestIP.IsUnspecified() {
		expectedClient = &net.UDPAddr{
			IP:   req.DestIP,
			Port: int(req.DestPort),
		}
	}

	// Create UDP association
	// Use the configured bind IP (inherited from SOCKS5 TCP listener)
	assoc, err := NewUDPAssociation(conn, h.udpHandler, h.udpBindIP)
	if err != nil {
		h.sendReply(conn, ReplyServerFailure, nil, 0)
		return fmt.Errorf("create UDP association: %w", err)
	}

	// Set expected client address
	if expectedClient != nil {
		assoc.SetExpectedClientAddr(expectedClient)
	}

	// Create association in mesh (get stream ID)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	streamID, err := h.udpHandler.CreateUDPAssociation(ctx, expectedClient)
	if err != nil {
		assoc.Close()
		h.sendReply(conn, ReplyServerFailure, nil, 0)
		return fmt.Errorf("create mesh association: %w", err)
	}
	assoc.SetStreamID(streamID)

	// Link the SOCKS5 association to the ingress stream for responses
	h.udpHandler.SetSOCKS5UDPAssociation(streamID, assoc)

	// Track the association
	h.udpAssocMu.Lock()
	h.udpAssociations[streamID] = assoc
	h.udpAssocMu.Unlock()

	// Send success reply with relay address
	// Use the TCP connection's local IP (the IP the client connected to)
	// rather than 0.0.0.0 which the client can't send to
	relayAddr := assoc.LocalAddr()
	var replyIP net.IP
	if tcpLocal, ok := conn.LocalAddr().(*net.TCPAddr); ok && !tcpLocal.IP.IsUnspecified() {
		replyIP = tcpLocal.IP
	} else {
		// Fallback to 127.0.0.1 if we can't determine the IP
		replyIP = net.IPv4(127, 0, 0, 1)
	}
	h.sendReply(conn, ReplySucceeded, replyIP, uint16(relayAddr.Port))

	// Clear deadlines
	conn.SetDeadline(time.Time{})

	// Start reading from UDP socket
	go assoc.ReadLoop()

	// Wait for TCP control connection to close
	// Per RFC 1928: "A UDP association terminates when the TCP connection
	// that the UDP ASSOCIATE request arrived terminates."
	buf := make([]byte, 1)
	for {
		_, err := conn.Read(buf)
		if err != nil {
			break
		}
	}

	// Clean up
	h.udpAssocMu.Lock()
	delete(h.udpAssociations, streamID)
	h.udpAssocMu.Unlock()

	assoc.Close()
	return nil
}

// GetUDPAssociation returns a UDP association by stream ID.
func (h *Handler) GetUDPAssociation(streamID uint64) *UDPAssociation {
	h.udpAssocMu.Lock()
	defer h.udpAssocMu.Unlock()
	return h.udpAssociations[streamID]
}

// authenticate performs the authentication handshake.
func (h *Handler) authenticate(conn net.Conn) (string, error) {
	// Read the greeting
	// +----+----------+----------+
	// |VER | NMETHODS | METHODS  |
	// +----+----------+----------+
	// | 1  |    1     | 1 to 255 |
	// +----+----------+----------+

	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil {
		return "", err
	}

	if header[0] != SOCKS5Version {
		return "", fmt.Errorf("unsupported SOCKS version: %d", header[0])
	}

	numMethods := int(header[1])
	methods := make([]byte, numMethods)
	if _, err := io.ReadFull(conn, methods); err != nil {
		return "", err
	}

	// Select authentication method
	var selectedAuth Authenticator
	for _, auth := range h.authenticators {
		for _, m := range methods {
			if m == auth.GetMethod() {
				selectedAuth = auth
				break
			}
		}
		if selectedAuth != nil {
			break
		}
	}

	if selectedAuth == nil {
		// No acceptable method
		conn.Write([]byte{SOCKS5Version, AuthMethodNoAcceptable})
		return "", errors.New("no acceptable authentication method")
	}

	// Send method selection
	// +----+--------+
	// |VER | METHOD |
	// +----+--------+
	// | 1  |   1    |
	// +----+--------+
	if _, err := conn.Write([]byte{SOCKS5Version, selectedAuth.GetMethod()}); err != nil {
		return "", err
	}

	// Perform authentication
	return selectedAuth.Authenticate(conn, conn)
}

// readRequest reads the SOCKS5 request.
func (h *Handler) readRequest(conn net.Conn) (*Request, error) {
	// +----+-----+-------+------+----------+----------+
	// |VER | CMD |  RSV  | ATYP | DST.ADDR | DST.PORT |
	// +----+-----+-------+------+----------+----------+
	// | 1  |  1  | X'00' |  1   | Variable |    2     |
	// +----+-----+-------+------+----------+----------+

	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return nil, err
	}

	if header[0] != SOCKS5Version {
		return nil, fmt.Errorf("unsupported SOCKS version: %d", header[0])
	}

	req := &Request{
		Version:  header[0],
		Command:  header[1],
		AddrType: header[3],
	}

	// Read destination address based on type
	switch req.AddrType {
	case AddrTypeIPv4:
		addr := make([]byte, 4)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return nil, err
		}
		req.DestIP = net.IP(addr)
		req.DestAddr = req.DestIP.String()
		req.RawDest = addr

	case AddrTypeDomain:
		// Read domain length
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(conn, lenBuf); err != nil {
			return nil, err
		}
		domainLen := int(lenBuf[0])
		if domainLen == 0 {
			h.sendReply(conn, ReplyServerFailure, nil, 0)
			return nil, fmt.Errorf("invalid zero-length domain name")
		}
		domain := make([]byte, domainLen)
		if _, err := io.ReadFull(conn, domain); err != nil {
			return nil, err
		}
		req.DestAddr = string(domain)
		req.RawDest = append(lenBuf, domain...)

	case AddrTypeIPv6:
		addr := make([]byte, 16)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return nil, err
		}
		req.DestIP = net.IP(addr)
		req.DestAddr = req.DestIP.String()
		req.RawDest = addr

	default:
		h.sendReply(conn, ReplyAddrNotSupported, nil, 0)
		return nil, fmt.Errorf("unsupported address type: %d", req.AddrType)
	}

	// Read port
	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBuf); err != nil {
		return nil, err
	}
	req.DestPort = binary.BigEndian.Uint16(portBuf)

	return req, nil
}

// sendReply sends a SOCKS5 reply.
func (h *Handler) sendReply(conn net.Conn, reply byte, bindIP net.IP, bindPort uint16) error {
	// +----+-----+-------+------+----------+----------+
	// |VER | REP |  RSV  | ATYP | BND.ADDR | BND.PORT |
	// +----+-----+-------+------+----------+----------+
	// | 1  |  1  | X'00' |  1   | Variable |    2     |
	// +----+-----+-------+------+----------+----------+

	var buf []byte

	if bindIP == nil {
		// Use all zeros for the bind address
		buf = make([]byte, 10)
		buf[0] = SOCKS5Version
		buf[1] = reply
		buf[2] = 0x00 // RSV
		buf[3] = AddrTypeIPv4
		// Bytes 4-7 are zeros (0.0.0.0)
		binary.BigEndian.PutUint16(buf[8:], bindPort)
	} else if ipv4 := bindIP.To4(); ipv4 != nil {
		buf = make([]byte, 10)
		buf[0] = SOCKS5Version
		buf[1] = reply
		buf[2] = 0x00 // RSV
		buf[3] = AddrTypeIPv4
		copy(buf[4:8], ipv4)
		binary.BigEndian.PutUint16(buf[8:], bindPort)
	} else {
		buf = make([]byte, 22)
		buf[0] = SOCKS5Version
		buf[1] = reply
		buf[2] = 0x00 // RSV
		buf[3] = AddrTypeIPv6
		copy(buf[4:20], bindIP)
		binary.BigEndian.PutUint16(buf[20:], bindPort)
	}

	_, err := conn.Write(buf)
	return err
}

// sendReplyForError maps network errors to SOCKS5 reply codes.
func (h *Handler) sendReplyForError(conn net.Conn, err error) {
	var reply byte = ReplyServerFailure

	if netErr, ok := err.(*net.OpError); ok {
		switch {
		case netErr.Timeout():
			reply = ReplyTTLExpired
		default:
			if netErr.Op == "dial" {
				reply = ReplyHostUnreachable
			}
		}
	}

	// Check for specific error types
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		reply = ReplyHostUnreachable
	}

	h.sendReply(conn, reply, nil, 0)
}

// relay copies data bidirectionally between two connections.
// Supports half-close on any connection type that implements halfCloser interface.
func relay(client, target net.Conn) error {
	errCh := make(chan error, 2)

	go func() {
		_, err := io.Copy(target, client)
		// Signal target that we're done writing (half-close)
		if hc, ok := target.(halfCloser); ok {
			hc.CloseWrite()
		}
		errCh <- err
	}()

	go func() {
		_, err := io.Copy(client, target)
		// Signal client that we're done writing (half-close)
		if hc, ok := client.(halfCloser); ok {
			hc.CloseWrite()
		}
		errCh <- err
	}()

	// Wait for both directions to complete
	err1 := <-errCh
	err2 := <-errCh

	if err1 != nil {
		return err1
	}
	return err2
}
