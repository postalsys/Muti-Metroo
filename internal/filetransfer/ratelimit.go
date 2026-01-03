package filetransfer

import (
	"context"
	"io"

	"golang.org/x/time/rate"
)

// RateLimitedReader wraps an io.Reader with rate limiting using a token bucket algorithm.
// It limits the read throughput to bytesPerSecond bytes per second.
type RateLimitedReader struct {
	r       io.Reader
	limiter *rate.Limiter
	ctx     context.Context
}

// NewRateLimitedReader creates a rate-limited reader that limits throughput to bytesPerSecond.
// If bytesPerSecond is 0 or negative, the reader is returned without rate limiting.
// The burst size is set to 16KB (one frame) for efficient transfer.
func NewRateLimitedReader(ctx context.Context, r io.Reader, bytesPerSecond int64) io.Reader {
	if bytesPerSecond <= 0 {
		return r
	}

	// Burst size is 16KB (one frame) for efficient transfer
	const burstSize = 16 * 1024

	// Create a rate limiter with the specified bytes per second
	// The rate is in events (bytes) per second, burst allows accumulating up to burstSize bytes
	limiter := rate.NewLimiter(rate.Limit(bytesPerSecond), burstSize)

	return &RateLimitedReader{
		r:       r,
		limiter: limiter,
		ctx:     ctx,
	}
}

// Read implements io.Reader with rate limiting.
// It waits for tokens from the rate limiter before returning data.
func (r *RateLimitedReader) Read(p []byte) (int, error) {
	// First, check if context is cancelled
	select {
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	default:
	}

	// Read from the underlying reader
	n, err := r.r.Read(p)
	if n <= 0 {
		return n, err
	}

	// Wait for rate limiter to allow this many bytes
	// WaitN blocks until n tokens are available or context is cancelled
	if waitErr := r.limiter.WaitN(r.ctx, n); waitErr != nil {
		return n, waitErr
	}

	return n, err
}

// RateLimitedWriter wraps an io.Writer with rate limiting using a token bucket algorithm.
// It limits the write throughput to bytesPerSecond bytes per second.
type RateLimitedWriter struct {
	w       io.Writer
	limiter *rate.Limiter
	ctx     context.Context
}

// NewRateLimitedWriter creates a rate-limited writer that limits throughput to bytesPerSecond.
// If bytesPerSecond is 0 or negative, the writer is returned without rate limiting.
// The burst size is set to 16KB (one frame) for efficient transfer.
func NewRateLimitedWriter(ctx context.Context, w io.Writer, bytesPerSecond int64) io.Writer {
	if bytesPerSecond <= 0 {
		return w
	}

	// Burst size is 16KB (one frame) for efficient transfer
	const burstSize = 16 * 1024

	// Create a rate limiter with the specified bytes per second
	limiter := rate.NewLimiter(rate.Limit(bytesPerSecond), burstSize)

	return &RateLimitedWriter{
		w:       w,
		limiter: limiter,
		ctx:     ctx,
	}
}

// Write implements io.Writer with rate limiting.
// It waits for tokens from the rate limiter before writing data.
func (w *RateLimitedWriter) Write(p []byte) (int, error) {
	// First, check if context is cancelled
	select {
	case <-w.ctx.Done():
		return 0, w.ctx.Err()
	default:
	}

	// Wait for rate limiter to allow this many bytes
	if err := w.limiter.WaitN(w.ctx, len(p)); err != nil {
		return 0, err
	}

	// Write to the underlying writer
	return w.w.Write(p)
}
