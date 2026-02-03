//go:build !gzip_embed

package aiforge

import (
	"embed"

	"github.com/yaklang/yaklang/common/utils/resources_monitor"
)

//go:embed buildinforge/**
var buildInForge embed.FS

func InitEmbedFS() {
	buildInForgeFS = resources_monitor.NewStandardResourceMonitor(buildInForge, "")
}

func init() {
	InitEmbedFS()
}
