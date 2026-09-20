//go:build gzip_embed

package embed

import (
	"embed"

	"github.com/yaklang/yaklang/common/utils/gzip_embed"
)

//go:embed resources.tar.gz
var FSArchive embed.FS

// Decode and decompress on first access, then reuse resident contents.
var FS *gzip_embed.PreprocessingEmbed

func init() {
	var err error
	FS, err = gzip_embed.NewPreprocessingEmbed(&FSArchive, "resources.tar.gz")
	if err != nil {
		panic(err)
	}
}
