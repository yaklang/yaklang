package test

import (
	"embed"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/gzip_embed"
)

//go:embed static.tar.gz
var resourceFS embed.FS

var FS *gzip_embed.PreprocessingEmbed

func init() {
	var err error
	FS, err = gzip_embed.NewPreprocessingEmbed(&resourceFS, "static.tar.gz", true)
	if err != nil {
		log.Errorf("init embed failed: %v", err)
		FS = gzip_embed.NewEmptyPreprocessingEmbed()
	}
}
