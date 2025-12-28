package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"

	"github.com/coinstash/muti-metroo/internal/identity"
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
}

// Encode serializes PeerHello to bytes.
func (p *PeerHello) Encode() []byte {
	// Calculate size
	size := 2 + 16 + 8 + 1 // version + agentID + timestamp + capLen
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
	if len(buf) < 27 { // 2 + 16 + 8 + 1
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

// StreamOpen is the payload for STREAM_OPEN frames.
type StreamOpen struct {
	RequestID     uint64
	AddressType   uint8
	Address       []byte // IPv4 (4), IPv6 (16), or domain (1+N)
	Port          uint16
	TTL           uint8
	RemainingPath []identity.AgentID
}

// Encode serializes StreamOpen to bytes.
func (s *StreamOpen) Encode() []byte {
	size := 8 + 1 + len(s.Address) + 2 + 1 + 1 + len(s.RemainingPath)*16
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

	return buf
}

// DecodeStreamOpen deserializes StreamOpen from bytes.
func DecodeStreamOpen(buf []byte) (*StreamOpen, error) {
	if len(buf) < 13 { // 8 + 1 + 2 + 1 + 1 (minimum)
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
	RequestID     uint64
	BoundAddrType uint8
	BoundAddr     []byte
	BoundPort     uint16
}

// Encode serializes StreamOpenAck to bytes.
func (s *StreamOpenAck) Encode() []byte {
	buf := make([]byte, 8+1+len(s.BoundAddr)+2)
	offset := 0

	binary.BigEndian.PutUint64(buf[offset:], s.RequestID)
	offset += 8

	buf[offset] = s.BoundAddrType
	offset++

	copy(buf[offset:], s.BoundAddr)
	offset += len(s.BoundAddr)

	binary.BigEndian.PutUint16(buf[offset:], s.BoundPort)

	return buf
}

// DecodeStreamOpenAck deserializes StreamOpenAck from bytes.
func DecodeStreamOpenAck(buf []byte) (*StreamOpenAck, error) {
	if len(buf) < 12 { // 8 + 1 + 1 + 2 minimum (empty addr)
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

	if offset+addrLen+2 > len(buf) {
		return nil, fmt.Errorf("%w: StreamOpenAck address truncated", ErrInvalidFrame)
	}

	s.BoundAddr = make([]byte, addrLen)
	copy(s.BoundAddr, buf[offset:offset+addrLen])
	offset += addrLen

	s.BoundPort = binary.BigEndian.Uint16(buf[offset:])

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
	OriginAgent identity.AgentID
	Sequence    uint64
	Routes      []Route
	Path        []identity.AgentID
	SeenBy      []identity.AgentID
}

// Encode serializes RouteAdvertise to bytes.
func (r *RouteAdvertise) Encode() []byte {
	// Calculate size
	size := 16 + 8 + 1 // origin + seq + routeCount
	for _, route := range r.Routes {
		if route.AddressFamily == AddrFamilyIPv4 {
			size += 2 + 4 + 2 // family + prefix + metric
		} else {
			size += 2 + 16 + 2
		}
	}
	size += 1 + len(r.Path)*16   // pathLen + path
	size += 1 + len(r.SeenBy)*16 // seenByLen + seenBy

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

	buf[offset] = uint8(len(r.Path))
	offset++
	for _, id := range r.Path {
		copy(buf[offset:], id[:])
		offset += 16
	}

	buf[offset] = uint8(len(r.SeenBy))
	offset++
	for _, id := range r.SeenBy {
		copy(buf[offset:], id[:])
		offset += 16
	}

	return buf[:offset]
}

// DecodeRouteAdvertise deserializes RouteAdvertise from bytes.
func DecodeRouteAdvertise(buf []byte) (*RouteAdvertise, error) {
	if len(buf) < 27 { // 16 + 8 + 1 + 1 + 1
		return nil, fmt.Errorf("%w: RouteAdvertise too short", ErrInvalidFrame)
	}

	r := &RouteAdvertise{}
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

	if offset >= len(buf) {
		return nil, fmt.Errorf("%w: RouteAdvertise path missing", ErrInvalidFrame)
	}
	pathLen := int(buf[offset])
	offset++

	r.Path = make([]identity.AgentID, pathLen)
	for i := 0; i < pathLen; i++ {
		if offset+16 > len(buf) {
			return nil, fmt.Errorf("%w: RouteAdvertise path truncated", ErrInvalidFrame)
		}
		copy(r.Path[i][:], buf[offset:offset+16])
		offset += 16
	}

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
