// Package metrics provides Prometheus metrics for Muti Metroo.
package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	namespace = "muti_metroo"
)

// Metrics contains all Prometheus metrics for the agent.
type Metrics struct {
	// Connection metrics
	PeersConnected   prometheus.Gauge
	PeersTotal       prometheus.Counter
	PeerConnections  *prometheus.CounterVec
	PeerDisconnects  *prometheus.CounterVec

	// Stream metrics
	StreamsActive     prometheus.Gauge
	StreamsOpened     prometheus.Counter
	StreamsClosed     prometheus.Counter
	StreamOpenLatency prometheus.Histogram
	StreamErrors      *prometheus.CounterVec

	// Data transfer metrics
	BytesSent     *prometheus.CounterVec
	BytesReceived *prometheus.CounterVec
	FramesSent    *prometheus.CounterVec
	FramesReceived *prometheus.CounterVec

	// Routing metrics
	RoutesTotal       prometheus.Gauge
	RouteAdvertises   prometheus.Counter
	RouteWithdrawals  prometheus.Counter
	RouteFloodLatency prometheus.Histogram

	// SOCKS5 metrics
	SOCKS5Connections    prometheus.Gauge
	SOCKS5ConnectionsTotal prometheus.Counter
	SOCKS5AuthFailures   prometheus.Counter
	SOCKS5ConnectLatency prometheus.Histogram

	// Exit handler metrics
	ExitConnections      prometheus.Gauge
	ExitConnectionsTotal prometheus.Counter
	ExitDNSQueries       prometheus.Counter
	ExitDNSLatency       prometheus.Histogram
	ExitErrors           *prometheus.CounterVec

	// Protocol metrics
	HandshakeLatency prometheus.Histogram
	HandshakeErrors  *prometheus.CounterVec
	KeepalivesSent   prometheus.Counter
	KeepalivesRecv   prometheus.Counter
	KeepaliveRTT     prometheus.Histogram
}

var (
	defaultMetrics *Metrics
	metricsOnce    sync.Once
)

// Default returns the default metrics instance.
func Default() *Metrics {
	metricsOnce.Do(func() {
		defaultMetrics = NewMetrics()
	})
	return defaultMetrics
}

// NewMetrics creates a new Metrics instance with all metrics registered.
func NewMetrics() *Metrics {
	return NewMetricsWithRegistry(prometheus.DefaultRegisterer)
}

// NewMetricsWithRegistry creates a new Metrics instance with a custom registry.
func NewMetricsWithRegistry(reg prometheus.Registerer) *Metrics {
	factory := promauto.With(reg)

	m := &Metrics{
		// Connection metrics
		PeersConnected: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "peers_connected",
			Help:      "Number of currently connected peers",
		}),
		PeersTotal: factory.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "peers_total",
			Help:      "Total number of peer connections established",
		}),
		PeerConnections: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "peer_connections_total",
			Help:      "Total peer connections by transport type",
		}, []string{"transport", "direction"}),
		PeerDisconnects: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "peer_disconnects_total",
			Help:      "Total peer disconnections by reason",
		}, []string{"reason"}),

		// Stream metrics
		StreamsActive: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "streams_active",
			Help:      "Number of currently active streams",
		}),
		StreamsOpened: factory.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "streams_opened_total",
			Help:      "Total number of streams opened",
		}),
		StreamsClosed: factory.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "streams_closed_total",
			Help:      "Total number of streams closed",
		}),
		StreamOpenLatency: factory.NewHistogram(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "stream_open_latency_seconds",
			Help:      "Histogram of stream open latency in seconds",
			Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		}),
		StreamErrors: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "stream_errors_total",
			Help:      "Total stream errors by type",
		}, []string{"error_type"}),

		// Data transfer metrics
		BytesSent: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "bytes_sent_total",
			Help:      "Total bytes sent by type",
		}, []string{"type"}),
		BytesReceived: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "bytes_received_total",
			Help:      "Total bytes received by type",
		}, []string{"type"}),
		FramesSent: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "frames_sent_total",
			Help:      "Total frames sent by type",
		}, []string{"frame_type"}),
		FramesReceived: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "frames_received_total",
			Help:      "Total frames received by type",
		}, []string{"frame_type"}),

		// Routing metrics
		RoutesTotal: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "routes_total",
			Help:      "Total number of routes in routing table",
		}),
		RouteAdvertises: factory.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "route_advertises_total",
			Help:      "Total route advertisements processed",
		}),
		RouteWithdrawals: factory.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "route_withdrawals_total",
			Help:      "Total route withdrawals processed",
		}),
		RouteFloodLatency: factory.NewHistogram(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "route_flood_latency_seconds",
			Help:      "Histogram of route flood propagation latency",
			Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1},
		}),

		// SOCKS5 metrics
		SOCKS5Connections: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "socks5_connections_active",
			Help:      "Number of active SOCKS5 connections",
		}),
		SOCKS5ConnectionsTotal: factory.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "socks5_connections_total",
			Help:      "Total SOCKS5 connections",
		}),
		SOCKS5AuthFailures: factory.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "socks5_auth_failures_total",
			Help:      "Total SOCKS5 authentication failures",
		}),
		SOCKS5ConnectLatency: factory.NewHistogram(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "socks5_connect_latency_seconds",
			Help:      "Histogram of SOCKS5 connect request latency",
			Buckets:   []float64{.01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		}),

		// Exit handler metrics
		ExitConnections: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "exit_connections_active",
			Help:      "Number of active exit connections",
		}),
		ExitConnectionsTotal: factory.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "exit_connections_total",
			Help:      "Total exit connections",
		}),
		ExitDNSQueries: factory.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "exit_dns_queries_total",
			Help:      "Total DNS queries performed",
		}),
		ExitDNSLatency: factory.NewHistogram(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "exit_dns_latency_seconds",
			Help:      "Histogram of DNS query latency",
			Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1},
		}),
		ExitErrors: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "exit_errors_total",
			Help:      "Total exit handler errors by type",
		}, []string{"error_type"}),

		// Protocol metrics
		HandshakeLatency: factory.NewHistogram(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "handshake_latency_seconds",
			Help:      "Histogram of peer handshake latency",
			Buckets:   []float64{.01, .025, .05, .1, .25, .5, 1, 2.5, 5},
		}),
		HandshakeErrors: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "handshake_errors_total",
			Help:      "Total handshake errors by type",
		}, []string{"error_type"}),
		KeepalivesSent: factory.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "keepalives_sent_total",
			Help:      "Total keepalive messages sent",
		}),
		KeepalivesRecv: factory.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "keepalives_received_total",
			Help:      "Total keepalive messages received",
		}),
		KeepaliveRTT: factory.NewHistogram(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "keepalive_rtt_seconds",
			Help:      "Histogram of keepalive round-trip time",
			Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1},
		}),
	}

	return m
}

// RecordPeerConnect records a new peer connection.
func (m *Metrics) RecordPeerConnect(transport, direction string) {
	m.PeersConnected.Inc()
	m.PeersTotal.Inc()
	m.PeerConnections.WithLabelValues(transport, direction).Inc()
}

// RecordPeerDisconnect records a peer disconnection.
func (m *Metrics) RecordPeerDisconnect(reason string) {
	m.PeersConnected.Dec()
	m.PeerDisconnects.WithLabelValues(reason).Inc()
}

// RecordStreamOpen records a stream being opened.
func (m *Metrics) RecordStreamOpen(latencySeconds float64) {
	m.StreamsActive.Inc()
	m.StreamsOpened.Inc()
	m.StreamOpenLatency.Observe(latencySeconds)
}

// RecordStreamClose records a stream being closed.
func (m *Metrics) RecordStreamClose() {
	m.StreamsActive.Dec()
	m.StreamsClosed.Inc()
}

// RecordStreamError records a stream error.
func (m *Metrics) RecordStreamError(errorType string) {
	m.StreamErrors.WithLabelValues(errorType).Inc()
}

// RecordBytesSent records bytes sent.
func (m *Metrics) RecordBytesSent(dataType string, bytes int) {
	m.BytesSent.WithLabelValues(dataType).Add(float64(bytes))
}

// RecordBytesReceived records bytes received.
func (m *Metrics) RecordBytesReceived(dataType string, bytes int) {
	m.BytesReceived.WithLabelValues(dataType).Add(float64(bytes))
}

// RecordFrameSent records a frame being sent.
func (m *Metrics) RecordFrameSent(frameType string) {
	m.FramesSent.WithLabelValues(frameType).Inc()
}

// RecordFrameReceived records a frame being received.
func (m *Metrics) RecordFrameReceived(frameType string) {
	m.FramesReceived.WithLabelValues(frameType).Inc()
}

// SetRoutesTotal sets the total number of routes.
func (m *Metrics) SetRoutesTotal(count int) {
	m.RoutesTotal.Set(float64(count))
}

// RecordRouteAdvertise records a route advertisement.
func (m *Metrics) RecordRouteAdvertise() {
	m.RouteAdvertises.Inc()
}

// RecordRouteWithdrawal records a route withdrawal.
func (m *Metrics) RecordRouteWithdrawal() {
	m.RouteWithdrawals.Inc()
}

// RecordHandshake records a successful handshake.
func (m *Metrics) RecordHandshake(latencySeconds float64) {
	m.HandshakeLatency.Observe(latencySeconds)
}

// RecordHandshakeError records a handshake error.
func (m *Metrics) RecordHandshakeError(errorType string) {
	m.HandshakeErrors.WithLabelValues(errorType).Inc()
}

// RecordKeepaliveSent records a keepalive sent.
func (m *Metrics) RecordKeepaliveSent() {
	m.KeepalivesSent.Inc()
}

// RecordKeepaliveRecv records a keepalive received with RTT.
func (m *Metrics) RecordKeepaliveRecv(rttSeconds float64) {
	m.KeepalivesRecv.Inc()
	m.KeepaliveRTT.Observe(rttSeconds)
}

// SOCKS5 metrics helpers

// RecordSOCKS5Connect records a SOCKS5 connection.
func (m *Metrics) RecordSOCKS5Connect() {
	m.SOCKS5Connections.Inc()
	m.SOCKS5ConnectionsTotal.Inc()
}

// RecordSOCKS5Disconnect records a SOCKS5 disconnection.
func (m *Metrics) RecordSOCKS5Disconnect() {
	m.SOCKS5Connections.Dec()
}

// RecordSOCKS5AuthFailure records a SOCKS5 auth failure.
func (m *Metrics) RecordSOCKS5AuthFailure() {
	m.SOCKS5AuthFailures.Inc()
}

// RecordSOCKS5Latency records SOCKS5 connect latency.
func (m *Metrics) RecordSOCKS5Latency(latencySeconds float64) {
	m.SOCKS5ConnectLatency.Observe(latencySeconds)
}

// Exit metrics helpers

// RecordExitConnect records an exit connection.
func (m *Metrics) RecordExitConnect() {
	m.ExitConnections.Inc()
	m.ExitConnectionsTotal.Inc()
}

// RecordExitDisconnect records an exit disconnection.
func (m *Metrics) RecordExitDisconnect() {
	m.ExitConnections.Dec()
}

// RecordExitDNS records a DNS query.
func (m *Metrics) RecordExitDNS(latencySeconds float64) {
	m.ExitDNSQueries.Inc()
	m.ExitDNSLatency.Observe(latencySeconds)
}

// RecordExitError records an exit error.
func (m *Metrics) RecordExitError(errorType string) {
	m.ExitErrors.WithLabelValues(errorType).Inc()
}
