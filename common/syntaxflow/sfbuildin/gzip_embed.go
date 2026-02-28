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

// GetEmbedRuleContent 从内置 embed FS 中按相对路径读取规则文件内容。
// path 为相对于 buildin/ 目录的路径
func GetEmbedRuleContent(path string) (string, bool) {
	// embed FS 内文件路径格式为 "buildin/<path>"
	fullPath := "buildin/" + path
	raw, err := ruleFSWithHash.ReadFile(fullPath)
	if err != nil {
		return "", false
	}
	return string(raw), true
}
