//go:build !gzip_embed && !irify_exclude

package sfbuildin

import (
	"embed"
	"io/fs"

	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

//go:embed buildin/***
var ruleFS embed.FS

// embedFSWithHash 包装 embed.FS 并添加 GetHash 方法
type embedFSWithHash struct {
	filesys_interface.FileSystem
	fs embed.FS
}

func (e *embedFSWithHash) GetHash() (string, error) {
	// Only calculate hash for .sf files
	return filesys.CreateEmbedFSHash(e.fs, filesys.WithIncludeExts(".sf"))
}

func InitEmbedFS() {
	ruleFSWithHash = &embedFSWithHash{
		FileSystem: filesys.NewEmbedFS(ruleFS),
		fs:         ruleFS,
	}
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

