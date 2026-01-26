//go:build gzip_embed

package coreplugin

import (
	"embed"

	"io/fs"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/gzip_embed"
)

//go:embed base-yak-plugin.tar.gz
var basePlugin embed.FS

func InitEmbedFS() {
	var err error
	fs, err := gzip_embed.NewPreprocessingEmbed(&basePlugin, "base-yak-plugin.tar.gz", true)
	if err != nil {
		log.Errorf("init embed failed: %v", err)
		fs = gzip_embed.NewEmptyPreprocessingEmbed()
	}
	basePluginFS = fs
}

func init() {
	InitEmbedFS()
}

// getBasePlugin returns the basePlugin embed.FS
func getBasePlugin() interface {
	ReadFile(name string) ([]byte, error)
	ReadDir(name string) ([]fs.DirEntry, error)
} {
	return basePlugin
}
