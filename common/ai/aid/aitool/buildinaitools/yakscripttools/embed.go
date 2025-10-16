package yakscripttools

import (
	"embed"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/gzip_embed"
)

//go:embed yakscriptforai.tar.gz
var resourceFS embed.FS

var yakScriptFS *gzip_embed.PreprocessingEmbed

func InitEmbedFS() {
	var err error
	yakScriptFS, err = gzip_embed.NewPreprocessingEmbed(&resourceFS, "yakscriptforai.tar.gz", true)
	if err != nil {
		log.Errorf("init embed failed: %v", err)
		yakScriptFS = gzip_embed.NewEmptyPreprocessingEmbed()
	}
}
