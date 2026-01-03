// Package protocol defines the wire protocol for Muti Metroo mesh communication.
package protocol

// Frame type constants
const (
	// Stream frames
	FrameStreamOpen    uint8 = 0x01 // Request to open stream
	FrameStreamOpenAck uint8 = 0x02 // Stream opened successfully
	FrameStreamOpenErr uint8 = 0x03 // Stream open failed
	FrameStreamData    uint8 = 0x04 // Payload data
	FrameStreamClose   uint8 = 0x05 // Graceful close
	FrameStreamReset   uint8 = 0x06 // Abort stream

	// Routing frames
	FrameRouteAdvertise    uint8 = 0x10 // Announce CIDR routes
	FrameRouteWithdraw     uint8 = 0x11 // Remove CIDR routes
	FrameNodeInfoAdvertise uint8 = 0x12 // Announce node metadata

	// Control frames
	FramePeerHello    uint8 = 0x20 // Initial handshake
	FramePeerHelloAck uint8 = 0x21 // Handshake response
	FrameKeepalive    uint8 = 0x22 // Liveness probe
	FrameKeepaliveAck uint8 = 0x23 // Liveness response

	// Mesh control frames (for remote metrics/status)
	FrameControlRequest  uint8 = 0x24 // Request metrics/status from remote agent
	FrameControlResponse uint8 = 0x25 // Response with metrics/status data
)

// Control request types
const (
	ControlTypeMetrics uint8 = 0x01 // Request Prometheus metrics
	ControlTypeStatus  uint8 = 0x02 // Request agent status
	ControlTypePeers   uint8 = 0x03 // Request peer list
	ControlTypeRoutes  uint8 = 0x04 // Request route table
	ControlTypeRPC     uint8 = 0x05 // Remote procedure call (shell command)
	// 0x06 and 0x07 reserved (previously used for legacy file transfer)
)

// Frame flags
const (
	FlagFinWrite uint8 = 0x01 // Sender done writing
	FlagFinRead  uint8 = 0x02 // Sender done reading
)

// Address type constants
const (
	AddrTypeIPv4   uint8 = 0x01 // 4 bytes
	AddrTypeIPv6   uint8 = 0x04 // 16 bytes
	AddrTypeDomain uint8 = 0x03 // 1-byte length + string
)

// Address family constants (for routes)
const (
	AddrFamilyIPv4 uint8 = 0x01
	AddrFamilyIPv6 uint8 = 0x02
)

// Error codes for STREAM_OPEN_ERR and STREAM_RESET
const (
	ErrNoRoute            uint16 = 1
	ErrConnectionRefused  uint16 = 2
	ErrConnectionTimeout  uint16 = 3
	ErrTTLExceeded        uint16 = 4
	ErrHostUnreachable    uint16 = 5
	ErrNetworkUnreachable uint16 = 6
	ErrDNSError           uint16 = 7
	ErrExitDisabled       uint16 = 8
	ErrResourceLimit      uint16 = 9
	ErrConnectionLimit    uint16 = 10
	ErrNotAllowed         uint16 = 11
	ErrFileTransferDenied uint16 = 12
	ErrAuthRequired       uint16 = 13
	ErrPathNotAllowed     uint16 = 14
	ErrFileTooLarge       uint16 = 15
	ErrFileNotFound       uint16 = 16
	ErrWriteFailed        uint16 = 17
	ErrGeneralFailure     uint16 = 18
	ErrResumeFailed       uint16 = 19  // File changed since partial transfer, resume not possible
)

// Protocol constants
const (
	// ProtocolVersion is the current protocol version
	ProtocolVersion uint16 = 1

	// HeaderSize is the size of a frame header in bytes
	HeaderSize = 14

	// MaxPayloadSize is the maximum frame payload size (16 KB)
	MaxPayloadSize = 16384

	// MaxFrameSize is the maximum total frame size
	MaxFrameSize = HeaderSize + MaxPayloadSize

	// ControlStreamID is reserved for control messages
	ControlStreamID uint64 = 0
)

// File transfer stream addresses (used with AddrTypeDomain)
const (
	// FileTransferUpload is the domain address for file upload streams
	FileTransferUpload = "file:upload"

	// FileTransferDownload is the domain address for file download streams
	FileTransferDownload = "file:download"
)

// FrameTypeName returns a human-readable name for a frame type.
func FrameTypeName(t uint8) string {
	switch t {
	case FrameStreamOpen:
		return "STREAM_OPEN"
	case FrameStreamOpenAck:
		return "STREAM_OPEN_ACK"
	case FrameStreamOpenErr:
		return "STREAM_OPEN_ERR"
	case FrameStreamData:
		return "STREAM_DATA"
	case FrameStreamClose:
		return "STREAM_CLOSE"
	case FrameStreamReset:
		return "STREAM_RESET"
	case FrameRouteAdvertise:
		return "ROUTE_ADVERTISE"
	case FrameRouteWithdraw:
		return "ROUTE_WITHDRAW"
	case FrameNodeInfoAdvertise:
		return "NODE_INFO_ADVERTISE"
	case FramePeerHello:
		return "PEER_HELLO"
	case FramePeerHelloAck:
		return "PEER_HELLO_ACK"
	case FrameKeepalive:
		return "KEEPALIVE"
	case FrameKeepaliveAck:
		return "KEEPALIVE_ACK"
	case FrameControlRequest:
		return "CONTROL_REQUEST"
	case FrameControlResponse:
		return "CONTROL_RESPONSE"
	default:
		return "UNKNOWN"
	}
}

// ErrorCodeName returns a human-readable name for an error code.
func ErrorCodeName(code uint16) string {
	switch code {
	case ErrNoRoute:
		return "NO_ROUTE"
	case ErrConnectionRefused:
		return "CONNECTION_REFUSED"
	case ErrConnectionTimeout:
		return "CONNECTION_TIMEOUT"
	case ErrTTLExceeded:
		return "TTL_EXCEEDED"
	case ErrHostUnreachable:
		return "HOST_UNREACHABLE"
	case ErrNetworkUnreachable:
		return "NETWORK_UNREACHABLE"
	case ErrDNSError:
		return "DNS_ERROR"
	case ErrExitDisabled:
		return "EXIT_DISABLED"
	case ErrResourceLimit:
		return "RESOURCE_LIMIT"
	case ErrConnectionLimit:
		return "CONNECTION_LIMIT"
	case ErrNotAllowed:
		return "NOT_ALLOWED"
	case ErrFileTransferDenied:
		return "FILE_TRANSFER_DENIED"
	case ErrAuthRequired:
		return "AUTH_REQUIRED"
	case ErrPathNotAllowed:
		return "PATH_NOT_ALLOWED"
	case ErrFileTooLarge:
		return "FILE_TOO_LARGE"
	case ErrFileNotFound:
		return "FILE_NOT_FOUND"
	case ErrWriteFailed:
		return "WRITE_FAILED"
	case ErrGeneralFailure:
		return "GENERAL_FAILURE"
	case ErrResumeFailed:
		return "RESUME_FAILED"
	default:
		return "UNKNOWN"
	}
}

// IsStreamFrame returns true if the frame type is a stream-related frame.
func IsStreamFrame(t uint8) bool {
	return t >= FrameStreamOpen && t <= FrameStreamReset
}

// IsRoutingFrame returns true if the frame type is a routing-related frame.
func IsRoutingFrame(t uint8) bool {
	return t == FrameRouteAdvertise || t == FrameRouteWithdraw || t == FrameNodeInfoAdvertise
}

// IsControlFrame returns true if the frame type is a control frame.
func IsControlFrame(t uint8) bool {
	return t >= FramePeerHello && t <= FrameControlResponse
}
