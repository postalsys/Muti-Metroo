package filetransfer

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestBrowse_Disabled(t *testing.T) {
	h := NewStreamHandler(StreamConfig{Enabled: false})
	resp := h.Browse(&BrowseRequest{Action: "list", Path: "/tmp"})
	if resp.Error == "" {
		t.Fatal("expected error for disabled handler")
	}
	if resp.Error != "file transfer is disabled" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
}

func TestBrowse_AuthRequired(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.MinCost)
	h := NewStreamHandler(StreamConfig{
		Enabled:      true,
		AllowedPaths: []string{"*"},
		PasswordHash: string(hash),
	})

	resp := h.Browse(&BrowseRequest{Action: "list", Path: "/tmp"})
	if resp.Error != "authentication required" {
		t.Fatalf("expected auth required, got: %s", resp.Error)
	}

	resp = h.Browse(&BrowseRequest{Action: "list", Path: "/tmp", Password: "wrong"})
	if resp.Error != "authentication failed" {
		t.Fatalf("expected auth failed, got: %s", resp.Error)
	}
}

func TestBrowse_AuthSuccess(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.MinCost)
	dir := t.TempDir()

	h := NewStreamHandler(StreamConfig{
		Enabled:      true,
		AllowedPaths: []string{dir},
		PasswordHash: string(hash),
	})

	resp := h.Browse(&BrowseRequest{Action: "list", Path: dir, Password: "secret"})
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
}

func TestBrowse_UnknownAction(t *testing.T) {
	h := NewStreamHandler(StreamConfig{Enabled: true, AllowedPaths: []string{"*"}})
	resp := h.Browse(&BrowseRequest{Action: "delete"})
	if resp.Error != "unknown action: delete" {
		t.Fatalf("expected unknown action error, got: %s", resp.Error)
	}
}

func TestBrowse_DefaultAction(t *testing.T) {
	dir := t.TempDir()
	h := NewStreamHandler(StreamConfig{Enabled: true, AllowedPaths: []string{dir}})

	// Empty action defaults to "list"
	resp := h.Browse(&BrowseRequest{Path: dir})
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if resp.Path != dir {
		t.Fatalf("expected path %s, got %s", dir, resp.Path)
	}
}

func TestBrowseList_Basic(t *testing.T) {
	dir := t.TempDir()

	// Create test files and directories
	os.WriteFile(filepath.Join(dir, "file_b.txt"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(dir, "file_a.txt"), []byte("world"), 0644)
	os.Mkdir(filepath.Join(dir, "dir_b"), 0755)
	os.Mkdir(filepath.Join(dir, "dir_a"), 0755)

	h := NewStreamHandler(StreamConfig{Enabled: true, AllowedPaths: []string{dir}})
	resp := h.Browse(&BrowseRequest{Action: "list", Path: dir})

	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if resp.Total != 4 {
		t.Fatalf("expected 4 entries, got %d", resp.Total)
	}
	if len(resp.Entries) != 4 {
		t.Fatalf("expected 4 entries returned, got %d", len(resp.Entries))
	}

	// Directories should come first, sorted alphabetically
	if resp.Entries[0].Name != "dir_a" || !resp.Entries[0].IsDir {
		t.Fatalf("expected dir_a first, got %s (isDir=%v)", resp.Entries[0].Name, resp.Entries[0].IsDir)
	}
	if resp.Entries[1].Name != "dir_b" || !resp.Entries[1].IsDir {
		t.Fatalf("expected dir_b second, got %s", resp.Entries[1].Name)
	}
	// Files after directories, sorted alphabetically
	if resp.Entries[2].Name != "file_a.txt" || resp.Entries[2].IsDir {
		t.Fatalf("expected file_a.txt third, got %s", resp.Entries[2].Name)
	}
	if resp.Entries[3].Name != "file_b.txt" || resp.Entries[3].IsDir {
		t.Fatalf("expected file_b.txt fourth, got %s", resp.Entries[3].Name)
	}

	// Check sizes
	if resp.Entries[2].Size != 5 {
		t.Fatalf("expected file_a.txt size 5, got %d", resp.Entries[2].Size)
	}

	if resp.Truncated {
		t.Fatal("should not be truncated")
	}
}

func TestBrowseList_Pagination(t *testing.T) {
	dir := t.TempDir()

	// Create 5 files
	for i := 0; i < 5; i++ {
		name := filepath.Join(dir, string(rune('a'+i))+".txt")
		os.WriteFile(name, []byte("data"), 0644)
	}

	h := NewStreamHandler(StreamConfig{Enabled: true, AllowedPaths: []string{dir}})

	// First page
	resp := h.Browse(&BrowseRequest{Action: "list", Path: dir, Limit: 2})
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if len(resp.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(resp.Entries))
	}
	if resp.Total != 5 {
		t.Fatalf("expected total 5, got %d", resp.Total)
	}
	if !resp.Truncated {
		t.Fatal("should be truncated")
	}

	// Second page
	resp = h.Browse(&BrowseRequest{Action: "list", Path: dir, Offset: 2, Limit: 2})
	if len(resp.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(resp.Entries))
	}
	if !resp.Truncated {
		t.Fatal("should be truncated")
	}

	// Last page
	resp = h.Browse(&BrowseRequest{Action: "list", Path: dir, Offset: 4, Limit: 2})
	if len(resp.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(resp.Entries))
	}
	if resp.Truncated {
		t.Fatal("should not be truncated")
	}

	// Beyond range
	resp = h.Browse(&BrowseRequest{Action: "list", Path: dir, Offset: 10, Limit: 2})
	if len(resp.Entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(resp.Entries))
	}
	if resp.Truncated {
		t.Fatal("should not be truncated")
	}
}

func TestBrowseList_LimitCap(t *testing.T) {
	dir := t.TempDir()
	h := NewStreamHandler(StreamConfig{Enabled: true, AllowedPaths: []string{dir}})

	// Request with limit > maxBrowseLimit should be capped
	resp := h.Browse(&BrowseRequest{Action: "list", Path: dir, Limit: 5000})
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	// No error means limit was silently capped
}

func TestBrowseList_PathRequired(t *testing.T) {
	h := NewStreamHandler(StreamConfig{Enabled: true, AllowedPaths: []string{"*"}})
	resp := h.Browse(&BrowseRequest{Action: "list"})
	if resp.Error != "path is required" {
		t.Fatalf("expected path required error, got: %s", resp.Error)
	}
}

func TestBrowseList_PathNotAllowed(t *testing.T) {
	h := NewStreamHandler(StreamConfig{Enabled: true, AllowedPaths: []string{"/tmp"}})
	resp := h.Browse(&BrowseRequest{Action: "list", Path: "/root"})
	if resp.Error == "" {
		t.Fatal("expected error for disallowed path")
	}
}

func TestBrowseList_NotADirectory(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "test.txt")
	os.WriteFile(file, []byte("data"), 0644)

	h := NewStreamHandler(StreamConfig{Enabled: true, AllowedPaths: []string{dir}})
	resp := h.Browse(&BrowseRequest{Action: "list", Path: file})
	if resp.Error == "" {
		t.Fatal("expected error for non-directory path")
	}
}

func TestBrowseList_NonexistentPath(t *testing.T) {
	dir := t.TempDir()
	h := NewStreamHandler(StreamConfig{Enabled: true, AllowedPaths: []string{dir}})
	resp := h.Browse(&BrowseRequest{Action: "list", Path: filepath.Join(dir, "nonexistent")})
	if resp.Error == "" {
		t.Fatal("expected error for nonexistent path")
	}
}

func TestBrowseList_Symlink(t *testing.T) {
	dir := t.TempDir()

	// Create a target file
	targetFile := filepath.Join(dir, "target.txt")
	os.WriteFile(targetFile, []byte("hello"), 0644)

	// Create a symlink
	linkPath := filepath.Join(dir, "link.txt")
	os.Symlink(targetFile, linkPath)

	h := NewStreamHandler(StreamConfig{Enabled: true, AllowedPaths: []string{dir}})
	resp := h.Browse(&BrowseRequest{Action: "list", Path: dir})

	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}

	// Find the symlink entry
	var linkEntry *FileEntry
	for i := range resp.Entries {
		if resp.Entries[i].Name == "link.txt" {
			linkEntry = &resp.Entries[i]
			break
		}
	}

	if linkEntry == nil {
		t.Fatal("symlink entry not found")
	}
	if !linkEntry.IsSymlink {
		t.Fatal("expected is_symlink to be true")
	}
	if linkEntry.LinkTarget != targetFile {
		t.Fatalf("expected link target %s, got %s", targetFile, linkEntry.LinkTarget)
	}
	if linkEntry.Size != 5 {
		t.Fatalf("expected resolved size 5, got %d", linkEntry.Size)
	}
}

func TestBrowseStat_File(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "test.txt")
	os.WriteFile(file, []byte("hello world"), 0644)

	h := NewStreamHandler(StreamConfig{Enabled: true, AllowedPaths: []string{dir}})
	resp := h.Browse(&BrowseRequest{Action: "stat", Path: file})

	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if resp.Entry == nil {
		t.Fatal("expected entry in response")
	}
	if resp.Entry.Name != "test.txt" {
		t.Fatalf("expected name test.txt, got %s", resp.Entry.Name)
	}
	if resp.Entry.Size != 11 {
		t.Fatalf("expected size 11, got %d", resp.Entry.Size)
	}
	if resp.Entry.IsDir {
		t.Fatal("expected is_dir false")
	}
	if resp.Entry.Mode != "0644" {
		t.Fatalf("expected mode 0644, got %s", resp.Entry.Mode)
	}
}

func TestBrowseStat_Directory(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "subdir")
	os.Mkdir(subdir, 0755)

	h := NewStreamHandler(StreamConfig{Enabled: true, AllowedPaths: []string{dir}})
	resp := h.Browse(&BrowseRequest{Action: "stat", Path: subdir})

	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if resp.Entry == nil {
		t.Fatal("expected entry in response")
	}
	if !resp.Entry.IsDir {
		t.Fatal("expected is_dir true")
	}
}

func TestBrowseStat_PathRequired(t *testing.T) {
	h := NewStreamHandler(StreamConfig{Enabled: true, AllowedPaths: []string{"*"}})
	resp := h.Browse(&BrowseRequest{Action: "stat"})
	if resp.Error != "path is required" {
		t.Fatalf("expected path required error, got: %s", resp.Error)
	}
}

func TestBrowseStat_Nonexistent(t *testing.T) {
	dir := t.TempDir()
	h := NewStreamHandler(StreamConfig{Enabled: true, AllowedPaths: []string{dir}})
	resp := h.Browse(&BrowseRequest{Action: "stat", Path: filepath.Join(dir, "nonexistent")})
	if resp.Error == "" {
		t.Fatal("expected error for nonexistent path")
	}
}

func TestBrowseRoots_Wildcard(t *testing.T) {
	h := NewStreamHandler(StreamConfig{Enabled: true, AllowedPaths: []string{"*"}})
	resp := h.Browse(&BrowseRequest{Action: "roots"})

	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if !resp.Wildcard {
		t.Fatal("expected wildcard true")
	}
	if len(resp.Roots) != 1 || resp.Roots[0] != "/" {
		t.Fatalf("expected roots [/], got %v", resp.Roots)
	}
}

func TestBrowseRoots_SpecificPaths(t *testing.T) {
	h := NewStreamHandler(StreamConfig{
		Enabled:      true,
		AllowedPaths: []string{"/tmp", "/data/**", "/home/*/uploads"},
	})
	resp := h.Browse(&BrowseRequest{Action: "roots"})

	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if resp.Wildcard {
		t.Fatal("expected wildcard false")
	}
	// /tmp, /data, /home - sorted
	if len(resp.Roots) != 3 {
		t.Fatalf("expected 3 roots, got %d: %v", len(resp.Roots), resp.Roots)
	}
	if resp.Roots[0] != "/data" {
		t.Fatalf("expected first root /data, got %s", resp.Roots[0])
	}
	if resp.Roots[1] != "/home" {
		t.Fatalf("expected second root /home, got %s", resp.Roots[1])
	}
	if resp.Roots[2] != "/tmp" {
		t.Fatalf("expected third root /tmp, got %s", resp.Roots[2])
	}
}

func TestBrowseRoots_EmptyAllowedPaths(t *testing.T) {
	h := NewStreamHandler(StreamConfig{Enabled: true, AllowedPaths: []string{}})
	resp := h.Browse(&BrowseRequest{Action: "roots"})
	if resp.Error == "" {
		t.Fatal("expected error for empty allowed_paths")
	}
}

func TestPatternBaseDir(t *testing.T) {
	tests := []struct {
		pattern string
		want    string
	}{
		{"/tmp", "/tmp"},
		{"/data/**", "/data"},
		{"/home/*/uploads", "/home"},
		{"/var/log/*.log", "/var/log"},
	}

	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			got := patternBaseDir(tt.pattern)
			if got != tt.want {
				t.Fatalf("patternBaseDir(%s) = %s, want %s", tt.pattern, got, tt.want)
			}
		})
	}
}

func TestBrowseList_EmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	h := NewStreamHandler(StreamConfig{Enabled: true, AllowedPaths: []string{dir}})
	resp := h.Browse(&BrowseRequest{Action: "list", Path: dir})

	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if resp.Total != 0 {
		t.Fatalf("expected 0 entries, got %d", resp.Total)
	}
	if len(resp.Entries) != 0 {
		t.Fatalf("expected 0 entries returned, got %d", len(resp.Entries))
	}
	if resp.Truncated {
		t.Fatal("should not be truncated")
	}
}

func TestBrowseList_FileMode(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "exec.sh")
	os.WriteFile(file, []byte("#!/bin/sh"), 0755)

	h := NewStreamHandler(StreamConfig{Enabled: true, AllowedPaths: []string{dir}})
	resp := h.Browse(&BrowseRequest{Action: "list", Path: dir})

	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if len(resp.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(resp.Entries))
	}
	if resp.Entries[0].Mode != "0755" {
		t.Fatalf("expected mode 0755, got %s", resp.Entries[0].Mode)
	}
}

func TestBrowseStat_Symlink(t *testing.T) {
	dir := t.TempDir()

	targetFile := filepath.Join(dir, "target.txt")
	os.WriteFile(targetFile, []byte("hello"), 0644)

	linkPath := filepath.Join(dir, "link.txt")
	os.Symlink(targetFile, linkPath)

	h := NewStreamHandler(StreamConfig{Enabled: true, AllowedPaths: []string{dir}})
	resp := h.Browse(&BrowseRequest{Action: "stat", Path: linkPath})

	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if resp.Entry == nil {
		t.Fatal("expected entry in response")
	}
	if !resp.Entry.IsSymlink {
		t.Fatal("expected is_symlink true")
	}
	if resp.Entry.LinkTarget != targetFile {
		t.Fatalf("expected link target %s, got %s", targetFile, resp.Entry.LinkTarget)
	}
	if resp.Entry.Size != 5 {
		t.Fatalf("expected resolved size 5, got %d", resp.Entry.Size)
	}
}
