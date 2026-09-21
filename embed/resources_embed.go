//go:build !gzip_embed

package embed

import "embed"

//go:embed data dataex
var FS embed.FS
