package aibalance

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

// 关键词: model_multiplier_test, 倍率双标识分层回落, 批量应用

// ==================== 纯内存分层回落测试（无 DB） ====================

func TestResolveBillingMultipliersFrom_AllNil_SystemConst(t *testing.T) {
	r := resolveBillingMultipliersFrom(nil, nil, nil)
	assert.Equal(t, 1.0, r.Input)
	assert.Equal(t, 1.0, r.Output)
	assert.Equal(t, 1.25, r.CacheCreate)
	assert.InDelta(t, 0.1, r.CacheHit, 1e-9)
}

func TestResolveBillingMultipliersFrom_GlobalDefaultApplies(t *testing.T) {
	// 全局默认只设 input=3，其它维回落系统常量
	global := &AiModelMultiplierConfig{InputTokenMultiplier: 3.0}
	r := resolveBillingMultipliersFrom(global, nil, nil)
	assert.Equal(t, 3.0, r.Input)
	assert.Equal(t, 1.0, r.Output)
	assert.Equal(t, 1.25, r.CacheCreate)
	assert.InDelta(t, 0.1, r.CacheHit, 1e-9)
}

func TestResolveBillingMultipliersFrom_WrapperOverGlobal(t *testing.T) {
	global := &AiModelMultiplierConfig{InputTokenMultiplier: 3.0}
	meta := &AiModelMeta{InputTokenMultiplier: 5.0}
	r := resolveBillingMultipliersFrom(global, meta, nil)
	// wrapper 显式 input=5 > 全局 3
	assert.Equal(t, 5.0, r.Input)
	// 其它维：wrapper 未设、全局未设 -> 系统常量
	assert.Equal(t, 1.0, r.Output)
	assert.Equal(t, 1.25, r.CacheCreate)
}

func TestResolveBillingMultipliersFrom_OverrideWins(t *testing.T) {
	global := &AiModelMultiplierConfig{InputTokenMultiplier: 3.0}
	meta := &AiModelMeta{InputTokenMultiplier: 5.0}
	override := &AiModelMultiplierOverride{InputTokenMultiplier: 7.0}
	r := resolveBillingMultipliersFrom(global, meta, override)
	assert.Equal(t, 7.0, r.Input)
}

func TestResolveBillingMultipliersFrom_PerDimFallthrough(t *testing.T) {
	// 逐维来自不同层：
	//   input  来自 wrapper(5)
	//   output 来自 override(9)
	//   cacheHit 来自 global(0.05)
	//   cacheCreate 无任何层 -> 系统常量 1.25
	global := &AiModelMultiplierConfig{CacheHitMultiplier: 0.05}
	meta := &AiModelMeta{InputTokenMultiplier: 5.0}
	override := &AiModelMultiplierOverride{OutputTokenMultiplier: 9.0}
	r := resolveBillingMultipliersFrom(global, meta, override)
	assert.Equal(t, 5.0, r.Input)
	assert.Equal(t, 9.0, r.Output)
	assert.Equal(t, 1.25, r.CacheCreate)
	assert.InDelta(t, 0.05, r.CacheHit, 1e-9)
}

func TestResolveBillingMultipliersFrom_LegacyTrafficNonDefault_ZeroBreak(t *testing.T) {
	// 存量零破坏：四维全 0 且 legacy=2.0（非默认）-> 按老 legacy 公式视为四维已配置
	meta := &AiModelMeta{TrafficMultiplier: 2.0}
	r := resolveBillingMultipliersFrom(nil, meta, nil)
	assert.Equal(t, 2.0, r.Input)
	assert.Equal(t, 2.0, r.Output)
	assert.Equal(t, 2.5, r.CacheCreate)
	assert.InDelta(t, 0.2, r.CacheHit, 1e-9)
}

func TestResolveBillingMultipliersFrom_LegacyDefaultDoesNotBlockGlobal(t *testing.T) {
	// legacy=1.0（默认）不阻断全局默认：新加的全局默认要能对存量 x1.00 模型生效
	global := &AiModelMultiplierConfig{InputTokenMultiplier: 3.0}
	meta := &AiModelMeta{TrafficMultiplier: 1.0}
	r := resolveBillingMultipliersFrom(global, meta, nil)
	assert.Equal(t, 3.0, r.Input)
}

func TestResolveBillingMultipliersFrom_ZeroBreakWithoutNewConfig(t *testing.T) {
	// 无任何新配置（global=nil, override=nil）时，结果必须与老 resolveMultipliers 完全一致
	cases := []*AiModelMeta{
		nil,
		{TrafficMultiplier: 2.0},
		{InputTokenMultiplier: 5.0}, // 部分新维
		{InputTokenMultiplier: 1.5, OutputTokenMultiplier: 2.0},
		{TrafficMultiplier: 1.0}, // 默认 legacy
	}
	for i, meta := range cases {
		old := resolveMultipliers(meta)
		neo := resolveBillingMultipliersFrom(nil, meta, nil)
		assert.InDelta(t, old.Input, neo.Input, 1e-9, "case %d input", i)
		assert.InDelta(t, old.Output, neo.Output, 1e-9, "case %d output", i)
		assert.InDelta(t, old.CacheCreate, neo.CacheCreate, 1e-9, "case %d cacheCreate", i)
		assert.InDelta(t, old.CacheHit, neo.CacheHit, 1e-9, "case %d cacheHit", i)
	}
}

func TestWeightUsage_WithRouteMultipliers(t *testing.T) {
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

// ==================== DB 落库测试 ====================

func ensureMultiplierTables(t *testing.T) {
	t.Helper()
	require.NoError(t, EnsureModelMultiplierOverrideTable())
	require.NoError(t, EnsureModelMultiplierConfigTable())
	require.NoError(t, EnsureModelMetaTable())
	require.NoError(t, EnsureProviderTable())
}

func TestSaveGetDeleteModelMultiplierOverride(t *testing.T) {
	ensureMultiplierTables(t)

	wrapper := fmt.Sprintf("wrap-%d", time.Now().UnixNano())
	internal := "internal-model-a"
	defer GetDB().Unscoped().Where("wrapper_name = ?", wrapper).Delete(&AiModelMultiplierOverride{})

	// 新建：input=2, output=3，其它跳过(-1)
	require.NoError(t, SaveModelMultiplierOverride(wrapper, internal, 2.0, 3.0, -1, -1))
	o, err := GetModelMultiplierOverride(wrapper, internal)
	require.NoError(t, err)
	require.NotNil(t, o)
	assert.Equal(t, 2.0, o.InputTokenMultiplier)
	assert.Equal(t, 3.0, o.OutputTokenMultiplier)
	assert.Equal(t, 0.0, o.CacheCreationMultiplier)
	assert.Equal(t, 0.0, o.CacheHitMultiplier)

	// 更新：只改 cacheHit=0.05，input/output 跳过(-1)应保留
	require.NoError(t, SaveModelMultiplierOverride(wrapper, internal, -1, -1, -1, 0.05))
	o2, err := GetModelMultiplierOverride(wrapper, internal)
	require.NoError(t, err)
	require.NotNil(t, o2)
	assert.Equal(t, 2.0, o2.InputTokenMultiplier, "input should be preserved when skipped")
	assert.InDelta(t, 0.05, o2.CacheHitMultiplier, 1e-9)

	// 清空某维：传 0
	require.NoError(t, SaveModelMultiplierOverride(wrapper, internal, 0, -1, -1, -1))
	o3, err := GetModelMultiplierOverride(wrapper, internal)
	require.NoError(t, err)
	assert.Equal(t, 0.0, o3.InputTokenMultiplier, "input should be cleared to 0")

	// 删除：回落到 nil
	require.NoError(t, DeleteModelMultiplierOverride(wrapper, internal))
	o4, err := GetModelMultiplierOverride(wrapper, internal)
	require.NoError(t, err)
	assert.Nil(t, o4)
}

func TestSaveModelMultiplierOverride_RequiresKeys(t *testing.T) {
	ensureMultiplierTables(t)
	assert.Error(t, SaveModelMultiplierOverride("", "x", 1, 1, 1, 1))
	assert.Error(t, SaveModelMultiplierOverride("x", "", 1, 1, 1, 1))
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

func TestResolveBillingMultipliers_DBLayered(t *testing.T) {
	ensureMultiplierTables(t)

	wrapper := fmt.Sprintf("layered-%d", time.Now().UnixNano())
	internal := "internal-x"
	defer GetDB().Unscoped().Where("wrapper_name = ?", wrapper).Delete(&AiModelMultiplierOverride{})
	defer GetDB().Unscoped().Where("model_name = ?", wrapper).Delete(&AiModelMeta{})
	defer GetDB().Unscoped().Where("id = ?", 1).Delete(&AiModelMultiplierConfig{})

	// 全局默认 cacheHit=0.05
	require.NoError(t, SaveGlobalMultiplierConfig(-1, -1, -1, 0.05))
	// wrapper 默认 input=5
	require.NoError(t, SaveModelMetaWithMultipliers(wrapper, "", "", -1, 5.0, -1, -1, -1))
	// (W,I) override output=9
	require.NoError(t, SaveModelMultiplierOverride(wrapper, internal, -1, 9.0, -1, -1))

	r := ResolveBillingMultipliers(wrapper, internal)
	assert.Equal(t, 5.0, r.Input, "input from wrapper")
	assert.Equal(t, 9.0, r.Output, "output from override")
	assert.Equal(t, 1.25, r.CacheCreate, "cacheCreate from system const")
	assert.InDelta(t, 0.05, r.CacheHit, 1e-9, "cacheHit from global default")

	// internalModelName 为空时跳过 override 层 -> output 回落系统常量 1.0
	r2 := ResolveBillingMultipliers(wrapper, "")
	assert.Equal(t, 5.0, r2.Input)
	assert.Equal(t, 1.0, r2.Output)
}

func TestGetDistinctModelRoutes_AndBatchWrite(t *testing.T) {
	ensureMultiplierTables(t)

	tag := fmt.Sprintf("%d", time.Now().UnixNano())
	wrapperA := "batch-wrap-a-" + tag
	wrapperB := "batch-wrap-b-" + tag
	defer GetDB().Unscoped().Where("wrapper_name IN (?)", []string{wrapperA, wrapperB}).Delete(&AiProvider{})
	defer GetDB().Unscoped().Where("wrapper_name IN (?)", []string{wrapperA, wrapperB}).Delete(&AiModelMultiplierOverride{})

	// wrapperA 下两个不同内部模型，wrapperB 下一个；wrapperA 的某内部模型重复一次(多 provider 同路由)
	providers := []*AiProvider{
		{WrapperName: wrapperA, ModelName: "qwen-real-1", APIKey: "k1"},
		{WrapperName: wrapperA, ModelName: "qwen-real-2", APIKey: "k2"},
		{WrapperName: wrapperA, ModelName: "qwen-real-1", APIKey: "k3"}, // 重复路由
		{WrapperName: wrapperB, ModelName: "glm-real-1", APIKey: "k4"},
	}
	for _, p := range providers {
		require.NoError(t, GetDB().Create(p).Error)
	}

	routes, err := GetDistinctModelRoutes()
	require.NoError(t, err)

	// 统计本测试创建的 distinct 路由（subset 校验，DB 可能已有其它数据）
	got := make(map[string]bool)
	for _, r := range routes {
		if r.WrapperName == wrapperA || r.WrapperName == wrapperB {
			got[r.WrapperName+"|"+r.InternalModelName] = true
		}
	}
	assert.True(t, got[wrapperA+"|qwen-real-1"])
	assert.True(t, got[wrapperA+"|qwen-real-2"])
	assert.True(t, got[wrapperB+"|glm-real-1"])
	assert.Equal(t, 3, len(got), "duplicate (wrapper, model) must be de-duplicated")

	// 模拟批量应用：给本测试的 3 条路由写 input=4
	applied := 0
	for _, r := range routes {
		if r.WrapperName != wrapperA && r.WrapperName != wrapperB {
			continue
		}
		require.NoError(t, SaveModelMultiplierOverride(r.WrapperName, r.InternalModelName, 4.0, -1, -1, -1))
		applied++
	}
	assert.Equal(t, 3, applied)

	all, err := GetAllModelMultiplierOverrides()
	require.NoError(t, err)
	assert.Equal(t, 4.0, all[modelOverrideKey(wrapperA, "qwen-real-1")].InputTokenMultiplier)
	assert.Equal(t, 4.0, all[modelOverrideKey(wrapperA, "qwen-real-2")].InputTokenMultiplier)
	assert.Equal(t, 4.0, all[modelOverrideKey(wrapperB, "glm-real-1")].InputTokenMultiplier)
}
