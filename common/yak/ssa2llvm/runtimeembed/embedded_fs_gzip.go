//go:build gzip_embed

package runtimeembed

import (
	"embed"

	"github.com/yaklang/yaklang/common/utils/resources_monitor"
)

//go:embed ssa2llvm-runtime.tar.gz
var embeddedRuntimeTarGz embed.FS

var runtimeFS resources_monitor.ResourceMonitor

func init() {
	runtimeFS = resources_monitor.NewGzipResourceMonitor(&embeddedRuntimeTarGz, "ssa2llvm-runtime.tar.gz", embeddedPrefix)
}

func embeddedRuntimeFS() (readFileFS, bool) {
	return runtimeFS, true
}
