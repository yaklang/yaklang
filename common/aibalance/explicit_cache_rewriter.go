package aibalance

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aispec"
)

// 本文件实现 dashscope (tongyi) 「显式上下文缓存(explicit context cache)」自动注入。
//
// 背景:
//   dashscope 把上下文缓存分成两种模式 ——「隐式缓存」与「显式缓存」。
//   隐式缓存对调用方完全透明, 系统按 prefix 自动匹配。但 dashscope
//   把「能否走隐式缓存」按 model 维度白名单管控: qwen-max / qwen-plus /
//   qwen-flash / qwen-turbo / qwen3-coder-* / deepseek-* / kimi-* 等支持;
//   而 qwen3.5-flash / qwen3.6-plus / qwen3.5-plus / qwen3.6-flash /
//   qwen3-vl-plus / qwen3-vl-flash 等新一代模型「不在隐式缓存白名单」,
//   必须由调用方在 messages 数组里显式插入 cache_control:{"type":"ephemeral"}
//   标记, 才能享受缓存命中折扣 (input_token 单价 10%)。
//
// 实测验证:
//   - 不加 cache_control 时, 上述 qwen3.x 系列对同一份 4030 tokens
//     system prompt 连续 6 次请求, cached_tokens 始终为 0;
//   - 加上 cache_control:{"type":"ephemeral"} 后, 第 1 次创建缓存,
//     第 2 次起 cached_tokens=4012 (命中率 99.5%)。
//
// 设计原则:
//   1. 严格 by provider type + model 白名单管控, 仅 type==tongyi 且
//      model 在 dashscope 文档官方公布的「显式缓存支持模型」列表中
//      时才注入, 其它一切场景 pass-through。
//   2. 永远不修改入参 messages, 必要时返回浅复制后的新切片, 旧调用方
//      继续持有未被污染的原始引用。
//   3. 仅改写「最末一条 role=system 的消息」的 content 字段, 不改写
//      user / assistant / tool 消息(那些场景需要更复杂的多标记策略,
//      暂不在本期范围)。
//   4. 兼容 system content 的 string / []*aispec.ChatContent /
//      []map[string]any / []any 四种常见形态, 找不到可识别形态时
//      pass-through 不报错。
//
// 关键词: dashscope explicit cache, cache_control 自动注入,
//        ephemeral cache_control, tongyi 显式缓存白名单,
//        RewriteMessagesForExplicitCache

// dashscopeExplicitCacheModels 来自 dashscope 上下文缓存官方文档(中国内地)
// 「显式缓存」-「支持的模型」一节, 严格按 model 名小写比对。
// 关键词: dashscope explicit cache 白名单, qwen3.x 系列必须显式缓存
var dashscopeExplicitCacheModels = map[string]struct{}{
	// 千问 Max
	"qwen3.6-max-preview": {},
	"qwen3-max":           {},
	// 千问 Plus
	"qwen3.6-plus":            {},
	"qwen3.5-plus":            {},
	"qwen3.5-plus-2026-04-20": {},
	"qwen-plus":               {},
	// 千问 Flash
	"qwen3.6-flash": {},
	"qwen3.5-flash": {},
	"qwen-flash":    {},
	// 千问 Coder
	"qwen3-coder-plus":  {},
	"qwen3-coder-flash": {},
	// 千问 VL
	"qwen3-vl-plus":  {},
	"qwen3-vl-flash": {},
	// DeepSeek
	"deepseek-v3.2": {},
	// Kimi
	"kimi-k2.6": {},
	"kimi-k2.5": {},
	// GLM
	"glm-5.1": {},
}

// IsTongyiExplicitCacheModel 判断 (provider type, model) 组合是否符合
// dashscope (tongyi) 官方公布的「显式缓存可注入 cache_control 标记」名单。
// type 必须严格等于 "tongyi" (大小写不敏感), model 在白名单中。
// 仅当此函数返回 true 时, 调用方才应该向 messages 注入 cache_control。
//
// 关键词: dashscope explicit cache 白名单判断, tongyi 限定
func IsTongyiExplicitCacheModel(providerType, modelName string) bool {
	if !strings.EqualFold(strings.TrimSpace(providerType), "tongyi") {
		return false
	}
	model := strings.ToLower(strings.TrimSpace(modelName))
	if model == "" {
		return false
	}
	_, ok := dashscopeExplicitCacheModels[model]
	return ok
}

// ephemeralCacheControl 是注入到 ChatContent.CacheControl 上的标准字面量,
// dashscope 显式缓存目前仅支持 type=ephemeral, 5 分钟 TTL。
// 用 map[string]any 而非 struct 是为了让 JSON 输出严格保持
// {"type":"ephemeral"} 形态, 避免任何 struct tag 带来的额外字段。
// 关键词: cache_control ephemeral 字面量
func ephemeralCacheControl() map[string]any {
	return map[string]any{"type": "ephemeral"}
}

// RewriteMessagesForExplicitCache 当 (providerType, modelName) 通过
// IsTongyiExplicitCacheModel 检查时, 对最末一条 role=system 消息的
// content 注入 cache_control:{"type":"ephemeral"} 标记, 让 dashscope
// 把 system prompt 作为命名缓存块缓存 5 分钟。
//
// 行为契约:
//   - 命中白名单 + 找到 system 消息 + content 形态可识别 -> 返回浅复制
//     的新切片, 仅最末 system 消息的 Content 被替换为带 cache_control
//     的新对象, 其它消息保留指针不变;
//   - 未命中白名单 / 没有 system 消息 / content 形态不可识别 ->
//     原样返回入参切片, 不做任何修改;
//   - 无论分支, 入参 messages 切片本身永不被原地修改(零副作用)。
//
// 兼容形态:
//   1. string                -> 包成 []*ChatContent{Type:"text",Text,CacheControl}
//   2. []*aispec.ChatContent -> 浅复制每项, 在最末 text 项加 CacheControl
//   3. []map[string]any      -> 浅复制每项 map, 在最末 type=text 项加 cache_control
//   4. []any                 -> 浅复制, 若末项是 map 则注入 cache_control, 否则 pass
//   5. 其它类型(nil/数字/struct 等) -> pass-through 不动
//
// 关键词: RewriteMessagesForExplicitCache, dashscope 显式缓存自动注入,
//        最末 system 改写, 浅复制零副作用
func RewriteMessagesForExplicitCache(messages []aispec.ChatDetail, providerType, modelName string) []aispec.ChatDetail {
	if len(messages) == 0 {
		return messages
	}
	if !IsTongyiExplicitCacheModel(providerType, modelName) {
		return messages
	}

	lastSysIdx := -1
	for i := len(messages) - 1; i >= 0; i-- {
		if strings.EqualFold(messages[i].Role, "system") {
			lastSysIdx = i
			break
		}
	}
	if lastSysIdx < 0 {
		return messages
	}

	sysMsg := messages[lastSysIdx]
	newContent, ok := injectCacheControlOnContent(sysMsg.Content)
	if !ok {
		return messages
	}

	out := make([]aispec.ChatDetail, len(messages))
	copy(out, messages)
	out[lastSysIdx] = aispec.ChatDetail{
		Role:         sysMsg.Role,
		Name:         sysMsg.Name,
		Content:      newContent,
		ToolCalls:    sysMsg.ToolCalls,
		ToolCallID:   sysMsg.ToolCallID,
		FunctionCall: sysMsg.FunctionCall,
	}
	return out
}

// injectCacheControlOnContent 在 message.Content 上注入 ephemeral cache_control。
// 返回 (newContent, true) 表示注入成功, (原值, false) 表示形态不可识别。
// 永远不修改入参里指向的对象。
// 关键词: injectCacheControlOnContent, content 形态多态适配
func injectCacheControlOnContent(content any) (any, bool) {
	switch v := content.(type) {
	case string:
		// 字符串 system content -> 包成 []*ChatContent 形式
		return []*aispec.ChatContent{
			{
				Type:         "text",
				Text:         v,
				CacheControl: ephemeralCacheControl(),
			},
		}, true

	case []*aispec.ChatContent:
		if len(v) == 0 {
			return content, false
		}
		// 浅复制每个元素, 在最末元素上加 CacheControl, 不修改原 slice / 原元素
		newSlice := make([]*aispec.ChatContent, len(v))
		for i, c := range v {
			if c == nil {
				newSlice[i] = nil
				continue
			}
			cp := *c
			newSlice[i] = &cp
		}
		// 选取最末一个非 nil 元素附加 cache_control
		for i := len(newSlice) - 1; i >= 0; i-- {
			if newSlice[i] == nil {
				continue
			}
			newSlice[i].CacheControl = ephemeralCacheControl()
			return newSlice, true
		}
		return content, false

	case []map[string]any:
		if len(v) == 0 {
			return content, false
		}
		newSlice := make([]map[string]any, len(v))
		for i, m := range v {
			cp := make(map[string]any, len(m)+1)
			for k, val := range m {
				cp[k] = val
			}
			newSlice[i] = cp
		}
		newSlice[len(newSlice)-1]["cache_control"] = ephemeralCacheControl()
		return newSlice, true

	case []any:
		if len(v) == 0 {
			return content, false
		}
		newSlice := make([]any, len(v))
		copy(newSlice, v)
		// 仅当末项是 map[string]any 时才能安全注入 cache_control
		if last, ok := newSlice[len(newSlice)-1].(map[string]any); ok {
			cp := make(map[string]any, len(last)+1)
			for k, val := range last {
				cp[k] = val
			}
			cp["cache_control"] = ephemeralCacheControl()
			newSlice[len(newSlice)-1] = cp
			return newSlice, true
		}
		return content, false

	default:
		return content, false
	}
}
