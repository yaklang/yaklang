package aibalance

import (
	"encoding/json"
	"strings"
)

// model_downgrade.go 实现「按客户端上报的模型用途类型(tier)对请求模型做降级」以保护用量。
//
// 客户端（aispec/aicommon）在调用 aibalance 时通过请求头 X-Yak-AI-Model-Usage-Type
// 上报本次是「高质 intelligent / 快速 lightweight / 视觉 vision」哪一档发起的；服务端按
// AiBalanceRateLimitConfig.ModelDowngradeRules 配置的规则匹配，命中则把 modelName 改写成
// 降级目标模型（如 lightweight + memfit-standard-free -> memfit-light-free）。
//
// 关键词: 模型用途类型降级, X-Yak-AI-Model-Usage-Type, ModelDowngradeRules, 轻量降级保护用量

// ModelUsageTypeHeader 是客户端上报模型用途类型(tier)的请求头名。
// 与 common/ai/aibalance/gateway.go 注入端、consts.Tier* 取值保持一致。
// 关键词: ModelUsageTypeHeader, X-Yak-AI-Model-Usage-Type
const ModelUsageTypeHeader = "X-Yak-AI-Model-Usage-Type"

// ModelDowngradeRule 描述一条模型降级规则：当客户端上报 tier==Tier（Tier 为空表示不限 tier）
// 且请求模型 == From 时，把模型降级为 To。
// 关键词: ModelDowngradeRule, tier/from/to 规则
type ModelDowngradeRule struct {
	Tier string `json:"tier"`
	From string `json:"from"`
	To   string `json:"to"`
}

// parseModelDowngradeRules 解析 ModelDowngradeRules JSON 数组，过滤 from/to 为空的脏规则。
// 任何异常都返回空切片，不阻塞业务。
// 关键词: parseModelDowngradeRules, JSON 数组解析, 容错
func parseModelDowngradeRules(raw string) []ModelDowngradeRule {
	out := make([]ModelDowngradeRule, 0)
	if strings.TrimSpace(raw) == "" {
		return out
	}
	var rules []ModelDowngradeRule
	if err := json.Unmarshal([]byte(raw), &rules); err != nil {
		return out
	}
	for _, r := range rules {
		r.Tier = strings.TrimSpace(r.Tier)
		r.From = strings.TrimSpace(r.From)
		r.To = strings.TrimSpace(r.To)
		if r.From == "" || r.To == "" {
			continue
		}
		out = append(out, r)
	}
	return out
}

// resolveModelDowngrade 根据客户端上报的模型用途类型(usageType) 与缓存的降级规则，
// 判断请求模型 modelName 是否需要降级。命中返回 (目标模型, true)，否则返回 (原模型, false)。
//
// 匹配规则（按 ServerConfig.modelDowngradeRules 顺序，命中即停）：
//   - 规则 From 必须精确等于 modelName；
//   - 规则 Tier 为空表示不限 tier，否则要求与 usageType 大小写不敏感相等；
//   - 目标模型 To 非空且与原模型不同才算有效降级。
//
// 关键词: resolveModelDowngrade, tier 匹配, From/To 改写, 缓存规则读取
func (c *ServerConfig) resolveModelDowngrade(usageType, modelName string) (string, bool) {
	usageType = strings.TrimSpace(usageType)
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return modelName, false
	}
	c.limitPolicyMu.RLock()
	rules := c.modelDowngradeRules
	c.limitPolicyMu.RUnlock()
	for _, r := range rules {
		if r.From != modelName {
			continue
		}
		if r.Tier != "" && !strings.EqualFold(r.Tier, usageType) {
			continue
		}
		if r.To == "" || r.To == modelName {
			continue
		}
		return r.To, true
	}
	return modelName, false
}
