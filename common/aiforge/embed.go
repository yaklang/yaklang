//go:build !gzip_embed

package aiforge

import (
	"embed"
	"io/fs"

	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

//go:embed buildinforge/**
var buildInForge embed.FS

// embedFSWithHash 包装 embed.FS 并添加 GetHash 方法
type embedFSWithHash struct {
	filesys_interface.FileSystem
	fs embed.FS
}

func (e *embedFSWithHash) GetHash() (string, error) {
	// Calculate hash for all files (no extension filter)
	return filesys.CreateEmbedFSHash(e.fs)
}

func InitEmbedFS() {
	buildInForgeFS = &embedFSWithHash{
		FileSystem: filesys.NewEmbedFS(buildInForge),
		fs:         buildInForge,
	}
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
