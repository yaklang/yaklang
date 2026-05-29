package aibalance

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseModelDowngradeRules 验证 JSON 解析与脏规则过滤。
// 关键词: parseModelDowngradeRules 解析, 脏规则过滤
func TestParseModelDowngradeRules(t *testing.T) {
	// 空字符串 / 非法 JSON 都应返回空切片而非 nil-panic
	assert.Len(t, parseModelDowngradeRules(""), 0)
	assert.Len(t, parseModelDowngradeRules("not-json"), 0)

	// from/to 为空的脏规则应被过滤，tier/from/to 应被 trim
	rules := parseModelDowngradeRules(`[
		{"tier":" lightweight ","from":" memfit-standard-free ","to":" memfit-light-free "},
		{"tier":"lightweight","from":"","to":"x"},
		{"tier":"lightweight","from":"y","to":""}
	]`)
	require.Len(t, rules, 1)
	assert.Equal(t, "lightweight", rules[0].Tier)
	assert.Equal(t, "memfit-standard-free", rules[0].From)
	assert.Equal(t, "memfit-light-free", rules[0].To)
}

// TestResolveModelDowngrade 验证基于缓存规则的降级匹配逻辑。
// 关键词: resolveModelDowngrade tier 匹配, lightweight 降级
func TestResolveModelDowngrade(t *testing.T) {
	cfg := NewServerConfig()
	defer cfg.Close()

	cfg.applyRateLimitConfig(&AiBalanceRateLimitConfig{
		ModelDowngradeRules: `[{"tier":"lightweight","from":"memfit-standard-free","to":"memfit-light-free"}]`,
	})

	// 命中：lightweight + memfit-standard-free -> memfit-light-free（tier 大小写不敏感）
	newModel, downgraded := cfg.resolveModelDowngrade("lightweight", "memfit-standard-free")
	assert.True(t, downgraded)
	assert.Equal(t, "memfit-light-free", newModel)

	newModel, downgraded = cfg.resolveModelDowngrade("LightWeight", "memfit-standard-free")
	assert.True(t, downgraded, "tier match should be case-insensitive")
	assert.Equal(t, "memfit-light-free", newModel)

	// 不命中：tier 不匹配
	newModel, downgraded = cfg.resolveModelDowngrade("intelligent", "memfit-standard-free")
	assert.False(t, downgraded)
	assert.Equal(t, "memfit-standard-free", newModel)

	// 不命中：模型名不匹配
	newModel, downgraded = cfg.resolveModelDowngrade("lightweight", "memfit-light-free")
	assert.False(t, downgraded)
	assert.Equal(t, "memfit-light-free", newModel)

	// 空模型名直接返回不降级
	newModel, downgraded = cfg.resolveModelDowngrade("lightweight", "")
	assert.False(t, downgraded)
	assert.Equal(t, "", newModel)
}

// TestResolveModelDowngrade_TierWildcard 验证 tier 为空表示不限 tier。
// 关键词: resolveModelDowngrade tier 通配
func TestResolveModelDowngrade_TierWildcard(t *testing.T) {
	cfg := NewServerConfig()
	defer cfg.Close()

	cfg.applyRateLimitConfig(&AiBalanceRateLimitConfig{
		ModelDowngradeRules: `[{"tier":"","from":"any-free","to":"cheap-free"}]`,
	})

	for _, usageType := range []string{"lightweight", "intelligent", "vision", ""} {
		newModel, downgraded := cfg.resolveModelDowngrade(usageType, "any-free")
		assert.True(t, downgraded, "empty tier rule should match any usage type: %q", usageType)
		assert.Equal(t, "cheap-free", newModel)
	}
}

// TestGateLightweightDowngrade 验证从请求头读取 usage-type 并改写模型名。
// 关键词: gateLightweightDowngrade X-Yak-AI-Model-Usage-Type 改写
func TestGateLightweightDowngrade(t *testing.T) {
	cfg := NewServerConfig()
	defer cfg.Close()

	cfg.applyRateLimitConfig(&AiBalanceRateLimitConfig{
		ModelDowngradeRules: `[{"tier":"lightweight","from":"memfit-standard-free","to":"memfit-light-free"}]`,
	})

	withHeader := []byte("POST /v1/chat/completions HTTP/1.1\r\n" +
		"Host: localhost\r\n" +
		ModelUsageTypeHeader + ": lightweight\r\n" +
		"Content-Type: application/json\r\n\r\n{}")
	assert.Equal(t, "memfit-light-free",
		cfg.gateLightweightDowngrade(withHeader, "memfit-standard-free"),
		"request carrying lightweight usage-type header should be downgraded")

	// 无 usage-type 头：保持原模型
	noHeader := []byte("POST /v1/chat/completions HTTP/1.1\r\n" +
		"Host: localhost\r\n" +
		"Content-Type: application/json\r\n\r\n{}")
	assert.Equal(t, "memfit-standard-free",
		cfg.gateLightweightDowngrade(noHeader, "memfit-standard-free"),
		"request without usage-type header should keep the original model")

	// 头存在但 tier 不命中：保持原模型
	intelligentHeader := []byte("POST /v1/chat/completions HTTP/1.1\r\n" +
		"Host: localhost\r\n" +
		ModelUsageTypeHeader + ": intelligent\r\n\r\n{}")
	assert.Equal(t, "memfit-standard-free",
		cfg.gateLightweightDowngrade(intelligentHeader, "memfit-standard-free"),
		"non-matching tier should keep the original model")
}
