package diagnostics

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/log"
)

// envDiagnosticsLevel 环境变量名，用于设置性能诊断粒度。见 Level 注释。
const envDiagnosticsLevel = "YAK_DIAGNOSTICS_LOG_LEVEL"

// Level 控制性能诊断记录的粒度。通过环境变量 YAK_DIAGNOSTICS_LOG_LEVEL 设置。
//
// 使用方法：
//
//	export YAK_DIAGNOSTICS_LOG_LEVEL=off      # 关闭所有性能日志
//	export YAK_DIAGNOSTICS_LOG_LEVEL=measure   # 常规编译/扫描性能
//	export YAK_DIAGNOSTICS_LOG_LEVEL=critical  # 仅关键路径（扫描按规则、Build 树等）
//	export YAK_DIAGNOSTICS_LOG_LEVEL=trace    # 最细粒度（含 ssadb、SyntaxFlow opcode）
//
// 级别与 API 对应关系（设置某 level 时，输出该 level 及以下）：
//
//	LevelHigh (critical/signal): Log、LogLow、LogTable（表格/树）、LogTableLow
//	LevelNormal (measure/monitor/routine): Log、LogLow、LogTableLow（分时间 + 总时间，不含表格/树）
//	LevelLow (trace/detail/verbose): LogLow、LogTableLow（仅总时间）
//	LevelOff: 全部不输出
//
// LevelNormal（分时间）：按文件/按规则/按节点逐项耗时，便于定位慢点。
//
//	+----------+--------+-----------+----------------------------------------------+
//	| 类型     | API    | 需 Level  | 示例                                          |
//	+----------+--------+-----------+----------------------------------------------+
//	| 分时间   | Log    | Normal    | AST[path] 123ms、Build[path] 45ms、Rule foo 12ms |
//	+----------+--------+-----------+----------------------------------------------+
//
// LevelLow（总时间）：按阶段汇总耗时及批次信息，仅概览。
//
//	+----------+--------+-----------+----------------------------------------------+
//	| 类型     | API    | 需 Level  | 示例                                          |
//	+----------+--------+-----------+----------------------------------------------+
//	| 总时间   | LogLow | Low       | total compile 5s、total scan 2m、SaveIrIndexBatch 100 items |
//	+----------+--------+-----------+----------------------------------------------+
//
// LevelHigh 与内存优化（未开启时不创建、不记录，节约内存）：
//
//	+------------+------------------------------------------------------------------------+
//	| 模块       | LevelHigh 未开启时不创建/不记录                                            |
//	+------------+------------------------------------------------------------------------+
//	| 项目编译   | perfRecorder（统一 Recorder）、build tree tracker（树形耗时追踪）        |
//	| AST/Build  | AST[path]、Build[path] 不记录                                             |
//	| Database   | SaveIrCodeBatch、SaveToDatabase 等不记录                                  |
//	| Scan       | ruleProfiler 不创建，规则耗时表不记录                                     |
//	| 静态分析   | perfRecorder 不创建                                                      |
//	+------------+------------------------------------------------------------------------+
//	建议：若无需打印表格/树，保持 LevelHigh 关闭以节省内存。
type Level int

const (
	LevelLow    Level = iota // trace/detail/verbose，最细粒度
	LevelNormal             // measure/monitor/routine，常规编译/扫描
	LevelHigh               // critical/signal/high，表格/树输出，会创建 Recorder 和 build tree
	LevelOff                // 关闭所有性能诊断
)

// levelNames 环境变量字符串到 Level 的映射，支持多别名
var levelNames = map[string]Level{
	"trace":    LevelLow,
	"detail":   LevelLow,
	"verbose":  LevelLow,
	"measure":  LevelNormal,
	"monitor":  LevelNormal,
	"routine":  LevelNormal,
	"critical": LevelHigh,
	"signal":   LevelHigh,
	"high":     LevelHigh, // 便捷别名
	"off":      LevelOff,
}

var levelStrings = map[Level]string{
	LevelLow:    "trace",
	LevelNormal: "measure",
	LevelHigh:   "critical",
	LevelOff:    "off",
}

var (
	levelMu sync.RWMutex
	level   = LevelLow // 默认 low（总时间概览）；可通过 YAK_DIAGNOSTICS_LOG_LEVEL 或 SetLevel 覆盖
)

func init() {
	if raw := strings.TrimSpace(os.Getenv(envDiagnosticsLevel)); raw != "" {
		if err := SetLevelFromString(raw); err != nil {
			log.Warnf("diagnostics: ignoring invalid log level %q: %v", raw, err)
		}
	}
}

// SetLevel overrides the diagnostics log level manually.
func SetLevel(lvl Level) {
	levelMu.Lock()
	level = lvl
	levelMu.Unlock()
}

// SetLevelFromString parses a string and applies the log level if valid.
func SetLevelFromString(raw string) error {
	parsed, ok := parseLevel(raw)
	if !ok {
		return fmt.Errorf("unknown diagnostics log level: %s", raw)
	}
	SetLevel(parsed)
	return nil
}

// GetLevel returns the current diagnostics log level.
func GetLevel() Level {
	levelMu.RLock()
	defer levelMu.RUnlock()
	return level
}

func parseLevel(raw string) (Level, bool) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	lvl, ok := levelNames[normalized]
	return lvl, ok
}

func (lvl Level) String() string {
	if s, ok := levelStrings[lvl]; ok {
		return s
	}
	return fmt.Sprintf("level-%d", lvl)
}

// Enabled 判断请求的 level 是否应输出：当前 level >= lvl 时 true
// 语义：High 输出 High+Mid+Low；Mid 输出 Mid+Low；Low 只输出 Low；Off 全部不输出
func Enabled(lvl Level) bool {
	if GetLevel() == LevelOff || lvl == LevelOff {
		return false
	}
	return GetLevel() >= lvl
}
