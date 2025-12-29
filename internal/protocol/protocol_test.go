package protocol

import (
	"bytes"
	"io"
	"testing"

	"github.com/postalsys/muti-metroo/internal/identity"
)

func TestFrameTypeName(t *testing.T) {
	tests := []struct {
		frameType uint8
		want      string
	}{
		{FrameStreamOpen, "STREAM_OPEN"},
		{FrameStreamOpenAck, "STREAM_OPEN_ACK"},
		{FrameStreamOpenErr, "STREAM_OPEN_ERR"},
		{FrameStreamData, "STREAM_DATA"},
		{FrameStreamClose, "STREAM_CLOSE"},
		{FrameStreamReset, "STREAM_RESET"},
		{FrameRouteAdvertise, "ROUTE_ADVERTISE"},
		{FrameRouteWithdraw, "ROUTE_WITHDRAW"},
		{FramePeerHello, "PEER_HELLO"},
		{FramePeerHelloAck, "PEER_HELLO_ACK"},
		{FrameKeepalive, "KEEPALIVE"},
		{FrameKeepaliveAck, "KEEPALIVE_ACK"},
		{0xFF, "UNKNOWN"},
	}

	for _, tt := range tests {
		if got := FrameTypeName(tt.frameType); got != tt.want {
			t.Errorf("FrameTypeName(%d) = %s, want %s", tt.frameType, got, tt.want)
		}
	}
}

func TestErrorCodeName(t *testing.T) {
	tests := []struct {
		code uint16
		want string
	}{
		{ErrNoRoute, "NO_ROUTE"},
		{ErrConnectionRefused, "CONNECTION_REFUSED"},
		{ErrConnectionTimeout, "CONNECTION_TIMEOUT"},
		{ErrTTLExceeded, "TTL_EXCEEDED"},
		{ErrHostUnreachable, "HOST_UNREACHABLE"},
		{ErrNetworkUnreachable, "NETWORK_UNREACHABLE"},
		{ErrDNSError, "DNS_ERROR"},
		{ErrExitDisabled, "EXIT_DISABLED"},
		{ErrResourceLimit, "RESOURCE_LIMIT"},
		{999, "UNKNOWN"},
	}

	for _, tt := range tests {
		if got := ErrorCodeName(tt.code); got != tt.want {
			t.Errorf("ErrorCodeName(%d) = %s, want %s", tt.code, got, tt.want)
		}
	}
}

func TestIsStreamFrame(t *testing.T) {
	streamFrames := []uint8{FrameStreamOpen, FrameStreamOpenAck, FrameStreamOpenErr, FrameStreamData, FrameStreamClose, FrameStreamReset}
	nonStreamFrames := []uint8{FrameRouteAdvertise, FrameRouteWithdraw, FramePeerHello, FrameKeepalive}

	for _, ft := range streamFrames {
		if !IsStreamFrame(ft) {
			t.Errorf("IsStreamFrame(%s) = false, want true", FrameTypeName(ft))
		}
	}
	for _, ft := range nonStreamFrames {
		if IsStreamFrame(ft) {
			t.Errorf("IsStreamFrame(%s) = true, want false", FrameTypeName(ft))
		}
	}
}

func TestIsRoutingFrame(t *testing.T) {
	if !IsRoutingFrame(FrameRouteAdvertise) {
		t.Error("IsRoutingFrame(ROUTE_ADVERTISE) = false")
	}
	if !IsRoutingFrame(FrameRouteWithdraw) {
		t.Error("IsRoutingFrame(ROUTE_WITHDRAW) = false")
	}
	if IsRoutingFrame(FrameStreamOpen) {
		t.Error("IsRoutingFrame(STREAM_OPEN) = true")
	}
}

func TestIsControlFrame(t *testing.T) {
	controlFrames := []uint8{FramePeerHello, FramePeerHelloAck, FrameKeepalive, FrameKeepaliveAck}
	for _, ft := range controlFrames {
		if !IsControlFrame(ft) {
			t.Errorf("IsControlFrame(%s) = false, want true", FrameTypeName(ft))
		}
	}
	if IsControlFrame(FrameStreamOpen) {
		t.Error("IsControlFrame(STREAM_OPEN) = true")
	}
}

func TestFrame_EncodeDecode(t *testing.T) {
	tests := []struct {
		name  string
		frame Frame
	}{
		{
			name: "empty payload",
			frame: Frame{
				Type:     FrameStreamOpen,
				Flags:    0,
				StreamID: 42,
				Payload:  []byte{},
			},
		},
		{
			name: "with payload",
			frame: Frame{
				Type:     FrameStreamData,
				Flags:    FlagFinWrite,
				StreamID: 12345678,
				Payload:  []byte("Hello, World!"),
			},
		},
		{
			name: "max stream ID",
			frame: Frame{
				Type:     FrameKeepalive,
				Flags:    0,
				StreamID: ^uint64(0),
				Payload:  []byte{0x01, 0x02, 0x03},
			},
		},
		{
			name: "control stream",
			frame: Frame{
				Type:     FramePeerHello,
				Flags:    0,
				StreamID: ControlStreamID,
				Payload:  make([]byte, 100),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encode
			data, err := tt.frame.Encode()
			if err != nil {
				t.Fatalf("Encode() error = %v", err)
			}

			// Verify header size
			if len(data) != HeaderSize+len(tt.frame.Payload) {
				t.Errorf("Encoded length = %d, want %d", len(data), HeaderSize+len(tt.frame.Payload))
			}

			// Decode
			decoded, err := Decode(data)
			if err != nil {
				t.Fatalf("Decode() error = %v", err)
			}

			// Verify
			if decoded.Type != tt.frame.Type {
				t.Errorf("Type = %d, want %d", decoded.Type, tt.frame.Type)
			}
			if decoded.Flags != tt.frame.Flags {
				t.Errorf("Flags = %d, want %d", decoded.Flags, tt.frame.Flags)
			}
			if decoded.StreamID != tt.frame.StreamID {
				t.Errorf("StreamID = %d, want %d", decoded.StreamID, tt.frame.StreamID)
			}
			if !bytes.Equal(decoded.Payload, tt.frame.Payload) {
				t.Errorf("Payload mismatch")
			}
		})
	}
}

func TestFrame_Encode_TooLarge(t *testing.T) {
	f := Frame{
		Type:    FrameStreamData,
		Payload: make([]byte, MaxPayloadSize+1),
	}

	_, err := f.Encode()
	if err != ErrFrameTooLarge {
		t.Errorf("Encode() error = %v, want ErrFrameTooLarge", err)
	}
}

func TestDecode_HeaderTooShort(t *testing.T) {
	_, err := Decode(make([]byte, HeaderSize-1))
	if err == nil {
		t.Error("Decode() should fail with short header")
	}
}

func TestDecode_PayloadTruncated(t *testing.T) {
	// Create valid header but truncated payload
	header := make([]byte, HeaderSize)
	header[0] = FrameStreamData
	// Set length to 100
	header[2] = 0
	header[3] = 0
	header[4] = 0
	header[5] = 100

	// Only provide 50 bytes of payload
	data := append(header, make([]byte, 50)...)

	_, err := Decode(data)
	if err == nil {
		t.Error("Decode() should fail with truncated payload")
	}
}

func TestFrame_String(t *testing.T) {
	f := Frame{
		Type:     FrameStreamOpen,
		Flags:    FlagFinWrite,
		StreamID: 42,
		Payload:  []byte("test"),
	}

	s := f.String()
	if s == "" {
		t.Error("String() returned empty string")
	}
	if !bytes.Contains([]byte(s), []byte("STREAM_OPEN")) {
		t.Error("String() should contain frame type name")
	}
}

func TestPeerHello_EncodeDecode(t *testing.T) {
	agentID, _ := identity.NewAgentID()

	original := &PeerHello{
		Version:      ProtocolVersion,
		AgentID:      agentID,
		Timestamp:    1703001234,
		Capabilities: []string{"exit", "socks5"},
	}

	data := original.Encode()
	decoded, err := DecodePeerHello(data)
	if err != nil {
		t.Fatalf("DecodePeerHello() error = %v", err)
	}

	if decoded.Version != original.Version {
		t.Errorf("Version = %d, want %d", decoded.Version, original.Version)
	}
	if !decoded.AgentID.Equal(original.AgentID) {
		t.Errorf("AgentID mismatch")
	}
	if decoded.Timestamp != original.Timestamp {
		t.Errorf("Timestamp = %d, want %d", decoded.Timestamp, original.Timestamp)
	}
	if len(decoded.Capabilities) != len(original.Capabilities) {
		t.Fatalf("Capabilities length = %d, want %d", len(decoded.Capabilities), len(original.Capabilities))
	}
	for i, cap := range decoded.Capabilities {
		if cap != original.Capabilities[i] {
			t.Errorf("Capabilities[%d] = %s, want %s", i, cap, original.Capabilities[i])
		}
	}
}

func TestPeerHello_EmptyCapabilities(t *testing.T) {
	agentID, _ := identity.NewAgentID()

	original := &PeerHello{
		Version:      1,
		AgentID:      agentID,
		Timestamp:    12345,
		Capabilities: []string{},
	}

	data := original.Encode()
	decoded, err := DecodePeerHello(data)
	if err != nil {
		t.Fatalf("DecodePeerHello() error = %v", err)
	}

	if len(decoded.Capabilities) != 0 {
		t.Errorf("Capabilities length = %d, want 0", len(decoded.Capabilities))
	}
}

func TestStreamOpen_EncodeDecode_IPv4(t *testing.T) {
	path1, _ := identity.NewAgentID()
	path2, _ := identity.NewAgentID()

	original := &StreamOpen{
		RequestID:     12345678,
		AddressType:   AddrTypeIPv4,
		Address:       []byte{10, 0, 0, 1},
		Port:          8080,
		TTL:           15,
		RemainingPath: []identity.AgentID{path1, path2},
	}

	data := original.Encode()
	decoded, err := DecodeStreamOpen(data)
	if err != nil {
		t.Fatalf("DecodeStreamOpen() error = %v", err)
	}

	if decoded.RequestID != original.RequestID {
		t.Errorf("RequestID = %d, want %d", decoded.RequestID, original.RequestID)
	}
	if decoded.AddressType != AddrTypeIPv4 {
		t.Errorf("AddressType = %d, want %d", decoded.AddressType, AddrTypeIPv4)
	}
	if !bytes.Equal(decoded.Address, original.Address) {
		t.Errorf("Address = %v, want %v", decoded.Address, original.Address)
	}
	if decoded.Port != original.Port {
		t.Errorf("Port = %d, want %d", decoded.Port, original.Port)
	}
	if decoded.TTL != original.TTL {
		t.Errorf("TTL = %d, want %d", decoded.TTL, original.TTL)
	}
	if len(decoded.RemainingPath) != 2 {
		t.Fatalf("RemainingPath length = %d, want 2", len(decoded.RemainingPath))
	}
}

func TestStreamOpen_EncodeDecode_Domain(t *testing.T) {
	domain := "example.com"
	address := make([]byte, 1+len(domain))
	address[0] = byte(len(domain))
	copy(address[1:], domain)

	original := &StreamOpen{
		RequestID:     1,
		AddressType:   AddrTypeDomain,
		Address:       address,
		Port:          443,
		TTL:           10,
		RemainingPath: []identity.AgentID{},
	}

	data := original.Encode()
	decoded, err := DecodeStreamOpen(data)
	if err != nil {
		t.Fatalf("DecodeStreamOpen() error = %v", err)
	}

	if decoded.GetDestinationDomain() != domain {
		t.Errorf("Domain = %s, want %s", decoded.GetDestinationDomain(), domain)
	}
}

func TestStreamOpen_GetDestinationIP(t *testing.T) {
	s := &StreamOpen{
		AddressType: AddrTypeIPv4,
		Address:     []byte{192, 168, 1, 100},
	}

	ip := s.GetDestinationIP()
	if ip == nil {
		t.Fatal("GetDestinationIP() returned nil")
	}
	if ip.String() != "192.168.1.100" {
		t.Errorf("IP = %s, want 192.168.1.100", ip.String())
	}
}

func TestStreamOpenAck_EncodeDecode(t *testing.T) {
	original := &StreamOpenAck{
		RequestID:     12345,
		BoundAddrType: AddrTypeIPv4,
		BoundAddr:     []byte{10, 0, 0, 1},
		BoundPort:     22,
	}

	data := original.Encode()
	decoded, err := DecodeStreamOpenAck(data)
	if err != nil {
		t.Fatalf("DecodeStreamOpenAck() error = %v", err)
	}

	if decoded.RequestID != original.RequestID {
		t.Errorf("RequestID = %d, want %d", decoded.RequestID, original.RequestID)
	}
	if decoded.BoundPort != original.BoundPort {
		t.Errorf("BoundPort = %d, want %d", decoded.BoundPort, original.BoundPort)
	}
}

func TestStreamOpenErr_EncodeDecode(t *testing.T) {
	original := &StreamOpenErr{
		RequestID: 12345,
		ErrorCode: ErrConnectionRefused,
		Message:   "Connection refused by target",
	}

	data := original.Encode()
	decoded, err := DecodeStreamOpenErr(data)
	if err != nil {
		t.Fatalf("DecodeStreamOpenErr() error = %v", err)
	}

	if decoded.RequestID != original.RequestID {
		t.Errorf("RequestID = %d, want %d", decoded.RequestID, original.RequestID)
	}
	if decoded.ErrorCode != original.ErrorCode {
		t.Errorf("ErrorCode = %d, want %d", decoded.ErrorCode, original.ErrorCode)
	}
	if decoded.Message != original.Message {
		t.Errorf("Message = %s, want %s", decoded.Message, original.Message)
	}
}

func TestStreamReset_EncodeDecode(t *testing.T) {
	original := &StreamReset{ErrorCode: ErrResourceLimit}

	data := original.Encode()
	decoded, err := DecodeStreamReset(data)
	if err != nil {
		t.Fatalf("DecodeStreamReset() error = %v", err)
	}

	if decoded.ErrorCode != original.ErrorCode {
		t.Errorf("ErrorCode = %d, want %d", decoded.ErrorCode, original.ErrorCode)
	}
}

func TestKeepalive_EncodeDecode(t *testing.T) {
	original := &Keepalive{Timestamp: 1703001234567}

	data := original.Encode()
	decoded, err := DecodeKeepalive(data)
	if err != nil {
		t.Fatalf("DecodeKeepalive() error = %v", err)
	}

	if decoded.Timestamp != original.Timestamp {
		t.Errorf("Timestamp = %d, want %d", decoded.Timestamp, original.Timestamp)
	}
}

func TestRouteAdvertise_EncodeDecode(t *testing.T) {
	origin, _ := identity.NewAgentID()
	path1, _ := identity.NewAgentID()
	seen1, _ := identity.NewAgentID()

	original := &RouteAdvertise{
		OriginAgent: origin,
		Sequence:    42,
		Routes: []Route{
			{
				AddressFamily: AddrFamilyIPv4,
				PrefixLength:  8,
				Prefix:        []byte{10, 0, 0, 0},
				Metric:        1,
			},
			{
				AddressFamily: AddrFamilyIPv4,
				PrefixLength:  16,
				Prefix:        []byte{192, 168, 0, 0},
				Metric:        2,
			},
		},
		Path:   []identity.AgentID{path1},
		SeenBy: []identity.AgentID{origin, seen1},
	}

	data := original.Encode()
	decoded, err := DecodeRouteAdvertise(data)
	if err != nil {
		t.Fatalf("DecodeRouteAdvertise() error = %v", err)
	}

	if !decoded.OriginAgent.Equal(original.OriginAgent) {
		t.Error("OriginAgent mismatch")
	}
	if decoded.Sequence != original.Sequence {
		t.Errorf("Sequence = %d, want %d", decoded.Sequence, original.Sequence)
	}
	if len(decoded.Routes) != len(original.Routes) {
		t.Fatalf("Routes length = %d, want %d", len(decoded.Routes), len(original.Routes))
	}
	for i, route := range decoded.Routes {
		if route.AddressFamily != original.Routes[i].AddressFamily {
			t.Errorf("Route[%d].AddressFamily = %d, want %d", i, route.AddressFamily, original.Routes[i].AddressFamily)
		}
		if route.PrefixLength != original.Routes[i].PrefixLength {
			t.Errorf("Route[%d].PrefixLength = %d, want %d", i, route.PrefixLength, original.Routes[i].PrefixLength)
		}
		if route.Metric != original.Routes[i].Metric {
			t.Errorf("Route[%d].Metric = %d, want %d", i, route.Metric, original.Routes[i].Metric)
		}
	}
	if len(decoded.Path) != 1 {
		t.Errorf("Path length = %d, want 1", len(decoded.Path))
	}
	if len(decoded.SeenBy) != 2 {
		t.Errorf("SeenBy length = %d, want 2", len(decoded.SeenBy))
	}
}

func TestRouteWithdraw_EncodeDecode(t *testing.T) {
	origin, _ := identity.NewAgentID()

	original := &RouteWithdraw{
		OriginAgent: origin,
		Sequence:    100,
		Routes: []Route{
			{
				AddressFamily: AddrFamilyIPv4,
				PrefixLength:  24,
				Prefix:        []byte{10, 5, 3, 0},
				Metric:        0,
			},
		},
		SeenBy: []identity.AgentID{origin},
	}

	data := original.Encode()
	decoded, err := DecodeRouteWithdraw(data)
	if err != nil {
		t.Fatalf("DecodeRouteWithdraw() error = %v", err)
	}

	if decoded.Sequence != original.Sequence {
		t.Errorf("Sequence = %d, want %d", decoded.Sequence, original.Sequence)
	}
	if len(decoded.Routes) != 1 {
		t.Fatalf("Routes length = %d, want 1", len(decoded.Routes))
	}
}

func TestFrameReaderWriter(t *testing.T) {
	// Create a buffer to simulate network connection
	buf := new(bytes.Buffer)

	writer := NewFrameWriter(buf)
	reader := NewFrameReader(buf)

	// Write multiple frames
	frames := []*Frame{
		{Type: FrameStreamOpen, StreamID: 1, Payload: []byte("open")},
		{Type: FrameStreamData, StreamID: 1, Payload: []byte("data payload here")},
		{Type: FrameStreamClose, Flags: FlagFinWrite, StreamID: 1, Payload: []byte{}},
	}

	for _, f := range frames {
		if err := writer.Write(f); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	}

	// Read frames back
	for i, expected := range frames {
		got, err := reader.Read()
		if err != nil {
			t.Fatalf("Read() frame %d error = %v", i, err)
		}

		if got.Type != expected.Type {
			t.Errorf("Frame %d Type = %d, want %d", i, got.Type, expected.Type)
		}
		if got.Flags != expected.Flags {
			t.Errorf("Frame %d Flags = %d, want %d", i, got.Flags, expected.Flags)
		}
		if got.StreamID != expected.StreamID {
			t.Errorf("Frame %d StreamID = %d, want %d", i, got.StreamID, expected.StreamID)
		}
		if !bytes.Equal(got.Payload, expected.Payload) {
			t.Errorf("Frame %d Payload mismatch", i)
		}
	}
}

func TestFrameReader_EOF(t *testing.T) {
	buf := new(bytes.Buffer)
	reader := NewFrameReader(buf)

	_, err := reader.Read()
	if err != io.EOF {
		t.Errorf("Read() on empty buffer error = %v, want io.EOF", err)
	}
}

func TestFrameWriter_WriteFrame(t *testing.T) {
	buf := new(bytes.Buffer)
	writer := NewFrameWriter(buf)

	err := writer.WriteFrame(FrameKeepalive, 0, ControlStreamID, []byte{1, 2, 3, 4, 5, 6, 7, 8})
	if err != nil {
		t.Fatalf("WriteFrame() error = %v", err)
	}

	reader := NewFrameReader(buf)
	f, err := reader.Read()
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	if f.Type != FrameKeepalive {
		t.Errorf("Type = %d, want %d", f.Type, FrameKeepalive)
	}
	if f.StreamID != ControlStreamID {
		t.Errorf("StreamID = %d, want %d", f.StreamID, ControlStreamID)
	}
}

func TestDecodeStreamOpen_InvalidAddressType(t *testing.T) {
	data := make([]byte, 20)
	data[8] = 0xFF // Invalid address type

	_, err := DecodeStreamOpen(data)
	if err == nil {
		t.Error("DecodeStreamOpen() should fail with invalid address type")
	}
}

func TestDecodePeerHello_TooShort(t *testing.T) {
	_, err := DecodePeerHello(make([]byte, 10))
	if err == nil {
		t.Error("DecodePeerHello() should fail with short data")
	}
}

func TestDecodeKeepalive_TooShort(t *testing.T) {
	_, err := DecodeKeepalive(make([]byte, 4))
	if err == nil {
		t.Error("DecodeKeepalive() should fail with short data")
	}
}

func TestDecodeStreamReset_TooShort(t *testing.T) {
	_, err := DecodeStreamReset(make([]byte, 1))
	if err == nil {
		t.Error("DecodeStreamReset() should fail with short data")
	}
}

func TestConstants(t *testing.T) {
	if HeaderSize != 14 {
		t.Errorf("HeaderSize = %d, want 14", HeaderSize)
	}
	if MaxPayloadSize != 16384 {
		t.Errorf("MaxPayloadSize = %d, want 16384", MaxPayloadSize)
	}
	if MaxFrameSize != HeaderSize+MaxPayloadSize {
		t.Errorf("MaxFrameSize = %d, want %d", MaxFrameSize, HeaderSize+MaxPayloadSize)
	}
	if ControlStreamID != 0 {
		t.Errorf("ControlStreamID = %d, want 0", ControlStreamID)
	}
	if ProtocolVersion != 1 {
		t.Errorf("ProtocolVersion = %d, want 1", ProtocolVersion)
	}
}

func BenchmarkFrame_Encode(b *testing.B) {
	f := &Frame{
		Type:     FrameStreamData,
		Flags:    0,
		StreamID: 12345,
		Payload:  make([]byte, 1024),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = f.Encode()
	}
}

func BenchmarkFrame_Decode(b *testing.B) {
	f := &Frame{
		Type:     FrameStreamData,
		Flags:    0,
		StreamID: 12345,
		Payload:  make([]byte, 1024),
	}
	data, _ := f.Encode()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Decode(data)
	}
}
