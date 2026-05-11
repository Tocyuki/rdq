package server

import "embed"

// frontendFS embeds the frontend build output.
// `make frontend-build` copies frontend/dist into this directory for releases.
//
//go:embed all:dist
var frontendFS embed.FS
