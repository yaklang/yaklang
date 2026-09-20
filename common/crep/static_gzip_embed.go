//go:build gzip_embed

package crep

import (
	"embed"

	"github.com/yaklang/yaklang/common/utils/gzip_embed"
)

//go:embed static.tar.gz
var staticFSArchive embed.FS

// Decode and decompress on first access, then reuse resident contents.
var staticFS *gzip_embed.PreprocessingEmbed

func init() {
	var err error
	staticFS, err = gzip_embed.NewPreprocessingEmbed(&staticFSArchive, "static.tar.gz")
	if err != nil {
		panic(err)
	}
}
