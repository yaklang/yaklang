//go:build !gzip_embed

package standards

import "embed"

//go:embed mappings.yaml
var mappingsFS embed.FS
