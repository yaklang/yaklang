package aibalance

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aispec"
)

// tool_calls_mode.go 是 aibalance 工具调用兼容性的唯一决策入口.
//
// 关键词: aibalance ResolveToolCallsMode, capability matrix resolver, env > DB > unknown
//
// 优先级 (从高到低):
//  1. env AIBALANCE_FLATTEN_TOOLCALLS_ALL=true             -> 强制 react (历史全局兜底, 完全保留)
//  2. env AIBALANCE_FLATTEN_TOOLCALLS_FOR_MODELS 命中       -> 强制 react (历史精细化兜底, 完全保留)
//  3. DB provider.ToolCallsRound1Mode / Round2Mode 非空    -> 使用 DB 值
//  4. DB 字段为空 (unknown)                                 -> 默认 native 透传 + 启用 AutoFallback flag
//
// AutoFallback 含义: server 路径在收到 finish_reason=stop + content="" 且消息含 tool 标记时,
// 自动用 react 模式做一次性 retry (不修改 DB), 并打 WARN 日志提示运维 "该 provider 该 probe 了".

// ToolCallsMode 是 ResolveToolCallsMode 的返回结果.
// 关键词: ToolCallsMode struct, Round1/Round2 mode, AutoFallback flag
type ToolCallsMode struct {
	// Round1 = "native" | "react"
	// "native": 客户端发 tools=[...] 时直接透传给上游
	// "react":  客户端发 tools=[...] 时, aibalance 在请求侧把 tools 描述追加到 system prompt,
	//          并在响应侧反解析 [tool_call name=...]args[/tool_call] 文本回 OpenAI tool_calls 结构
	Round1 string

	// Round2 = "native" | "react"
	// "native": 客户端发 assistant.tool_calls + role=tool 时直接透传给上游
	// "react":  客户端发 round-trip 消息时, aibalance 把它们扁平化成 ReAct 文本再透传
	Round2 string

	// AutoFallback = true 表示当 Round2 透传后上游空回 (finish_reason=stop + content=""),
	// 应立即用 react 模式做一次性 retry. 只在 mode 来源 = "unknown" (default native) 时为 true.
	AutoFallback bool

	// Source 描述决策来源 (env / db / default), 仅用于日志和监控.
	Source string
}

// ResolveToolCallsMode 综合 env / DB / 默认值, 给出当前 provider 的工具调用模式.
// 关键词: ResolveToolCallsMode, capability resolver entrypoint
func ResolveToolCallsMode(p *Provider, modelName string) ToolCallsMode {
	// 优先级 1: 全局 env kill switch
	if loadFlattenAllFromEnv() {
		return ToolCallsMode{
			Round1:       "react",
			Round2:       "react",
			AutoFallback: false,
			Source:       "env:all",
		}
	}

	// 优先级 2: 模型/wrapper 精细 env 白名单
	if hitFlattenModelEnv(modelName, p) {
		return ToolCallsMode{
			Round1:       "react",
			Round2:       "react",
			AutoFallback: false,
			Source:       "env:model",
		}
	}

	// 优先级 3: DB 字段
	round1Db, round2Db := readDbToolCallsMode(p)
	if round1Db != "" || round2Db != "" {
		mode := ToolCallsMode{
			Round1:       normalizeMode(round1Db),
			Round2:       normalizeMode(round2Db),
			AutoFallback: false,
			Source:       "db",
		}
		// DB 中可能只填了其中一个, 另一个保持默认 native
		if mode.Round1 == "" {
			mode.Round1 = "native"
		}
		if mode.Round2 == "" {
			mode.Round2 = "native"
		}
		return mode
	}

	// 优先级 4: 默认 unknown -> native + AutoFallback
	return ToolCallsMode{
		Round1:       "native",
		Round2:       "native",
		AutoFallback: true,
		Source:       "default",
	}
}

// normalizeMode 把 DB 中可能的脏数据归一化为 "native" / "react" 二选一.
// 关键词: normalizeMode, DB 字段健壮性
func normalizeMode(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	switch v {
	case "native", "passthrough", "direct":
		return "native"
	case "react", "react-text", "flatten":
		return "react"
	}
	return ""
}

// readDbToolCallsMode 从 Provider.DbProvider 安全读取 DB mode 字段.
// 关键词: readDbToolCallsMode, capability matrix DB read
func readDbToolCallsMode(p *Provider) (string, string) {
	if p == nil || p.DbProvider == nil {
		return "", ""
	}
	return p.DbProvider.ToolCallsRound1Mode, p.DbProvider.ToolCallsRound2Mode
}

// hitFlattenModelEnv 检查 modelName 或 provider 的 wrapper/type 是否命中
// env AIBALANCE_FLATTEN_TOOLCALLS_FOR_MODELS 白名单.
// 关键词: hitFlattenModelEnv, 历史 env 白名单兼容
func hitFlattenModelEnv(modelName string, p *Provider) bool {
	set := loadFlattenModelSetFromEnv()
	if len(set) == 0 {
		return false
	}
	if modelName != "" && set[strings.ToLower(strings.TrimSpace(modelName))] {
		return true
	}
	if p != nil {
		if p.WrapperName != "" && set[strings.ToLower(strings.TrimSpace(p.WrapperName))] {
			return true
		}
		if p.TypeName != "" && set[strings.ToLower(strings.TrimSpace(p.TypeName))] {
			return true
		}
	}
	return false
}

// MessagesHaveToolMarker 判断 messages 数组里是否含有 OpenAI tool_calls round-trip 标记
// (assistant.tool_calls 或 role=tool 消息).
// 关键词: MessagesHaveToolMarker, round-trip detector
//
// 与 IsRoundTripFlattenEligible 行为等价 (后者保留作为历史导出符号).
func MessagesHaveToolMarker(msgs []aispec.ChatDetail) bool {
	return IsRoundTripFlattenEligible(msgs)
}
