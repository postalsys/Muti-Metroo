package filetransfer

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPartialPaths(t *testing.T) {
	tests := []struct {
		input        string
		partialPath  string
		infoPath     string
	}{
		{"/tmp/file.iso", "/tmp/file.iso.partial", "/tmp/file.iso.partial.json"},
		{"/home/user/data.bin", "/home/user/data.bin.partial", "/home/user/data.bin.partial.json"},
		{"relative/path.txt", "relative/path.txt.partial", "relative/path.txt.partial.json"},
	}

	for _, tt := range tests {
		if got := GetPartialPath(tt.input); got != tt.partialPath {
			t.Errorf("GetPartialPath(%q) = %q, want %q", tt.input, got, tt.partialPath)
		}
		if got := GetPartialInfoPath(tt.input); got != tt.infoPath {
			t.Errorf("GetPartialInfoPath(%q) = %q, want %q", tt.input, got, tt.infoPath)
		}
	}
}

func TestPartialInfo_WriteRead(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "testfile.bin")

	info := &PartialInfo{
		OriginalSize: 1024 * 1024,
		BytesWritten: 512 * 1024,
		StartedAt:    time.Now().Add(-5 * time.Minute),
		SourcePath:   "/remote/path/file.bin",
	}

	// Write info
	if err := WritePartialInfo(filePath, info); err != nil {
		t.Fatalf("WritePartialInfo failed: %v", err)
	}

	// Verify file was created
	infoPath := GetPartialInfoPath(filePath)
	if _, err := os.Stat(infoPath); err != nil {
		t.Fatalf("info file not created: %v", err)
	}

	// Read info back
	readInfo, err := ReadPartialInfo(filePath)
	if err != nil {
		t.Fatalf("ReadPartialInfo failed: %v", err)
	}

	if readInfo.OriginalSize != info.OriginalSize {
		t.Errorf("OriginalSize mismatch: got %d, want %d", readInfo.OriginalSize, info.OriginalSize)
	}
	if readInfo.BytesWritten != info.BytesWritten {
		t.Errorf("BytesWritten mismatch: got %d, want %d", readInfo.BytesWritten, info.BytesWritten)
	}
	if readInfo.SourcePath != info.SourcePath {
		t.Errorf("SourcePath mismatch: got %q, want %q", readInfo.SourcePath, info.SourcePath)
	}
}

func TestPartialInfo_ReadNonexistent(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "nonexistent.bin")

	info, err := ReadPartialInfo(filePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info != nil {
		t.Error("expected nil for nonexistent file")
	}
}

func TestCleanupPartial(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "testfile.bin")

	// Create partial file and info
	partialPath := GetPartialPath(filePath)
	if err := os.WriteFile(partialPath, []byte("partial data"), 0644); err != nil {
		t.Fatalf("failed to create partial file: %v", err)
	}

	info := &PartialInfo{OriginalSize: 100, BytesWritten: 50}
	if err := WritePartialInfo(filePath, info); err != nil {
		t.Fatalf("failed to write partial info: %v", err)
	}

	// Cleanup
	if err := CleanupPartial(filePath); err != nil {
		t.Fatalf("CleanupPartial failed: %v", err)
	}

	// Verify files are removed
	if _, err := os.Stat(partialPath); !os.IsNotExist(err) {
		t.Error("partial file should be removed")
	}
	if _, err := os.Stat(GetPartialInfoPath(filePath)); !os.IsNotExist(err) {
		t.Error("partial info file should be removed")
	}
}

func TestHasPartialFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "testfile.bin")

	// No partial file
	info, err := HasPartialFile(filePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info != nil {
		t.Error("expected nil when no partial exists")
	}

	// Create partial file with correct size
	partialPath := GetPartialPath(filePath)
	if err := os.WriteFile(partialPath, []byte("12345"), 0644); err != nil {
		t.Fatalf("failed to create partial file: %v", err)
	}

	partialInfo := &PartialInfo{
		OriginalSize: 100,
		BytesWritten: 5, // matches file size
	}
	if err := WritePartialInfo(filePath, partialInfo); err != nil {
		t.Fatalf("failed to write partial info: %v", err)
	}

	// Should find valid partial
	info, err = HasPartialFile(filePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info == nil {
		t.Error("expected to find partial file")
	}
	if info.BytesWritten != 5 {
		t.Errorf("expected BytesWritten=5, got %d", info.BytesWritten)
	}
}

func TestHasPartialFile_SizeMismatch(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "testfile.bin")

	// Create partial file with wrong size
	partialPath := GetPartialPath(filePath)
	if err := os.WriteFile(partialPath, []byte("12345"), 0644); err != nil {
		t.Fatalf("failed to create partial file: %v", err)
	}

	partialInfo := &PartialInfo{
		OriginalSize: 100,
		BytesWritten: 10, // doesn't match actual file size (5)
	}
	if err := WritePartialInfo(filePath, partialInfo); err != nil {
		t.Fatalf("failed to write partial info: %v", err)
	}

	// Should clean up and return nil (corrupted partial)
	info, err := HasPartialFile(filePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info != nil {
		t.Error("expected nil due to size mismatch")
	}

	// Verify cleanup happened
	if _, err := os.Stat(partialPath); !os.IsNotExist(err) {
		t.Error("corrupted partial file should be cleaned up")
	}
}

func TestCreatePartialFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "testfile.bin")

	f, err := CreatePartialFile(filePath, 1024, "/remote/file.bin", 0644)
	if err != nil {
		t.Fatalf("CreatePartialFile failed: %v", err)
	}
	defer f.Close()

	// Write some data
	if _, err := f.Write([]byte("test data")); err != nil {
		t.Fatalf("failed to write: %v", err)
	}
	f.Close()

	// Verify partial file exists
	partialPath := GetPartialPath(filePath)
	if _, err := os.Stat(partialPath); err != nil {
		t.Errorf("partial file should exist: %v", err)
	}

	// Verify info file exists with correct values
	info, err := ReadPartialInfo(filePath)
	if err != nil {
		t.Fatalf("failed to read info: %v", err)
	}
	if info.OriginalSize != 1024 {
		t.Errorf("expected OriginalSize=1024, got %d", info.OriginalSize)
	}
	if info.SourcePath != "/remote/file.bin" {
		t.Errorf("expected SourcePath=/remote/file.bin, got %q", info.SourcePath)
	}
}

func TestFinalizePartial(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "testfile.bin")

	// Create partial file
	partialPath := GetPartialPath(filePath)
	if err := os.WriteFile(partialPath, []byte("complete data"), 0644); err != nil {
		t.Fatalf("failed to create partial file: %v", err)
	}

	// Create info file
	info := &PartialInfo{OriginalSize: 13, BytesWritten: 13}
	if err := WritePartialInfo(filePath, info); err != nil {
		t.Fatalf("failed to write info: %v", err)
	}

	// Finalize
	if err := FinalizePartial(filePath, 0755); err != nil {
		t.Fatalf("FinalizePartial failed: %v", err)
	}

	// Verify final file exists
	if _, err := os.Stat(filePath); err != nil {
		t.Errorf("final file should exist: %v", err)
	}

	// Verify partial files are cleaned up
	if _, err := os.Stat(partialPath); !os.IsNotExist(err) {
		t.Error("partial file should be removed after finalize")
	}
	if _, err := os.Stat(GetPartialInfoPath(filePath)); !os.IsNotExist(err) {
		t.Error("info file should be removed after finalize")
	}

	// Verify file mode
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("failed to stat final file: %v", err)
	}
	if fileInfo.Mode().Perm() != 0755 {
		t.Errorf("expected mode 0755, got %o", fileInfo.Mode().Perm())
	}
}
