package diagnostics

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/log"
)

// PerfLogMarker 性能日志特殊标记，便于在输出中 grep 提取
// 使用方式: grep -F '[SSA_PERF]' 或 grep '\[SSA_PERF\]'
// 仅当 file-perf-log 或 rule-perf-log 至少开启一个时输出
const PerfLogMarker = "[SSA_PERF]"

// LogPerfIf 当 enable 为 true 时输出带 [SSA_PERF] 标记的日志，用于中途/流式统计
// enable 应由 file-perf-log 或 rule-perf-log 控制
func LogPerfIf(enable bool, content string) {
	if !enable {
		return
	}
	content = strings.TrimRight(content, "\n")
	if content == "" {
		return
	}
	for _, line := range strings.Split(content, "\n") {
		log.Info(PerfLogMarker + " " + line)
	}
}

// LogPerfLineIf 当 enable 为 true 时输出单行带 [SSA_PERF] 标记的日志
func LogPerfLineIf(enable bool, format string, args ...interface{}) {
	if !enable {
		return
	}
	line := format
	if len(args) > 0 {
		line = fmt.Sprintf(format, args...)
	}
	log.Info(PerfLogMarker + " " + line)
}
