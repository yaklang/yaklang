//go:build gzip_embed

package resources

import (
	"embed"

	"github.com/yaklang/yaklang/common/log"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/gzip_embed"
)

//go:embed static.tar.gz
var resourceFS embed.FS

var YsoResourceFS fi.FileSystem

func InitEmbedFS() {
	var err error
	YsoResourceFS, err = gzip_embed.NewPreprocessingEmbed(&resourceFS, "static.tar.gz", true)
	if err != nil {
		log.Errorf("init embed failed: %v", err)
		YsoResourceFS = gzip_embed.NewEmptyPreprocessingEmbed()
	}
}
