package filetransfer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestReadFileForTransfer(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir, err := os.MkdirTemp("", "filetransfer-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	t.Run("small file without compression", func(t *testing.T) {
		// Create a small file (below compression threshold)
		smallFile := filepath.Join(tmpDir, "small.txt")
		content := "Hello World"
		if err := os.WriteFile(smallFile, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		data, mode, size, compressed, err := ReadFileForTransfer(smallFile)
		if err != nil {
			t.Fatalf("ReadFileForTransfer failed: %v", err)
		}

		if compressed {
			t.Error("small file should not be compressed")
		}
		if size != int64(len(content)) {
			t.Errorf("size mismatch: got %d, want %d", size, len(content))
		}
		if mode != 0644 {
			t.Errorf("mode mismatch: got %o, want %o", mode, 0644)
		}
		if data == "" {
			t.Error("data should not be empty")
		}
	})

	t.Run("large file with compression", func(t *testing.T) {
		// Create a larger file (above compression threshold)
		largeFile := filepath.Join(tmpDir, "large.txt")
		// Create repetitive content that compresses well
		content := ""
		for i := 0; i < 200; i++ {
			content += "This is a test line that should compress well.\n"
		}
		if err := os.WriteFile(largeFile, []byte(content), 0755); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		data, mode, size, compressed, err := ReadFileForTransfer(largeFile)
		if err != nil {
			t.Fatalf("ReadFileForTransfer failed: %v", err)
		}

		if !compressed {
			t.Error("large repetitive file should be compressed")
		}
		if size != int64(len(content)) {
			t.Errorf("size mismatch: got %d, want %d", size, len(content))
		}
		if mode != 0755 {
			t.Errorf("mode mismatch: got %o, want %o", mode, 0755)
		}
		if data == "" {
			t.Error("data should not be empty")
		}
	})

	t.Run("non-existent file", func(t *testing.T) {
		_, _, _, _, err := ReadFileForTransfer(filepath.Join(tmpDir, "nonexistent.txt"))
		if err == nil {
			t.Error("expected error for non-existent file")
		}
	})

	t.Run("directory instead of file", func(t *testing.T) {
		_, _, _, _, err := ReadFileForTransfer(tmpDir)
		if err == nil {
			t.Error("expected error for directory")
		}
	})
}

func TestWriteFileFromTransfer(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "filetransfer-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	t.Run("write uncompressed file", func(t *testing.T) {
		outPath := filepath.Join(tmpDir, "output1.txt")
		content := "Hello World"
		// Base64 of "Hello World"
		data := "SGVsbG8gV29ybGQ="

		written, err := WriteFileFromTransfer(outPath, data, 0644, false)
		if err != nil {
			t.Fatalf("WriteFileFromTransfer failed: %v", err)
		}

		if written != int64(len(content)) {
			t.Errorf("written bytes mismatch: got %d, want %d", written, len(content))
		}

		// Verify content
		readContent, err := os.ReadFile(outPath)
		if err != nil {
			t.Fatalf("failed to read output file: %v", err)
		}
		if string(readContent) != content {
			t.Errorf("content mismatch: got %q, want %q", string(readContent), content)
		}

		// Verify permissions
		info, err := os.Stat(outPath)
		if err != nil {
			t.Fatalf("failed to stat output file: %v", err)
		}
		if info.Mode().Perm() != 0644 {
			t.Errorf("mode mismatch: got %o, want %o", info.Mode().Perm(), 0644)
		}
	})

	t.Run("write to nested directory", func(t *testing.T) {
		outPath := filepath.Join(tmpDir, "nested", "dir", "output.txt")
		data := "SGVsbG8=" // Base64 of "Hello"

		written, err := WriteFileFromTransfer(outPath, data, 0600, false)
		if err != nil {
			t.Fatalf("WriteFileFromTransfer failed: %v", err)
		}

		if written != 5 {
			t.Errorf("written bytes mismatch: got %d, want 5", written)
		}

		// Verify file exists
		if _, err := os.Stat(outPath); err != nil {
			t.Errorf("output file should exist: %v", err)
		}
	})

	t.Run("invalid base64", func(t *testing.T) {
		outPath := filepath.Join(tmpDir, "invalid.txt")
		_, err := WriteFileFromTransfer(outPath, "not-valid-base64!!!", 0644, false)
		if err == nil {
			t.Error("expected error for invalid base64")
		}
	})
}

func TestRoundTrip(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "filetransfer-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	t.Run("round trip small file", func(t *testing.T) {
		srcPath := filepath.Join(tmpDir, "src-small.txt")
		dstPath := filepath.Join(tmpDir, "dst-small.txt")
		content := "Small file content"

		if err := os.WriteFile(srcPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write source file: %v", err)
		}

		// Read
		data, mode, _, compressed, err := ReadFileForTransfer(srcPath)
		if err != nil {
			t.Fatalf("ReadFileForTransfer failed: %v", err)
		}

		// Write
		_, err = WriteFileFromTransfer(dstPath, data, mode, compressed)
		if err != nil {
			t.Fatalf("WriteFileFromTransfer failed: %v", err)
		}

		// Verify
		dstContent, err := os.ReadFile(dstPath)
		if err != nil {
			t.Fatalf("failed to read destination file: %v", err)
		}
		if string(dstContent) != content {
			t.Errorf("content mismatch: got %q, want %q", string(dstContent), content)
		}
	})

	t.Run("round trip large file with compression", func(t *testing.T) {
		srcPath := filepath.Join(tmpDir, "src-large.txt")
		dstPath := filepath.Join(tmpDir, "dst-large.txt")

		// Create content that compresses well
		content := ""
		for i := 0; i < 100; i++ {
			content += "Repetitive content for compression test\n"
		}

		if err := os.WriteFile(srcPath, []byte(content), 0755); err != nil {
			t.Fatalf("failed to write source file: %v", err)
		}

		// Read
		data, mode, _, compressed, err := ReadFileForTransfer(srcPath)
		if err != nil {
			t.Fatalf("ReadFileForTransfer failed: %v", err)
		}

		if !compressed {
			t.Log("Note: file was not compressed (may be expected if compression doesn't reduce size)")
		}

		// Write
		_, err = WriteFileFromTransfer(dstPath, data, mode, compressed)
		if err != nil {
			t.Fatalf("WriteFileFromTransfer failed: %v", err)
		}

		// Verify content
		dstContent, err := os.ReadFile(dstPath)
		if err != nil {
			t.Fatalf("failed to read destination file: %v", err)
		}
		if string(dstContent) != content {
			t.Errorf("content mismatch after round trip")
		}

		// Verify permissions
		info, err := os.Stat(dstPath)
		if err != nil {
			t.Fatalf("failed to stat destination file: %v", err)
		}
		if info.Mode().Perm() != 0755 {
			t.Errorf("mode mismatch: got %o, want %o", info.Mode().Perm(), 0755)
		}
	})
}

func TestHandler(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "filetransfer-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	t.Run("upload disabled", func(t *testing.T) {
		h := NewHandler(Config{Enabled: false})

		req := FileUploadRequest{Path: "/tmp/test.txt", Data: "SGVsbG8="}
		reqData, _ := json.Marshal(req)

		resp, err := h.HandleUpload(reqData)
		if err != nil {
			t.Fatalf("HandleUpload returned error: %v", err)
		}

		var uploadResp FileUploadResponse
		if err := json.Unmarshal(resp, &uploadResp); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}

		if uploadResp.Success {
			t.Error("upload should fail when disabled")
		}
		if uploadResp.Error != "file transfer is disabled" {
			t.Errorf("unexpected error: %s", uploadResp.Error)
		}
	})

	t.Run("upload with authentication", func(t *testing.T) {
		password := "testpassword"
		hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)

		h := NewHandler(Config{
			Enabled:      true,
			MaxFileSize:  1024 * 1024,
			PasswordHash: string(hash),
		})

		// Without password
		req := FileUploadRequest{Path: "/tmp/test.txt", Data: "SGVsbG8="}
		reqData, _ := json.Marshal(req)
		resp, _ := h.HandleUpload(reqData)
		var uploadResp FileUploadResponse
		json.Unmarshal(resp, &uploadResp)

		if uploadResp.Success {
			t.Error("upload should fail without password")
		}
		if uploadResp.Error != "authentication required" {
			t.Errorf("unexpected error: %s", uploadResp.Error)
		}

		// With wrong password
		req.Password = "wrongpassword"
		reqData, _ = json.Marshal(req)
		resp, _ = h.HandleUpload(reqData)
		json.Unmarshal(resp, &uploadResp)

		if uploadResp.Success {
			t.Error("upload should fail with wrong password")
		}
		if uploadResp.Error != "authentication failed" {
			t.Errorf("unexpected error: %s", uploadResp.Error)
		}

		// With correct password
		req.Password = password
		req.Path = filepath.Join(tmpDir, "auth-test.txt")
		req.Mode = 0644
		req.Size = 5
		reqData, _ = json.Marshal(req)
		resp, _ = h.HandleUpload(reqData)
		json.Unmarshal(resp, &uploadResp)

		if !uploadResp.Success {
			t.Errorf("upload should succeed with correct password: %s", uploadResp.Error)
		}
	})

	t.Run("upload path validation", func(t *testing.T) {
		h := NewHandler(Config{
			Enabled:     true,
			MaxFileSize: 1024 * 1024,
		})

		// Relative path
		req := FileUploadRequest{Path: "relative/path.txt", Data: "SGVsbG8="}
		reqData, _ := json.Marshal(req)
		resp, _ := h.HandleUpload(reqData)
		var uploadResp FileUploadResponse
		json.Unmarshal(resp, &uploadResp)

		if uploadResp.Success {
			t.Error("upload should fail with relative path")
		}
	})

	t.Run("upload allowed paths", func(t *testing.T) {
		h := NewHandler(Config{
			Enabled:      true,
			MaxFileSize:  1024 * 1024,
			AllowedPaths: []string{"/allowed"},
		})

		// Path not in allowed list
		req := FileUploadRequest{Path: "/notallowed/test.txt", Data: "SGVsbG8="}
		reqData, _ := json.Marshal(req)
		resp, _ := h.HandleUpload(reqData)
		var uploadResp FileUploadResponse
		json.Unmarshal(resp, &uploadResp)

		if uploadResp.Success {
			t.Error("upload should fail for path not in allowed list")
		}
	})

	t.Run("upload size limit", func(t *testing.T) {
		h := NewHandler(Config{
			Enabled:     true,
			MaxFileSize: 10, // 10 bytes
		})

		req := FileUploadRequest{
			Path: filepath.Join(tmpDir, "size-test.txt"),
			Data: "SGVsbG8gV29ybGQh", // "Hello World!" - 12 bytes
			Mode: 0644,
			Size: 12,
		}
		reqData, _ := json.Marshal(req)
		resp, _ := h.HandleUpload(reqData)
		var uploadResp FileUploadResponse
		json.Unmarshal(resp, &uploadResp)

		if uploadResp.Success {
			t.Error("upload should fail when exceeding size limit")
		}
	})

	t.Run("download disabled", func(t *testing.T) {
		h := NewHandler(Config{Enabled: false})

		req := FileDownloadRequest{Path: "/tmp/test.txt"}
		reqData, _ := json.Marshal(req)

		resp, err := h.HandleDownload(reqData)
		if err != nil {
			t.Fatalf("HandleDownload returned error: %v", err)
		}

		var downloadResp FileDownloadResponse
		if err := json.Unmarshal(resp, &downloadResp); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}

		if downloadResp.Success {
			t.Error("download should fail when disabled")
		}
	})

	t.Run("download non-existent file", func(t *testing.T) {
		h := NewHandler(Config{
			Enabled:     true,
			MaxFileSize: 1024 * 1024,
		})

		req := FileDownloadRequest{Path: "/nonexistent/file.txt"}
		reqData, _ := json.Marshal(req)
		resp, _ := h.HandleDownload(reqData)
		var downloadResp FileDownloadResponse
		json.Unmarshal(resp, &downloadResp)

		if downloadResp.Success {
			t.Error("download should fail for non-existent file")
		}
	})

	t.Run("download success", func(t *testing.T) {
		// Create a test file
		testFile := filepath.Join(tmpDir, "download-test.txt")
		content := "Download test content"
		if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		h := NewHandler(Config{
			Enabled:     true,
			MaxFileSize: 1024 * 1024,
		})

		req := FileDownloadRequest{Path: testFile}
		reqData, _ := json.Marshal(req)
		resp, _ := h.HandleDownload(reqData)
		var downloadResp FileDownloadResponse
		json.Unmarshal(resp, &downloadResp)

		if !downloadResp.Success {
			t.Errorf("download should succeed: %s", downloadResp.Error)
		}
		if downloadResp.Size != int64(len(content)) {
			t.Errorf("size mismatch: got %d, want %d", downloadResp.Size, len(content))
		}
		if downloadResp.Mode != 0644 {
			t.Errorf("mode mismatch: got %o, want %o", downloadResp.Mode, 0644)
		}
		if downloadResp.Data == "" {
			t.Error("data should not be empty")
		}
	})
}

func TestHandlerInvalidJSON(t *testing.T) {
	h := NewHandler(Config{Enabled: true, MaxFileSize: 1024 * 1024})

	t.Run("invalid upload JSON", func(t *testing.T) {
		resp, err := h.HandleUpload([]byte("not json"))
		if err != nil {
			t.Fatalf("HandleUpload should not return error: %v", err)
		}

		var uploadResp FileUploadResponse
		if err := json.Unmarshal(resp, &uploadResp); err != nil {
			t.Fatalf("response should be valid JSON: %v", err)
		}

		if uploadResp.Success {
			t.Error("should fail for invalid JSON")
		}
	})

	t.Run("invalid download JSON", func(t *testing.T) {
		resp, err := h.HandleDownload([]byte("not json"))
		if err != nil {
			t.Fatalf("HandleDownload should not return error: %v", err)
		}

		var downloadResp FileDownloadResponse
		if err := json.Unmarshal(resp, &downloadResp); err != nil {
			t.Fatalf("response should be valid JSON: %v", err)
		}

		if downloadResp.Success {
			t.Error("should fail for invalid JSON")
		}
	})
}
