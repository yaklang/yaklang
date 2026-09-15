package resources

import (
	"embed"

	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"

	"github.com/yaklang/yaklang/common/utils/filesys"
)

// ysoXorKey 用于 XOR 编码嵌入的 static.tar.gz
const ysoXorKey = "yaklang-yso-v1"

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
