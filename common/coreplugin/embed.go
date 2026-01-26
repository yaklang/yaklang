//go:build !gzip_embed

package coreplugin

import (
	"embed"
	"io/fs"

	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

//go:embed base-yak-plugin
var basePlugin embed.FS

// embedFSWithHash 包装 embed.FS 并添加 GetHash 方法
type embedFSWithHash struct {
	fi.FileSystem
	fs embed.FS
}

func (e *embedFSWithHash) GetHash() (string, error) {
	// Only calculate hash for .yak files
	return filesys.CreateEmbedFSHash(e.fs, filesys.WithIncludeExts(".yak"))
}

func InitEmbedFS() {
	basePluginFS = &embedFSWithHash{
		FileSystem: filesys.NewEmbedFS(basePlugin),
		fs:         basePlugin,
	}
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

