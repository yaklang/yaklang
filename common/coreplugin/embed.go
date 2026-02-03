//go:build !gzip_embed

package coreplugin

import (
	"embed"

	"github.com/yaklang/yaklang/common/utils/resources_monitor"
)

//go:embed base-yak-plugin
var basePlugin embed.FS

func InitEmbedFS() {
	basePluginFS = resources_monitor.NewStandardResourceMonitor(basePlugin, ".yak")
}

func init() {
	InitEmbedFS()
}
