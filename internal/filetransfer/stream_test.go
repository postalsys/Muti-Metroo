package filetransfer

import (
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestStreamHandler_ValidateUploadMetadata(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.DefaultCost)

	tests := []struct {
		name    string
		cfg     StreamConfig
		meta    *TransferMetadata
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid upload",
			cfg: StreamConfig{
				Enabled: true,
			},
			meta: &TransferMetadata{
				Path: "/tmp/test.txt",
				Size: 100,
			},
			wantErr: false,
		},
		{
			name: "disabled",
			cfg: StreamConfig{
				Enabled: false,
			},
			meta: &TransferMetadata{
				Path: "/tmp/test.txt",
			},
			wantErr: true,
			errMsg:  "file transfer is disabled",
		},
		{
			name: "auth required",
			cfg: StreamConfig{
				Enabled:      true,
				PasswordHash: string(hash),
			},
			meta: &TransferMetadata{
				Path: "/tmp/test.txt",
			},
			wantErr: true,
			errMsg:  "authentication required",
		},
		{
			name: "auth failed",
			cfg: StreamConfig{
				Enabled:      true,
				PasswordHash: string(hash),
			},
			meta: &TransferMetadata{
				Path:     "/tmp/test.txt",
				Password: "wrong",
			},
			wantErr: true,
			errMsg:  "authentication failed",
		},
		{
			name: "auth success",
			cfg: StreamConfig{
				Enabled:      true,
				PasswordHash: string(hash),
			},
			meta: &TransferMetadata{
				Path:     "/tmp/test.txt",
				Password: "secret",
			},
			wantErr: false,
		},
		{
			name: "relative path",
			cfg: StreamConfig{
				Enabled: true,
			},
			meta: &TransferMetadata{
				Path: "relative/path.txt",
			},
			wantErr: true,
			errMsg:  "path must be absolute",
		},
		{
			name: "path not allowed",
			cfg: StreamConfig{
				Enabled:      true,
				AllowedPaths: []string{"/allowed"},
			},
			meta: &TransferMetadata{
				Path: "/not-allowed/test.txt",
			},
			wantErr: true,
			errMsg:  "path not in allowed list",
		},
		{
			name: "path allowed",
			cfg: StreamConfig{
				Enabled:      true,
				AllowedPaths: []string{"/allowed"},
			},
			meta: &TransferMetadata{
				Path: "/allowed/test.txt",
			},
			wantErr: false,
		},
		{
			name: "file too large",
			cfg: StreamConfig{
				Enabled:     true,
				MaxFileSize: 100,
			},
			meta: &TransferMetadata{
				Path: "/tmp/test.txt",
				Size: 200,
			},
			wantErr: true,
			errMsg:  "file too large",
		},
		{
			name: "directory size not checked",
			cfg: StreamConfig{
				Enabled:     true,
				MaxFileSize: 100,
			},
			meta: &TransferMetadata{
				Path:        "/tmp/testdir",
				Size:        -1,
				IsDirectory: true,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewStreamHandler(tt.cfg)
			err := h.ValidateUploadMetadata(tt.meta)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateUploadMetadata() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.errMsg != "" && err != nil {
				if !bytes.Contains([]byte(err.Error()), []byte(tt.errMsg)) {
					t.Errorf("error message %q does not contain %q", err.Error(), tt.errMsg)
				}
			}
		})
	}
}

func TestStreamHandler_WriteUploadedFile(t *testing.T) {
	h := NewStreamHandler(StreamConfig{Enabled: true})

	t.Run("write uncompressed file", func(t *testing.T) {
		destDir := t.TempDir()
		destPath := filepath.Join(destDir, "test.txt")

		content := []byte("hello world")
		r := bytes.NewReader(content)

		written, err := h.WriteUploadedFile(destPath, r, 0644, false, false)
		if err != nil {
			t.Fatalf("WriteUploadedFile failed: %v", err)
		}
		if written != int64(len(content)) {
			t.Errorf("written = %d, want %d", written, len(content))
		}

		// Verify file content
		data, err := os.ReadFile(destPath)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != string(content) {
			t.Errorf("content = %q, want %q", string(data), string(content))
		}
	})

	t.Run("write compressed file", func(t *testing.T) {
		destDir := t.TempDir()
		destPath := filepath.Join(destDir, "test.txt")

		content := []byte("hello world compressed")

		// Compress the content
		var buf bytes.Buffer
		gzw := gzip.NewWriter(&buf)
		gzw.Write(content)
		gzw.Close()

		written, err := h.WriteUploadedFile(destPath, &buf, 0644, false, true)
		if err != nil {
			t.Fatalf("WriteUploadedFile failed: %v", err)
		}
		if written != int64(len(content)) {
			t.Errorf("written = %d, want %d", written, len(content))
		}

		// Verify file content (should be decompressed)
		data, err := os.ReadFile(destPath)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != string(content) {
			t.Errorf("content = %q, want %q", string(data), string(content))
		}
	})

	t.Run("write directory", func(t *testing.T) {
		// First create a source directory to tar
		srcDir := t.TempDir()
		os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("file1"), 0644)
		os.Mkdir(filepath.Join(srcDir, "subdir"), 0755)
		os.WriteFile(filepath.Join(srcDir, "subdir", "file2.txt"), []byte("file2"), 0644)

		// Tar the directory
		var buf bytes.Buffer
		if err := TarDirectory(srcDir, &buf); err != nil {
			t.Fatalf("TarDirectory failed: %v", err)
		}

		// Write as directory
		destDir := t.TempDir()
		destPath := filepath.Join(destDir, "extracted")

		written, err := h.WriteUploadedFile(destPath, &buf, 0755, true, false)
		if err != nil {
			t.Fatalf("WriteUploadedFile failed: %v", err)
		}
		if written == 0 {
			t.Error("written = 0, expected > 0")
		}

		// Verify extracted files
		data, err := os.ReadFile(filepath.Join(destPath, "file1.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "file1" {
			t.Errorf("file1 content = %q, want %q", string(data), "file1")
		}

		data, err = os.ReadFile(filepath.Join(destPath, "subdir", "file2.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "file2" {
			t.Errorf("file2 content = %q, want %q", string(data), "file2")
		}
	})
}

func TestStreamHandler_ReadFileForDownload(t *testing.T) {
	h := NewStreamHandler(StreamConfig{Enabled: true})

	t.Run("read file uncompressed", func(t *testing.T) {
		srcDir := t.TempDir()
		srcPath := filepath.Join(srcDir, "test.txt")
		content := []byte("hello world")
		os.WriteFile(srcPath, content, 0644)

		r, size, mode, isDir, err := h.ReadFileForDownload(srcPath, false)
		if err != nil {
			t.Fatalf("ReadFileForDownload failed: %v", err)
		}

		if size != int64(len(content)) {
			t.Errorf("size = %d, want %d", size, len(content))
		}
		if mode != 0644 {
			t.Errorf("mode = %o, want %o", mode, 0644)
		}
		if isDir {
			t.Error("isDir = true, want false")
		}

		// Read content
		data, err := readAll(r)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != string(content) {
			t.Errorf("content = %q, want %q", string(data), string(content))
		}
	})

	t.Run("read file compressed", func(t *testing.T) {
		srcDir := t.TempDir()
		srcPath := filepath.Join(srcDir, "test.txt")
		content := []byte("hello world compressed read")
		os.WriteFile(srcPath, content, 0644)

		r, size, _, _, err := h.ReadFileForDownload(srcPath, true)
		if err != nil {
			t.Fatalf("ReadFileForDownload failed: %v", err)
		}

		// Size is unknown when compressed
		if size != -1 {
			t.Errorf("size = %d, want -1 (unknown)", size)
		}

		// Read and decompress
		compressed, err := readAll(r)
		if err != nil {
			t.Fatal(err)
		}

		gzr, err := gzip.NewReader(bytes.NewReader(compressed))
		if err != nil {
			t.Fatal(err)
		}
		data, err := readAll(gzr)
		gzr.Close()
		if err != nil {
			t.Fatal(err)
		}

		if string(data) != string(content) {
			t.Errorf("content = %q, want %q", string(data), string(content))
		}
	})

	t.Run("read directory", func(t *testing.T) {
		srcDir := t.TempDir()
		os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("file1"), 0644)
		os.Mkdir(filepath.Join(srcDir, "subdir"), 0755)
		os.WriteFile(filepath.Join(srcDir, "subdir", "file2.txt"), []byte("file2"), 0644)

		r, size, _, isDir, err := h.ReadFileForDownload(srcDir, false)
		if err != nil {
			t.Fatalf("ReadFileForDownload failed: %v", err)
		}

		if size != -1 {
			t.Errorf("size = %d, want -1 for directories", size)
		}
		if !isDir {
			t.Error("isDir = false, want true")
		}

		// Read tar.gz and extract to verify
		tarData, err := readAll(r)
		if err != nil {
			t.Fatal(err)
		}

		destDir := t.TempDir()
		if err := UntarDirectory(bytes.NewReader(tarData), destDir); err != nil {
			t.Fatalf("UntarDirectory failed: %v", err)
		}

		// Verify extracted files
		data, err := os.ReadFile(filepath.Join(destDir, "file1.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "file1" {
			t.Errorf("file1 content = %q, want %q", string(data), "file1")
		}
	})

	t.Run("read nonexistent", func(t *testing.T) {
		_, _, _, _, err := h.ReadFileForDownload("/nonexistent/path", false)
		if err == nil {
			t.Error("expected error for nonexistent path")
		}
	})
}

func TestMetadataEncoding(t *testing.T) {
	meta := &TransferMetadata{
		Path:        "/tmp/test.txt",
		Mode:        0644,
		Size:        1024,
		IsDirectory: false,
		Password:    "secret",
		Compress:    true,
		Checksum:    "abc123",
	}

	// Encode
	data, err := EncodeMetadata(meta)
	if err != nil {
		t.Fatalf("EncodeMetadata failed: %v", err)
	}

	// Decode
	decoded, err := ParseMetadata(data)
	if err != nil {
		t.Fatalf("ParseMetadata failed: %v", err)
	}

	// Verify
	if decoded.Path != meta.Path {
		t.Errorf("Path = %q, want %q", decoded.Path, meta.Path)
	}
	if decoded.Mode != meta.Mode {
		t.Errorf("Mode = %o, want %o", decoded.Mode, meta.Mode)
	}
	if decoded.Size != meta.Size {
		t.Errorf("Size = %d, want %d", decoded.Size, meta.Size)
	}
	if decoded.IsDirectory != meta.IsDirectory {
		t.Errorf("IsDirectory = %v, want %v", decoded.IsDirectory, meta.IsDirectory)
	}
	if decoded.Password != meta.Password {
		t.Errorf("Password = %q, want %q", decoded.Password, meta.Password)
	}
	if decoded.Compress != meta.Compress {
		t.Errorf("Compress = %v, want %v", decoded.Compress, meta.Compress)
	}
	if decoded.Checksum != meta.Checksum {
		t.Errorf("Checksum = %q, want %q", decoded.Checksum, meta.Checksum)
	}
}

func TestResultEncoding(t *testing.T) {
	result := &TransferResult{
		Success: true,
		Written: 1024,
	}

	data, err := EncodeResult(result)
	if err != nil {
		t.Fatalf("EncodeResult failed: %v", err)
	}

	decoded, err := ParseResult(data)
	if err != nil {
		t.Fatalf("ParseResult failed: %v", err)
	}

	if decoded.Success != result.Success {
		t.Errorf("Success = %v, want %v", decoded.Success, result.Success)
	}
	if decoded.Written != result.Written {
		t.Errorf("Written = %d, want %d", decoded.Written, result.Written)
	}

	// Test error case
	errResult := &TransferResult{
		Success: false,
		Error:   "something went wrong",
	}

	data, _ = EncodeResult(errResult)
	decoded, _ = ParseResult(data)
	if decoded.Error != errResult.Error {
		t.Errorf("Error = %q, want %q", decoded.Error, errResult.Error)
	}
}

func TestCountingWriter(t *testing.T) {
	var buf bytes.Buffer
	cw := &CountingWriter{W: &buf}

	cw.Write([]byte("hello"))
	cw.Write([]byte(" world"))

	if cw.Written != 11 {
		t.Errorf("Written = %d, want 11", cw.Written)
	}
	if buf.String() != "hello world" {
		t.Errorf("buffer = %q, want %q", buf.String(), "hello world")
	}
}

func TestCountingReader(t *testing.T) {
	buf := bytes.NewReader([]byte("hello world"))
	cr := &CountingReader{R: buf}

	data := make([]byte, 5)
	cr.Read(data)
	if cr.BytesRead != 5 {
		t.Errorf("BytesRead = %d, want 5", cr.BytesRead)
	}

	cr.Read(data)
	if cr.BytesRead != 10 {
		t.Errorf("BytesRead = %d, want 10", cr.BytesRead)
	}
}

func TestProgressWriter(t *testing.T) {
	var buf bytes.Buffer
	var progressCalls []int64

	pw := &ProgressWriter{
		W:     &buf,
		Total: 100,
		OnProgress: func(written, total int64) {
			progressCalls = append(progressCalls, written)
		},
	}

	pw.Write([]byte("12345"))
	pw.Write([]byte("67890"))

	if len(progressCalls) != 2 {
		t.Errorf("progress calls = %d, want 2", len(progressCalls))
	}
	if progressCalls[0] != 5 {
		t.Errorf("first progress = %d, want 5", progressCalls[0])
	}
	if progressCalls[1] != 10 {
		t.Errorf("second progress = %d, want 10", progressCalls[1])
	}
}

// Helper function to read all data from a reader
func readAll(r interface{}) ([]byte, error) {
	switch v := r.(type) {
	case *bytes.Reader:
		data := make([]byte, v.Len())
		_, err := v.Read(data)
		return data, err
	case *os.File:
		defer v.Close()
		return os.ReadFile(v.Name())
	default:
		var buf bytes.Buffer
		_, err := buf.ReadFrom(r.(interface{ Read([]byte) (int, error) }))
		return buf.Bytes(), err
	}
}
