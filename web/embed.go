// Package web embeds the compiled SvelteKit dashboard assets.
//
// The build/ directory is produced by running "npm run build" inside the
// web/ directory. adapter-static outputs a fully pre-rendered SPA with an
// index.html fallback, suitable for serving from an embedded filesystem.
//
// Rebuild before compiling the Go binary:
//
//	cd web && npm run build
package web

import "embed"

// Build contains the static SvelteKit build output.
// The directory tree is rooted at "build", so callers should use
// fs.Sub(Build, "build") to get a filesystem rooted at the assets.
//
//go:embed build
var Build embed.FS
