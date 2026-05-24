package web

import "embed"

//go:embed templates/*.html
var Assets embed.FS
