//go:build !gzip_embed

package crep

import "embed"

//go:embed static/*
var staticFS embed.FS
