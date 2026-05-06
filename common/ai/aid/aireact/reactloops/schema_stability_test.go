package reactloops

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/omap"
)

// makeSchemaStabilityTestLoop 构造一个最小可用的 ReActLoop, 注入最常见的 4 个
// LoopAction (require_tool / directly_call_tool / directly_answer / finish) 用于
// schema 稳定性测试. 不调用 NewReActLoop 是因为后者依赖完整 invoker 链, 测试
// 只需要 generateSchemaString 内部依赖项 (actions + config + a few callbacks).
//
// 关键词: schema test helper, ReActLoop minimal, P2.1 schema 字节稳定测试
func makeSchemaStabilityTestLoop(cfg aicommon.AICallerConfigIf) *ReActLoop {
	loop := &ReActLoop{
		config:                     cfg,
		emitter:                    cfg.GetEmitter(),
		verificationMutex:          new(sync.Mutex),
		taskMutex:                  new(sync.Mutex),
		vars:                       omap.NewEmptyOrderedMap[string, any](),
		actions:                    omap.NewEmptyOrderedMap[string, *LoopAction](),
		loopActions:                omap.NewEmptyOrderedMap[string, LoopActionFactory](),
		streamFields:               omap.NewEmptyOrderedMap[string, *LoopStreamField](),
		aiTagFields:                omap.NewEmptyOrderedMap[string, *LoopAITagField](),
		currentMemories:            omap.NewEmptyOrderedMap[string, *aicommon.MemoryEntity](),
		extraCapabilities:          NewExtraCapabilitiesManager(),
		actionHistory:              make([]*ActionRecord, 0),
		actionHistoryMutex:         new(sync.Mutex),
		historySatisfactionReasons: make([]*SatisfactionRecord, 0),
	}

	// 注册 4 个常见 action, 模拟 ReAct 主循环
	requireToolAction := &LoopAction{
		ActionType:  schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL,
		Description: "discover and require a new tool",
		Options: []aitool.ToolOption{
			aitool.WithStringParam("tool_require_payload",
				aitool.WithParam_Description("name or natural-language description of the desired tool")),
		},
	}
	directlyCallAction := &LoopAction{
		ActionType:  schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL,
		Description: "directly call a recently used tool",
		Options: []aitool.ToolOption{
			aitool.WithStringParam("directly_call_tool_name"),
			aitool.WithStringParam("directly_call_tool_params"),
		},
	}
	directlyAnswerAction := loopAction_DirectlyAnswer
	finishAction := loopAction_Finish

	loop.actions.Set(requireToolAction.ActionType, requireToolAction)
	loop.actions.Set(directlyCallAction.ActionType, directlyCallAction)
	loop.actions.Set(directlyAnswerAction.ActionType, directlyAnswerAction)
	loop.actions.Set(finishAction.ActionType, finishAction)

	return loop
}

// TestGenerateSchema_StableAcrossHasRecentlyUsedTools 验证 P2.1 schema 字节
// 稳定化核心断言: 同一 ReActLoop 在 toolManager.HasRecentlyUsedTools() 0->1 跳变
// 前后, generateSchemaString 输出字节完全一致 (semi-dynamic 段 cache 命中前提).
//
// 历史: 旧实现 (P2.1 之前) 在 HasRecentlyUsedTools=false 时把 directly_call_tool
// 加进 disableActionList, schema enum / desc 缩短; HasRecentlyUsedTools=true 时
// 保留, schema enum / desc 增长. 这导致每会话首次调用工具时 semi-dynamic 段
// hash 翻转, prefix cache 必然失效一次. P2.1 改成永远保留 directly_call_tool
// 在 schema 中, 由 ActionVerifier 在 LLM 误选时报错触发 retry.
//
// 关键词: P2.1, schema 字节稳定, HasRecentlyUsedTools 跳变消除, byte-equal 断言
func TestGenerateSchema_StableAcrossHasRecentlyUsedTools(t *testing.T) {
	cfg := aicommon.NewConfig(context.Background())
	loop := makeSchemaStabilityTestLoop(cfg)

	// turn 1: toolManager 存在但 recent tools 为空
	tm := cfg.GetAiToolManager()
	require.NotNil(t, tm, "real Config must provide a non-nil tool manager")
	require.False(t, tm.HasRecentlyUsedTools(),
		"fresh tool manager should report no recent tools")

	schemaBefore, err := loop.generateSchemaString(false)
	require.NoError(t, err)
	require.NotEmpty(t, schemaBefore)
	require.Contains(t, schemaBefore, schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL,
		"directly_call_tool MUST stay in schema even when no recent tools (P2.1 invariant)")

	// turn 2: 真实加一个 recent tool, HasRecentlyUsedTools 跳到 true
	probe := aitool.NewWithoutCallback("schema_stability_probe",
		aitool.WithDescription("probe tool for schema stability test"),
		aitool.WithStringParam("payload"),
	)
	tm.AddRecentlyUsedTool(probe)
	require.True(t, tm.HasRecentlyUsedTools(),
		"tool manager should now report recent tools present")

	schemaAfter, err := loop.generateSchemaString(false)
	require.NoError(t, err)

	require.Equal(t, schemaBefore, schemaAfter,
		"P2.1 invariant: schema must be byte-identical across HasRecentlyUsedTools 0->1 transition")
}

// TestGenerateSchema_DirectlyCallToolDisabledWhenNoToolManager 验证 P2.1 之后
// 唯一保留的 disable 路径: toolManager 完全 nil 时, directly_call_tool 仍然
// 应被 disable (避免运行时 NPE). 现实中只在极少数测试场景出现.
//
// 关键词: P2.1, schema, toolManager nil 兜底, 仅此一项 disable
func TestGenerateSchema_DirectlyCallToolDisabledWhenNoToolManager(t *testing.T) {
	cfg := aicommon.NewConfig(context.Background())
	loop := makeSchemaStabilityTestLoop(cfg)

	// 强制把 config 上的 toolManager 设为 nil. aicommon.Config 没有公开 setter,
	// 这里通过 ReActLoop.config 字段替换为一个 nil-toolManager wrapper 实现.
	loop.config = &configWithNilToolManager{AICallerConfigIf: cfg}

	out, err := loop.generateSchemaString(false)
	require.NoError(t, err)
	require.NotEmpty(t, out)
	require.NotContains(t, out, schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL,
		"directly_call_tool MUST be disabled when toolManager is nil (NPE 兜底)")
}

// TestGenerateSchema_StableAcrossMultipleAdds 双重保险: 多次 AddRecentlyUsedTool
// 后 schema 仍与初始空 schema 字节相等 (验证 LRU 增减不影响 schema 字节).
//
// 关键词: P2.1, schema 字节稳定, recent tools LRU 增减不影响
func TestGenerateSchema_StableAcrossMultipleAdds(t *testing.T) {
	cfg := aicommon.NewConfig(context.Background())
	loop := makeSchemaStabilityTestLoop(cfg)

	tm := cfg.GetAiToolManager()
	require.NotNil(t, tm)

	schema0, err := loop.generateSchemaString(false)
	require.NoError(t, err)

	for i := 0; i < 5; i++ {
		tool := aitool.NewWithoutCallback(
			"probe_tool_iter",
			aitool.WithDescription("probe"),
			aitool.WithStringParam("p"),
		)
		tm.AddRecentlyUsedTool(tool)
	}

	schemaN, err := loop.generateSchemaString(false)
	require.NoError(t, err)
	require.Equal(t, schema0, schemaN,
		"adding multiple recent tools must not perturb schema bytes (P2.1 invariant)")
}

// configWithNilToolManager wraps an AICallerConfigIf and forces GetAiToolManager
// to return nil, used to test the toolManager-nil disable path without
// touching the real Config internals.
type configWithNilToolManager struct {
	aicommon.AICallerConfigIf
}

func (c *configWithNilToolManager) GetAiToolManager() *buildinaitools.AiToolManager {
	return nil
}
