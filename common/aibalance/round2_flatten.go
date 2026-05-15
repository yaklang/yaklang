package aibalance

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils"
)

// round2 ReAct flatten 兼容兜底:
// 当客户端按 OpenAI tool_calls 标准协议发起 round2 (即 messages 数组里出现
// `assistant.tool_calls` 或 `role=tool` 消息) 时, 部分上游 wrapper 不识别
// OpenAI tool_calls 协议字段, 收到后会立即 finish_reason=stop 给出空响应,
// 让 OpenAI 兼容客户端 (OpenAI Python/Node SDK / Codex / OpenCode / litellm
// 等) 在 round2 拿不到模型的自然语言总结。
//
// 修复: 在 aibalance 中转层引入一个**纯函数 + opt-in 开关**的 flatten 兜底,
// 当 (provider/model) 被标记为「上游 wrapper 不支持 tool_calls round-trip」时,
// 把客户端发来的 round2 messages 自动改写为 ReAct 文本风格 (assistant 消息
// 用文本描述工具调用, role=tool 改成 role=user 文本回填工具结果), 让上游
// wrapper 仍能基于纯文本对话历史给出正确的 NL 响应。
//
// 设计原则:
//   1. **零侵入**: 不改 ChatDetail 结构, 不改 RawMessages 透传链路, 仅在
//      server.go messagesForUpstream 计算前注入一次纯函数转换。
//   2. **opt-in**: 默认不启用, 只对 portal 配置 / env 白名单声明的
//      "已知 wrapper 不支持 tool_calls round-trip" 的 provider/model 启用,
//      不影响其它 provider 的原生 tool_calls 协议体验。
//   3. **触发条件最小化**: 仅当 messages 数组里**真的**出现 OpenAI tool_calls
//      round-trip 标记 (assistant.tool_calls 非空 或 role=tool) 时才 flatten,
//      纯文本对话或 round1 调用工具完全不受影响。
//
// 关键词: round2 ReAct flatten, OpenAI tool_calls round-trip 兼容兜底,
//        z-deepseek-v4-pro round2 空响应修复, traditional client compatibility

const (
	// envFlattenToolCallsForModels: 逗号/分号分隔的 model 名/wrapper 名白名单,
	// 命中即对该 model 的 round2 messages 自动启用 ReAct flatten。
	// 关键词: AIBALANCE_FLATTEN_TOOLCALLS_FOR_MODELS env 配置
	envFlattenToolCallsForModels = "AIBALANCE_FLATTEN_TOOLCALLS_FOR_MODELS"

	// envFlattenToolCallsAll: 设为 "1"/"true"/"on"/"yes" 时对**所有** provider
	// 的 round2 messages 启用 ReAct flatten (作为紧急 kill-switch, 谨慎使用)。
	// 关键词: AIBALANCE_FLATTEN_TOOLCALLS_ALL kill switch
	envFlattenToolCallsAll = "AIBALANCE_FLATTEN_TOOLCALLS_ALL"
)

var (
	// flattenModelSetCache 缓存 env 解析结果, 避免每次请求都解析字符串。
	// 关键词: flattenModelSetCache, env 一次性解析
	flattenModelSetCache     map[string]bool
	flattenModelSetCacheOnce sync.Once
	flattenModelSetCacheMu   sync.RWMutex

	flattenAllCache     bool
	flattenAllCacheOnce sync.Once
)

// resetFlattenEnvCacheForTest 让单元测试能在重新设 env 后清缓存。
// 仅供 _test.go 文件调用。
// 关键词: round2 flatten env 缓存重置, 测试专用
func resetFlattenEnvCacheForTest() {
	flattenModelSetCacheMu.Lock()
	flattenModelSetCache = nil
	flattenModelSetCacheOnce = sync.Once{}
	flattenAllCacheOnce = sync.Once{}
	flattenAllCache = false
	flattenModelSetCacheMu.Unlock()
}

// loadFlattenModelSetFromEnv 解析 envFlattenToolCallsForModels, 返回小写
// model/wrapper 名集合; 不命中返回空 map。
// 关键词: 环境变量解析, 大小写不敏感
func loadFlattenModelSetFromEnv() map[string]bool {
	flattenModelSetCacheOnce.Do(func() {
		raw := strings.TrimSpace(os.Getenv(envFlattenToolCallsForModels))
		set := map[string]bool{}
		if raw == "" {
			flattenModelSetCacheMu.Lock()
			flattenModelSetCache = set
			flattenModelSetCacheMu.Unlock()
			return
		}
		for _, item := range splitFlattenList(raw) {
			item = strings.ToLower(strings.TrimSpace(item))
			if item != "" {
				set[item] = true
			}
		}
		flattenModelSetCacheMu.Lock()
		flattenModelSetCache = set
		flattenModelSetCacheMu.Unlock()
	})
	flattenModelSetCacheMu.RLock()
	defer flattenModelSetCacheMu.RUnlock()
	return flattenModelSetCache
}

// splitFlattenList 把 "a,b;c\n d" 这种多分隔符字符串拆成 ["a","b","c","d"],
// 容忍逗号 / 分号 / 空白作为分隔符, 方便运维 env 配置。
// 关键词: env 多分隔符解析
func splitFlattenList(s string) []string {
	repl := strings.NewReplacer(",", "\n", ";", "\n", "\t", "\n", " ", "\n")
	return strings.Split(repl.Replace(s), "\n")
}

// loadFlattenAllFromEnv 解析 envFlattenToolCallsAll, 返回 true 表示
// 对所有 provider 启用 round2 flatten (kill-switch, 默认关闭)。
// 关键词: 全局 flatten 开关
func loadFlattenAllFromEnv() bool {
	flattenAllCacheOnce.Do(func() {
		raw := strings.ToLower(strings.TrimSpace(os.Getenv(envFlattenToolCallsAll)))
		switch raw {
		case "1", "true", "on", "yes", "enable", "enabled":
			flattenAllCache = true
		default:
			flattenAllCache = false
		}
	})
	return flattenAllCache
}

// ResolveFlattenForModel 根据 modelName / wrapperName / provider 配置判断
// 是否对该请求执行 round2 ReAct flatten。
//
// 触发顺序 (任一命中即返回 true):
//   1. envFlattenToolCallsAll 全局开关 = true
//   2. provider.FlattenToolCalls 显式 opt-in (portal/yaml 后续可加该字段)
//   3. envFlattenToolCallsForModels 白名单包含 modelName 或 wrapperName
//      (大小写不敏感)
//
// 关键词: ResolveFlattenForModel, round2 flatten 触发条件, opt-in 优先
func ResolveFlattenForModel(modelName, wrapperName string) bool {
	if loadFlattenAllFromEnv() {
		return true
	}
	set := loadFlattenModelSetFromEnv()
	if len(set) == 0 {
		return false
	}
	if modelName != "" && set[strings.ToLower(strings.TrimSpace(modelName))] {
		return true
	}
	if wrapperName != "" && set[strings.ToLower(strings.TrimSpace(wrapperName))] {
		return true
	}
	return false
}

// IsRoundTripFlattenEligible 判断 messages 是否包含 OpenAI tool_calls
// round-trip 标记 (assistant.tool_calls 非空 或 role=tool 消息), 仅在命中
// 时才需要执行 flatten; 否则 messages 原样透传。
// 关键词: IsRoundTripFlattenEligible, round2 标记检测
func IsRoundTripFlattenEligible(msgs []aispec.ChatDetail) bool {
	for _, m := range msgs {
		if strings.EqualFold(strings.TrimSpace(m.Role), "tool") {
			return true
		}
		if len(m.ToolCalls) > 0 {
			return true
		}
	}
	return false
}

// FlattenToolCallsForRoundTrip 把 messages 中的 OpenAI tool_calls round-trip
// 字段扁平化为 ReAct 文本风格:
//   - assistant.tool_calls 非空: 把 tool_calls 渲染为
//     `[tool_call]name(arguments)[/tool_call]` 文本拼到 content 末尾,
//     然后清空 tool_calls 字段。原 content (string 或 []ChatContent) 保留。
//   - role=tool: 改成 role=user, content 改为
//     `[tool_result tool_call_id=...]...原 content...[/tool_result]`
//     (Name 保留, 让上游模型仍能从 user 消息文本里识别工具名)。
//
// 输出 messages 的元素是新建的 ChatDetail 副本, 不改入参。
// 关键词: FlattenToolCallsForRoundTrip, ReAct 文本化, tool_calls 扁平化
func FlattenToolCallsForRoundTrip(msgs []aispec.ChatDetail) []aispec.ChatDetail {
	if len(msgs) == 0 {
		return msgs
	}
	out := make([]aispec.ChatDetail, 0, len(msgs))
	for _, m := range msgs {
		switch {
		case strings.EqualFold(strings.TrimSpace(m.Role), "tool"):
			out = append(out, flattenToolMessage(m))
		case len(m.ToolCalls) > 0:
			out = append(out, flattenAssistantWithToolCalls(m))
		default:
			out = append(out, cloneChatDetail(m))
		}
	}
	return out
}

// flattenToolMessage role=tool -> role=user, content 渲染成 ReAct 风格文本,
// 保留 tool_call_id 与 name 让上游模型能在文本里看到工具关联信息。
// 关键词: role=tool flatten -> role=user, ReAct 文本格式
func flattenToolMessage(m aispec.ChatDetail) aispec.ChatDetail {
	contentStr := chatContentToPlainText(m.Content)
	name := strings.TrimSpace(m.Name)
	tcid := strings.TrimSpace(m.ToolCallID)
	var b strings.Builder
	b.WriteString("[tool_result")
	if name != "" {
		b.WriteString(fmt.Sprintf(" name=%q", name))
	}
	if tcid != "" {
		b.WriteString(fmt.Sprintf(" tool_call_id=%q", tcid))
	}
	b.WriteString("]\n")
	b.WriteString(contentStr)
	if !strings.HasSuffix(contentStr, "\n") {
		b.WriteString("\n")
	}
	b.WriteString("[/tool_result]")
	return aispec.ChatDetail{
		Role:    "user",
		Content: b.String(),
	}
}

// flattenAssistantWithToolCalls assistant.tool_calls -> assistant.content +
// 文本化的工具调用描述, 同时清空 tool_calls 字段避免上游 wrapper 因
// 不识别 tool_calls 字段而立即 finish_reason=stop。
// 关键词: assistant.tool_calls flatten, 文本化工具调用描述
func flattenAssistantWithToolCalls(m aispec.ChatDetail) aispec.ChatDetail {
	contentStr := chatContentToPlainText(m.Content)
	var b strings.Builder
	if contentStr != "" {
		b.WriteString(contentStr)
		if !strings.HasSuffix(contentStr, "\n") {
			b.WriteString("\n")
		}
	}
	for _, tc := range m.ToolCalls {
		if tc == nil {
			continue
		}
		name := strings.TrimSpace(tc.Function.Name)
		args := strings.TrimSpace(tc.Function.Arguments)
		id := strings.TrimSpace(tc.ID)
		b.WriteString("[tool_call")
		if id != "" {
			b.WriteString(fmt.Sprintf(" id=%q", id))
		}
		if name != "" {
			b.WriteString(fmt.Sprintf(" name=%q", name))
		}
		b.WriteString("]\n")
		if args != "" {
			b.WriteString(args)
			if !strings.HasSuffix(args, "\n") {
				b.WriteString("\n")
			}
		}
		b.WriteString("[/tool_call]\n")
	}
	role := strings.TrimSpace(m.Role)
	if role == "" {
		role = "assistant"
	}
	return aispec.ChatDetail{
		Role:    role,
		Name:    m.Name,
		Content: strings.TrimRight(b.String(), "\n"),
	}
}

// cloneChatDetail 浅拷贝 ChatDetail, 让 flatten 输出与入参完全独立,
// 调用方可以放心继续操作原 slice。content 是 any, 也按引用拷贝;
// 因为 flatten 不会原地改 content, 共享引用是安全的。
// 关键词: ChatDetail 浅拷贝
func cloneChatDetail(m aispec.ChatDetail) aispec.ChatDetail {
	return aispec.ChatDetail{
		Role:         m.Role,
		Name:         m.Name,
		Content:      m.Content,
		ToolCalls:    m.ToolCalls,
		ToolCallID:   m.ToolCallID,
		FunctionCall: m.FunctionCall,
	}
}

// chatContentToPlainText 把 ChatDetail.Content (string / []*ChatContent /
// 其他) 渲染成纯文本, 用于 ReAct flatten 消息的 content 字段。
// 关键词: ChatDetail.Content -> plain text 渲染
func chatContentToPlainText(content any) string {
	if content == nil {
		return ""
	}
	switch v := content.(type) {
	case string:
		return v
	case []*aispec.ChatContent:
		var b strings.Builder
		for i, c := range v {
			if c == nil {
				continue
			}
			if i > 0 {
				b.WriteString("\n")
			}
			switch c.Type {
			case "text":
				b.WriteString(c.Text)
			case "image_url":
				if url, ok := mapStringFromImageOrVideo(c.ImageUrl); ok && url != "" {
					b.WriteString(fmt.Sprintf("[image_url]%s[/image_url]", url))
				}
			case "video_url":
				if url, ok := mapStringFromImageOrVideo(c.VideoUrl); ok && url != "" {
					b.WriteString(fmt.Sprintf("[video_url]%s[/video_url]", url))
				}
			default:
				if c.Text != "" {
					b.WriteString(c.Text)
				}
			}
		}
		return b.String()
	case []any:
		var b strings.Builder
		for i, item := range v {
			if i > 0 {
				b.WriteString("\n")
			}
			b.WriteString(anyContentItemToString(item))
		}
		return b.String()
	default:
		raw, err := json.Marshal(content)
		if err == nil {
			return string(raw)
		}
		return utils.InterfaceToString(content)
	}
}

// mapStringFromImageOrVideo 从 ChatContent.ImageUrl/VideoUrl (any) 提取 url
// 字段; 接受字符串或 {"url": "..."} 的 map 形态。
// 关键词: image_url/video_url URL 提取
func mapStringFromImageOrVideo(v any) (string, bool) {
	if v == nil {
		return "", false
	}
	switch x := v.(type) {
	case string:
		return x, true
	case map[string]any:
		if u, ok := x["url"].(string); ok {
			return u, true
		}
	case map[string]string:
		if u, ok := x["url"]; ok {
			return u, true
		}
	}
	return "", false
}

// anyContentItemToString 把 messages 数组里 content 的单项 (通常 OpenAI
// 风格 multimodal element {"type":"text","text":...} 或 {"type":"image_url",
// "image_url":{"url":...}}) 渲染成纯文本; 其它形态走 fallback JSON 序列化。
// 关键词: any content item -> text
func anyContentItemToString(item any) string {
	if item == nil {
		return ""
	}
	if s, ok := item.(string); ok {
		return s
	}
	if m, ok := item.(map[string]any); ok {
		t := utils.MapGetString(m, "type")
		switch t {
		case "text":
			return utils.MapGetString(m, "text")
		case "image_url":
			if url := utils.MapGetString(utils.MapGetMapRaw(m, "image_url"), "url"); url != "" {
				return fmt.Sprintf("[image_url]%s[/image_url]", url)
			}
		case "video_url":
			if url := utils.MapGetString(utils.MapGetMapRaw(m, "video_url"), "url"); url != "" {
				return fmt.Sprintf("[video_url]%s[/video_url]", url)
			}
		}
	}
	raw, err := json.Marshal(item)
	if err == nil {
		return string(raw)
	}
	return utils.InterfaceToString(item)
}
