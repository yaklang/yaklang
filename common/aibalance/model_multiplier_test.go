package aibalance

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

// 关键词: model_multiplier_test, 实际模型计费分层回落, 模式匹配批量, 勾选批量

// ==================== 纯内存分层回落测试（无 DB） ====================

func TestResolveModelMultipliersFrom_AllNil_SystemConst(t *testing.T) {
	r := resolveModelMultipliersFrom(nil, nil)
	assert.Equal(t, 1.0, r.Input)
	assert.Equal(t, 1.0, r.Output)
	assert.Equal(t, 1.25, r.CacheCreate)
	assert.InDelta(t, 0.1, r.CacheHit, 1e-9)
}

func TestResolveModelMultipliersFrom_GlobalDefaultApplies(t *testing.T) {
	// 全局默认只设 input=3，其它维回落系统常量
	global := &AiModelMultiplierConfig{InputTokenMultiplier: 3.0}
	r := resolveModelMultipliersFrom(global, nil)
	assert.Equal(t, 3.0, r.Input)
	assert.Equal(t, 1.0, r.Output)
	assert.Equal(t, 1.25, r.CacheCreate)
	assert.InDelta(t, 0.1, r.CacheHit, 1e-9)
}

func TestResolveModelMultipliersFrom_ModelOverGlobal(t *testing.T) {
	global := &AiModelMultiplierConfig{InputTokenMultiplier: 3.0}
	m := &AiModelMultiplier{InputTokenMultiplier: 5.0}
	r := resolveModelMultipliersFrom(global, m)
	// 实际模型 input=5 > 全局 3
	assert.Equal(t, 5.0, r.Input)
	// 其它维：实际模型未设、全局未设 -> 系统常量
	assert.Equal(t, 1.0, r.Output)
	assert.Equal(t, 1.25, r.CacheCreate)
}

func TestResolveModelMultipliersFrom_PerDimFallthrough(t *testing.T) {
	// 逐维来自不同层：
	//   input  来自 实际模型(5)
	//   output 来自 全局(2)
	//   cacheHit 来自 全局(0.05)
	//   cacheCreate 无任何层 -> 系统常量 1.25
	global := &AiModelMultiplierConfig{OutputTokenMultiplier: 2.0, CacheHitMultiplier: 0.05}
	m := &AiModelMultiplier{InputTokenMultiplier: 5.0}
	r := resolveModelMultipliersFrom(global, m)
	assert.Equal(t, 5.0, r.Input)
	assert.Equal(t, 2.0, r.Output)
	assert.Equal(t, 1.25, r.CacheCreate)
	assert.InDelta(t, 0.05, r.CacheHit, 1e-9)
}

func TestWeightUsage_WithModelMultipliers(t *testing.T) {
	mul := resolvedMultipliers{Input: 2.0, Output: 3.0, CacheCreate: 1.25, CacheHit: 0.1}
	usage := &aispec.ChatUsage{
		PromptTokens:     1000,
		CompletionTokens: 200,
		PromptTokensDetails: &aispec.PromptTokensDetails{
			CachedTokens:             300,
			CacheCreationInputTokens: 100,
		},
	}
	// pureInput=600; weighted=600*2 + 200*3 + 100*1.25 + 300*0.1 = 1200+600+125+30 = 1955
	assert.Equal(t, int64(1955), WeightUsage(mul, usage))
}

// ==================== 模式匹配测试（纯函数） ====================

func TestMatchInternalModelPattern(t *testing.T) {
	// 子串匹配（无通配符），大小写不敏感
	assert.True(t, matchInternalModelPattern("kimi2.5-pro", "kimi2.5"))
	assert.True(t, matchInternalModelPattern("moonshot/Kimi2.5-air", "kimi2.5"))
	assert.False(t, matchInternalModelPattern("glm-4.6", "kimi2.5"))

	// glob 匹配（含通配符）
	assert.True(t, matchInternalModelPattern("kimi2.5-pro", "kimi2.5-*"))
	assert.True(t, matchInternalModelPattern("kimi2.5-air", "*2.5*"))
	assert.False(t, matchInternalModelPattern("glm-4.6", "kimi*"))

	// 空模式不匹配
	assert.False(t, matchInternalModelPattern("anything", ""))
	assert.False(t, matchInternalModelPattern("anything", "   "))
}

// ==================== DB 落库测试 ====================

func ensureMultiplierTables(t *testing.T) {
	t.Helper()
	require.NoError(t, EnsureModelMultiplierTable())
	require.NoError(t, EnsureModelMultiplierConfigTable())
	require.NoError(t, EnsureProviderTable())
}

func TestSaveGetDeleteModelMultiplier(t *testing.T) {
	ensureMultiplierTables(t)

	internal := fmt.Sprintf("real-model-%d", time.Now().UnixNano())
	defer GetDB().Unscoped().Where("internal_model_name = ?", internal).Delete(&AiModelMultiplier{})

	// 新建：input=2, output=3，其它跳过(-1)
	require.NoError(t, SaveModelMultiplier(internal, 2.0, 3.0, -1, -1))
	m, err := GetModelMultiplier(internal)
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.Equal(t, 2.0, m.InputTokenMultiplier)
	assert.Equal(t, 3.0, m.OutputTokenMultiplier)
	assert.Equal(t, 0.0, m.CacheCreationMultiplier)
	assert.Equal(t, 0.0, m.CacheHitMultiplier)

	// 更新：只改 cacheHit=0.05，input/output 跳过(-1)应保留
	require.NoError(t, SaveModelMultiplier(internal, -1, -1, -1, 0.05))
	m2, err := GetModelMultiplier(internal)
	require.NoError(t, err)
	require.NotNil(t, m2)
	assert.Equal(t, 2.0, m2.InputTokenMultiplier, "input should be preserved when skipped")
	assert.InDelta(t, 0.05, m2.CacheHitMultiplier, 1e-9)

	// 清空某维：传 0
	require.NoError(t, SaveModelMultiplier(internal, 0, -1, -1, -1))
	m3, err := GetModelMultiplier(internal)
	require.NoError(t, err)
	assert.Equal(t, 0.0, m3.InputTokenMultiplier, "input should be cleared to 0")

	// 删除：回落到 nil
	require.NoError(t, DeleteModelMultiplier(internal))
	m4, err := GetModelMultiplier(internal)
	require.NoError(t, err)
	assert.Nil(t, m4)
}

func TestSaveModelMultiplier_RequiresKey(t *testing.T) {
	ensureMultiplierTables(t)
	assert.Error(t, SaveModelMultiplier("", 1, 1, 1, 1))
}

func TestGlobalMultiplierConfig_SaveGet(t *testing.T) {
	ensureMultiplierTables(t)
	// 用例后清掉单例，避免污染其它用例
	defer GetDB().Unscoped().Where("id = ?", 1).Delete(&AiModelMultiplierConfig{})

	require.NoError(t, SaveGlobalMultiplierConfig(3.0, -1, -1, -1))
	cfg, err := GetGlobalMultiplierConfig()
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, 3.0, cfg.InputTokenMultiplier)
	assert.Equal(t, 0.0, cfg.OutputTokenMultiplier)

	// 更新 output，input 跳过应保留
	require.NoError(t, SaveGlobalMultiplierConfig(-1, 4.0, -1, -1))
	cfg2, err := GetGlobalMultiplierConfig()
	require.NoError(t, err)
	assert.Equal(t, 3.0, cfg2.InputTokenMultiplier)
	assert.Equal(t, 4.0, cfg2.OutputTokenMultiplier)
}

func TestResolveModelMultipliers_DBLayered(t *testing.T) {
	ensureMultiplierTables(t)

	internal := fmt.Sprintf("layered-%d", time.Now().UnixNano())
	defer GetDB().Unscoped().Where("internal_model_name = ?", internal).Delete(&AiModelMultiplier{})
	defer GetDB().Unscoped().Where("id = ?", 1).Delete(&AiModelMultiplierConfig{})

	// 全局默认 cacheHit=0.05
	require.NoError(t, SaveGlobalMultiplierConfig(-1, -1, -1, 0.05))
	// 实际模型 input=5
	require.NoError(t, SaveModelMultiplier(internal, 5.0, -1, -1, -1))

	r := ResolveModelMultipliers(internal)
	assert.Equal(t, 5.0, r.Input, "input from actual model")
	assert.Equal(t, 1.0, r.Output, "output from system const")
	assert.Equal(t, 1.25, r.CacheCreate, "cacheCreate from system const")
	assert.InDelta(t, 0.05, r.CacheHit, 1e-9, "cacheHit from global default")

	// internalModelName 为空时跳过实际模型层 -> input 回落系统常量 1.0，cacheHit 仍取全局默认
	r2 := ResolveModelMultipliers("")
	assert.Equal(t, 1.0, r2.Input)
	assert.InDelta(t, 0.05, r2.CacheHit, 1e-9)
}

func TestGetDistinctInternalModels_AndBatchWrite(t *testing.T) {
	ensureMultiplierTables(t)

	tag := fmt.Sprintf("%d", time.Now().UnixNano())
	wrapperA := "wrap-a-" + tag
	wrapperB := "wrap-b-" + tag
	n1 := "k25-pro-" + tag
	n2 := "k25-air-" + tag
	n3 := "glm-" + tag
	defer GetDB().Unscoped().Where("wrapper_name IN (?)", []string{wrapperA, wrapperB}).Delete(&AiProvider{})
	defer GetDB().Unscoped().Where("internal_model_name IN (?)", []string{n1, n2, n3}).Delete(&AiModelMultiplier{})

	// 同一实际模型 n1 经由两个 wrapper 暴露；应去重为一个实际模型、聚合两个 wrapper。
	providers := []*AiProvider{
		{WrapperName: wrapperA, ModelName: n1, APIKey: "k1"},
		{WrapperName: wrapperA, ModelName: n2, APIKey: "k2"},
		{WrapperName: wrapperB, ModelName: n1, APIKey: "k3"}, // 同实际模型，不同 wrapper
		{WrapperName: wrapperB, ModelName: n3, APIKey: "k4"},
	}
	for _, p := range providers {
		require.NoError(t, GetDB().Create(p).Error)
	}

	models, err := GetDistinctInternalModels()
	require.NoError(t, err)

	got := make(map[string]InternalModelInfo)
	for _, m := range models {
		if m.InternalModelName == n1 || m.InternalModelName == n2 || m.InternalModelName == n3 {
			got[m.InternalModelName] = m
		}
	}
	require.Len(t, got, 3, "three distinct internal models expected")
	// n1 关联两个 wrapper（排序后），count=2
	assert.Equal(t, []string{wrapperA, wrapperB}, got[n1].Wrappers)
	assert.Equal(t, 2, got[n1].ProviderCount)
	assert.Equal(t, []string{wrapperA}, got[n2].Wrappers)
	assert.Equal(t, []string{wrapperB}, got[n3].Wrappers)

	// 模拟「按模式批量」：把 input=4 应用到匹配 "k25-*-"+tag 的实际模型（命中 n1,n2，不含 n3）。
	pattern := "k25-*-" + tag
	applied := 0
	for _, m := range models {
		if !matchInternalModelPattern(m.InternalModelName, pattern) {
			continue
		}
		require.NoError(t, SaveModelMultiplier(m.InternalModelName, 4.0, -1, -1, -1))
		applied++
	}
	assert.Equal(t, 2, applied)

	all, err := GetAllModelMultipliers()
	require.NoError(t, err)
	require.NotNil(t, all[n1])
	require.NotNil(t, all[n2])
	assert.Equal(t, 4.0, all[n1].InputTokenMultiplier)
	assert.Equal(t, 4.0, all[n2].InputTokenMultiplier)
	assert.Nil(t, all[n3], "glm model should not be matched by k25 pattern")
}

func TestApplyModelMultiplierToModels_Selection(t *testing.T) {
	ensureMultiplierTables(t)

	tag := fmt.Sprintf("%d", time.Now().UnixNano())
	n1 := "sel-a-" + tag
	n2 := "sel-b-" + tag
	defer GetDB().Unscoped().Where("internal_model_name IN (?)", []string{n1, n2}).Delete(&AiModelMultiplier{})

	// 模拟「按勾选批量」：直接对显式列表逐个写。
	for _, internal := range []string{n1, n2} {
		require.NoError(t, SaveModelMultiplier(internal, -1, 7.0, -1, -1))
	}
	all, err := GetAllModelMultipliers()
	require.NoError(t, err)
	require.NotNil(t, all[n1])
	require.NotNil(t, all[n2])
	assert.Equal(t, 7.0, all[n1].OutputTokenMultiplier)
	assert.Equal(t, 7.0, all[n2].OutputTokenMultiplier)
}
