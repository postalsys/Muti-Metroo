package filetransfer

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	defaultBrowseLimit = 100
	maxBrowseLimit     = 200
)

// BrowseRequest is the request payload for file browsing operations.
type BrowseRequest struct {
	Action   string `json:"action"`            // "list", "stat", "roots"
	Path     string `json:"path,omitempty"`     // Required for "list" and "stat"
	Password string `json:"password,omitempty"` // Authentication password
	Offset   int    `json:"offset,omitempty"`   // Pagination offset (list only)
	Limit    int    `json:"limit,omitempty"`    // Pagination limit (list only, default 100, max 200)
}

// BrowseResponse is the response payload for file browsing operations.
type BrowseResponse struct {
	Path      string      `json:"path,omitempty"`      // Echoed back for list/stat
	Entries   []FileEntry `json:"entries,omitempty"`    // Directory entries (list only)
	Total     int         `json:"total"`                // Total entry count before pagination (list only)
	Truncated bool        `json:"truncated"`            // True when more entries exist beyond offset+limit
	Entry     *FileEntry  `json:"entry,omitempty"`      // Single entry (stat only)
	Roots     []string    `json:"roots,omitempty"`      // Browsable root paths (roots only)
	Wildcard  bool        `json:"wildcard,omitempty"`   // True when allowed_paths contains "*" (roots only)
	Error     string      `json:"error,omitempty"`      // Error message
}

// FileEntry represents a single file or directory entry.
type FileEntry struct {
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	Mode       string `json:"mode"`                          // e.g. "0755"
	ModTime    string `json:"mod_time"`                      // ISO 8601
	IsDir      bool   `json:"is_dir"`
	IsSymlink  bool   `json:"is_symlink,omitempty"`
	LinkTarget string `json:"link_target,omitempty"`
}

// Browse handles file browsing requests (list, stat, roots).
func (h *StreamHandler) Browse(req *BrowseRequest) *BrowseResponse {
	if !h.cfg.Enabled {
		return &BrowseResponse{Error: "file transfer is disabled"}
	}

	if err := h.authenticate(req.Password); err != nil {
		return &BrowseResponse{Error: err.Error()}
	}

	switch req.Action {
	case "list", "":
		return h.browseList(req)
	case "stat":
		return h.browseStat(req)
	case "roots":
		return h.browseRoots()
	default:
		return &BrowseResponse{Error: fmt.Sprintf("unknown action: %s", req.Action)}
	}
}

// browseList lists directory contents with pagination.
func (h *StreamHandler) browseList(req *BrowseRequest) *BrowseResponse {
	if req.Path == "" {
		return &BrowseResponse{Error: "path is required"}
	}

	if err := h.validatePath(req.Path); err != nil {
		return &BrowseResponse{Error: err.Error()}
	}

	cleanPath := filepath.Clean(req.Path)

	info, err := os.Stat(cleanPath)
	if err != nil {
		return &BrowseResponse{Error: fmt.Sprintf("path not found: %s", cleanPath)}
	}
	if !info.IsDir() {
		return &BrowseResponse{Error: fmt.Sprintf("not a directory: %s", cleanPath)}
	}

	dirEntries, err := os.ReadDir(cleanPath)
	if err != nil {
		return &BrowseResponse{Error: fmt.Sprintf("failed to read directory: %v", err)}
	}

	entries := make([]FileEntry, 0, len(dirEntries))
	for _, de := range dirEntries {
		entries = append(entries, buildFileEntry(cleanPath, de))
	}

	// Sort: directories first, then alphabetical by name
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return entries[i].Name < entries[j].Name
	})

	total := len(entries)

	// Apply pagination
	limit := req.Limit
	if limit <= 0 {
		limit = defaultBrowseLimit
	}
	if limit > maxBrowseLimit {
		limit = maxBrowseLimit
	}

	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	if offset >= total {
		return &BrowseResponse{
			Path:      cleanPath,
			Entries:   []FileEntry{},
			Total:     total,
			Truncated: false,
		}
	}

	end := offset + limit
	if end > total {
		end = total
	}

	return &BrowseResponse{
		Path:      cleanPath,
		Entries:   entries[offset:end],
		Total:     total,
		Truncated: end < total,
	}
}

// browseStat returns info about a single path.
func (h *StreamHandler) browseStat(req *BrowseRequest) *BrowseResponse {
	if req.Path == "" {
		return &BrowseResponse{Error: "path is required"}
	}

	if err := h.validatePath(req.Path); err != nil {
		return &BrowseResponse{Error: err.Error()}
	}

	cleanPath := filepath.Clean(req.Path)

	entry, err := statPath(cleanPath)
	if err != nil {
		return &BrowseResponse{Error: err.Error()}
	}

	return &BrowseResponse{
		Path:  cleanPath,
		Entry: entry,
	}
}

// browseRoots returns the browsable root paths derived from allowed_paths config.
func (h *StreamHandler) browseRoots() *BrowseResponse {
	if len(h.cfg.AllowedPaths) == 0 {
		return &BrowseResponse{Error: "no paths are allowed (allowed_paths is empty)"}
	}

	for _, p := range h.cfg.AllowedPaths {
		if p == "*" {
			return &BrowseResponse{
				Roots:    []string{"/"},
				Wildcard: true,
			}
		}
	}

	// Derive root directories from allowed patterns
	seen := make(map[string]bool)
	var roots []string
	for _, pattern := range h.cfg.AllowedPaths {
		root := patternBaseDir(pattern)
		if root != "" && !seen[root] {
			seen[root] = true
			roots = append(roots, root)
		}
	}

	sort.Strings(roots)

	return &BrowseResponse{
		Roots:    roots,
		Wildcard: false,
	}
}

// patternBaseDir extracts the base directory from an allowed path pattern.
// For example:
//
//	"/tmp"       -> "/tmp"
//	"/data/**"   -> "/data"
//	"/home/*/uploads" -> "/home"
//	"*"          -> "" (handled separately)
func patternBaseDir(pattern string) string {
	clean := normalizePath(pattern)

	// Strip recursive glob suffix
	if strings.HasSuffix(clean, "/**") {
		return strings.TrimSuffix(clean, "/**")
	}

	// If pattern contains glob characters, walk up to the first non-glob component
	if strings.ContainsAny(clean, "*?[") {
		parts := strings.Split(clean, string(filepath.Separator))
		var base []string
		for _, p := range parts {
			if strings.ContainsAny(p, "*?[") {
				break
			}
			base = append(base, p)
		}
		result := strings.Join(base, string(filepath.Separator))
		if result == "" {
			return "/"
		}
		return result
	}

	return clean
}

// populateFromFileInfo fills a FileEntry's size, mode, directory flag, and modification time
// from an os.FileInfo.
func populateFromFileInfo(entry *FileEntry, info os.FileInfo) {
	entry.Size = info.Size()
	entry.IsDir = info.IsDir()
	entry.Mode = fmt.Sprintf("%04o", info.Mode().Perm())
	entry.ModTime = info.ModTime().UTC().Format("2006-01-02T15:04:05Z")
}

// resolveSymlink populates symlink-specific fields on a FileEntry.
// It reads the link target and resolves the symlink to get the target's file info.
// If the symlink is broken, it falls back to the lstat info.
func resolveSymlink(entry *FileEntry, path string, linfo os.FileInfo) {
	entry.IsSymlink = true
	target, err := os.Readlink(path)
	if err == nil {
		entry.LinkTarget = target
	}

	info, err := os.Stat(path)
	if err != nil {
		// Broken symlink -- fall back to lstat info
		populateFromFileInfo(entry, linfo)
		return
	}
	populateFromFileInfo(entry, info)
}

// buildFileEntry creates a FileEntry from an os.DirEntry.
func buildFileEntry(dir string, de os.DirEntry) FileEntry {
	name := de.Name()
	fullPath := filepath.Join(dir, name)
	entry := FileEntry{Name: name}

	linfo, err := os.Lstat(fullPath)
	if err != nil {
		entry.Mode = "0000"
		entry.ModTime = "0001-01-01T00:00:00Z"
		return entry
	}

	if linfo.Mode()&os.ModeSymlink != 0 {
		resolveSymlink(&entry, fullPath, linfo)
		return entry
	}

	info, err := de.Info()
	if err != nil {
		entry.Mode = "0000"
		entry.ModTime = "0001-01-01T00:00:00Z"
		return entry
	}
	populateFromFileInfo(&entry, info)
	return entry
}

// statPath returns a FileEntry for a single path.
func statPath(path string) (*FileEntry, error) {
	linfo, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("path not found: %s", path)
	}

	entry := FileEntry{Name: filepath.Base(path)}

	if linfo.Mode()&os.ModeSymlink != 0 {
		resolveSymlink(&entry, path, linfo)
		return &entry, nil
	}

	populateFromFileInfo(&entry, linfo)
	return &entry, nil
}
