//go:build gzip_embed && !irify_exclude

package sfbuildin

import (
	"embed"

	"github.com/yaklang/yaklang/common/utils/resources_monitor"
)

//go:embed buildin.tar.gz
var ruleFS embed.FS

func InitEmbedFS() {
	ruleFSWithHash = resources_monitor.NewGzipResourceMonitor(&ruleFS, "buildin.tar.gz", "buildin")
}

func init() {
	InitEmbedFS()
}

// InitEmbedFSWithNotify 带进度通知的初始化
func InitEmbedFSWithNotify(notify func(process float64, ruleName string)) {
	ruleFSWithHash.SetNotify(notify)
}
