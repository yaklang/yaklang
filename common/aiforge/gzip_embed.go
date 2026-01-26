//go:build gzip_embed

package aiforge

import (
	"embed"
	"io/fs"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/gzip_embed"
)

//go:embed buildinforge.tar.gz
var buildInForge embed.FS

func InitEmbedFS() {
	var err error
	fs, err := gzip_embed.NewPreprocessingEmbed(&buildInForge, "buildinforge.tar.gz", true)
	if err != nil {
		log.Errorf("init embed failed: %v", err)
		fs = gzip_embed.NewEmptyPreprocessingEmbed()
	}
	buildInForgeFS = fs
}

func init() {
	InitEmbedFS()
}

// getBuildInForge returns the buildInForge embed.FS
func getBuildInForge() interface {
	ReadFile(name string) ([]byte, error)
	ReadDir(name string) ([]fs.DirEntry, error)
} {
	return buildInForge
}
