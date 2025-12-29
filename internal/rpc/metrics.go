package rpc

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// RPCCallsTotal counts total RPC calls by result type.
	RPCCallsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "muti_metroo",
			Subsystem: "rpc",
			Name:      "calls_total",
			Help:      "Total number of RPC calls by result type",
		},
		[]string{"result", "command"},
	)

	// RPCCallDuration measures RPC call duration.
	RPCCallDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "muti_metroo",
			Subsystem: "rpc",
			Name:      "call_duration_seconds",
			Help:      "Duration of RPC calls in seconds",
			Buckets:   prometheus.ExponentialBuckets(0.001, 2, 15), // 1ms to ~30s
		},
		[]string{"command"},
	)

	// RPCBytesReceived measures bytes received in RPC requests.
	RPCBytesReceived = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: "muti_metroo",
			Subsystem: "rpc",
			Name:      "bytes_received_total",
			Help:      "Total bytes received in RPC requests (stdin)",
		},
	)

	// RPCBytesSent measures bytes sent in RPC responses.
	RPCBytesSent = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: "muti_metroo",
			Subsystem: "rpc",
			Name:      "bytes_sent_total",
			Help:      "Total bytes sent in RPC responses (stdout + stderr)",
		},
	)
)

// Result type constants for metrics.
const (
	ResultSuccess     = "success"       // Exit code 0
	ResultFailed      = "failed"        // Exit code > 0
	ResultRejected    = "rejected"      // Not whitelisted
	ResultAuthFailed  = "auth_failed"   // Authentication failed
	ResultError       = "error"         // Execution error
)

// RecordSuccess records a successful RPC call (exit code 0).
func RecordSuccess(command string, duration float64, outputSize int) {
	RPCCallsTotal.WithLabelValues(ResultSuccess, command).Inc()
	RPCCallDuration.WithLabelValues(command).Observe(duration)
	RPCBytesSent.Add(float64(outputSize))
}

// RecordFailed records a failed RPC call (exit code > 0).
func RecordFailed(command string, duration float64, outputSize int) {
	RPCCallsTotal.WithLabelValues(ResultFailed, command).Inc()
	RPCCallDuration.WithLabelValues(command).Observe(duration)
	RPCBytesSent.Add(float64(outputSize))
}

// RecordRejected records a rejected RPC call (not whitelisted).
func RecordRejected(command string) {
	RPCCallsTotal.WithLabelValues(ResultRejected, command).Inc()
}

// RecordAuthFailed records an authentication failure.
func RecordAuthFailed() {
	RPCCallsTotal.WithLabelValues(ResultAuthFailed, "").Inc()
}

// RecordError records an execution error.
func RecordError(command string) {
	RPCCallsTotal.WithLabelValues(ResultError, command).Inc()
}

// RecordRequestSize records the size of an incoming RPC request.
func RecordRequestSize(size int) {
	RPCBytesReceived.Add(float64(size))
}
