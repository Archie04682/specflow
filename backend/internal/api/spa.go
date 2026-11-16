package api

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// spaHandler serves the embedded frontend. Any GET or HEAD request that does
// not resolve to an embedded file returns index.html, so client-side routes
// survive a full page reload.
type spaHandler struct {
	staticFS fs.FS
}

func (h spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.NotFound(w, r)
		return
	}

	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name == "" || name == "." {
		name = "index.html"
	}

	if info, err := fs.Stat(h.staticFS, name); err == nil && !info.IsDir() {
		http.FileServer(http.FS(h.staticFS)).ServeHTTP(w, r)
		return
	}

	index, err := fs.ReadFile(h.staticFS, "index.html")
	if err != nil {
		http.Error(w, "web UI is not built", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(index)
}
