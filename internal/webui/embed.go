// Package webui provides an embedded web dashboard for Muti Metroo.
package webui

import "embed"

//go:embed all:static
var staticFS embed.FS
