//go:build gzip_embed

package yakscripttools

import (
	"embed"

	"github.com/yaklang/yaklang/common/utils/resources_monitor"
)

//go:embed yakscriptforai.tar.gz
var resourceFS embed.FS

var yakScriptFS FileSystemWithHash

func InitEmbedFS() {
	yakScriptFS = resources_monitor.NewGzipResourceMonitor(&resourceFS, "yakscriptforai.tar.gz", "yakscriptforai")
}
