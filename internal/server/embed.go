package server

import "embed"

// frontendFS embeds the frontend build output.
// The dist directory is symlinked from frontend/dist during build.
//
//go:embed all:dist
var frontendFS embed.FS
