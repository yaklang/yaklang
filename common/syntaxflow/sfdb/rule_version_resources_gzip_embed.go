//go:build gzip_embed && !irify_exclude

package sfdb

import (
	"embed"

	"github.com/yaklang/yaklang/common/utils/gzip_embed"
)

//go:embed rule_versions.tar.gz
var ruleVersionFSArchive embed.FS

// Decode and decompress on first access, then reuse resident contents.
var ruleVersionFS *gzip_embed.PreprocessingEmbed

func init() {
	var err error
	ruleVersionFS, err = gzip_embed.NewPreprocessingEmbed(&ruleVersionFSArchive, "rule_versions.tar.gz")
	if err != nil {
		panic(err)
	}
}
