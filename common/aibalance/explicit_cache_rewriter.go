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
	if !IsCacheControlAwareProvider(providerType) {
		return false
	}
	model := strings.ToLower(strings.TrimSpace(modelName))
	if model == "" {
		return false
	}
	_, ok := dashscopeExplicitCacheModels[model]
	return ok
}

// IsCacheControlAwareProvider 判断给定 provider type 是否在「会识别并尊重
// cache_control 字段」的 provider 名单内。
//
// 当前仅 dashscope (tongyi) 兼容协议会处理 cache_control:
//   - 在显式缓存白名单内的 model -> 触发显式缓存命名块创建/命中
//   - 不在白名单内的 model -> dashscope 服务端 silently 忽略, 但接口仍接受
//
// 其它所有 provider (siliconflow / openai / openrouter / anthropic / azure /
// 等等) 均不应该收到 cache_control 字段:
//   - 部分 provider 的 OpenAI 兼容层会校验未知字段直接 400 报错
//   - 部分 provider 不报错但语义不符 (例如 anthropic 的 cache_control 与
//     dashscope 的语义虽然字段名相同但触发条件/计费模型完全不同, 把 dashscope
//     风格的 cc 透传到 anthropic 会产生意料外计费)
//   - 部分 provider 不识别字段直接 echo 进 logging, 在客户端 trace 里产生
//     可能误导调用方的额外信息
//
// 因此 aibalance 在路由到非 tongyi provider 时必须主动 strip 所有 cc 字段,
// 这是兼容性 + 安全性的硬约束, 无论 cc 来自客户端 SDK 还是来自 aicache
// hijacker 自管注入。
//
// 关键词: IsCacheControlAwareProvider, provider 白名单, tongyi 仅识别 cc,
//        cross-provider 安全, cc strip 触发条件
func IsCacheControlAwareProvider(providerType string) bool {
	return strings.EqualFold(strings.TrimSpace(providerType), "tongyi")
}

// ephemeralCacheControl 是注入到 ChatContent.CacheControl 上的标准字面量,
// dashscope 显式缓存目前仅支持 type=ephemeral, 5 分钟 TTL。
// 用 map[string]any 而非 struct 是为了让 JSON 输出严格保持
// {"type":"ephemeral"} 形态, 避免任何 struct tag 带来的额外字段。
// 关键词: cache_control ephemeral 字面量
func ephemeralCacheControl() map[string]any {
	return map[string]any{"type": "ephemeral"}
}

// RewriteMessagesForProvider 是 aibalance 路由层的统一 cc 处理主入口,
// 按 provider type 分发到两条路径:
//
//  1. **cache-control aware provider** (当前仅 tongyi):
//     走 RewriteMessagesForExplicitCache, 行为见其文档:
//     - 客户端任意位置自带 cc -> 完全 pass-through
//     - 否则给最末 system 消息注入单 cc 作 baseline 兜底 (仅显式缓存
//       白名单 model 才触发, 其他 model 也 pass-through)
//
//  2. **不识别 cc 的 provider** (siliconflow / openai / openrouter /
//     anthropic / azure / 等所有非 tongyi 的 provider):
//     走 StripCacheControlFromMessages 强制 strip 所有位置的 cc 字段:
//     - 兼容性: 部分 provider OpenAI 兼容层会因为 cache_control 未知字段
//       直接 400 报错
//     - 安全性: 避免 dashscope 风格 cc 被误透传到其它 provider 引发
//       意料外的计费/路由行为
//     这是硬约束, 无论 cc 来自客户端 SDK 还是来自 aicache hijacker 自管
//     注入, 都必须移除。
//
// 这个分发逻辑保证了:
//   - aicache hijacker 可以"无脑给 system+user1 打双 cc", 不需要知道
//     下游 provider 是不是 tongyi (跨 provider 安全由 aibalance 兜底)
//   - 调用方 SDK 也可以照搬 dashscope 文档的 cc 用法, 切到其他 provider
//     时 aibalance 会自动剥离, 不会泄漏到不识别的服务端
//
// 入参 messages 永不被原地修改; 走 strip 路径时若发现确实有 cc 才会做
// 浅复制+剥离, 没有 cc 时直接返回原切片 (零分配)。
//
// 关键词: RewriteMessagesForProvider, aibalance cc 路由层主入口,
//        provider-aware cc 处理, tongyi 保留 / 其他 strip,
//        跨 provider 安全
func RewriteMessagesForProvider(messages []aispec.ChatDetail, providerType, modelName string) []aispec.ChatDetail {
	if len(messages) == 0 {
		return messages
	}
	if IsCacheControlAwareProvider(providerType) {
		return RewriteMessagesForExplicitCache(messages, providerType, modelName)
	}
	return StripCacheControlFromMessages(messages)
}

// StripCacheControlFromMessages 移除 messages 中所有位置出现的 cache_control
// 字段, 用于把 messages 安全地路由到不识别 cc 的 provider (例如 siliconflow,
// openai, anthropic 等)。
//
// 行为契约:
//   - 入参 messages 永不被原地修改
//   - 没有任何 cc 时直接返回原切片 (零分配, 同切片头)
//   - 有 cc 时返回浅复制后的新切片, 仅被 strip 的 message.Content 是新对象,
//     其他 message 保留原指针
//
// 兼容形态 (与 contentHasCacheControl / injectCacheControlOnContent 对齐):
//  1. []*aispec.ChatContent: 浅复制每项, 把 CacheControl 置 nil
//  2. []map[string]any:      浅复制每项 map, 删除 "cache_control" 键
//  3. []any:                 浅复制, 若元素是 map 则删除 "cache_control" 键
//  4. 其它形态 (string / nil / 数字 / 自定义 struct):
//     不可能承载 cc, 原样返回 (与 contentHasCacheControl 的"不识别"规则对齐)
//
// 关键词: StripCacheControlFromMessages, cc strip, 跨 provider 安全,
//        浅复制零副作用
func StripCacheControlFromMessages(messages []aispec.ChatDetail) []aispec.ChatDetail {
	if len(messages) == 0 {
		return messages
	}
	if !messagesAlreadyHaveCacheControl(messages) {
		return messages
	}

	out := make([]aispec.ChatDetail, len(messages))
	copy(out, messages)
	for i := range out {
		if !contentHasCacheControl(out[i].Content) {
			continue
		}
		stripped, ok := stripCacheControlOnContent(out[i].Content)
		if !ok {
			continue
		}
		src := out[i]
		out[i] = aispec.ChatDetail{
			Role:         src.Role,
			Name:         src.Name,
			Content:      stripped,
			ToolCalls:    src.ToolCalls,
			ToolCallID:   src.ToolCallID,
			FunctionCall: src.FunctionCall,
		}
	}
	return out
}

// stripCacheControlOnContent 在 message.Content 上移除 cache_control 字段,
// 返回 (newContent, true) 表示成功剥离, (原值, false) 表示形态不可识别。
// 永远不修改入参里指向的对象。
//
// 与 injectCacheControlOnContent 对偶: inject 注入到 4 种形态, strip 也支持
// 同样 4 种形态。string 形态不可能承载 cc 故不进入此函数 (上层 contentHasCacheControl
// 已过滤)。
//
// 关键词: stripCacheControlOnContent, content 形态多态适配, cc strip
func stripCacheControlOnContent(content any) (any, bool) {
	switch v := content.(type) {
	case []*aispec.ChatContent:
		if len(v) == 0 {
			return content, false
		}
		newSlice := make([]*aispec.ChatContent, len(v))
		for i, c := range v {
			if c == nil {
				newSlice[i] = nil
				continue
			}
			cp := *c
			cp.CacheControl = nil
			newSlice[i] = &cp
		}
		return newSlice, true

	case []map[string]any:
		if len(v) == 0 {
			return content, false
		}
		newSlice := make([]map[string]any, len(v))
		for i, m := range v {
			cp := make(map[string]any, len(m))
			for k, val := range m {
				if k == "cache_control" {
					continue
				}
				cp[k] = val
			}
			newSlice[i] = cp
		}
		return newSlice, true

	case []any:
		if len(v) == 0 {
			return content, false
		}
		newSlice := make([]any, len(v))
		for i, item := range v {
			if m, ok := item.(map[string]any); ok {
				cp := make(map[string]any, len(m))
				for k, val := range m {
					if k == "cache_control" {
						continue
					}
					cp[k] = val
				}
				newSlice[i] = cp
				continue
			}
			newSlice[i] = item
		}
		return newSlice, true

	default:
		return content, false
	}
}

// RewriteMessagesForExplicitCache 当 (providerType, modelName) 通过
// IsTongyiExplicitCacheModel 检查时, 给最末一条 role=system 消息的
// content 注入 cache_control:{"type":"ephemeral"} 标记作为通用网关层
// 的 baseline 兜底, 让 dashscope 把 system prompt 作为命名缓存块缓存
// 5 分钟 (后续相同前缀请求按 input_token 单价 10% 计费)。
//
// 行为契约:
//   - 客户端 messages 任意位置已自带 cache_control 标记 (任何形态: 字段
//     `CacheControl` 非空 / map["cache_control"] 存在) -> 视为客户端自管
//     缓存策略 (例如 aicache hijacker 走 3 段路径时主动给 system+user1
//     双 cc), aibalance 完全 pass-through 不做任何修改, 直接返回原切片
//     (零浅复制零副作用);
//   - 未命中白名单 / 没有 system 消息 / content 形态不可识别 ->
//     原样返回入参切片, 不做任何修改;
//   - 命中白名单 + 找到 system 消息 + 客户端无自带 cc + content 形态可
//     识别 -> 返回浅复制的新切片, 仅最末 system 消息的 Content 被替换为
//     带 cache_control 的新对象, 其它消息保留指针不变;
//   - 无论分支, 入参 messages 切片本身永不被原地修改(零副作用)。
//
// 设计意图 (TONGYI_CACHE_REPORT.md §7.7.7 职责重排版):
//   - aibalance 是"通用网关层", 默认只给最末 system 注入单 cc 作 baseline;
//   - 高级缓存策略 (例如 aicache hijacker 的 system+user1 双 cc 命中) 由
//     应用层自管, aibalance 看到自带 cc 即完全退让, 不做任何重叠注入。
//   - 这避免了"双注入"风险, 也尊重了所有外部 SDK 客户端已经自管 cc 的场景。
//
// 兼容形态(同 injectCacheControlOnContent):
//   1. string                -> 包成 []*ChatContent{Type:"text",Text,CacheControl}
//   2. []*aispec.ChatContent -> 浅复制每项, 在最末 text 项加 CacheControl
//   3. []map[string]any      -> 浅复制每项 map, 在最末 type=text 项加 cache_control
//   4. []any                 -> 浅复制, 若末项是 map 则注入 cache_control, 否则 pass
//   5. 其它类型(nil/数字/struct 等) -> pass-through 不动
//
// 关键词: RewriteMessagesForExplicitCache, dashscope 显式缓存兜底注入,
//        最末 system 单 cc, 客户端自带 cc 退让, §7.7.7 职责重排,
//        浅复制零副作用
func RewriteMessagesForExplicitCache(messages []aispec.ChatDetail, providerType, modelName string) []aispec.ChatDetail {
	if len(messages) == 0 {
		return messages
	}
	if !IsTongyiExplicitCacheModel(providerType, modelName) {
		return messages
	}

	// 客户端自带 cc 退让: 任何位置出现 cache_control 标记都视为客户端
	// 自管缓存策略, 整体 pass-through 不做任何修改 (零浅复制)。
	if messagesAlreadyHaveCacheControl(messages) {
		return messages
	}

	lastSys := pickLastSystemIndex(messages)
	if lastSys < 0 {
		return messages
	}

	sysMsg := messages[lastSys]
	newContent, ok := injectCacheControlOnContent(sysMsg.Content)
	if !ok {
		return messages
	}

	out := make([]aispec.ChatDetail, len(messages))
	copy(out, messages)
	out[lastSys] = aispec.ChatDetail{
		Role:         sysMsg.Role,
		Name:         sysMsg.Name,
		Content:      newContent,
		ToolCalls:    sysMsg.ToolCalls,
		ToolCallID:   sysMsg.ToolCallID,
		FunctionCall: sysMsg.FunctionCall,
	}
	return out
}

// pickLastSystemIndex 返回 messages 中最末一条 role=system 消息的索引,
// 不存在则返回 -1。
// 关键词: pickLastSystemIndex, 最末 system 索引
func pickLastSystemIndex(messages []aispec.ChatDetail) int {
	for i := len(messages) - 1; i >= 0; i-- {
		if strings.EqualFold(messages[i].Role, "system") {
			return i
		}
	}
	return -1
}

// messagesAlreadyHaveCacheControl 检测 messages 数组中任意位置是否已经
// 带有 cache_control 标记。客户端 (含 aicache hijacker 主动管理 cc 的
// 场景, 也含外部 SDK 用户自带 cc 的场景) 在任何 message 的任何 content
// 位置上挂了 cache_control, 都被视为"客户端自管缓存策略", aibalance
// 应当完全退让不做任何注入。
//
// 兼容 4 种 content 形态, 任一命中即返回 true:
//   1. []*aispec.ChatContent: 任一非 nil 元素的 CacheControl != nil
//   2. []map[string]any:      任一 map 含 "cache_control" 键且值非 nil
//   3. []any:                 任一元素若是 map[string]any 含 "cache_control" 键
//   4. 其它形态 (string / nil / 数字 / 自定义 struct):
//      字符串无法承载 cc, 直接当作"无 cc"; 自定义 struct 走 reflect 风险大
//      暂不识别 (这与 injectCacheControlOnContent 的"其它类型 pass-through"
//      契约对齐, 不会出现 inject 不识别但 detect 识别的内部矛盾)
//
// 关键词: messagesAlreadyHaveCacheControl, 客户端自带 cc 检测,
//        cache_control 任意位置识别, 4 种 content 形态兼容
func messagesAlreadyHaveCacheControl(messages []aispec.ChatDetail) bool {
	for _, msg := range messages {
		if contentHasCacheControl(msg.Content) {
			return true
		}
	}
	return false
}

// contentHasCacheControl 检测单个 message.Content 内是否任意位置带有
// cache_control 标记。识别规则同 messagesAlreadyHaveCacheControl 文档。
// 关键词: contentHasCacheControl, 单 content cc 检测
func contentHasCacheControl(content any) bool {
	switch v := content.(type) {
	case []*aispec.ChatContent:
		for _, c := range v {
			if c != nil && c.CacheControl != nil {
				return true
			}
		}
	case []map[string]any:
		for _, m := range v {
			if cc, ok := m["cache_control"]; ok && cc != nil {
				return true
			}
		}
	case []any:
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				if cc, ok := m["cache_control"]; ok && cc != nil {
					return true
				}
			}
		}
	}
	return false
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
