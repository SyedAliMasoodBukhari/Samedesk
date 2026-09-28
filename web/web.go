// Package web holds the dashboard UI, compiled into the Shared Hub binary.
package web

import "embed"

// FS contains index.html and the 3D icons (Microsoft Fluent Emoji, MIT licence).
//
//go:embed index.html assets
var FS embed.FS
