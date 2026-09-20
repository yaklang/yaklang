//go:build gzip_embed

package thirdparty_bin

import (
	"embed"

	"github.com/yaklang/yaklang/common/utils/gzip_embed"
)

//go:embed config.tar.gz
var configFSArchive embed.FS

// Decode and decompress on first access, then reuse resident contents.
var configFS *gzip_embed.PreprocessingEmbed

func init() {
	var err error
	configFS, err = gzip_embed.NewPreprocessingEmbed(&configFSArchive, "config.tar.gz")
	if err != nil {
		panic(err)
	}
}
