package main

import (
	"io/fs"
	"net/http"
	"strings"

	webui "github.com/t0mer/cronus/web"
)

// uiHandler returns the embedded single-page app handler, or nil when no
// frontend build is embedded (so the server runs API-only). Static assets are
// served directly; unknown non-API paths fall back to index.html so client-side
// routes resolve.
func uiHandler() http.Handler {
	if !webui.HasBuild() {
		return nil
	}
	dist, err := webui.FS()
	if err != nil {
		return nil
	}
	fileServer := http.FileServer(http.FS(dist))

	index, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		return nil
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			serveIndex(w, index)
			return
		}
		if f, err := dist.Open(p); err == nil {
			_ = f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}
		// SPA fallback for client-side routes.
		serveIndex(w, index)
	})
}

func serveIndex(w http.ResponseWriter, index []byte) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(index)
}
