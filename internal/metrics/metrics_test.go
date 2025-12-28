package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestNewMetrics(t *testing.T) {
	// Create a new registry for isolated testing
	reg := prometheus.NewRegistry()
	m := NewMetricsWithRegistry(reg)

	if m == nil {
		t.Fatal("NewMetricsWithRegistry returned nil")
	}

	// Verify metrics are registered
	if m.PeersConnected == nil {
		t.Error("PeersConnected metric is nil")
	}
	if m.StreamsActive == nil {
		t.Error("StreamsActive metric is nil")
	}
	if m.BytesSent == nil {
		t.Error("BytesSent metric is nil")
	}
}

func TestRecordPeerConnect(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetricsWithRegistry(reg)

	// Record some peer connections
	m.RecordPeerConnect("quic", "outbound")
	m.RecordPeerConnect("quic", "inbound")
	m.RecordPeerConnect("h2", "outbound")

	// Check PeersConnected gauge
	peersConnected := testutil.ToFloat64(m.PeersConnected)
	if peersConnected != 3 {
		t.Errorf("PeersConnected = %v, want 3", peersConnected)
	}

	// Check PeersTotal counter
	peersTotal := testutil.ToFloat64(m.PeersTotal)
	if peersTotal != 3 {
		t.Errorf("PeersTotal = %v, want 3", peersTotal)
	}
}

func TestRecordPeerDisconnect(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetricsWithRegistry(reg)

	// Connect some peers
	m.RecordPeerConnect("quic", "outbound")
	m.RecordPeerConnect("quic", "inbound")

	// Disconnect one
	m.RecordPeerDisconnect("timeout")

	peersConnected := testutil.ToFloat64(m.PeersConnected)
	if peersConnected != 1 {
		t.Errorf("PeersConnected = %v, want 1", peersConnected)
	}
}

func TestRecordStreamOpenClose(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetricsWithRegistry(reg)

	// Open streams
	m.RecordStreamOpen(0.1)
	m.RecordStreamOpen(0.2)
	m.RecordStreamOpen(0.05)

	activeStreams := testutil.ToFloat64(m.StreamsActive)
	if activeStreams != 3 {
		t.Errorf("StreamsActive = %v, want 3", activeStreams)
	}

	// Close a stream
	m.RecordStreamClose()

	activeStreams = testutil.ToFloat64(m.StreamsActive)
	if activeStreams != 2 {
		t.Errorf("StreamsActive = %v, want 2", activeStreams)
	}

	// Verify streams opened counter
	streamsOpened := testutil.ToFloat64(m.StreamsOpened)
	if streamsOpened != 3 {
		t.Errorf("StreamsOpened = %v, want 3", streamsOpened)
	}

	streamsClosed := testutil.ToFloat64(m.StreamsClosed)
	if streamsClosed != 1 {
		t.Errorf("StreamsClosed = %v, want 1", streamsClosed)
	}
}

func TestRecordBytesTransfer(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetricsWithRegistry(reg)

	m.RecordBytesSent("stream", 1000)
	m.RecordBytesSent("stream", 500)
	m.RecordBytesSent("control", 100)

	m.RecordBytesReceived("stream", 2000)
	m.RecordBytesReceived("control", 50)

	// Check bytes sent
	streamSent := testutil.ToFloat64(m.BytesSent.WithLabelValues("stream"))
	if streamSent != 1500 {
		t.Errorf("BytesSent[stream] = %v, want 1500", streamSent)
	}

	controlSent := testutil.ToFloat64(m.BytesSent.WithLabelValues("control"))
	if controlSent != 100 {
		t.Errorf("BytesSent[control] = %v, want 100", controlSent)
	}

	// Check bytes received
	streamRecv := testutil.ToFloat64(m.BytesReceived.WithLabelValues("stream"))
	if streamRecv != 2000 {
		t.Errorf("BytesReceived[stream] = %v, want 2000", streamRecv)
	}
}

func TestRecordFrames(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetricsWithRegistry(reg)

	m.RecordFrameSent("STREAM_DATA")
	m.RecordFrameSent("STREAM_DATA")
	m.RecordFrameSent("KEEPALIVE")
	m.RecordFrameReceived("STREAM_DATA")

	streamDataSent := testutil.ToFloat64(m.FramesSent.WithLabelValues("STREAM_DATA"))
	if streamDataSent != 2 {
		t.Errorf("FramesSent[STREAM_DATA] = %v, want 2", streamDataSent)
	}

	keepaliveSent := testutil.ToFloat64(m.FramesSent.WithLabelValues("KEEPALIVE"))
	if keepaliveSent != 1 {
		t.Errorf("FramesSent[KEEPALIVE] = %v, want 1", keepaliveSent)
	}
}

func TestRecordRouting(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetricsWithRegistry(reg)

	m.SetRoutesTotal(100)
	m.RecordRouteAdvertise()
	m.RecordRouteAdvertise()
	m.RecordRouteWithdrawal()

	routesTotal := testutil.ToFloat64(m.RoutesTotal)
	if routesTotal != 100 {
		t.Errorf("RoutesTotal = %v, want 100", routesTotal)
	}

	routeAdv := testutil.ToFloat64(m.RouteAdvertises)
	if routeAdv != 2 {
		t.Errorf("RouteAdvertises = %v, want 2", routeAdv)
	}

	routeWithdraw := testutil.ToFloat64(m.RouteWithdrawals)
	if routeWithdraw != 1 {
		t.Errorf("RouteWithdrawals = %v, want 1", routeWithdraw)
	}
}

func TestRecordHandshake(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetricsWithRegistry(reg)

	m.RecordHandshake(0.5)
	m.RecordHandshake(0.3)
	m.RecordHandshakeError("timeout")
	m.RecordHandshakeError("version_mismatch")
	m.RecordHandshakeError("timeout")

	timeoutErrors := testutil.ToFloat64(m.HandshakeErrors.WithLabelValues("timeout"))
	if timeoutErrors != 2 {
		t.Errorf("HandshakeErrors[timeout] = %v, want 2", timeoutErrors)
	}

	versionErrors := testutil.ToFloat64(m.HandshakeErrors.WithLabelValues("version_mismatch"))
	if versionErrors != 1 {
		t.Errorf("HandshakeErrors[version_mismatch] = %v, want 1", versionErrors)
	}
}

func TestRecordKeepalive(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetricsWithRegistry(reg)

	m.RecordKeepaliveSent()
	m.RecordKeepaliveSent()
	m.RecordKeepaliveRecv(0.01)
	m.RecordKeepaliveRecv(0.02)

	sent := testutil.ToFloat64(m.KeepalivesSent)
	if sent != 2 {
		t.Errorf("KeepalivesSent = %v, want 2", sent)
	}

	recv := testutil.ToFloat64(m.KeepalivesRecv)
	if recv != 2 {
		t.Errorf("KeepalivesRecv = %v, want 2", recv)
	}
}

func TestRecordSOCKS5(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetricsWithRegistry(reg)

	m.RecordSOCKS5Connect()
	m.RecordSOCKS5Connect()
	m.RecordSOCKS5Disconnect()
	m.RecordSOCKS5AuthFailure()
	m.RecordSOCKS5Latency(0.5)

	active := testutil.ToFloat64(m.SOCKS5Connections)
	if active != 1 {
		t.Errorf("SOCKS5Connections = %v, want 1", active)
	}

	total := testutil.ToFloat64(m.SOCKS5ConnectionsTotal)
	if total != 2 {
		t.Errorf("SOCKS5ConnectionsTotal = %v, want 2", total)
	}

	failures := testutil.ToFloat64(m.SOCKS5AuthFailures)
	if failures != 1 {
		t.Errorf("SOCKS5AuthFailures = %v, want 1", failures)
	}
}

func TestRecordExit(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetricsWithRegistry(reg)

	m.RecordExitConnect()
	m.RecordExitConnect()
	m.RecordExitDisconnect()
	m.RecordExitDNS(0.01)
	m.RecordExitError("connection_refused")

	active := testutil.ToFloat64(m.ExitConnections)
	if active != 1 {
		t.Errorf("ExitConnections = %v, want 1", active)
	}

	total := testutil.ToFloat64(m.ExitConnectionsTotal)
	if total != 2 {
		t.Errorf("ExitConnectionsTotal = %v, want 2", total)
	}

	dnsQueries := testutil.ToFloat64(m.ExitDNSQueries)
	if dnsQueries != 1 {
		t.Errorf("ExitDNSQueries = %v, want 1", dnsQueries)
	}

	errors := testutil.ToFloat64(m.ExitErrors.WithLabelValues("connection_refused"))
	if errors != 1 {
		t.Errorf("ExitErrors[connection_refused] = %v, want 1", errors)
	}
}

func TestDefaultMetrics(t *testing.T) {
	m1 := Default()
	m2 := Default()

	if m1 != m2 {
		t.Error("Default() should return same instance")
	}

	if m1 == nil {
		t.Error("Default() returned nil")
	}
}

func TestStreamErrors(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetricsWithRegistry(reg)

	m.RecordStreamError("timeout")
	m.RecordStreamError("reset")
	m.RecordStreamError("timeout")

	timeoutErrors := testutil.ToFloat64(m.StreamErrors.WithLabelValues("timeout"))
	if timeoutErrors != 2 {
		t.Errorf("StreamErrors[timeout] = %v, want 2", timeoutErrors)
	}

	resetErrors := testutil.ToFloat64(m.StreamErrors.WithLabelValues("reset"))
	if resetErrors != 1 {
		t.Errorf("StreamErrors[reset] = %v, want 1", resetErrors)
	}
}
