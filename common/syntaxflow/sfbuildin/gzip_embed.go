//go:build gzip_embed && !irify_exclude

package sfbuildin

import (
	"embed"
	"io/fs"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
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

// GetRuleFileSystem 返回规则文件系统实例（gzip 版本）
func GetRuleFileSystem() filesys_interface.FileSystem {
	// ruleFSWithHash 在 gzip 版本中已经是 PreprocessingEmbed，实现了 FileSystem 接口
	return ruleFSWithHash.(filesys_interface.FileSystem)
}

// CheckDuplicateTitles 检查规则中的 title 和 title_zh 是否重复（gzip 版本不需要检查）
func CheckDuplicateTitles(fsInstance filesys_interface.FileSystem) error {
	// Gzip 版本跳过重复检查，因为 tar.gz 文件在打包时已经验证过
	return nil
}
