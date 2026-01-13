package icmp

import (
	"fmt"
	"net"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

// IANA protocol numbers for ICMP
const (
	ICMPv4ProtocolNumber = 1
	ICMPv6ProtocolNumber = 58
)

// Socket wraps an ICMP packet connection with IP version information.
type Socket struct {
	conn   *icmp.PacketConn
	isIPv6 bool
}

// NewSocket creates an ICMP socket for the given IP address.
// For IPv4 addresses, creates an IPv4 ICMP socket.
// For IPv6 addresses, creates an IPv6 ICMP socket.
func NewSocket(destIP net.IP) (*Socket, error) {
	isIPv6 := destIP.To4() == nil

	if isIPv6 {
		return NewSocketV6()
	}
	return NewSocketV4()
}

// NewSocketV4 creates a new unprivileged IPv4 ICMP socket.
// Uses "udp4" network which allows unprivileged ICMP on Linux when
// net.ipv4.ping_group_range sysctl is properly configured.
func NewSocketV4() (*Socket, error) {
	conn, err := icmp.ListenPacket("udp4", "0.0.0.0")
	if err != nil {
		return nil, fmt.Errorf("create ICMP socket: %w", err)
	}
	return &Socket{conn: conn, isIPv6: false}, nil
}

// NewSocketV6 creates a new unprivileged IPv6 ICMP socket.
// Uses "udp6" network which allows unprivileged ICMPv6 on Linux.
func NewSocketV6() (*Socket, error) {
	conn, err := icmp.ListenPacket("udp6", "::")
	if err != nil {
		return nil, fmt.Errorf("create ICMPv6 socket: %w", err)
	}
	return &Socket{conn: conn, isIPv6: true}, nil
}

// Close closes the socket.
func (s *Socket) Close() error {
	return s.conn.Close()
}

// SendEchoRequest sends an ICMP echo request to the destination.
func (s *Socket) SendEchoRequest(destIP net.IP, id, seq uint16, payload []byte) error {
	var msgType icmp.Type
	var destAddr net.Addr

	if s.isIPv6 {
		msgType = ipv6.ICMPTypeEchoRequest
		destAddr = &net.UDPAddr{IP: destIP.To16()}
	} else {
		msgType = ipv4.ICMPTypeEcho
		ip4 := destIP.To4()
		if ip4 == nil {
			return fmt.Errorf("IPv4 socket cannot send to IPv6 address %s", destIP)
		}
		destAddr = &net.UDPAddr{IP: ip4}
	}

	msg := icmp.Message{
		Type: msgType,
		Code: 0,
		Body: &icmp.Echo{
			ID:   int(id),
			Seq:  int(seq),
			Data: payload,
		},
	}

	msgBytes, err := msg.Marshal(nil)
	if err != nil {
		return fmt.Errorf("marshal ICMP message: %w", err)
	}

	_, err = s.conn.WriteTo(msgBytes, destAddr)
	if err != nil {
		return fmt.Errorf("send ICMP: %w", err)
	}

	return nil
}

// EchoReply contains the parsed ICMP echo reply data.
type EchoReply struct {
	ID      uint16
	Seq     uint16
	Payload []byte
	SrcIP   net.IP
}

// ReadEchoReply reads an ICMP echo reply from the socket.
// Returns the reply data or an error if timeout is reached.
func (s *Socket) ReadEchoReply(timeout time.Duration) (*EchoReply, error) {
	if err := s.conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return nil, fmt.Errorf("set read deadline: %w", err)
	}

	buf := make([]byte, 1500)
	n, peer, err := s.conn.ReadFrom(buf)
	if err != nil {
		return nil, err // Timeout or other error
	}

	// Parse ICMP message based on IP version
	var protoNum int
	var expectedType icmp.Type

	if s.isIPv6 {
		protoNum = ICMPv6ProtocolNumber
		expectedType = ipv6.ICMPTypeEchoReply
	} else {
		protoNum = ICMPv4ProtocolNumber
		expectedType = ipv4.ICMPTypeEchoReply
	}

	msg, err := icmp.ParseMessage(protoNum, buf[:n])
	if err != nil {
		return nil, fmt.Errorf("parse ICMP: %w", err)
	}

	// Check for echo reply
	if msg.Type != expectedType {
		return nil, fmt.Errorf("unexpected ICMP type: %v (expected %v)", msg.Type, expectedType)
	}

	echo, ok := msg.Body.(*icmp.Echo)
	if !ok {
		return nil, fmt.Errorf("invalid echo body")
	}

	// Extract source IP
	var srcIP net.IP
	switch addr := peer.(type) {
	case *net.UDPAddr:
		srcIP = addr.IP
	case *net.IPAddr:
		srcIP = addr.IP
	default:
		srcIP = nil
	}

	return &EchoReply{
		ID:      uint16(echo.ID),
		Seq:     uint16(echo.Seq),
		Payload: echo.Data,
		SrcIP:   srcIP,
	}, nil
}

// ReadEchoReplyFiltered reads echo replies with timeout.
// Note: For unprivileged ICMP sockets, we accept any reply since
// each session has its own socket.
func (s *Socket) ReadEchoReplyFiltered(expectedID uint16, timeout time.Duration) (*EchoReply, error) {
	deadline := time.Now().Add(timeout)

	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, fmt.Errorf("timeout waiting for echo reply")
		}

		reply, err := s.ReadEchoReply(remaining)
		if err != nil {
			return nil, err
		}

		// Note: Kernel may assign a different ID for unprivileged sockets.
		// For unprivileged ICMP, we accept any reply on this socket
		// since each session has its own socket.
		return reply, nil
	}
}

// IsIPv6 returns true if this socket is for IPv6.
func (s *Socket) IsIPv6() bool {
	return s.isIPv6
}

// Legacy compatibility functions for existing code

// NewICMPSocket creates a new unprivileged IPv4 ICMP socket.
// Deprecated: Use NewSocketV4() instead.
func NewICMPSocket() (*icmp.PacketConn, error) {
	conn, err := icmp.ListenPacket("udp4", "0.0.0.0")
	if err != nil {
		return nil, fmt.Errorf("create ICMP socket: %w", err)
	}
	return conn, nil
}

// SendEchoRequest sends an IPv4 ICMP echo request.
// Deprecated: Use Socket.SendEchoRequest() instead.
func SendEchoRequest(conn *icmp.PacketConn, destIP net.IP, id, seq uint16, payload []byte) error {
	msg := icmp.Message{
		Type: ipv4.ICMPTypeEcho,
		Code: 0,
		Body: &icmp.Echo{
			ID:   int(id),
			Seq:  int(seq),
			Data: payload,
		},
	}

	msgBytes, err := msg.Marshal(nil)
	if err != nil {
		return fmt.Errorf("marshal ICMP message: %w", err)
	}

	// For unprivileged ICMP sockets, use UDP address
	destAddr := &net.UDPAddr{IP: destIP.To4()}
	_, err = conn.WriteTo(msgBytes, destAddr)
	if err != nil {
		return fmt.Errorf("send ICMP: %w", err)
	}

	return nil
}

// ReadEchoReply reads an IPv4 ICMP echo reply.
// Deprecated: Use Socket.ReadEchoReply() instead.
func ReadEchoReply(conn *icmp.PacketConn, timeout time.Duration) (*EchoReply, error) {
	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return nil, fmt.Errorf("set read deadline: %w", err)
	}

	buf := make([]byte, 1500)
	n, peer, err := conn.ReadFrom(buf)
	if err != nil {
		return nil, err // Timeout or other error
	}

	// Parse ICMP message
	msg, err := icmp.ParseMessage(ICMPv4ProtocolNumber, buf[:n])
	if err != nil {
		return nil, fmt.Errorf("parse ICMP: %w", err)
	}

	// Check for echo reply
	if msg.Type != ipv4.ICMPTypeEchoReply {
		return nil, fmt.Errorf("unexpected ICMP type: %v", msg.Type)
	}

	echo, ok := msg.Body.(*icmp.Echo)
	if !ok {
		return nil, fmt.Errorf("invalid echo body")
	}

	// Extract source IP
	var srcIP net.IP
	switch addr := peer.(type) {
	case *net.UDPAddr:
		srcIP = addr.IP
	case *net.IPAddr:
		srcIP = addr.IP
	default:
		srcIP = nil
	}

	return &EchoReply{
		ID:      uint16(echo.ID),
		Seq:     uint16(echo.Seq),
		Payload: echo.Data,
		SrcIP:   srcIP,
	}, nil
}

// ReadEchoReplyFiltered reads IPv4 echo replies with timeout.
// Deprecated: Use Socket.ReadEchoReplyFiltered() instead.
func ReadEchoReplyFiltered(conn *icmp.PacketConn, expectedID uint16, timeout time.Duration) (*EchoReply, error) {
	deadline := time.Now().Add(timeout)

	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, fmt.Errorf("timeout waiting for echo reply")
		}

		reply, err := ReadEchoReply(conn, remaining)
		if err != nil {
			return nil, err
		}

		// Note: Kernel may assign a different ID for unprivileged sockets.
		// For unprivileged ICMP, we accept any reply on this socket
		// since each session has its own socket.
		return reply, nil
	}
}
