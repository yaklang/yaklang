package resources

import (
	"embed"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/gzip_embed"
)

//go:embed static.tar.gz
var resourceFS embed.FS

var YsoResourceFS *gzip_embed.PreprocessingEmbed

func init() {
	var err error
	YsoResourceFS, err = gzip_embed.NewPreprocessingEmbed(&resourceFS, "static.tar.gz", true)
	if err != nil {
		log.Errorf("init embed failed: %v", err)
		YsoResourceFS = gzip_embed.NewEmptyPreprocessingEmbed()
	}
}
