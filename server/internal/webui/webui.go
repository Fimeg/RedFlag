// Package webui embeds the production web dashboard build into the server
// binary so a native install needs no nginx and no separate web container.
//
// The build pipeline copies web/dist into this package's dist/ directory
// before `go build`. When that copy has not happened (plain `go build` from
// a clean checkout), the embed holds only the .gitkeep placeholder and the
// server runs API-only — Present() reports false and the caller logs it.
package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// FS returns the embedded UI file tree rooted at the dist directory.
func FS() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}

// Present reports whether a real UI build is embedded (index.html exists),
// as opposed to the empty placeholder tree from a UI-less build.
func Present() bool {
	sub, err := FS()
	if err != nil {
		return false
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return false
	}
	return true
}
