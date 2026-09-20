//go:build gzip_embed && !irify_exclude

package sfdb

import (
	"embed"

	"github.com/yaklang/yaklang/common/utils/gzip_embed"
)

//go:embed rule_versions.tar.gz
var ruleVersionFSArchive embed.FS

// Match development resource paths without retaining a global decompressed copy.
var ruleVersionFS *gzip_embed.PreprocessingEmbed

func init() {
	var err error
	ruleVersionFS, err = gzip_embed.NewPreprocessingEmbed(&ruleVersionFSArchive, "rule_versions.tar.gz", false)
	if err != nil {
		panic(err)
	}
}
