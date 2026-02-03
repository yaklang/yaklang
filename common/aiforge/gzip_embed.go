//go:build gzip_embed

package aiforge

import (
	"embed"

	"github.com/yaklang/yaklang/common/utils/resources_monitor"
)

//go:embed buildinforge.tar.gz
var buildInForge embed.FS

func InitEmbedFS() {
	buildInForgeFS = resources_monitor.NewGzipResourceMonitor(&buildInForge, "buildinforge.tar.gz", "buildinforge")
}

func init() {
	InitEmbedFS()
}
