//go:build !gzip_embed

package yakscripttools

import (
	"embed"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

//go:embed yakscriptforai
var resourceFS embed.FS

var yakScriptFS FileSystemWithHash

// embedFSWithHash 包装 embed.FS 并添加 GetHash 方法
type embedFSWithHash struct {
	fi.FileSystem
	fs embed.FS
}

func (e *embedFSWithHash) GetHash() (string, error) {
	return filesys.CreateEmbedFSHash(e.fs)
}

func InitEmbedFS() {
	yakScriptFS = &embedFSWithHash{
		FileSystem: filesys.NewEmbedFS(resourceFS),
		fs:         resourceFS,
	}
	log.Info("init embed fs successfully")
}
