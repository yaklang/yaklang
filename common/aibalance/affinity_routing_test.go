package aibalance

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/schema"
)

// 关键词: aibalance, 亲和性路由, 单元测试
//
// 这些单测覆盖 PeekOrderedProvidersWithAffinity 的核心契约：
//   - 单 provider 时直接返回，与 affinityKey 无关
//   - affinityKey 为空时退化为完全随机（与 PeekOrderedProviders 一致）
//   - affinityKey 非空时同一健康集合下稳定路由到同一主 provider
//   - 不同 affinityKey 在足够多次取样下能覆盖多个 provider（分布性）
//   - 主 provider 始终位于返回列表第 0 位

func newTestProvider(typeName, domain, apiKey string, latencyMs int64, healthy bool) *Provider {
	return &Provider{
		TypeName:    typeName,
		DomainOrURL: domain,
		APIKey:      apiKey,
		ModelName:   "test-model",
		DbProvider: &schema.AiProvider{
			TypeName:    typeName,
			DomainOrURL: domain,
			APIKey:      apiKey,
			ModelName:   "test-model",
			IsHealthy:   healthy,
			LastLatency: latencyMs,
		},
	}
}

func newEntrypointsWithProviders(model string, ps []*Provider) *Entrypoints {
	e := NewEntrypoints()
	e.providers[model] = ps
	return e
}

// 单 provider 直接返回，无视 affinityKey
// 关键词: 亲和性路由, 单 provider 直返
func TestPeekOrderedProvidersWithAffinity_SingleProvider(t *testing.T) {
	p1 := newTestProvider("openai-1", "https://api1.test", "k1", 200, true)
	e := newEntrypointsWithProviders("test-model", []*Provider{p1})

	for _, key := range []string{"", "abc", "xyz"} {
		got := e.PeekOrderedProvidersWithAffinity("test-model", key)
		assert.Len(t, got, 1, "single provider should always be returned")
		assert.Equal(t, p1, got[0])
	}
}

// affinityKey 非空 + 多 provider：同一 key 多次调用返回的首个 provider 必须稳定
// 关键词: 亲和性路由, 主 provider 稳定
func TestPeekOrderedProvidersWithAffinity_StablePrimary(t *testing.T) {
	p1 := newTestProvider("p1", "d1", "k1", 200, true)
	p2 := newTestProvider("p2", "d2", "k2", 200, true)
	p3 := newTestProvider("p3", "d3", "k3", 200, true)
	e := newEntrypointsWithProviders("test-model", []*Provider{p1, p2, p3})

	const key = "stable-affinity-key"
	first := e.PeekOrderedProvidersWithAffinity("test-model", key)
	assert.Len(t, first, 3)

	// 重复 100 次，主 provider 必须始终是第 0 位且与首次一致
	expectedPrimary := first[0]
	for i := 0; i < 100; i++ {
		got := e.PeekOrderedProvidersWithAffinity("test-model", key)
		assert.Len(t, got, 3)
		assert.Same(t, expectedPrimary, got[0],
			"affinityKey must produce stable primary provider, iteration %d", i)
	}
}

// affinityKey 为空时退化为完全随机：多次调用首位 provider 应能覆盖到多个
// 关键词: 亲和性路由, 空 key 退化
func TestPeekOrderedProvidersWithAffinity_EmptyKeyRandom(t *testing.T) {
	p1 := newTestProvider("p1", "d1", "k1", 200, true)
	p2 := newTestProvider("p2", "d2", "k2", 200, true)
	p3 := newTestProvider("p3", "d3", "k3", 200, true)
	e := newEntrypointsWithProviders("test-model", []*Provider{p1, p2, p3})

	seen := make(map[*Provider]int)
	for i := 0; i < 300; i++ {
		got := e.PeekOrderedProvidersWithAffinity("test-model", "")
		assert.Len(t, got, 3)
		seen[got[0]]++
	}
	// 300 次随机洗牌，每个 provider 出现在首位的概率约 1/3，应该都被覆盖到
	assert.Len(t, seen, 3,
		"empty affinityKey should produce random primary distribution, got %v", seen)
}

// 不同 affinityKey 在大量取样下应覆盖到多个 provider，验证分布性
// 关键词: 亲和性路由, 分布性
func TestPeekOrderedProvidersWithAffinity_DistributesAcrossKeys(t *testing.T) {
	p1 := newTestProvider("p1", "d1", "k1", 200, true)
	p2 := newTestProvider("p2", "d2", "k2", 200, true)
	p3 := newTestProvider("p3", "d3", "k3", 200, true)
	e := newEntrypointsWithProviders("test-model", []*Provider{p1, p2, p3})

	seen := make(map[*Provider]int)
	for i := 0; i < 300; i++ {
		key := fmt.Sprintf("user-%d-prompt-hash", i)
		got := e.PeekOrderedProvidersWithAffinity("test-model", key)
		assert.Len(t, got, 3)
		seen[got[0]]++
	}
	assert.Len(t, seen, 3,
		"different affinityKeys should distribute across providers, got %v", seen)
}

// 健康集合发生变化（一个 provider 变为不健康/高延迟）时，
// 之前命中该 provider 的 affinityKey 应平滑迁移到剩余健康 provider
// 关键词: 亲和性路由, 健康集合变化, 平滑迁移
func TestPeekOrderedProvidersWithAffinity_HealthSetChange(t *testing.T) {
	p1 := newTestProvider("p1", "d1", "k1", 200, true)
	p2 := newTestProvider("p2", "d2", "k2", 200, true)
	p3 := newTestProvider("p3", "d3", "k3", 200, true)
	e := newEntrypointsWithProviders("test-model", []*Provider{p1, p2, p3})

	const key = "migrate-key"
	before := e.PeekOrderedProvidersWithAffinity("test-model", key)
	assert.Len(t, before, 3)
	primary := before[0]

	// 把主 provider 标记为高延迟（>10s），使其被过滤
	primary.DbProvider.LastLatency = 15000

	// 再次调用，应仍能返回剩余 2 个健康 provider
	after := e.PeekOrderedProvidersWithAffinity("test-model", key)
	assert.Len(t, after, 2)
	assert.NotSame(t, primary, after[0],
		"unhealthy provider should not be primary anymore")

	// 多次调用，剩余 2 个 provider 内的主 provider 必须稳定
	expectedNewPrimary := after[0]
	for i := 0; i < 50; i++ {
		got := e.PeekOrderedProvidersWithAffinity("test-model", key)
		assert.Len(t, got, 2)
		assert.Same(t, expectedNewPrimary, got[0],
			"primary in shrunk set must remain stable, iteration %d", i)
	}
}

// PeekOrderedProviders（无 affinity 参数版本）行为应与
// PeekOrderedProvidersWithAffinity(model, "") 完全一致：纯随机洗牌
// 关键词: 亲和性路由, 向后兼容
func TestPeekOrderedProviders_BackwardCompatible(t *testing.T) {
	p1 := newTestProvider("p1", "d1", "k1", 200, true)
	p2 := newTestProvider("p2", "d2", "k2", 200, true)
	p3 := newTestProvider("p3", "d3", "k3", 200, true)
	e := newEntrypointsWithProviders("test-model", []*Provider{p1, p2, p3})

	seen := make(map[*Provider]int)
	for i := 0; i < 300; i++ {
		got := e.PeekOrderedProviders("test-model")
		assert.Len(t, got, 3)
		seen[got[0]]++
	}
	assert.Len(t, seen, 3,
		"PeekOrderedProviders without affinity should remain fully random, got %v", seen)
}

// BuildPromptAffinityKey: 同一 (prompt, apiKey, model) 三元组必须产生同一 key
// 关键词: BuildPromptAffinityKey, 确定性
func TestBuildPromptAffinityKey_Deterministic(t *testing.T) {
	prompt := "<|PROMPT_SECTION_high-static|>system instruction here<|PROMPT_SECTION_END_high-static|>"
	k1 := BuildPromptAffinityKey(prompt, "user-key-1", "qwen-max", 2048)
	k2 := BuildPromptAffinityKey(prompt, "user-key-1", "qwen-max", 2048)
	assert.Equal(t, k1, k2, "same input must produce same affinityKey")
}

// BuildPromptAffinityKey: 不同 apiKey 必须产生不同 key（账号隔离）
// 关键词: BuildPromptAffinityKey, 账号隔离
func TestBuildPromptAffinityKey_DifferentByApiKey(t *testing.T) {
	prompt := "same prompt content"
	k1 := BuildPromptAffinityKey(prompt, "user-A", "qwen-max", 2048)
	k2 := BuildPromptAffinityKey(prompt, "user-B", "qwen-max", 2048)
	assert.NotEqual(t, k1, k2,
		"different apiKey must produce different affinityKey (account isolation)")
}

// BuildPromptAffinityKey: 不同 model 必须产生不同 key（模型隔离）
// 关键词: BuildPromptAffinityKey, 模型隔离
func TestBuildPromptAffinityKey_DifferentByModel(t *testing.T) {
	prompt := "same prompt content"
	k1 := BuildPromptAffinityKey(prompt, "user-A", "qwen-max", 2048)
	k2 := BuildPromptAffinityKey(prompt, "user-A", "qwen-plus", 2048)
	assert.NotEqual(t, k1, k2,
		"different model must produce different affinityKey (model isolation)")
}

// BuildPromptAffinityKey: prompt 后缀差异（前缀相同）应产生同一 key
// 这正是为隐式缓存优化的核心：仅按"前缀"决定路由
// 关键词: BuildPromptAffinityKey, 前缀路由
func TestBuildPromptAffinityKey_PrefixOnly(t *testing.T) {
	prefix := "abcdefg" // 长度 7
	k1 := BuildPromptAffinityKey(prefix+"---tail-A", "user-A", "qwen-max", 7)
	k2 := BuildPromptAffinityKey(prefix+"---tail-B", "user-A", "qwen-max", 7)
	assert.Equal(t, k1, k2,
		"prompts sharing the first prefixLen bytes should produce same key")

	// prefix 内容不同：key 必须不同
	k3 := BuildPromptAffinityKey("xyzdefg---tail-A", "user-A", "qwen-max", 7)
	assert.NotEqual(t, k1, k3,
		"prompts with different prefix bytes should produce different keys")
}

// BuildPromptAffinityKey: prefixLen <= 0 时使用默认值
// 关键词: BuildPromptAffinityKey, 默认 prefixLen
func TestBuildPromptAffinityKey_DefaultPrefixLen(t *testing.T) {
	prompt := "short prompt"
	k1 := BuildPromptAffinityKey(prompt, "user-A", "qwen-max", 0)
	k2 := BuildPromptAffinityKey(prompt, "user-A", "qwen-max", 2048)
	assert.Equal(t, k1, k2,
		"prefixLen <= 0 must fall back to default 2048, identical to explicit 2048")
}

// hashAffinityKey: 同一字符串必须返回同一 uint32
// 关键词: hashAffinityKey, FNV 稳定性
func TestHashAffinityKey_Deterministic(t *testing.T) {
	v1 := hashAffinityKey("hello")
	v2 := hashAffinityKey("hello")
	assert.Equal(t, v1, v2)

	// 不同字符串不应轻易碰撞
	v3 := hashAffinityKey("world")
	assert.NotEqual(t, v1, v3)
}

// sortProvidersStably: 排序结果应基于 TypeName+DomainOrURL+APIKey，跨调用稳定
// 关键词: sortProvidersStably, 稳定排序
func TestSortProvidersStably(t *testing.T) {
	pa := newTestProvider("z-type", "d", "k", 100, true)
	pb := newTestProvider("a-type", "d", "k", 100, true)
	pc := newTestProvider("m-type", "d", "k", 100, true)

	in := []*Provider{pa, pb, pc}
	sortProvidersStably(in)
	assert.Equal(t, []*Provider{pb, pc, pa}, in,
		"providers should be sorted by TypeName|DomainOrURL|APIKey ascending")
}
