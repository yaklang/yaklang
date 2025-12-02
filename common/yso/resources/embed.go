//go:build !gzip_embed

package resources

import (
	"embed"

	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

//go:embed static
var resourceFS embed.FS

var YsoResourceFS fi.FileSystem

// embedFSWithHash 包装 embed.FS 并添加 GetHash 方法
type embedFSWithHash struct {
	fi.FileSystem
	fs embed.FS
}

func (e *embedFSWithHash) GetHash() (string, error) {
	return filesys.CreateEmbedFSHash(e.fs)
}

func InitEmbedFS() {
	YsoResourceFS = filesys.NewEmbedFS(resourceFS)
}
