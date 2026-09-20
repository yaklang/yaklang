//go:build gzip_embed

package standards

import (
	"embed"

	"github.com/yaklang/yaklang/common/utils/gzip_embed"
)

//go:embed mappings.tar.gz
var mappingsFSArchive embed.FS

// Match development resource paths without retaining a global decompressed copy.
var mappingsFS *gzip_embed.PreprocessingEmbed

func init() {
	var err error
	mappingsFS, err = gzip_embed.NewPreprocessingEmbed(&mappingsFSArchive, "mappings.tar.gz", false)
	if err != nil {
		panic(err)
	}
}
