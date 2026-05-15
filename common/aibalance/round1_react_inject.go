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
const (
	reactToolCallOpen   = "[tool_call"
	reactToolCallClose  = "[/tool_call]"
	reactToolResultOpen = "[tool_result"
	reactToolResultEnd  = "[/tool_result]"
)

// reactSystemPromptHeader 是工具调用说明的固定头部.
// 模型必须严格遵循该格式输出 [tool_call name=NAME]ARG_JSON[/tool_call] 才能被反解析识别.
const reactSystemPromptHeader = `You have access to the following tools. When you decide to call a tool, output exactly this format (and nothing else):

[tool_call name=TOOL_NAME]JSON_ARGUMENTS[/tool_call]

Rules:
  - TOOL_NAME must be one of the tools listed below.
  - JSON_ARGUMENTS must be valid JSON matching the tool's parameter schema (use {} for empty arguments).
  - Output [/tool_call] immediately after the JSON. Do not add any explanation, prefix, or suffix.
  - If a tool result is provided to you in the format [tool_result tool_call_id=ID]...[/tool_result], read it and answer the user in plain text.
  - You may call multiple tools in parallel by emitting multiple [tool_call ...]...[/tool_call] blocks back-to-back.

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
