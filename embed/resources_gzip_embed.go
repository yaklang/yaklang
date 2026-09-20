//go:build gzip_embed

package embed

import (
	"embed"

	"github.com/yaklang/yaklang/common/utils/gzip_embed"
)

//go:embed resources.tar.gz
var FSArchive embed.FS

// Match development resource paths without retaining a global decompressed copy.
var FS *gzip_embed.PreprocessingEmbed

func init() {
	var err error
	FS, err = gzip_embed.NewPreprocessingEmbed(&FSArchive, "resources.tar.gz", false)
	if err != nil {
		panic(err)
	}
}
