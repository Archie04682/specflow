// Package web embeds the compiled frontend that the SpecFlow binary serves.
//
// The dist directory is produced by the frontend build (Vite writes into it)
// and the committed dist/.gitkeep keeps the embed pattern valid on a fresh
// clone, before the first frontend build has run.
package web

import "embed"

// DistDir is the path prefix of the embedded frontend inside StaticFS.
const DistDir = "dist"

// StaticFS holds the built frontend assets.
//
//go:embed all:dist
var StaticFS embed.FS
