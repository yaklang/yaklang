//go:build gzip_embed

package standards

import (
	"embed"

	"github.com/yaklang/yaklang/common/utils/gzip_embed"
)

//go:embed mappings.tar.gz
var mappingsFSArchive embed.FS

// Decode and decompress on first access, then reuse resident contents.
var mappingsFS *gzip_embed.PreprocessingEmbed

func init() {
	var err error
	mappingsFS, err = gzip_embed.NewPreprocessingEmbed(&mappingsFSArchive, "mappings.tar.gz")
	if err != nil {
		panic(err)
	}
}
