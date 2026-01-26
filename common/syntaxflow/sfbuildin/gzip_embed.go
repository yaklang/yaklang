//go:build gzip_embed && !irify_exclude

package sfbuildin

import (
	"embed"
	"io/fs"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/gzip_embed"
)

//go:embed buildin.tar.gz
var ruleFS embed.FS

func InitEmbedFS() {
	var err error
	fs, err := gzip_embed.NewPreprocessingEmbed(&ruleFS, "buildin.tar.gz", true)
	if err != nil {
		log.Errorf("init embed failed: %v", err)
		fs = gzip_embed.NewEmptyPreprocessingEmbed()
	}
	ruleFSWithHash = fs
}

func init() {
	InitEmbedFS()
}

// getRuleFS returns the ruleFS embed.FS
func getRuleFS() interface {
	ReadFile(name string) ([]byte, error)
	ReadDir(name string) ([]fs.DirEntry, error)
} {
	return ruleFS
}
