package aibalance

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aispec"
)

// round1_react_inject.go 实现 aibalance round1 的 ReAct 兼容降级:
// 客户端发 tools=[...] 但上游 wrapper 不识别 OpenAI tool_calls 协议时,
// aibalance 把 tools 描述渲染成统一文本块追加到末位 system 消息,
// 让上游用纯文本 ReAct 风格响应; 响应侧由 react_tool_extractor.go
// 把 [tool_call name=...]args[/tool_call] 文本反解析回 OpenAI tool_calls 结构.
//
// 关键词: aibalance round1 react inject, ToolsAsReactPrompt, OpenAI tool_calls 兼容降级
//
// 设计契约 (与 FlattenToolCallsForRoundTrip 对齐):
//  1. 纯函数, 不修改入参 messages / tools.
//  2. 末位是 system 消息时, 把 prompt 追加到该 system content; 否则在数组开头插入一条 system.
//  3. tools 不论数量, 渲染为人类可读 + 模型可遵循的统一格式.
//  4. 文本格式必须与 react_tool_extractor.go 的解析格式 / FlattenToolCallsForRoundTrip 一致,
//     保证整条 round-trip (round1 react -> round2 react) 协议自洽.

// ReactToolCallTag = "[tool_call"        // 反解析时的开括号匹配
// ReactToolCallTagClose = "[/tool_call]" // 反解析时的闭括号匹配
//
// 兼容性补丁: 部分上游模型 (如 deepseek-v4-pro 在 thinking + react 模式下)
// 会自由发挥, 把 tool_call 渲染成多种漂移格式, 实测见过的混合形态:
//   - 规范方括号:          [tool_call name=NAME]ARGS[/tool_call]
//   - 纯尖括号 (XML):       <tool_call name="NAME">ARGS</tool_call>
//   - 方括号 + > headerEnd: [tool_call id="..." name="bash">ARGS[/tool_call]
//   - 尖括号 + ] headerEnd: <tool_call id="..." name="bash"]ARGS</tool_call>
//   - 方/尖括号交叉 close:  [tool_call ...>ARGS</tool_call> 与 <tool_call ...]ARGS[/tool_call]
//
// 任何穷举式 variant 都不能预先知道全部组合, react_tool_extractor.go 改用
// "open / headerEnd / close 三段独立解析 + quote-aware header end" 来处理
// 上面所有混合. 新写出的 round1/round2 文本仍然只用方括号 (与历史协议一致),
// 这些常量仅在反解析侧作为候选 token 使用.
//
// v2 raw passthrough 扩展: 在原有方/尖括号基础上, 再支持 3 类业界主流漂移形态,
// args body 不再解析, 只做 name 提取 + 原文塞 ToolCall.Function.Arguments. 详见
// react_tool_extractor.go extractNameAndArgsRaw / drainLocked 特例路径.
//
//   - chinese-invoke:  [调用 NAME] ... [/tool_call]
//   - deepseek-fullwidth: <｜tool_calls_begin｜> ... <｜tool_calls_end｜>  外层包裹多个
//                         <｜tool_call_begin｜>NAME<｜tool_sep｜>ARGS<｜tool_call_end｜> 子帧
//   - mistral-toolcalls: [TOOL_CALLS] [{"name":"X","arguments":{...}}, ...]
//                        JSON 数组自闭合, brace-balanced 扫描, 不走通用 close 候选
//
// 关键词: react tool_call 多格式漂移兼容, deepseek thinking hallucinate,
//
//	opencode TUI 失败修复, 三段独立解析, raw passthrough variants
const (
	// 反解析侧 open token 候选. 写出仍只用 reactToolCallOpen.
	reactToolCallOpen           = "[tool_call"
	reactToolCallOpenAngle      = "<tool_call"
	reactToolCallOpenChinese    = "[调用"               // chinese-invoke open, 中文动词 hallucinate
	reactToolCallOpenDeepseekFW = "<｜tool_calls_begin｜>" // deepseek V3.1 全角外层包裹 open (复数 calls)
	reactToolCallOpenMistral    = "[TOOL_CALLS]"        // mistral 数组 open, JSON 数组自闭合

	// 反解析侧 close token 候选. 写出仍只用 reactToolCallClose.
	reactToolCallClose           = "[/tool_call]"
	reactToolCallCloseAngle      = "</tool_call>"
	reactToolCallCloseMixedBA    = "[/tool_call>"          // 方括号开 + 尖括号闭 hallucinate
	reactToolCallCloseMixedAB    = "</tool_call]"          // 尖括号开 + 方括号闭 hallucinate
	reactToolCallCloseDeepseekFW = "<｜tool_calls_end｜>" // deepseek V3.1 全角外层 close, 仅在 deepseek 特例路径用, 不进通用候选

	// deepseek V3.1 内部子帧分隔符, 仅在 extractDeepseekFullwidthAll 内部使用.
	reactDeepseekSubFrameBegin = "<｜tool_call_begin｜>" // 单数 call, 子帧开始
	reactDeepseekSubFrameSep   = "<｜tool_sep｜>"        // name 与 args 分隔
	reactDeepseekSubFrameEnd   = "<｜tool_call_end｜>"   // 单数 call, 子帧结束

	reactToolResultOpen = "[tool_result"
	reactToolResultEnd  = "[/tool_result]"
)

// reactSystemPromptHeader 是工具调用说明的固定头部.
// 模型必须严格遵循该格式输出 [tool_call name=NAME]ARG_JSON[/tool_call] 才能被反解析识别.
//
// v2 raw passthrough 在末尾追加 negative example 段, 显式禁止 5 类业界主流漂移格式.
// 即便模型忽略禁令仍然漂移, react_tool_extractor.go 也会按 raw passthrough 路径兜底.
// 这一段是 "预防 > 治疗" 的预防层, 降低漂移概率.
//
// 关键词: reactSystemPromptHeader, negative example, hallucinate 预防,
//
//	anthropic xml param / chinese invoke / hermes body / mistral / deepseek fullwidth 禁令
const reactSystemPromptHeader = `You have access to the following tools. When you decide to call a tool, output exactly this format (and nothing else):

[tool_call name=TOOL_NAME]JSON_ARGUMENTS[/tool_call]

Rules:
  - TOOL_NAME must be one of the tools listed below.
  - JSON_ARGUMENTS must be valid JSON matching the tool's parameter schema (use {} for empty arguments).
  - Output [/tool_call] immediately after the JSON. Do not add any explanation, prefix, or suffix.
  - If a tool result is provided to you in the format [tool_result tool_call_id=ID]...[/tool_result], read it and answer the user in plain text.
  - You may call multiple tools in parallel by emitting multiple [tool_call ...]...[/tool_call] blocks back-to-back.

DO NOT use any of the following alternative formats. They will be parsed as best-effort raw passthrough but degrade tool execution reliability:
  - <tool_call name="X"><parameter name="K">V</parameter></tool_call>                              (Anthropic XML parameter nesting)
  - [调用 NAME] {...} [/tool_call]                                                                  (Chinese verb invoke)
  - <tool_call>{"name":"X","arguments":{...}}</tool_call>                                           (Hermes / Qwen body-name)
  - [TOOL_CALLS] [{"name":"X","arguments":{...}}, ...]                                              (Mistral array)
  - <｜tool_calls_begin｜><｜tool_call_begin｜>X<｜tool_sep｜>{...}<｜tool_call_end｜><｜tool_calls_end｜>  (DeepSeek V3.1 fullwidth)

Use ONLY: [tool_call name=NAME]JSON_ARGUMENTS[/tool_call]

Available tools:
`

// ToolsAsReactPrompt 把 tools 数组渲染成 ReAct 风格的描述文本块, 用于追加到 system prompt.
// 关键词: ToolsAsReactPrompt, react tool description rendering
func ToolsAsReactPrompt(tools []aispec.Tool) string {
	if len(tools) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(reactSystemPromptHeader)
	for _, t := range tools {
		if t.Type != "" && t.Type != "function" {
			continue
		}
		name := strings.TrimSpace(t.Function.Name)
		if name == "" {
			continue
		}
		sb.WriteString("\n- name: ")
		sb.WriteString(name)
		if d := strings.TrimSpace(t.Function.Description); d != "" {
			sb.WriteString("\n  description: ")
			sb.WriteString(d)
		}
		if t.Function.Parameters != nil {
			paramsJSON, err := json.Marshal(t.Function.Parameters)
			if err == nil && len(paramsJSON) > 0 {
				sb.WriteString("\n  parameters: ")
				sb.Write(paramsJSON)
			}
		}
	}
	return sb.String()
}

// InjectToolsAsReactPrompt 返回一个新的 messages 切片, 末位 system 消息被追加了 tools ReAct 描述.
// 如果原 messages 没有 system 消息, 则在头部插入一条 system 消息. 入参 messages 不被修改.
//
// 关键词: InjectToolsAsReactPrompt, round1 react request rewrite
func InjectToolsAsReactPrompt(messages []aispec.ChatDetail, tools []aispec.Tool) []aispec.ChatDetail {
	if len(tools) == 0 {
		return messages
	}
	prompt := ToolsAsReactPrompt(tools)
	if prompt == "" {
		return messages
	}

	out := make([]aispec.ChatDetail, len(messages))
	copy(out, messages)

	// 找最后一个 role=system 消息的位置 (从前往后)
	lastSystemIdx := -1
	for i, m := range out {
		if strings.EqualFold(strings.TrimSpace(m.Role), "system") {
			lastSystemIdx = i
		}
	}

	if lastSystemIdx >= 0 {
		// 追加到末位 system 的 content
		origText := chatContentToPlainText(out[lastSystemIdx].Content)
		merged := strings.TrimRight(origText, " \n\t") + "\n\n" + prompt
		newMsg := cloneChatDetail(out[lastSystemIdx])
		newMsg.Content = merged
		out[lastSystemIdx] = newMsg
		return out
	}

	// 没有 system 消息, 在头部插入一条
	sys := aispec.ChatDetail{
		Role:    "system",
		Content: prompt,
	}
	merged := make([]aispec.ChatDetail, 0, len(out)+1)
	merged = append(merged, sys)
	merged = append(merged, out...)
	return merged
}

// FormatReactToolCallText 把单个 tool call 渲染成 [tool_call name=NAME]ARGS_JSON[/tool_call].
// 供 round2 flatten / 测试 / 内部一致性使用.
// 关键词: FormatReactToolCallText, react tool_call serialization
func FormatReactToolCallText(name, argsJSON string) string {
	if argsJSON == "" {
		argsJSON = "{}"
	}
	return fmt.Sprintf("%s name=%s]%s%s", reactToolCallOpen, name, argsJSON, reactToolCallClose)
}

// FormatReactToolResultText 把单个 tool result 渲染成 [tool_result tool_call_id=ID]CONTENT[/tool_result].
// 关键词: FormatReactToolResultText, react tool_result serialization
func FormatReactToolResultText(toolCallID, content string) string {
	if content == "" {
		content = "{}"
	}
	return fmt.Sprintf("%s tool_call_id=%s]%s%s", reactToolResultOpen, toolCallID, content, reactToolResultEnd)
}
