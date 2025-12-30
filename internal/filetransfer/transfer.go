// Package filetransfer provides file upload and download capabilities for agents.
package filetransfer

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// CompressionMinSize is the minimum file size to apply compression (1KB).
const CompressionMinSize = 1024

// FileUploadRequest represents a request to upload a file.
type FileUploadRequest struct {
	Password   string `json:"password,omitempty"` // Authentication password
	Path       string `json:"path"`               // Absolute remote path
	Mode       uint32 `json:"mode"`               // File permissions (e.g., 0644)
	Size       int64  `json:"size"`               // File size in bytes (uncompressed)
	Compressed bool   `json:"compressed"`         // Whether data is gzip compressed
	Data       string `json:"data"`               // Base64-encoded file content
}

// FileUploadResponse represents the response to a file upload.
type FileUploadResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
	Written int64  `json:"written"` // Bytes written
}

// FileDownloadRequest represents a request to download a file.
type FileDownloadRequest struct {
	Password string `json:"password,omitempty"` // Authentication password
	Path     string `json:"path"`               // Absolute remote path
}

// FileDownloadResponse represents the response to a file download.
type FileDownloadResponse struct {
	Success    bool   `json:"success"`
	Error      string `json:"error,omitempty"`
	Size       int64  `json:"size"`       // File size (uncompressed)
	Mode       uint32 `json:"mode"`       // File permissions
	Compressed bool   `json:"compressed"` // Whether data is gzip compressed
	Data       string `json:"data"`       // Base64-encoded file content
}

// Config holds file transfer configuration.
type Config struct {
	Enabled      bool
	MaxFileSize  int64
	AllowedPaths []string
	PasswordHash string
}

// Handler handles file transfer operations.
type Handler struct {
	cfg Config
}

// NewHandler creates a new file transfer handler.
func NewHandler(cfg Config) *Handler {
	return &Handler{cfg: cfg}
}

// HandleUpload processes a file upload request.
func (h *Handler) HandleUpload(data []byte) ([]byte, error) {
	var req FileUploadRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return h.errorResponse("invalid request: " + err.Error())
	}

	// Check if enabled
	if !h.cfg.Enabled {
		return h.errorResponse("file transfer is disabled")
	}

	// Authenticate
	if err := h.authenticate(req.Password); err != nil {
		return h.errorResponse(err.Error())
	}

	// Validate path
	if err := h.validatePath(req.Path); err != nil {
		return h.errorResponse(err.Error())
	}

	// Check size limit
	if req.Size > h.cfg.MaxFileSize {
		return h.errorResponse(fmt.Sprintf("file too large: %d bytes (max %d)", req.Size, h.cfg.MaxFileSize))
	}

	// Write the file
	written, err := WriteFileFromTransfer(req.Path, req.Data, req.Mode, req.Compressed)
	if err != nil {
		return h.errorResponse("write failed: " + err.Error())
	}

	resp := FileUploadResponse{
		Success: true,
		Written: written,
	}
	return json.Marshal(resp)
}

// HandleDownload processes a file download request.
func (h *Handler) HandleDownload(data []byte) ([]byte, error) {
	var req FileDownloadRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return h.errorResponse("invalid request: " + err.Error())
	}

	// Check if enabled
	if !h.cfg.Enabled {
		return h.downloadErrorResponse("file transfer is disabled")
	}

	// Authenticate
	if err := h.authenticate(req.Password); err != nil {
		return h.downloadErrorResponse(err.Error())
	}

	// Validate path
	if err := h.validatePath(req.Path); err != nil {
		return h.downloadErrorResponse(err.Error())
	}

	// Check file size before reading
	info, err := os.Stat(req.Path)
	if err != nil {
		return h.downloadErrorResponse("file not found: " + err.Error())
	}
	if info.IsDir() {
		return h.downloadErrorResponse("path is a directory")
	}
	if info.Size() > h.cfg.MaxFileSize {
		return h.downloadErrorResponse(fmt.Sprintf("file too large: %d bytes (max %d)", info.Size(), h.cfg.MaxFileSize))
	}

	// Read the file
	encodedData, mode, size, compressed, err := ReadFileForTransfer(req.Path)
	if err != nil {
		return h.downloadErrorResponse("read failed: " + err.Error())
	}

	resp := FileDownloadResponse{
		Success:    true,
		Size:       size,
		Mode:       mode,
		Compressed: compressed,
		Data:       encodedData,
	}
	return json.Marshal(resp)
}

// authenticate checks if the password is correct.
func (h *Handler) authenticate(password string) error {
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

// validatePath checks if the path is allowed.
func (h *Handler) validatePath(path string) error {
	// Must be absolute
	if !filepath.IsAbs(path) {
		return fmt.Errorf("path must be absolute: %s", path)
	}

	// Check for directory traversal
	cleanPath := filepath.Clean(path)
	if strings.Contains(cleanPath, "..") {
		return fmt.Errorf("directory traversal not allowed")
	}

	// Check allowed paths
	if len(h.cfg.AllowedPaths) > 0 {
		allowed := false
		for _, prefix := range h.cfg.AllowedPaths {
			if strings.HasPrefix(cleanPath, prefix) {
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

// errorResponse creates an upload error response.
func (h *Handler) errorResponse(msg string) ([]byte, error) {
	resp := FileUploadResponse{
		Success: false,
		Error:   msg,
	}
	return json.Marshal(resp)
}

// downloadErrorResponse creates a download error response.
func (h *Handler) downloadErrorResponse(msg string) ([]byte, error) {
	resp := FileDownloadResponse{
		Success: false,
		Error:   msg,
	}
	return json.Marshal(resp)
}

// ReadFileForTransfer reads a file, compresses it if beneficial, and returns base64-encoded data.
func ReadFileForTransfer(path string) (data string, mode uint32, size int64, compressed bool, err error) {
	// Get file info
	info, err := os.Stat(path)
	if err != nil {
		return "", 0, 0, false, err
	}
	if info.IsDir() {
		return "", 0, 0, false, fmt.Errorf("path is a directory")
	}

	size = info.Size()
	mode = uint32(info.Mode().Perm())

	// Read file content
	content, err := os.ReadFile(path)
	if err != nil {
		return "", 0, 0, false, err
	}

	// Compress if file is large enough
	var encoded string
	if size >= CompressionMinSize {
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		if _, err := gz.Write(content); err != nil {
			return "", 0, 0, false, fmt.Errorf("compression failed: %w", err)
		}
		if err := gz.Close(); err != nil {
			return "", 0, 0, false, fmt.Errorf("compression close failed: %w", err)
		}

		// Only use compressed version if it's smaller
		if buf.Len() < len(content) {
			encoded = base64.StdEncoding.EncodeToString(buf.Bytes())
			compressed = true
		} else {
			encoded = base64.StdEncoding.EncodeToString(content)
			compressed = false
		}
	} else {
		encoded = base64.StdEncoding.EncodeToString(content)
		compressed = false
	}

	return encoded, mode, size, compressed, nil
}

// WriteFileFromTransfer decodes, decompresses, and writes a file with the given permissions.
func WriteFileFromTransfer(path string, data string, mode uint32, compressed bool) (written int64, err error) {
	// Decode base64
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return 0, fmt.Errorf("base64 decode failed: %w", err)
	}

	// Decompress if needed
	var content []byte
	if compressed {
		gz, err := gzip.NewReader(bytes.NewReader(decoded))
		if err != nil {
			return 0, fmt.Errorf("gzip reader failed: %w", err)
		}
		content, err = io.ReadAll(gz)
		if err != nil {
			return 0, fmt.Errorf("decompression failed: %w", err)
		}
		gz.Close()
	} else {
		content = decoded
	}

	// Create parent directories if needed
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return 0, fmt.Errorf("failed to create directory: %w", err)
	}

	// Write file
	if err := os.WriteFile(path, content, os.FileMode(mode)); err != nil {
		return 0, fmt.Errorf("write failed: %w", err)
	}

	return int64(len(content)), nil
}
