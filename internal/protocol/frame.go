package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"

	"github.com/postalsys/muti-metroo/internal/identity"
)

var (
	// ErrFrameTooLarge is returned when a frame exceeds the maximum size
	ErrFrameTooLarge = errors.New("frame payload exceeds maximum size")

	// ErrInvalidFrame is returned when a frame is malformed
	ErrInvalidFrame = errors.New("invalid frame")

	// ErrUnknownFrameType is returned for unrecognized frame types
	ErrUnknownFrameType = errors.New("unknown frame type")
)

// Frame represents a wire protocol frame.
// Header format (14 bytes):
//
//	Type     [1 byte]  - Frame type
//	Flags    [1 byte]  - Frame flags
//	Length   [4 bytes] - Payload length (big-endian)
//	StreamID [8 bytes] - Stream identifier (big-endian)
type Frame struct {
	Type     uint8
	Flags    uint8
	StreamID uint64
	Payload  []byte
}

// Encode serializes the frame to bytes.
func (f *Frame) Encode() ([]byte, error) {
	if len(f.Payload) > MaxPayloadSize {
		return nil, ErrFrameTooLarge
	}

	buf := make([]byte, HeaderSize+len(f.Payload))

	// Header
	buf[0] = f.Type
	buf[1] = f.Flags
	binary.BigEndian.PutUint32(buf[2:6], uint32(len(f.Payload)))
	binary.BigEndian.PutUint64(buf[6:14], f.StreamID)

	// Payload
	copy(buf[14:], f.Payload)

	return buf, nil
}

// DecodeHeader decodes a frame header from bytes.
func DecodeHeader(buf []byte) (frameType uint8, flags uint8, length uint32, streamID uint64, err error) {
	if len(buf) < HeaderSize {
		return 0, 0, 0, 0, fmt.Errorf("%w: header too short", ErrInvalidFrame)
	}

	frameType = buf[0]
	flags = buf[1]
	length = binary.BigEndian.Uint32(buf[2:6])
	streamID = binary.BigEndian.Uint64(buf[6:14])

	if length > MaxPayloadSize {
		return 0, 0, 0, 0, ErrFrameTooLarge
	}

	return
}

// Decode deserializes a frame from bytes.
func Decode(buf []byte) (*Frame, error) {
	frameType, flags, length, streamID, err := DecodeHeader(buf)
	if err != nil {
		return nil, err
	}

	if len(buf) < HeaderSize+int(length) {
		return nil, fmt.Errorf("%w: buffer too short for payload", ErrInvalidFrame)
	}

	payload := make([]byte, length)
	copy(payload, buf[HeaderSize:HeaderSize+length])

	return &Frame{
		Type:     frameType,
		Flags:    flags,
		StreamID: streamID,
		Payload:  payload,
	}, nil
}

// String returns a debug representation of the frame.
func (f *Frame) String() string {
	return fmt.Sprintf("Frame{Type=%s, Flags=0x%02x, StreamID=%d, PayloadLen=%d}",
		FrameTypeName(f.Type), f.Flags, f.StreamID, len(f.Payload))
}

// ============================================================================
// Payload structures
// ============================================================================

// PeerHello is the payload for PEER_HELLO and PEER_HELLO_ACK frames.
type PeerHello struct {
	Version      uint16
	AgentID      identity.AgentID
	Timestamp    uint64
	Capabilities []string
	DisplayName  string // Added for topology visualization
}

// Encode serializes PeerHello to bytes.
func (p *PeerHello) Encode() []byte {
	// Calculate size
	// version(2) + agentID(16) + timestamp(8) + displayNameLen(1) + displayName + capLen(1) + caps
	size := 2 + 16 + 8 + 1 + len(p.DisplayName) + 1
	for _, cap := range p.Capabilities {
		size += 1 + len(cap)
	}

	buf := make([]byte, size)
	offset := 0

	binary.BigEndian.PutUint16(buf[offset:], p.Version)
	offset += 2

	copy(buf[offset:], p.AgentID[:])
	offset += 16

	binary.BigEndian.PutUint64(buf[offset:], p.Timestamp)
	offset += 8

	// DisplayName (length-prefixed string)
	buf[offset] = uint8(len(p.DisplayName))
	offset++
	copy(buf[offset:], p.DisplayName)
	offset += len(p.DisplayName)

	buf[offset] = uint8(len(p.Capabilities))
	offset++

	for _, cap := range p.Capabilities {
		buf[offset] = uint8(len(cap))
		offset++
		copy(buf[offset:], cap)
		offset += len(cap)
	}

	return buf
}

// DecodePeerHello deserializes PeerHello from bytes.
func DecodePeerHello(buf []byte) (*PeerHello, error) {
	if len(buf) < 28 { // 2 + 16 + 8 + 1 + 1 (min: empty displayName + capLen)
		return nil, fmt.Errorf("%w: PeerHello too short", ErrInvalidFrame)
	}

	p := &PeerHello{}
	offset := 0

	p.Version = binary.BigEndian.Uint16(buf[offset:])
	offset += 2

	copy(p.AgentID[:], buf[offset:offset+16])
	offset += 16

	p.Timestamp = binary.BigEndian.Uint64(buf[offset:])
	offset += 8

	// DisplayName
	displayNameLen := int(buf[offset])
	offset++
	if offset+displayNameLen > len(buf) {
		return nil, fmt.Errorf("%w: PeerHello displayName truncated", ErrInvalidFrame)
	}
	p.DisplayName = string(buf[offset : offset+displayNameLen])
	offset += displayNameLen

	if offset >= len(buf) {
		return nil, fmt.Errorf("%w: PeerHello capabilities truncated", ErrInvalidFrame)
	}

	capLen := int(buf[offset])
	offset++

	p.Capabilities = make([]string, 0, capLen)
	for i := 0; i < capLen; i++ {
		if offset >= len(buf) {
			return nil, fmt.Errorf("%w: PeerHello capabilities truncated", ErrInvalidFrame)
		}
		strLen := int(buf[offset])
		offset++
		if offset+strLen > len(buf) {
			return nil, fmt.Errorf("%w: PeerHello capability string truncated", ErrInvalidFrame)
		}
		p.Capabilities = append(p.Capabilities, string(buf[offset:offset+strLen]))
		offset += strLen
	}

	return p, nil
}

// EphemeralKeySize is the size of X25519 ephemeral public keys.
const EphemeralKeySize = 32

// StreamOpen is the payload for STREAM_OPEN frames.
type StreamOpen struct {
	RequestID       uint64
	AddressType     uint8
	Address         []byte // IPv4 (4), IPv6 (16), or domain (1+N)
	Port            uint16
	TTL             uint8
	RemainingPath   []identity.AgentID
	EphemeralPubKey [EphemeralKeySize]byte // Initiator's ephemeral public key for E2E encryption
}

// Encode serializes StreamOpen to bytes.
func (s *StreamOpen) Encode() []byte {
	size := 8 + 1 + len(s.Address) + 2 + 1 + 1 + len(s.RemainingPath)*16 + EphemeralKeySize
	buf := make([]byte, size)
	offset := 0

	binary.BigEndian.PutUint64(buf[offset:], s.RequestID)
	offset += 8

	buf[offset] = s.AddressType
	offset++

	copy(buf[offset:], s.Address)
	offset += len(s.Address)

	binary.BigEndian.PutUint16(buf[offset:], s.Port)
	offset += 2

	buf[offset] = s.TTL
	offset++

	buf[offset] = uint8(len(s.RemainingPath))
	offset++

	for _, agentID := range s.RemainingPath {
		copy(buf[offset:], agentID[:])
		offset += 16
	}

	// Append ephemeral public key
	copy(buf[offset:], s.EphemeralPubKey[:])

	return buf
}

// DecodeStreamOpen deserializes StreamOpen from bytes.
func DecodeStreamOpen(buf []byte) (*StreamOpen, error) {
	if len(buf) < 13+EphemeralKeySize { // 8 + 1 + 2 + 1 + 1 + 32 (minimum with key)
		return nil, fmt.Errorf("%w: StreamOpen too short", ErrInvalidFrame)
	}

	s := &StreamOpen{}
	offset := 0

	s.RequestID = binary.BigEndian.Uint64(buf[offset:])
	offset += 8

	s.AddressType = buf[offset]
	offset++

	var addrLen int
	switch s.AddressType {
	case AddrTypeIPv4:
		addrLen = 4
	case AddrTypeIPv6:
		addrLen = 16
	case AddrTypeDomain:
		if offset >= len(buf) {
			return nil, fmt.Errorf("%w: StreamOpen domain length missing", ErrInvalidFrame)
		}
		addrLen = 1 + int(buf[offset])
	default:
		return nil, fmt.Errorf("%w: unknown address type %d", ErrInvalidFrame, s.AddressType)
	}

	if offset+addrLen > len(buf) {
		return nil, fmt.Errorf("%w: StreamOpen address truncated", ErrInvalidFrame)
	}
	s.Address = make([]byte, addrLen)
	copy(s.Address, buf[offset:offset+addrLen])
	offset += addrLen

	if offset+4 > len(buf) {
		return nil, fmt.Errorf("%w: StreamOpen port/TTL missing", ErrInvalidFrame)
	}

	s.Port = binary.BigEndian.Uint16(buf[offset:])
	offset += 2

	s.TTL = buf[offset]
	offset++

	pathLen := int(buf[offset])
	offset++

	s.RemainingPath = make([]identity.AgentID, pathLen)
	for i := 0; i < pathLen; i++ {
		if offset+16 > len(buf) {
			return nil, fmt.Errorf("%w: StreamOpen path truncated", ErrInvalidFrame)
		}
		copy(s.RemainingPath[i][:], buf[offset:offset+16])
		offset += 16
	}

	// Read ephemeral public key
	if offset+EphemeralKeySize > len(buf) {
		return nil, fmt.Errorf("%w: StreamOpen ephemeral key missing", ErrInvalidFrame)
	}
	copy(s.EphemeralPubKey[:], buf[offset:offset+EphemeralKeySize])

	return s, nil
}

// GetDestinationIP returns the destination IP for IPv4/IPv6 addresses.
func (s *StreamOpen) GetDestinationIP() net.IP {
	switch s.AddressType {
	case AddrTypeIPv4:
		return net.IP(s.Address)
	case AddrTypeIPv6:
		return net.IP(s.Address)
	default:
		return nil
	}
}

// GetDestinationDomain returns the domain name for domain type addresses.
func (s *StreamOpen) GetDestinationDomain() string {
	if s.AddressType == AddrTypeDomain && len(s.Address) > 1 {
		return string(s.Address[1:])
	}
	return ""
}

// StreamOpenAck is the payload for STREAM_OPEN_ACK frames.
type StreamOpenAck struct {
	RequestID       uint64
	BoundAddrType   uint8
	BoundAddr       []byte
	BoundPort       uint16
	EphemeralPubKey [EphemeralKeySize]byte // Responder's ephemeral public key for E2E encryption
}

// Encode serializes StreamOpenAck to bytes.
func (s *StreamOpenAck) Encode() []byte {
	buf := make([]byte, 8+1+len(s.BoundAddr)+2+EphemeralKeySize)
	offset := 0

	binary.BigEndian.PutUint64(buf[offset:], s.RequestID)
	offset += 8

	buf[offset] = s.BoundAddrType
	offset++

	copy(buf[offset:], s.BoundAddr)
	offset += len(s.BoundAddr)

	binary.BigEndian.PutUint16(buf[offset:], s.BoundPort)
	offset += 2

	// Append ephemeral public key
	copy(buf[offset:], s.EphemeralPubKey[:])

	return buf
}

// DecodeStreamOpenAck deserializes StreamOpenAck from bytes.
func DecodeStreamOpenAck(buf []byte) (*StreamOpenAck, error) {
	if len(buf) < 12+EphemeralKeySize { // 8 + 1 + 1 + 2 + 32 minimum (empty addr + key)
		return nil, fmt.Errorf("%w: StreamOpenAck too short", ErrInvalidFrame)
	}

	s := &StreamOpenAck{}
	offset := 0

	s.RequestID = binary.BigEndian.Uint64(buf[offset:])
	offset += 8

	s.BoundAddrType = buf[offset]
	offset++

	var addrLen int
	switch s.BoundAddrType {
	case AddrTypeIPv4:
		addrLen = 4
	case AddrTypeIPv6:
		addrLen = 16
	default:
		addrLen = 0
	}

	if offset+addrLen+2+EphemeralKeySize > len(buf) {
		return nil, fmt.Errorf("%w: StreamOpenAck address truncated", ErrInvalidFrame)
	}

	s.BoundAddr = make([]byte, addrLen)
	copy(s.BoundAddr, buf[offset:offset+addrLen])
	offset += addrLen

	s.BoundPort = binary.BigEndian.Uint16(buf[offset:])
	offset += 2

	// Read ephemeral public key
	copy(s.EphemeralPubKey[:], buf[offset:offset+EphemeralKeySize])

	return s, nil
}

// StreamOpenErr is the payload for STREAM_OPEN_ERR frames.
type StreamOpenErr struct {
	RequestID uint64
	ErrorCode uint16
	Message   string
}

// Encode serializes StreamOpenErr to bytes.
func (s *StreamOpenErr) Encode() []byte {
	msgBytes := []byte(s.Message)
	if len(msgBytes) > 255 {
		msgBytes = msgBytes[:255]
	}

	buf := make([]byte, 8+2+1+len(msgBytes))
	offset := 0

	binary.BigEndian.PutUint64(buf[offset:], s.RequestID)
	offset += 8

	binary.BigEndian.PutUint16(buf[offset:], s.ErrorCode)
	offset += 2

	buf[offset] = uint8(len(msgBytes))
	offset++

	copy(buf[offset:], msgBytes)

	return buf
}

// DecodeStreamOpenErr deserializes StreamOpenErr from bytes.
func DecodeStreamOpenErr(buf []byte) (*StreamOpenErr, error) {
	if len(buf) < 11 { // 8 + 2 + 1
		return nil, fmt.Errorf("%w: StreamOpenErr too short", ErrInvalidFrame)
	}

	s := &StreamOpenErr{}
	offset := 0

	s.RequestID = binary.BigEndian.Uint64(buf[offset:])
	offset += 8

	s.ErrorCode = binary.BigEndian.Uint16(buf[offset:])
	offset += 2

	msgLen := int(buf[offset])
	offset++

	if offset+msgLen > len(buf) {
		return nil, fmt.Errorf("%w: StreamOpenErr message truncated", ErrInvalidFrame)
	}

	s.Message = string(buf[offset : offset+msgLen])

	return s, nil
}

// StreamReset is the payload for STREAM_RESET frames.
type StreamReset struct {
	ErrorCode uint16
}

// Encode serializes StreamReset to bytes.
func (s *StreamReset) Encode() []byte {
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, s.ErrorCode)
	return buf
}

// DecodeStreamReset deserializes StreamReset from bytes.
func DecodeStreamReset(buf []byte) (*StreamReset, error) {
	if len(buf) < 2 {
		return nil, fmt.Errorf("%w: StreamReset too short", ErrInvalidFrame)
	}
	return &StreamReset{
		ErrorCode: binary.BigEndian.Uint16(buf),
	}, nil
}

// Keepalive is the payload for KEEPALIVE and KEEPALIVE_ACK frames.
type Keepalive struct {
	Timestamp uint64
}

// Encode serializes Keepalive to bytes.
func (k *Keepalive) Encode() []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, k.Timestamp)
	return buf
}

// DecodeKeepalive deserializes Keepalive from bytes.
func DecodeKeepalive(buf []byte) (*Keepalive, error) {
	if len(buf) < 8 {
		return nil, fmt.Errorf("%w: Keepalive too short", ErrInvalidFrame)
	}
	return &Keepalive{
		Timestamp: binary.BigEndian.Uint64(buf),
	}, nil
}

// Route represents a single route in ROUTE_ADVERTISE/WITHDRAW.
type Route struct {
	AddressFamily uint8
	PrefixLength  uint8
	Prefix        []byte // 4 or 16 bytes
	Metric        uint16
}

// Encode serializes Route to bytes.
func (r *Route) Encode() []byte {
	buf := make([]byte, 2+len(r.Prefix)+2)
	buf[0] = r.AddressFamily
	buf[1] = r.PrefixLength
	copy(buf[2:], r.Prefix)
	binary.BigEndian.PutUint16(buf[2+len(r.Prefix):], r.Metric)
	return buf
}

// RouteAdvertise is the payload for ROUTE_ADVERTISE frames.
type RouteAdvertise struct {
	OriginAgent       identity.AgentID
	OriginDisplayName string // Display name of the origin agent for topology visualization
	Sequence          uint64
	Routes            []Route
	Path              []identity.AgentID // Route path (may be decrypted from EncPath)
	EncPath           *EncryptedData     // Encrypted path data (nil if not using encryption)
	SeenBy            []identity.AgentID
}

// Encode serializes RouteAdvertise to bytes.
// Format with encryption support:
//   origin(16) + displayNameLen(1) + displayName + seq(8) + routeCount(1) + routes +
//   EncryptedData(flag+len+path) + seenByLen(1) + seenBy
func (r *RouteAdvertise) Encode() []byte {
	// Prepare path data (encrypted or plaintext)
	var encPath *EncryptedData
	if r.EncPath != nil {
		encPath = r.EncPath
	} else {
		encPath = &EncryptedData{
			Encrypted: false,
			Data:      EncodePath(r.Path),
		}
	}
	encPathBytes := EncodeEncryptedData(encPath)

	// Calculate size
	size := 16 + 1 + len(r.OriginDisplayName) + 8 + 1
	for _, route := range r.Routes {
		if route.AddressFamily == AddrFamilyIPv4 {
			size += 2 + 4 + 2 // family + prefix + metric
		} else {
			size += 2 + 16 + 2
		}
	}
	size += len(encPathBytes)        // encrypted path data
	size += 1 + len(r.SeenBy)*16     // seenByLen + seenBy

	buf := make([]byte, size)
	offset := 0

	copy(buf[offset:], r.OriginAgent[:])
	offset += 16

	// OriginDisplayName (length-prefixed string)
	buf[offset] = uint8(len(r.OriginDisplayName))
	offset++
	copy(buf[offset:], r.OriginDisplayName)
	offset += len(r.OriginDisplayName)

	binary.BigEndian.PutUint64(buf[offset:], r.Sequence)
	offset += 8

	buf[offset] = uint8(len(r.Routes))
	offset++

	for _, route := range r.Routes {
		buf[offset] = route.AddressFamily
		offset++
		buf[offset] = route.PrefixLength
		offset++
		prefixLen := 4
		if route.AddressFamily == AddrFamilyIPv6 {
			prefixLen = 16
		}
		copy(buf[offset:], route.Prefix[:prefixLen])
		offset += prefixLen
		binary.BigEndian.PutUint16(buf[offset:], route.Metric)
		offset += 2
	}

	// Encrypted path data
	copy(buf[offset:], encPathBytes)
	offset += len(encPathBytes)

	buf[offset] = uint8(len(r.SeenBy))
	offset++
	for _, id := range r.SeenBy {
		copy(buf[offset:], id[:])
		offset += 16
	}

	return buf[:offset]
}

// DecodeRouteAdvertise deserializes RouteAdvertise from bytes.
// Supports new format with encrypted path:
//   origin(16) + displayNameLen(1) + displayName + seq(8) + routeCount(1) + routes +
//   EncryptedData(flag+len+path) + seenByLen(1) + seenBy
func DecodeRouteAdvertise(buf []byte) (*RouteAdvertise, error) {
	if len(buf) < 28 { // Minimum size
		return nil, fmt.Errorf("%w: RouteAdvertise too short", ErrInvalidFrame)
	}

	r := &RouteAdvertise{}
	offset := 0

	copy(r.OriginAgent[:], buf[offset:offset+16])
	offset += 16

	// OriginDisplayName (length-prefixed string)
	displayNameLen := int(buf[offset])
	offset++
	if offset+displayNameLen > len(buf) {
		return nil, fmt.Errorf("%w: RouteAdvertise displayName truncated", ErrInvalidFrame)
	}
	r.OriginDisplayName = string(buf[offset : offset+displayNameLen])
	offset += displayNameLen

	if offset+8 > len(buf) {
		return nil, fmt.Errorf("%w: RouteAdvertise sequence truncated", ErrInvalidFrame)
	}
	r.Sequence = binary.BigEndian.Uint64(buf[offset:])
	offset += 8

	if offset >= len(buf) {
		return nil, fmt.Errorf("%w: RouteAdvertise routes missing", ErrInvalidFrame)
	}
	routeCount := int(buf[offset])
	offset++

	r.Routes = make([]Route, routeCount)
	for i := 0; i < routeCount; i++ {
		if offset+2 > len(buf) {
			return nil, fmt.Errorf("%w: RouteAdvertise routes truncated", ErrInvalidFrame)
		}
		route := &r.Routes[i]
		route.AddressFamily = buf[offset]
		offset++
		route.PrefixLength = buf[offset]
		offset++

		prefixLen := 4
		if route.AddressFamily == AddrFamilyIPv6 {
			prefixLen = 16
		}
		if offset+prefixLen+2 > len(buf) {
			return nil, fmt.Errorf("%w: RouteAdvertise route prefix truncated", ErrInvalidFrame)
		}
		route.Prefix = make([]byte, prefixLen)
		copy(route.Prefix, buf[offset:offset+prefixLen])
		offset += prefixLen
		route.Metric = binary.BigEndian.Uint16(buf[offset:])
		offset += 2
	}

	// Encrypted path data
	encPath, consumed, err := DecodeEncryptedData(buf[offset:])
	if err != nil {
		return nil, fmt.Errorf("decode path: %w", err)
	}
	offset += consumed
	r.EncPath = encPath

	// If not encrypted, decode path immediately
	if !encPath.Encrypted {
		path, err := DecodePath(encPath.Data)
		if err != nil {
			return nil, fmt.Errorf("decode path: %w", err)
		}
		r.Path = path
	}
	// If encrypted, Path stays nil - caller must decrypt using SealedBox

	if offset >= len(buf) {
		return nil, fmt.Errorf("%w: RouteAdvertise seenBy missing", ErrInvalidFrame)
	}
	seenByLen := int(buf[offset])
	offset++

	r.SeenBy = make([]identity.AgentID, seenByLen)
	for i := 0; i < seenByLen; i++ {
		if offset+16 > len(buf) {
			return nil, fmt.Errorf("%w: RouteAdvertise seenBy truncated", ErrInvalidFrame)
		}
		copy(r.SeenBy[i][:], buf[offset:offset+16])
		offset += 16
	}

	return r, nil
}

// RouteWithdraw is the payload for ROUTE_WITHDRAW frames.
type RouteWithdraw struct {
	OriginAgent identity.AgentID
	Sequence    uint64
	Routes      []Route
	SeenBy      []identity.AgentID
}

// Encode serializes RouteWithdraw to bytes.
func (r *RouteWithdraw) Encode() []byte {
	// Similar to RouteAdvertise but without Path
	size := 16 + 8 + 1 // origin + seq + routeCount
	for _, route := range r.Routes {
		if route.AddressFamily == AddrFamilyIPv4 {
			size += 2 + 4 + 2
		} else {
			size += 2 + 16 + 2
		}
	}
	size += 1 + len(r.SeenBy)*16

	buf := make([]byte, size)
	offset := 0

	copy(buf[offset:], r.OriginAgent[:])
	offset += 16

	binary.BigEndian.PutUint64(buf[offset:], r.Sequence)
	offset += 8

	buf[offset] = uint8(len(r.Routes))
	offset++

	for _, route := range r.Routes {
		buf[offset] = route.AddressFamily
		offset++
		buf[offset] = route.PrefixLength
		offset++
		prefixLen := 4
		if route.AddressFamily == AddrFamilyIPv6 {
			prefixLen = 16
		}
		copy(buf[offset:], route.Prefix[:prefixLen])
		offset += prefixLen
		binary.BigEndian.PutUint16(buf[offset:], route.Metric)
		offset += 2
	}

	buf[offset] = uint8(len(r.SeenBy))
	offset++
	for _, id := range r.SeenBy {
		copy(buf[offset:], id[:])
		offset += 16
	}

	return buf[:offset]
}

// DecodeRouteWithdraw deserializes RouteWithdraw from bytes.
func DecodeRouteWithdraw(buf []byte) (*RouteWithdraw, error) {
	if len(buf) < 26 { // 16 + 8 + 1 + 1
		return nil, fmt.Errorf("%w: RouteWithdraw too short", ErrInvalidFrame)
	}

	r := &RouteWithdraw{}
	offset := 0

	copy(r.OriginAgent[:], buf[offset:offset+16])
	offset += 16

	r.Sequence = binary.BigEndian.Uint64(buf[offset:])
	offset += 8

	routeCount := int(buf[offset])
	offset++

	r.Routes = make([]Route, routeCount)
	for i := 0; i < routeCount; i++ {
		if offset+2 > len(buf) {
			return nil, fmt.Errorf("%w: RouteWithdraw routes truncated", ErrInvalidFrame)
		}
		route := &r.Routes[i]
		route.AddressFamily = buf[offset]
		offset++
		route.PrefixLength = buf[offset]
		offset++

		prefixLen := 4
		if route.AddressFamily == AddrFamilyIPv6 {
			prefixLen = 16
		}
		if offset+prefixLen+2 > len(buf) {
			return nil, fmt.Errorf("%w: RouteWithdraw route prefix truncated", ErrInvalidFrame)
		}
		route.Prefix = make([]byte, prefixLen)
		copy(route.Prefix, buf[offset:offset+prefixLen])
		offset += prefixLen
		route.Metric = binary.BigEndian.Uint16(buf[offset:])
		offset += 2
	}

	if offset >= len(buf) {
		return nil, fmt.Errorf("%w: RouteWithdraw seenBy missing", ErrInvalidFrame)
	}
	seenByLen := int(buf[offset])
	offset++

	r.SeenBy = make([]identity.AgentID, seenByLen)
	for i := 0; i < seenByLen; i++ {
		if offset+16 > len(buf) {
			return nil, fmt.Errorf("%w: RouteWithdraw seenBy truncated", ErrInvalidFrame)
		}
		copy(r.SeenBy[i][:], buf[offset:offset+16])
		offset += 16
	}

	return r, nil
}

// ============================================================================
// Encrypted data wrappers for management key encryption
// ============================================================================

// EncryptedData wraps data that may be encrypted with the management key.
// Used for NodeInfo and route paths to protect mesh topology from compromised agents.
type EncryptedData struct {
	Encrypted bool   // True if Data contains encrypted blob
	Data      []byte // Plaintext bytes OR encrypted blob (ephemeral_pub + nonce + ciphertext + tag)
}

// EncodeEncryptedData encodes an EncryptedData wrapper to bytes.
// Format: Encrypted(1) + DataLen(2) + Data
func EncodeEncryptedData(e *EncryptedData) []byte {
	buf := make([]byte, 1+2+len(e.Data))
	if e.Encrypted {
		buf[0] = 1
	} else {
		buf[0] = 0
	}
	binary.BigEndian.PutUint16(buf[1:], uint16(len(e.Data)))
	copy(buf[3:], e.Data)
	return buf
}

// DecodeEncryptedData decodes an EncryptedData wrapper from bytes.
// Returns the wrapper and the number of bytes consumed.
func DecodeEncryptedData(buf []byte) (*EncryptedData, int, error) {
	if len(buf) < 3 {
		return nil, 0, fmt.Errorf("%w: EncryptedData too short", ErrInvalidFrame)
	}

	e := &EncryptedData{
		Encrypted: buf[0] == 1,
	}
	dataLen := int(binary.BigEndian.Uint16(buf[1:]))

	if len(buf) < 3+dataLen {
		return nil, 0, fmt.Errorf("%w: EncryptedData data truncated", ErrInvalidFrame)
	}

	e.Data = make([]byte, dataLen)
	copy(e.Data, buf[3:3+dataLen])

	return e, 3 + dataLen, nil
}

// ============================================================================
// Node info frames
// ============================================================================

// PeerConnectionInfo describes a peer connection for an agent.
// Used in NodeInfo to advertise connected peers to the mesh.
type PeerConnectionInfo struct {
	PeerID    [16]byte // Remote peer AgentID
	Transport string   // Transport type: "quic", "h2", "ws"
	RTTMs     int64    // Round-trip time in milliseconds (0 if unknown)
	IsDialer  bool     // True if this agent initiated the connection
}

// MaxPeersInNodeInfo is the maximum number of peers to include in NodeInfo.
const MaxPeersInNodeInfo = 50

// NodeInfo contains metadata about an agent in the mesh.
type NodeInfo struct {
	DisplayName string                     // Human-readable name (from config)
	Hostname    string                     // System hostname
	OS          string                     // Operating system (runtime.GOOS)
	Arch        string                     // Architecture (runtime.GOARCH)
	Version     string                     // Agent version
	StartTime   int64                      // Agent start time (Unix timestamp)
	IPAddresses []string                   // Local IP addresses (non-loopback)
	Peers       []PeerConnectionInfo       // Connected peers (max 50)
	PublicKey   [EphemeralKeySize]byte     // Agent's static X25519 public key for E2E encryption
}

// EncodeNodeInfo encodes just the NodeInfo portion to bytes.
// This is used for encryption - the returned bytes can be encrypted with the management key.
func EncodeNodeInfo(info *NodeInfo) []byte {
	// Limit peers to max
	peers := info.Peers
	if len(peers) > MaxPeersInNodeInfo {
		peers = peers[:MaxPeersInNodeInfo]
	}

	// Calculate size
	size := 1 + len(info.DisplayName)
	size += 1 + len(info.Hostname)
	size += 1 + len(info.OS)
	size += 1 + len(info.Arch)
	size += 1 + len(info.Version)
	size += 8 // StartTime
	size += 1 // IPCount
	for _, ip := range info.IPAddresses {
		size += 1 + len(ip)
	}
	// Peers
	size += 1 // PeerCount
	for _, peer := range peers {
		size += 16                      // PeerID
		size += 1 + len(peer.Transport) // TransportLen + Transport
		size += 8                       // RTTMs
		size += 1                       // IsDialer
	}
	size += EphemeralKeySize // PublicKey

	buf := make([]byte, size)
	offset := 0

	// DisplayName
	buf[offset] = uint8(len(info.DisplayName))
	offset++
	copy(buf[offset:], info.DisplayName)
	offset += len(info.DisplayName)

	// Hostname
	buf[offset] = uint8(len(info.Hostname))
	offset++
	copy(buf[offset:], info.Hostname)
	offset += len(info.Hostname)

	// OS
	buf[offset] = uint8(len(info.OS))
	offset++
	copy(buf[offset:], info.OS)
	offset += len(info.OS)

	// Arch
	buf[offset] = uint8(len(info.Arch))
	offset++
	copy(buf[offset:], info.Arch)
	offset += len(info.Arch)

	// Version
	buf[offset] = uint8(len(info.Version))
	offset++
	copy(buf[offset:], info.Version)
	offset += len(info.Version)

	// StartTime
	binary.BigEndian.PutUint64(buf[offset:], uint64(info.StartTime))
	offset += 8

	// IPAddresses
	buf[offset] = uint8(len(info.IPAddresses))
	offset++
	for _, ip := range info.IPAddresses {
		buf[offset] = uint8(len(ip))
		offset++
		copy(buf[offset:], ip)
		offset += len(ip)
	}

	// Peers
	buf[offset] = uint8(len(peers))
	offset++
	for _, peer := range peers {
		copy(buf[offset:], peer.PeerID[:])
		offset += 16
		buf[offset] = uint8(len(peer.Transport))
		offset++
		copy(buf[offset:], peer.Transport)
		offset += len(peer.Transport)
		binary.BigEndian.PutUint64(buf[offset:], uint64(peer.RTTMs))
		offset += 8
		if peer.IsDialer {
			buf[offset] = 1
		} else {
			buf[offset] = 0
		}
		offset++
	}

	// PublicKey
	copy(buf[offset:], info.PublicKey[:])

	return buf
}

// DecodeNodeInfo decodes just the NodeInfo portion from bytes.
// This is the inverse of EncodeNodeInfo, used after decryption.
func DecodeNodeInfo(buf []byte) (*NodeInfo, error) {
	if len(buf) < 5+EphemeralKeySize {
		return nil, fmt.Errorf("%w: NodeInfo too short", ErrInvalidFrame)
	}

	info := &NodeInfo{}
	offset := 0

	// DisplayName
	if offset >= len(buf) {
		return nil, fmt.Errorf("%w: NodeInfo displayName length missing", ErrInvalidFrame)
	}
	displayNameLen := int(buf[offset])
	offset++
	if offset+displayNameLen > len(buf) {
		return nil, fmt.Errorf("%w: NodeInfo displayName truncated", ErrInvalidFrame)
	}
	info.DisplayName = string(buf[offset : offset+displayNameLen])
	offset += displayNameLen

	// Hostname
	if offset >= len(buf) {
		return nil, fmt.Errorf("%w: NodeInfo hostname length missing", ErrInvalidFrame)
	}
	hostnameLen := int(buf[offset])
	offset++
	if offset+hostnameLen > len(buf) {
		return nil, fmt.Errorf("%w: NodeInfo hostname truncated", ErrInvalidFrame)
	}
	info.Hostname = string(buf[offset : offset+hostnameLen])
	offset += hostnameLen

	// OS
	if offset >= len(buf) {
		return nil, fmt.Errorf("%w: NodeInfo os length missing", ErrInvalidFrame)
	}
	osLen := int(buf[offset])
	offset++
	if offset+osLen > len(buf) {
		return nil, fmt.Errorf("%w: NodeInfo os truncated", ErrInvalidFrame)
	}
	info.OS = string(buf[offset : offset+osLen])
	offset += osLen

	// Arch
	if offset >= len(buf) {
		return nil, fmt.Errorf("%w: NodeInfo arch length missing", ErrInvalidFrame)
	}
	archLen := int(buf[offset])
	offset++
	if offset+archLen > len(buf) {
		return nil, fmt.Errorf("%w: NodeInfo arch truncated", ErrInvalidFrame)
	}
	info.Arch = string(buf[offset : offset+archLen])
	offset += archLen

	// Version
	if offset >= len(buf) {
		return nil, fmt.Errorf("%w: NodeInfo version length missing", ErrInvalidFrame)
	}
	versionLen := int(buf[offset])
	offset++
	if offset+versionLen > len(buf) {
		return nil, fmt.Errorf("%w: NodeInfo version truncated", ErrInvalidFrame)
	}
	info.Version = string(buf[offset : offset+versionLen])
	offset += versionLen

	// StartTime
	if offset+8 > len(buf) {
		return nil, fmt.Errorf("%w: NodeInfo startTime truncated", ErrInvalidFrame)
	}
	info.StartTime = int64(binary.BigEndian.Uint64(buf[offset:]))
	offset += 8

	// IPAddresses
	if offset >= len(buf) {
		return nil, fmt.Errorf("%w: NodeInfo ipCount missing", ErrInvalidFrame)
	}
	ipCount := int(buf[offset])
	offset++
	info.IPAddresses = make([]string, ipCount)
	for i := 0; i < ipCount; i++ {
		if offset >= len(buf) {
			return nil, fmt.Errorf("%w: NodeInfo ip length missing", ErrInvalidFrame)
		}
		ipLen := int(buf[offset])
		offset++
		if offset+ipLen > len(buf) {
			return nil, fmt.Errorf("%w: NodeInfo ip truncated", ErrInvalidFrame)
		}
		info.IPAddresses[i] = string(buf[offset : offset+ipLen])
		offset += ipLen
	}

	// Peers
	if offset >= len(buf) {
		return nil, fmt.Errorf("%w: NodeInfo peerCount missing", ErrInvalidFrame)
	}
	peerCount := int(buf[offset])
	offset++
	if peerCount > MaxPeersInNodeInfo {
		peerCount = MaxPeersInNodeInfo
	}
	info.Peers = make([]PeerConnectionInfo, 0, peerCount)
	for i := 0; i < peerCount; i++ {
		if offset+16 > len(buf) {
			break
		}
		var peer PeerConnectionInfo
		copy(peer.PeerID[:], buf[offset:offset+16])
		offset += 16

		if offset >= len(buf) {
			break
		}
		transportLen := int(buf[offset])
		offset++
		if offset+transportLen > len(buf) {
			break
		}
		peer.Transport = string(buf[offset : offset+transportLen])
		offset += transportLen

		if offset+8 > len(buf) {
			break
		}
		peer.RTTMs = int64(binary.BigEndian.Uint64(buf[offset:]))
		offset += 8

		if offset >= len(buf) {
			break
		}
		peer.IsDialer = buf[offset] != 0
		offset++

		info.Peers = append(info.Peers, peer)
	}

	// PublicKey
	if offset+EphemeralKeySize > len(buf) {
		return nil, fmt.Errorf("%w: NodeInfo publicKey missing", ErrInvalidFrame)
	}
	copy(info.PublicKey[:], buf[offset:offset+EphemeralKeySize])

	return info, nil
}

// EncodePath encodes a path (slice of AgentIDs) to bytes.
// This is used for encryption - the returned bytes can be encrypted with the management key.
func EncodePath(path []identity.AgentID) []byte {
	buf := make([]byte, 1+len(path)*16)
	buf[0] = uint8(len(path))
	offset := 1
	for _, id := range path {
		copy(buf[offset:], id[:])
		offset += 16
	}
	return buf
}

// DecodePath decodes a path from bytes.
// This is the inverse of EncodePath, used after decryption.
func DecodePath(buf []byte) ([]identity.AgentID, error) {
	if len(buf) < 1 {
		return nil, fmt.Errorf("%w: Path too short", ErrInvalidFrame)
	}

	pathLen := int(buf[0])
	if len(buf) < 1+pathLen*16 {
		return nil, fmt.Errorf("%w: Path truncated", ErrInvalidFrame)
	}

	path := make([]identity.AgentID, pathLen)
	offset := 1
	for i := 0; i < pathLen; i++ {
		copy(path[i][:], buf[offset:offset+16])
		offset += 16
	}

	return path, nil
}

// NodeInfoAdvertise is the payload for NODE_INFO_ADVERTISE frames.
// Used to announce node metadata to all agents in the mesh.
type NodeInfoAdvertise struct {
	OriginAgent identity.AgentID   // Agent advertising its info
	Sequence    uint64             // Monotonically increasing sequence
	Info        NodeInfo           // Node metadata (may be decrypted from EncryptedInfo)
	EncInfo     *EncryptedData     // Encrypted NodeInfo data (nil if not using encryption)
	SeenBy      []identity.AgentID // Loop prevention (agents that have seen this)
}

// Encode serializes NodeInfoAdvertise to bytes.
// New format (v2) with encryption support:
//   OriginAgent(16) + Sequence(8) + EncryptedData(flag+len+data) + SeenByLen(1) + SeenBy(N*16)
// Where EncryptedData contains NodeInfo (plaintext or encrypted blob).
func (n *NodeInfoAdvertise) Encode() []byte {
	// Prepare NodeInfo data (encrypted or plaintext)
	var encData *EncryptedData
	if n.EncInfo != nil {
		// Use pre-computed encrypted data
		encData = n.EncInfo
	} else {
		// Encode NodeInfo as plaintext
		encData = &EncryptedData{
			Encrypted: false,
			Data:      EncodeNodeInfo(&n.Info),
		}
	}

	// Calculate size
	encDataBytes := EncodeEncryptedData(encData)
	size := 16 + 8 + len(encDataBytes) + 1 + len(n.SeenBy)*16

	buf := make([]byte, size)
	offset := 0

	// OriginAgent
	copy(buf[offset:], n.OriginAgent[:])
	offset += 16

	// Sequence
	binary.BigEndian.PutUint64(buf[offset:], n.Sequence)
	offset += 8

	// EncryptedData (contains NodeInfo, encrypted or plaintext)
	copy(buf[offset:], encDataBytes)
	offset += len(encDataBytes)

	// SeenBy
	buf[offset] = uint8(len(n.SeenBy))
	offset++
	for _, id := range n.SeenBy {
		copy(buf[offset:], id[:])
		offset += 16
	}

	return buf
}

// DecodeNodeInfoAdvertise deserializes NodeInfoAdvertise from bytes.
// Supports new format (v2) with encryption:
//   OriginAgent(16) + Sequence(8) + EncryptedData(flag+len+data) + SeenByLen(1) + SeenBy(N*16)
func DecodeNodeInfoAdvertise(buf []byte) (*NodeInfoAdvertise, error) {
	if len(buf) < 28 { // Minimum: 16 + 8 + 3 (encData min) + 1 (seenByLen)
		return nil, fmt.Errorf("%w: NodeInfoAdvertise too short", ErrInvalidFrame)
	}

	n := &NodeInfoAdvertise{}
	offset := 0

	// OriginAgent
	copy(n.OriginAgent[:], buf[offset:offset+16])
	offset += 16

	// Sequence
	n.Sequence = binary.BigEndian.Uint64(buf[offset:])
	offset += 8

	// EncryptedData (contains NodeInfo, encrypted or plaintext)
	encData, consumed, err := DecodeEncryptedData(buf[offset:])
	if err != nil {
		return nil, err
	}
	offset += consumed
	n.EncInfo = encData

	// If not encrypted, decode NodeInfo immediately
	if !encData.Encrypted {
		info, err := DecodeNodeInfo(encData.Data)
		if err != nil {
			return nil, fmt.Errorf("decode NodeInfo: %w", err)
		}
		n.Info = *info
	}
	// If encrypted, Info stays empty - caller must decrypt using SealedBox

	// SeenBy
	if offset >= len(buf) {
		return nil, fmt.Errorf("%w: NodeInfoAdvertise seenByLen missing", ErrInvalidFrame)
	}
	seenByLen := int(buf[offset])
	offset++
	n.SeenBy = make([]identity.AgentID, seenByLen)
	for i := 0; i < seenByLen; i++ {
		if offset+16 > len(buf) {
			return nil, fmt.Errorf("%w: NodeInfoAdvertise seenBy truncated", ErrInvalidFrame)
		}
		copy(n.SeenBy[i][:], buf[offset:offset+16])
		offset += 16
	}

	return n, nil
}

// ============================================================================
// Control frames (for remote metrics/status)
// ============================================================================

// ControlRequest is the payload for CONTROL_REQUEST frames.
// Used to request metrics, status, or other information from remote agents.
type ControlRequest struct {
	RequestID   uint64             // Unique request ID for correlation
	ControlType uint8              // Type of control request (ControlTypeMetrics, etc.)
	TargetAgent identity.AgentID   // Target agent to forward request to (zero = this agent)
	Path        []identity.AgentID // Remaining path to target
	Data        []byte             // Optional request data (e.g., RPC request payload)
}

// Encode serializes ControlRequest to bytes.
func (c *ControlRequest) Encode() []byte {
	// Format: RequestID(8) + ControlType(1) + TargetAgent(16) + PathLen(1) + Path(N*16) + DataLen(4) + Data
	size := 8 + 1 + 16 + 1 + len(c.Path)*16 + 4 + len(c.Data)
	buf := make([]byte, size)
	offset := 0

	binary.BigEndian.PutUint64(buf[offset:], c.RequestID)
	offset += 8

	buf[offset] = c.ControlType
	offset++

	copy(buf[offset:], c.TargetAgent[:])
	offset += 16

	buf[offset] = uint8(len(c.Path))
	offset++

	for _, id := range c.Path {
		copy(buf[offset:], id[:])
		offset += 16
	}

	binary.BigEndian.PutUint32(buf[offset:], uint32(len(c.Data)))
	offset += 4

	copy(buf[offset:], c.Data)

	return buf
}

// DecodeControlRequest deserializes ControlRequest from bytes.
func DecodeControlRequest(buf []byte) (*ControlRequest, error) {
	if len(buf) < 30 { // 8 + 1 + 16 + 1 + 4 (minimum with empty path and data)
		return nil, fmt.Errorf("%w: ControlRequest too short", ErrInvalidFrame)
	}

	c := &ControlRequest{}
	offset := 0

	c.RequestID = binary.BigEndian.Uint64(buf[offset:])
	offset += 8

	c.ControlType = buf[offset]
	offset++

	copy(c.TargetAgent[:], buf[offset:offset+16])
	offset += 16

	pathLen := int(buf[offset])
	offset++

	c.Path = make([]identity.AgentID, pathLen)
	for i := 0; i < pathLen; i++ {
		if offset+16 > len(buf) {
			return nil, fmt.Errorf("%w: ControlRequest path truncated", ErrInvalidFrame)
		}
		copy(c.Path[i][:], buf[offset:offset+16])
		offset += 16
	}

	if offset+4 > len(buf) {
		return nil, fmt.Errorf("%w: ControlRequest data length truncated", ErrInvalidFrame)
	}
	dataLen := int(binary.BigEndian.Uint32(buf[offset:]))
	offset += 4

	if offset+dataLen > len(buf) {
		return nil, fmt.Errorf("%w: ControlRequest data truncated", ErrInvalidFrame)
	}
	if dataLen > 0 {
		c.Data = make([]byte, dataLen)
		copy(c.Data, buf[offset:offset+dataLen])
	}

	return c, nil
}

// ControlResponse is the payload for CONTROL_RESPONSE frames.
// Contains the response data from a control request.
type ControlResponse struct {
	RequestID   uint64 // Matches the request ID
	ControlType uint8  // Type of control response
	Success     bool   // Whether the request succeeded
	Data        []byte // Response data (Prometheus text, JSON status, etc.)
}

// Encode serializes ControlResponse to bytes.
func (c *ControlResponse) Encode() []byte {
	// Limit data size to fit in payload
	data := c.Data
	if len(data) > MaxPayloadSize-12 {
		data = data[:MaxPayloadSize-12]
	}

	buf := make([]byte, 8+1+1+2+len(data))
	offset := 0

	binary.BigEndian.PutUint64(buf[offset:], c.RequestID)
	offset += 8

	buf[offset] = c.ControlType
	offset++

	if c.Success {
		buf[offset] = 1
	} else {
		buf[offset] = 0
	}
	offset++

	binary.BigEndian.PutUint16(buf[offset:], uint16(len(data)))
	offset += 2

	copy(buf[offset:], data)

	return buf
}

// DecodeControlResponse deserializes ControlResponse from bytes.
func DecodeControlResponse(buf []byte) (*ControlResponse, error) {
	if len(buf) < 12 { // 8 + 1 + 1 + 2
		return nil, fmt.Errorf("%w: ControlResponse too short", ErrInvalidFrame)
	}

	c := &ControlResponse{}
	offset := 0

	c.RequestID = binary.BigEndian.Uint64(buf[offset:])
	offset += 8

	c.ControlType = buf[offset]
	offset++

	c.Success = buf[offset] == 1
	offset++

	dataLen := binary.BigEndian.Uint16(buf[offset:])
	offset += 2

	if offset+int(dataLen) > len(buf) {
		return nil, fmt.Errorf("%w: ControlResponse data truncated", ErrInvalidFrame)
	}

	c.Data = make([]byte, dataLen)
	copy(c.Data, buf[offset:offset+int(dataLen)])

	return c, nil
}

// ============================================================================
// Frame Reader/Writer
// ============================================================================

// FrameReader reads frames from an io.Reader.
type FrameReader struct {
	r      io.Reader
	header [HeaderSize]byte
}

// NewFrameReader creates a new FrameReader.
func NewFrameReader(r io.Reader) *FrameReader {
	return &FrameReader{r: r}
}

// Read reads the next frame.
func (fr *FrameReader) Read() (*Frame, error) {
	// Read header
	if _, err := io.ReadFull(fr.r, fr.header[:]); err != nil {
		return nil, err
	}

	frameType, flags, length, streamID, err := DecodeHeader(fr.header[:])
	if err != nil {
		return nil, err
	}

	// Read payload
	payload := make([]byte, length)
	if length > 0 {
		if _, err := io.ReadFull(fr.r, payload); err != nil {
			return nil, err
		}
	}

	return &Frame{
		Type:     frameType,
		Flags:    flags,
		StreamID: streamID,
		Payload:  payload,
	}, nil
}

// FrameWriter writes frames to an io.Writer.
type FrameWriter struct {
	w io.Writer
}

// NewFrameWriter creates a new FrameWriter.
func NewFrameWriter(w io.Writer) *FrameWriter {
	return &FrameWriter{w: w}
}

// Write writes a frame.
func (fw *FrameWriter) Write(f *Frame) error {
	data, err := f.Encode()
	if err != nil {
		return err
	}
	_, err = fw.w.Write(data)
	return err
}

// WriteFrame is a convenience method to write a frame with the given parameters.
func (fw *FrameWriter) WriteFrame(frameType uint8, flags uint8, streamID uint64, payload []byte) error {
	return fw.Write(&Frame{
		Type:     frameType,
		Flags:    flags,
		StreamID: streamID,
		Payload:  payload,
	})
}
