// Package webui provides an embedded web dashboard for Muti Metroo.
package webui

import (
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// Handler returns an http.Handler that serves the embedded static files.
// The returned handler serves files from the /static directory in the embedded FS.
func Handler() http.Handler {
	// Strip "static/" prefix from embedded filesystem
	staticContent, err := fs.Sub(staticFS, "static")
	if err != nil {
		// This should never happen with properly embedded files
		panic("failed to access embedded static files: " + err.Error())
	}
	return &fileHandler{fs: staticContent}
}

// fileHandler serves files from an embedded filesystem
type fileHandler struct {
	fs fs.FS
}

func (h *fileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Clean and normalize the path
	urlPath := path.Clean(r.URL.Path)
	if urlPath == "" || urlPath == "/" {
		urlPath = "/index.html"
	}
	urlPath = strings.TrimPrefix(urlPath, "/")

	// Try to open the file
	f, err := h.fs.Open(urlPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()

	// Get file info
	stat, err := f.Stat()
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// If it's a directory, try to serve index.html
	if stat.IsDir() {
		indexPath := path.Join(urlPath, "index.html")
		indexFile, err := h.fs.Open(indexPath)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer indexFile.Close()
		f = indexFile
		stat, _ = indexFile.Stat()
	}

	// Set content type based on extension
	contentType := getContentType(urlPath)
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}

	// Read and serve the file content
	content, err := io.ReadAll(f)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(content)
}

func getContentType(filePath string) string {
	switch {
	case strings.HasSuffix(filePath, ".html"):
		return "text/html; charset=utf-8"
	case strings.HasSuffix(filePath, ".css"):
		return "text/css; charset=utf-8"
	case strings.HasSuffix(filePath, ".js"):
		return "application/javascript; charset=utf-8"
	case strings.HasSuffix(filePath, ".json"):
		return "application/json; charset=utf-8"
	case strings.HasSuffix(filePath, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(filePath, ".png"):
		return "image/png"
	case strings.HasSuffix(filePath, ".ico"):
		return "image/x-icon"
	default:
		return ""
	}
}
