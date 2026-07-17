package reactloops

import (
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

// registerHandle 为给定子任务创建 SubAgentHandle 并注册到 registry，返回 handle
//（registry 为 nil 时返回 nil）。调用方在子 loop 创建后需自行设置
// handle.SubLoop 字段。子 loop 结束或失败后，调用方必须调用
// unregisterHandle(...) 移除 handle 并标记为已完成。
func registerHandle(
	registry *ProgressRegistry,
	subTaskID, identifier string,
	subTask aicommon.AIStatefulTask,
	startedAt time.Time,
) *SubAgentHandle {
	if registry == nil {
		return nil
	}
	return registry.Register(NewSubAgentHandle(subTaskID, identifier, subTask, startedAt))
}

// unregisterHandle 从 registry 移除 handle（如果存在）并以给定错误标记其为已
// 完成。handle 或 registry 为 nil 时安全调用。
func unregisterHandle(handle *SubAgentHandle, registry *ProgressRegistry, subTaskID string, execErr error) {
	if handle == nil || registry == nil {
		return
	}
	registry.Unregister(subTaskID, execErr)
}

// deriveScopeName 从候选列表中选取第一个非空的（trim 后）显示名，全部为空时
// 回退到 def。被 nested-loop 辅助函数（RunNestedLoop / runNestedInPlace）用来从
// job.TaskName / job.Identifier / job.LoopName 推导子任务的 scope 名。
func deriveScopeName(def string, candidates ...string) string {
	for _, c := range candidates {
		if s := strings.TrimSpace(c); s != "" {
			return s
		}
	}
	return def
}
