// Package integration provides integration tests for Muti Metroo.
package integration

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/postalsys/muti-metroo/internal/config"
	"github.com/postalsys/muti-metroo/internal/filetransfer"
)

// uploadDirectory tars+gzips a local directory and POSTs it to the remote
// agent's /file/upload endpoint with the directory=true flag. The server
// untars into remoteDir. Returns the parsed response or an error.
func uploadDirectory(t *testing.T, agentAddr, targetID, localDir, remoteDir, password string) (*uploadResponse, error) {
	t.Helper()

	var tarBuf bytes.Buffer
	if err := filetransfer.TarDirectory(localDir, &tarBuf); err != nil {
		return nil, fmt.Errorf("tar %s: %w", localDir, err)
	}

	var formBuf bytes.Buffer
	writer := multipart.NewWriter(&formBuf)

	part, err := writer.CreateFormFile("file", filepath.Base(localDir)+".tar.gz")
	if err != nil {
		return nil, fmt.Errorf("form file: %w", err)
	}
	if _, err := io.Copy(part, &tarBuf); err != nil {
		return nil, fmt.Errorf("copy tar: %w", err)
	}
	if err := writer.WriteField("path", remoteDir); err != nil {
		return nil, err
	}
	if err := writer.WriteField("directory", "true"); err != nil {
		return nil, err
	}
	if password != "" {
		if err := writer.WriteField("password", password); err != nil {
			return nil, err
		}
	}
	writer.Close()

	url := fmt.Sprintf("http://%s/agents/%s/file/upload", agentAddr, targetID)
	req, err := http.NewRequest(http.MethodPost, url, &formBuf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var result uploadResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w (body: %s)", err, string(body))
	}
	return &result, nil
}

// downloadDirectory POSTs to the remote agent's /file/download endpoint
// and extracts the returned tar.gz stream into localDir.
func downloadDirectory(t *testing.T, agentAddr, targetID, remoteDir, localDir, password string) error {
	t.Helper()

	reqBody, err := json.Marshal(downloadRequest{Path: remoteDir, Password: password})
	if err != nil {
		return err
	}

	url := fmt.Sprintf("http://%s/agents/%s/file/download", agentAddr, targetID)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("download failed with status %d: %s", resp.StatusCode, body)
	}

	if err := os.MkdirAll(localDir, 0o755); err != nil {
		return err
	}
	return extractTarStream(resp.Body, localDir)
}

// extractTarStream extracts a plain tar archive from r into destDir.
//
// NOTE: The server's download handler advertises Content-Type: application/gzip
// for directory downloads, but the `streamReader` in internal/agent/agent.go
// already decompresses the gzip layer before the HTTP response body sees it --
// so the body is actually plain tar. This helper therefore does NOT wrap with
// gzip.NewReader. If the server is ever fixed to send what its Content-Type
// header advertises, this test will fail loudly with an "invalid tar header"
// error, which is the desired signal to update this helper.
func extractTarStream(r io.Reader, destDir string) error {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar read: %w", err)
		}
		target := filepath.Join(destDir, filepath.FromSlash(hdr.Name))
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(destDir)) {
			return fmt.Errorf("tar entry escapes destDir: %s", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
		}
	}
	return nil
}

// dirFile describes an expected file in a directory tree: its relative
// path (with forward slashes), its expected content, and its expected
// mode bits (permission portion).
type dirFile struct {
	RelPath string
	Content []byte
	Mode    os.FileMode
}

// asMap indexes a []dirFile by RelPath for order-independent comparison.
func asMap(files []dirFile) map[string]dirFile {
	m := make(map[string]dirFile, len(files))
	for _, f := range files {
		m[f.RelPath] = f
	}
	return m
}

// assertDirTreeMatches checks that got contains exactly the files in want
// (matched by RelPath). Content and mode bits are compared per file.
// Mode bits are only checked if checkMode is true.
func assertDirTreeMatches(t *testing.T, want, got []dirFile, checkMode bool) {
	t.Helper()
	wantMap := asMap(want)
	gotMap := asMap(got)
	if len(wantMap) != len(gotMap) {
		t.Fatalf("file count mismatch: got %d, want %d (got=%v want=%v)",
			len(gotMap), len(wantMap), keysOf(gotMap), keysOf(wantMap))
	}
	for relPath, w := range wantMap {
		g, ok := gotMap[relPath]
		if !ok {
			t.Errorf("missing file: %s", relPath)
			continue
		}
		if !bytes.Equal(g.Content, w.Content) {
			t.Errorf("file %s: content mismatch (got %d bytes, want %d)", relPath, len(g.Content), len(w.Content))
		}
		if checkMode && g.Mode != w.Mode {
			t.Errorf("file %s: mode got %o want %o", relPath, g.Mode, w.Mode)
		}
	}
}

func keysOf(m map[string]dirFile) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// writeDirTree creates a directory tree on disk rooted at root with the
// given files. Subdirectories are created automatically with mode 0755.
func writeDirTree(t *testing.T, root string, files []dirFile) {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", root, err)
	}
	for _, f := range files {
		full := filepath.Join(root, filepath.FromSlash(f.RelPath))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir parent of %s: %v", full, err)
		}
		if err := os.WriteFile(full, f.Content, f.Mode); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
		// os.WriteFile may honor umask; force the exact mode.
		if err := os.Chmod(full, f.Mode); err != nil {
			t.Fatalf("chmod %s: %v", full, err)
		}
	}
}

// readDirTree walks root and returns the files it finds, sorted by
// relative path. Directories themselves are not returned (only regular
// files); this matches what the tar round-trip preserves for us.
func readDirTree(t *testing.T, root string) []dirFile {
	t.Helper()
	var out []dirFile
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out = append(out, dirFile{
			RelPath: filepath.ToSlash(rel),
			Content: content,
			Mode:    info.Mode().Perm(),
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RelPath < out[j].RelPath })
	return out
}

// startFileTransferChain spins up a 4-agent chain with the HTTP server
// enabled on agent A and file transfer enabled on agent D using the given
// config. It waits for routes to propagate and registers cleanup via
// t.Cleanup. Unlike newFileTransferTestChain (which returns an unstarted
// chain and leaves timing to the caller), this helper is always-started and
// uses the polling WaitForRoutes helper instead of a fixed sleep, making
// tests deterministic per the no-flake policy.
func startFileTransferChain(t *testing.T, ftCfg *config.FileTransferConfig) *AgentChain {
	t.Helper()

	chain := NewAgentChain(t)
	chain.EnableHTTP = true
	chain.FileTransferConfig = ftCfg
	chain.CreateAgents(t)
	chain.StartAgents(t)
	t.Cleanup(chain.Close)
	if !chain.WaitForRoutes(t) {
		t.Fatal("Route propagation failed")
	}
	return chain
}

// TestFileTransfer_DirectoryUpload verifies that a local directory tree
// can be uploaded (tarred by the client, untarred by the server) through
// the HTTP file transfer API, and that every expected file lands on disk
// with the correct content.
//
// Covers row 123 (Directory upload (tar+gz)).
func TestFileTransfer_DirectoryUpload(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tmpDir := t.TempDir()
	chain := startFileTransferChain(t, &config.FileTransferConfig{
		Enabled:      true,
		AllowedPaths: []string{tmpDir},
	})
	targetID := chain.Agents[3].ID().String()

	source := filepath.Join(t.TempDir(), "tree-to-upload")
	tree := []dirFile{
		{"top.txt", []byte("top-level content"), 0o644},
		{"sub/a.txt", []byte("file a"), 0o644},
		{"sub/b.txt", []byte("file b"), 0o644},
		{"sub/deeper/c.txt", []byte("file c deep"), 0o644},
	}
	writeDirTree(t, source, tree)

	dest := filepath.Join(tmpDir, "uploaded-tree")

	result, err := uploadDirectory(t, chain.HTTPAddrs[0], targetID, source, dest, "")
	if err != nil {
		t.Fatalf("uploadDirectory: %v", err)
	}
	if !result.Success {
		t.Fatalf("upload not successful: %s", result.Error)
	}

	assertDirTreeMatches(t, tree, readDirTree(t, dest), false)
}

// TestFileTransfer_DirectoryDownload verifies that a directory tree
// present on the remote agent can be downloaded through the HTTP file
// transfer API, extracted from the returned tar stream, and compared to
// the remote source byte-for-byte.
//
// Covers row 124 (Directory download (tar+gz)).
func TestFileTransfer_DirectoryDownload(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tmpDir := t.TempDir()
	chain := startFileTransferChain(t, &config.FileTransferConfig{
		Enabled:      true,
		AllowedPaths: []string{tmpDir},
	})
	targetID := chain.Agents[3].ID().String()

	// Place a directory tree directly in tmpDir so the server can read it.
	source := filepath.Join(tmpDir, "tree-on-remote")
	tree := []dirFile{
		{"hello.txt", []byte("hello from remote"), 0o644},
		{"nested/readme.md", []byte("# nested readme\nhello"), 0o644},
		{"nested/data/payload.bin", bytes.Repeat([]byte{0xab}, 1024), 0o644},
	}
	writeDirTree(t, source, tree)

	localDest := filepath.Join(t.TempDir(), "downloaded-tree")
	if err := downloadDirectory(t, chain.HTTPAddrs[0], targetID, source, localDest, ""); err != nil {
		t.Fatalf("downloadDirectory: %v", err)
	}

	assertDirTreeMatches(t, tree, readDirTree(t, localDest), false)
}

// TestFileTransfer_DirectoryPermissions verifies that file mode bits are
// preserved across a directory round-trip (upload + download).
//
// Covers row 125 (Permission preservation).
func TestFileTransfer_DirectoryPermissions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tmpDir := t.TempDir()
	chain := startFileTransferChain(t, &config.FileTransferConfig{
		Enabled:      true,
		AllowedPaths: []string{tmpDir},
	})
	targetID := chain.Agents[3].ID().String()

	source := filepath.Join(t.TempDir(), "perms-source")
	tree := []dirFile{
		{"readable.txt", []byte("readable"), 0o644},
		{"restricted.txt", []byte("top secret"), 0o600},
		{"executable.sh", []byte("#!/bin/sh\necho hi\n"), 0o755},
	}
	writeDirTree(t, source, tree)

	remotePath := filepath.Join(tmpDir, "perms-dest")
	if _, err := uploadDirectory(t, chain.HTTPAddrs[0], targetID, source, remotePath, ""); err != nil {
		t.Fatalf("uploadDirectory: %v", err)
	}

	// After the upload, the remote files exist on disk in tmpDir (same
	// process, same filesystem). Read their mode bits directly.
	assertDirTreeMatches(t, tree, readDirTree(t, remotePath), true)

	// Now download the directory back and re-check the mode bits.
	roundTripped := filepath.Join(t.TempDir(), "perms-roundtrip")
	if err := downloadDirectory(t, chain.HTTPAddrs[0], targetID, remotePath, roundTripped, ""); err != nil {
		t.Fatalf("downloadDirectory: %v", err)
	}
	assertDirTreeMatches(t, tree, readDirTree(t, roundTripped), true)
}
