package shell

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// ShellSessionsActive is a gauge of active shell sessions.
	ShellSessionsActive = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "muti_metroo",
			Subsystem: "shell",
			Name:      "sessions_active",
			Help:      "Number of currently active shell sessions",
		},
	)

	// ShellSessionsTotal counts total shell sessions by type and result.
	ShellSessionsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "muti_metroo",
			Subsystem: "shell",
			Name:      "sessions_total",
			Help:      "Total number of shell sessions by type and result",
		},
		[]string{"type", "result"},
	)

	// ShellDurationSeconds measures shell session duration.
	ShellDurationSeconds = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: "muti_metroo",
			Subsystem: "shell",
			Name:      "duration_seconds",
			Help:      "Duration of shell sessions in seconds",
			Buckets:   prometheus.ExponentialBuckets(1, 2, 15), // 1s to ~9 hours
		},
	)

	// ShellBytesTotal measures bytes transferred in shell sessions.
	ShellBytesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "muti_metroo",
			Subsystem: "shell",
			Name:      "bytes_total",
			Help:      "Total bytes transferred in shell sessions",
		},
		[]string{"direction"},
	)
)

// Session type constants for metrics.
const (
	TypeStream      = "stream"      // Streaming mode (no PTY)
	TypeInteractive = "interactive" // Interactive mode (PTY)
)

// Result constants for metrics.
const (
	ResultSuccess  = "success"  // Normal exit
	ResultError    = "error"    // Error during session
	ResultTimeout  = "timeout"  // Session timed out
	ResultRejected = "rejected" // Session rejected (auth, whitelist)
)

// Direction constants for byte metrics.
const (
	DirectionStdin  = "stdin"
	DirectionStdout = "stdout"
	DirectionStderr = "stderr"
)

// SessionStarted records a new shell session starting.
func SessionStarted() {
	ShellSessionsActive.Inc()
}

// SessionEnded records a shell session ending.
func SessionEnded(sessionType, result string, duration float64) {
	ShellSessionsActive.Dec()
	ShellSessionsTotal.WithLabelValues(sessionType, result).Inc()
	ShellDurationSeconds.Observe(duration)
}

// RecordBytes records bytes transferred in a shell session.
func RecordBytes(direction string, bytes int) {
	ShellBytesTotal.WithLabelValues(direction).Add(float64(bytes))
}

// RecordStdinBytes records bytes received from stdin.
func RecordStdinBytes(bytes int) {
	RecordBytes(DirectionStdin, bytes)
}

// RecordStdoutBytes records bytes sent to stdout.
func RecordStdoutBytes(bytes int) {
	RecordBytes(DirectionStdout, bytes)
}

// RecordStderrBytes records bytes sent to stderr.
func RecordStderrBytes(bytes int) {
	RecordBytes(DirectionStderr, bytes)
}

// RecordRejected records a rejected session (auth failure, not whitelisted).
func RecordRejected(sessionType string) {
	ShellSessionsTotal.WithLabelValues(sessionType, ResultRejected).Inc()
}
