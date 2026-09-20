//go:build gzip_embed

package thirdparty_bin

import (
	"embed"

	"github.com/yaklang/yaklang/common/utils/gzip_embed"
)

//go:embed config.tar.gz
var configFSArchive embed.FS

// Match development resource paths without retaining a global decompressed copy.
var configFS *gzip_embed.PreprocessingEmbed

func init() {
	var err error
	configFS, err = gzip_embed.NewPreprocessingEmbed(&configFSArchive, "config.tar.gz", false)
	if err != nil {
		panic(err)
	}
}
