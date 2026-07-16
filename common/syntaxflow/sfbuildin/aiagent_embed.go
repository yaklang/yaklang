//go:build !gzip_embed && !irify_exclude

package sfbuildin

import (
	"embed"
	"sync"

	"github.com/yaklang/yaklang/common/utils/resources_monitor"
)

//go:embed aiagent/***
var aiagentRuleFS embed.FS

var (
	aiagentFSOnce     sync.Once
	aiagentFSInstance resources_monitor.ResourceMonitor
)

// InitAIAgentEmbedFS 初始化 AI Agent 规则包的嵌入文件系统
func InitAIAgentEmbedFS() {
	aiagentFSInstance = resources_monitor.NewStandardResourceMonitor(aiagentRuleFS, ".sf")
}

func init() {
	InitAIAgentEmbedFS()
}

// GetAIAgentRuleFS 返回 AI Agent 规则包的文件系统实例
func GetAIAgentRuleFS() resources_monitor.ResourceMonitor {
	aiagentFSOnce.Do(func() {
		if aiagentFSInstance == nil {
			InitAIAgentEmbedFS()
		}
	})
	return aiagentFSInstance
}
