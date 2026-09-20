//go:build !gzip_embed

package thirdparty_bin

import "embed"

//go:embed bin_cfg.yml
var configFS embed.FS
