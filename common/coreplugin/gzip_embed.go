//go:build gzip_embed

package coreplugin

import (
	"embed"
	"github.com/yaklang/yaklang/common/utils/builtinmetadata"

	"github.com/yaklang/yaklang/common/utils/resources_monitor"
)

//go:embed base-yak-plugin.tar.gz
var basePlugin embed.FS

func InitEmbedFS() {
	builtinmetadata.Register("coreplugin", "base-yak-plugin", func() ([]byte, error) { return basePlugin.ReadFile("base-yak-plugin.tar.gz") })
	basePluginFS = resources_monitor.NewGzipResourceMonitor(&basePlugin, "base-yak-plugin.tar.gz", "base-yak-plugin")
}

func init() {
	InitEmbedFS()
}
