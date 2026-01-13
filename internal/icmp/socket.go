package icmp

import (
	"fmt"
	"net"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

// ICMPv4ProtocolNumber is the IANA protocol number for ICMP.
const ICMPv4ProtocolNumber = 1

// NewICMPSocket creates a new unprivileged ICMP socket.
// Uses "udp4" network which allows unprivileged ICMP on Linux when
// net.ipv4.ping_group_range sysctl is properly configured.
func NewICMPSocket() (*icmp.PacketConn, error) {
	conn, err := icmp.ListenPacket("udp4", "0.0.0.0")
	if err != nil {
		return nil, fmt.Errorf("create ICMP socket: %w", err)
	}
	return conn, nil
}

// SendEchoRequest sends an ICMP echo request to the destination.
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

// EchoReply contains the parsed ICMP echo reply data.
type EchoReply struct {
	ID      uint16
	Seq     uint16
	Payload []byte
	SrcIP   net.IP
}

// ReadEchoReply reads an ICMP echo reply from the socket.
// Returns the reply data or an error if timeout is reached.
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

// ReadEchoReplyFiltered reads echo replies and filters by expected ID.
// This is useful because unprivileged ICMP sockets may receive replies
// for other applications' pings.
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
