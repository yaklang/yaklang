package loop_ssa_api_discovery

import (
	"context"
	"os"
	"strings"
	"time"
)

// 环境变量 YAK_SSA_API_DISCOVERY_SYNTAXFLOW_TIMEOUT：单次 SyntaxFlow 扫描最长时间（如 90m、2h），默认 90m。
// 使用 context.WithoutCancel 避免因父任务 / HTTP 流式上下文先结束导致 scan_error=client canceled（仍可用进程级取消）。

func syntaxFlowScanMaxDuration() time.Duration {
	const def = 90 * time.Minute
	s := strings.TrimSpace(os.Getenv("YAK_SSA_API_DISCOVERY_SYNTAXFLOW_TIMEOUT"))
	if s == "" {
		return def
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return def
	}
	if d < 5*time.Minute {
		return 5 * time.Minute
	}
	return d
}

func detachSyntaxFlowScanContext(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(context.WithoutCancel(parent), syntaxFlowScanMaxDuration())
}
