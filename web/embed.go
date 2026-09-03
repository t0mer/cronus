// Package webui embeds the built Cronus single-page application. The Vite build
// writes to web/dist, which is embedded here and served by the HTTP layer in
// production. A tracked dist/.gitkeep keeps this compiling before any frontend
// build has run.
package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// FS returns the embedded frontend rooted at the dist directory.
func FS() (fs.FS, error) {
	return fs.Sub(dist, "dist")
}

// HasBuild reports whether a real build (an index.html) is embedded, as opposed
// to only the placeholder.
func HasBuild() bool {
	f, err := dist.Open("dist/index.html")
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}
