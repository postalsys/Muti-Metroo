package filetransfer

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/text/unicode/norm"
)

// TransferMetadata is sent as the first data frame in a file transfer stream.
type TransferMetadata struct {
	Path        string `json:"path"`                   // Absolute destination path
	Mode        uint32 `json:"mode"`                   // File permissions (e.g., 0644)
	Size        int64  `json:"size"`                   // File size in bytes (-1 for directories)
	IsDirectory bool   `json:"is_directory"`           // True if transferring a directory
	Password    string `json:"password,omitempty"`     // Authentication password
	Compress    bool   `json:"compress"`               // Whether data is gzip compressed
	Checksum    string `json:"checksum,omitempty"`     // Optional SHA256 checksum (hex)
}

// TransferResult is sent back after a transfer completes (in download response metadata).
type TransferResult struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
	Written int64  `json:"written"` // Bytes written
}

// StreamConfig holds configuration for stream-based file transfer.
type StreamConfig struct {
	Enabled      bool
	MaxFileSize  int64    // 0 = unlimited
	AllowedPaths []string // Empty = all absolute paths allowed
	PasswordHash string   // bcrypt hash
	Compression  bool     // Whether to compress data (default true)
}

// StreamHandler handles file transfer stream operations.
type StreamHandler struct {
	cfg StreamConfig
}

// NewStreamHandler creates a new stream-based file transfer handler.
func NewStreamHandler(cfg StreamConfig) *StreamHandler {
	return &StreamHandler{cfg: cfg}
}

// ValidateUploadMetadata validates upload metadata and returns an error if invalid.
func (h *StreamHandler) ValidateUploadMetadata(meta *TransferMetadata) error {
	// Check if enabled
	if !h.cfg.Enabled {
		return fmt.Errorf("file transfer is disabled")
	}

	// Authenticate
	if err := h.authenticate(meta.Password); err != nil {
		return err
	}

	// Validate path
	if err := h.validatePath(meta.Path); err != nil {
		return err
	}

	// Check size limit (if not directory and size is known)
	if !meta.IsDirectory && meta.Size > 0 && h.cfg.MaxFileSize > 0 {
		if meta.Size > h.cfg.MaxFileSize {
			return fmt.Errorf("file too large: %d bytes (max %d)", meta.Size, h.cfg.MaxFileSize)
		}
	}

	return nil
}

// ValidateDownloadMetadata validates download metadata and returns an error if invalid.
func (h *StreamHandler) ValidateDownloadMetadata(meta *TransferMetadata) error {
	// Check if enabled
	if !h.cfg.Enabled {
		return fmt.Errorf("file transfer is disabled")
	}

	// Authenticate
	if err := h.authenticate(meta.Password); err != nil {
		return err
	}

	// Validate path
	if err := h.validatePath(meta.Path); err != nil {
		return err
	}

	// Check for symlinks and validate their targets
	if err := h.validateSymlinkTarget(meta.Path); err != nil {
		return err
	}

	// Check if path exists (follows symlinks)
	info, err := os.Stat(meta.Path)
	if err != nil {
		return fmt.Errorf("path not found: %w", err)
	}

	// Check size limit for files
	if !info.IsDir() && h.cfg.MaxFileSize > 0 {
		if info.Size() > h.cfg.MaxFileSize {
			return fmt.Errorf("file too large: %d bytes (max %d)", info.Size(), h.cfg.MaxFileSize)
		}
	}

	return nil
}

// validateSymlinkTarget checks if a path is a symlink and validates that its target
// is within the allowed paths. This prevents symlink-based escape attacks.
func (h *StreamHandler) validateSymlinkTarget(path string) error {
	// Use Lstat to check if the path itself is a symlink (doesn't follow symlinks)
	info, err := os.Lstat(path)
	if err != nil {
		// Path doesn't exist or can't be accessed - let later checks handle this
		return nil
	}

	// If not a symlink, nothing to validate
	if info.Mode()&os.ModeSymlink == 0 {
		return nil
	}

	// Resolve the symlink target
	target, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("cannot resolve symlink: %w", err)
	}

	// Validate the resolved target path
	if err := h.validatePath(target); err != nil {
		return fmt.Errorf("symlink target not allowed: %w", err)
	}

	return nil
}

// authenticate checks if the password is correct.
func (h *StreamHandler) authenticate(password string) error {
	if h.cfg.PasswordHash == "" {
		return nil // No authentication required
	}
	if password == "" {
		return fmt.Errorf("authentication required")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(h.cfg.PasswordHash), []byte(password)); err != nil {
		return fmt.Errorf("authentication failed")
	}
	return nil
}

// containsDangerousChars checks for null bytes and other dangerous characters in paths.
func containsDangerousChars(path string) bool {
	for _, r := range path {
		// Check for null bytes
		if r == 0 {
			return true
		}
		// Check for control characters (except common whitespace)
		if unicode.IsControl(r) && r != '\t' && r != '\n' && r != '\r' {
			return true
		}
	}
	return false
}

// normalizePath applies Unicode NFC normalization and cleans the path.
func normalizePath(path string) string {
	// Apply NFC normalization to prevent Unicode normalization attacks
	normalized := norm.NFC.String(path)
	// Clean the path to resolve . and .. components
	return filepath.Clean(normalized)
}

// isPathUnderPrefix checks if path is exactly prefix or is under prefix directory.
// This prevents prefix bypass attacks like /var/wwwevil matching /var/www.
func isPathUnderPrefix(path, prefix string) bool {
	// Normalize both paths
	cleanPath := normalizePath(path)
	cleanPrefix := normalizePath(prefix)

	// Exact match
	if cleanPath == cleanPrefix {
		return true
	}

	// Path must be under prefix (prefix + separator + something)
	// Ensure prefix ends with separator for proper matching
	if !strings.HasSuffix(cleanPrefix, string(filepath.Separator)) {
		cleanPrefix += string(filepath.Separator)
	}

	return strings.HasPrefix(cleanPath, cleanPrefix)
}

// validatePath checks if the path is allowed.
func (h *StreamHandler) validatePath(path string) error {
	// Check for null bytes and dangerous characters first (before any processing)
	if containsDangerousChars(path) {
		return fmt.Errorf("path contains dangerous characters")
	}

	// Apply Unicode normalization
	normalizedPath := normalizePath(path)

	// Must be absolute
	if !filepath.IsAbs(normalizedPath) {
		return fmt.Errorf("path must be absolute: %s", path)
	}

	// Check for directory traversal (after cleaning)
	if strings.Contains(normalizedPath, "..") {
		return fmt.Errorf("directory traversal not allowed")
	}

	// Check allowed paths with proper prefix matching
	if len(h.cfg.AllowedPaths) > 0 {
		allowed := false
		for _, prefix := range h.cfg.AllowedPaths {
			if isPathUnderPrefix(normalizedPath, prefix) {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("path not in allowed list: %s", path)
		}
	}

	return nil
}

// WriteUploadedFile writes uploaded data from a reader to the specified path.
// If isDirectory is true, it expects tar.gz data and extracts it.
// Returns the number of bytes written.
func (h *StreamHandler) WriteUploadedFile(path string, r io.Reader, mode uint32, isDirectory bool, compressed bool) (int64, error) {
	path = filepath.Clean(path)

	if isDirectory {
		// For directories, extract tar.gz to the path
		// UntarDirectory handles gzip decompression internally
		if err := UntarDirectory(r, path); err != nil {
			return 0, fmt.Errorf("failed to extract directory: %w", err)
		}
		// Calculate extracted size
		size, _ := CalculateDirectorySize(path)
		return size, nil
	}

	// For files, create parent directories
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return 0, fmt.Errorf("failed to create directory: %w", err)
	}

	// Create the file
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(mode))
	if err != nil {
		return 0, fmt.Errorf("failed to create file: %w", err)
	}
	defer f.Close()

	// If compressed, wrap with gzip reader
	var reader io.Reader = r
	if compressed {
		gzr, err := gzip.NewReader(r)
		if err != nil {
			return 0, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gzr.Close()
		reader = gzr
	}

	// Copy data
	written, err := io.Copy(f, reader)
	if err != nil {
		return written, fmt.Errorf("failed to write file: %w", err)
	}

	return written, nil
}

// ReadFileForDownload creates a reader for the given path.
// If the path is a directory, it returns a tar.gz stream.
// The returned reader should be read fully and closed is handled by the caller.
// Returns: reader, size (-1 for directories), mode, isDirectory, error
func (h *StreamHandler) ReadFileForDownload(path string, compress bool) (io.Reader, int64, uint32, bool, error) {
	path = filepath.Clean(path)

	info, err := os.Stat(path)
	if err != nil {
		return nil, 0, 0, false, fmt.Errorf("path not found: %w", err)
	}

	if info.IsDir() {
		// For directories, create a tar.gz stream via pipe
		pr, pw := io.Pipe()

		go func() {
			err := TarDirectory(path, pw)
			if err != nil {
				pw.CloseWithError(err)
			} else {
				pw.Close()
			}
		}()

		return pr, -1, uint32(info.Mode().Perm()), true, nil
	}

	// For files, open and optionally wrap with gzip
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, 0, false, fmt.Errorf("failed to open file: %w", err)
	}

	if compress {
		// Create a pipe for compressed data
		pr, pw := io.Pipe()

		go func() {
			gzw := gzip.NewWriter(pw)
			_, copyErr := io.Copy(gzw, f)
			f.Close()
			gzw.Close()
			if copyErr != nil {
				pw.CloseWithError(copyErr)
			} else {
				pw.Close()
			}
		}()

		// Size is unknown when compressed
		return pr, -1, uint32(info.Mode().Perm()), false, nil
	}

	// Return uncompressed file
	return f, info.Size(), uint32(info.Mode().Perm()), false, nil
}

// ParseMetadata parses transfer metadata from JSON bytes.
func ParseMetadata(data []byte) (*TransferMetadata, error) {
	var meta TransferMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("invalid metadata: %w", err)
	}
	return &meta, nil
}

// EncodeMetadata encodes transfer metadata to JSON bytes.
func EncodeMetadata(meta *TransferMetadata) ([]byte, error) {
	return json.Marshal(meta)
}

// EncodeResult encodes transfer result to JSON bytes.
func EncodeResult(result *TransferResult) ([]byte, error) {
	return json.Marshal(result)
}

// ParseResult parses transfer result from JSON bytes.
func ParseResult(data []byte) (*TransferResult, error) {
	var result TransferResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("invalid result: %w", err)
	}
	return &result, nil
}

// CountingWriter wraps a writer and counts bytes written.
type CountingWriter struct {
	W       io.Writer
	Written int64
}

func (cw *CountingWriter) Write(p []byte) (int, error) {
	n, err := cw.W.Write(p)
	cw.Written += int64(n)
	return n, err
}

// CountingReader wraps a reader and counts bytes read.
type CountingReader struct {
	R        io.Reader
	BytesRead int64
}

func (cr *CountingReader) Read(p []byte) (int, error) {
	n, err := cr.R.Read(p)
	cr.BytesRead += int64(n)
	return n, err
}

// ProgressWriter wraps a writer and reports progress.
type ProgressWriter struct {
	W          io.Writer
	Total      int64 // Total expected bytes (-1 if unknown)
	Written    int64
	OnProgress func(written, total int64)
}

func (pw *ProgressWriter) Write(p []byte) (int, error) {
	n, err := pw.W.Write(p)
	pw.Written += int64(n)
	if pw.OnProgress != nil {
		pw.OnProgress(pw.Written, pw.Total)
	}
	return n, err
}

// ProgressReader wraps a reader and reports progress.
type ProgressReader struct {
	R          io.Reader
	Total      int64 // Total expected bytes (-1 if unknown)
	BytesRead  int64
	OnProgress func(read, total int64)
}

func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.R.Read(p)
	pr.BytesRead += int64(n)
	if pr.OnProgress != nil {
		pr.OnProgress(pr.BytesRead, pr.Total)
	}
	return n, err
}
