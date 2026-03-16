//go:build ssa2llvm_gzip_embed

package embed

import (
	"embed"

	"github.com/yaklang/yaklang/common/utils/resources_monitor"
)

//go:embed ssa2llvm-runtime.tar.gz
var embeddedRuntimeTarGz embed.FS

//go:embed ssa2llvm-runtime-src.tar.gz
var embeddedRuntimeSrcTarGz embed.FS

var runtimeFS resources_monitor.ResourceMonitor
var runtimeSourceFS resources_monitor.ResourceMonitor

func init() {
	runtimeFS = resources_monitor.NewGzipResourceMonitor(&embeddedRuntimeTarGz, "ssa2llvm-runtime.tar.gz", embeddedPrefix)
	runtimeSourceFS = resources_monitor.NewGzipResourceMonitor(&embeddedRuntimeSrcTarGz, "ssa2llvm-runtime-src.tar.gz", embeddedSrcPrefix)
}

func embeddedRuntimeFS() (readFileFS, bool) {
	return runtimeFS, true
}

func embeddedRuntimeSourceFS() (readDirFileFS, bool) {
	return runtimeSourceFS, true
}
