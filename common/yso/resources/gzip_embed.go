//go:build gzip_embed

package resources

import (
	"embed"

	"github.com/yaklang/yaklang/common/log"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/gzip_embed"
)

// ysoXorKey 用于 XOR 编码嵌入的 static.tar.gz，避免杀软直接解压
// 扫描到 Java class 和序列化 payload（RuntimeExec、TcpReverseShell 等）。
const ysoXorKey = "yaklang-yso-v1"

//go:embed static.tar.gz
var resourceFS embed.FS

var YsoResourceFS fi.FileSystem

func InitEmbedFS() {
	var err error
	YsoResourceFS, err = gzip_embed.NewPreprocessingEmbedWithXORKey(&resourceFS, "static.tar.gz", true, []byte(ysoXorKey))
	if err != nil {
		log.Errorf("init embed failed: %v", err)
		YsoResourceFS = gzip_embed.NewEmptyPreprocessingEmbed()
	}
}
