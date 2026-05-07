package aireact

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicache"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/ytoken"
	"github.com/yaklang/yaklang/common/schema"
)

func mustLoopPromptSections(t *testing.T, raw any) []*reactloops.PromptSectionObservation {
	t.Helper()
	sections, ok := raw.([]*reactloops.PromptSectionObservation)
	require.True(t, ok, "loop prompt sections should be []*reactloops.PromptSectionObservation")
	return sections
}

// TestPromptManager_AssembleLoopPrompt_SectionOrder 验证"按稳定性分层"路径
// 下 6 段顺序: high_static -> frozen_block -> semi_dynamic_1 (Skills + CacheToolCall)
// -> semi_dynamic_2 (TaskInstruction + Schema + OutputExample) -> timeline_open ->
// dynamic; 以及 frozen_block 段被 AI_CACHE_FROZEN_semi-dynamic 标签包裹,
// semi_dynamic_1 / semi_dynamic_2 段分别被 AI_CACHE_SEMI / AI_CACHE_SEMI2 包裹.
//
// 关键词: AssembleLoopPrompt section order, 6 段顺序, frozen-block, AI_CACHE_FROZEN,
//
//	AI_CACHE_SEMI, AI_CACHE_SEMI2, P1.1 拆 semi
func TestPromptManager_AssembleLoopPrompt_SectionOrder(t *testing.T) {
	react, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"object","next_action":{"type":"directly_answer","answer_payload":"ok"},"cumulative_summary":"ok","human_readable_thought":"ok"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)

	tool := aitool.NewWithoutCallback("tool-a", aitool.WithDescription("tool a desc"))
	react.promptManager.cpm.Register("provider-one", func(config aicommon.AICallerConfigIf, emitter *aicommon.Emitter, key string) (string, error) {
		return "auto ctx body", nil
	})
	react.config.SetUserInputHistory([]schema.AIAgentUserInputRecord{
		{Round: 1, Timestamp: time.Date(2026, 4, 29, 10, 0, 0, 0, time.Local), UserInput: "previous input"},
	})
	react.AddToTimeline("test", "timeline content")

	result, err := react.promptManager.AssembleLoopPrompt([]*aitool.Tool{tool}, &reactloops.LoopPromptAssemblyInput{
		Nonce:           "n123",
		UserQuery:       "current user query",
		TaskInstruction: "follow task rules",
		OutputExample:   "example output",
		Schema:          `{"type":"object","properties":{"@action":{"type":"string"}}}`,
		SkillsContext:   "<|SKILLS_CONTEXT_skills_context|>\nloaded skill\n<|SKILLS_CONTEXT_END_skills_context|>",
		ReactiveData:    "reactive state",
		InjectedMemory:  "memory content",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	sections := mustLoopPromptSections(t, result.Sections)
	require.Len(t, sections, 6)

	require.Equal(t, "section.high_static", sections[0].Key)
	require.Equal(t, "section.frozen_block", sections[1].Key)
	require.Equal(t, "section.semi_dynamic_1", sections[2].Key)
	require.Equal(t, "section.semi_dynamic_2", sections[3].Key)
	require.Equal(t, "section.timeline_open", sections[4].Key)
	require.Equal(t, "section.dynamic", sections[5].Key)
	require.Equal(t, reactloops.PromptSectionRoleHighStatic, sections[0].Role)
	require.Equal(t, reactloops.PromptSectionRoleZHHighStatic, sections[0].RoleZh)
	require.Equal(t, reactloops.PromptSectionRoleFrozenBlock, sections[1].Role)
	require.Equal(t, reactloops.PromptSectionRoleSemiDynamic1, sections[2].Role)
	require.Equal(t, reactloops.PromptSectionRoleZHSemiDynamic1, sections[2].RoleZh)
	require.Equal(t, reactloops.PromptSectionRoleSemiDynamic2, sections[3].Role)
	require.Equal(t, reactloops.PromptSectionRoleZHSemiDynamic2, sections[3].RoleZh)
	require.Equal(t, reactloops.PromptSectionRoleTimelineOpen, sections[4].Role)
	require.Equal(t, reactloops.PromptSectionRoleDynamic, sections[5].Role)

	prompt := result.Prompt
	traitsIdx := strings.Index(prompt, "<|TRAITS|>")
	workspaceIdx := strings.Index(prompt, "# Workspace Context")
	timelineIdx := strings.Index(prompt, "# Timeline Memory")
	currentTimeIdx := strings.Index(prompt, "# Current Time")
	toolInventoryIdx := strings.Index(prompt, "# Tool Inventory")
	skillsIdx := strings.Index(prompt, "<|SKILLS_CONTEXT_skills_context|>")
	persistentIdx := strings.Index(prompt, "<|PERSISTENT|>")
	schemaIdx := strings.Index(prompt, "<|SCHEMA|>")
	frozenStartIdx := strings.Index(prompt, "<|AI_CACHE_FROZEN_semi-dynamic|>")
	frozenEndIdx := strings.Index(prompt, "<|AI_CACHE_FROZEN_END_semi-dynamic|>")
	semiSection1Idx := strings.Index(prompt, "<|PROMPT_SECTION_semi-dynamic-1|>")
	semiSection2Idx := strings.Index(prompt, "<|PROMPT_SECTION_semi-dynamic-2|>")
	timelineOpenSectionIdx := strings.Index(prompt, "<|PROMPT_SECTION_timeline-open|>")
	userQueryIdx := strings.Index(prompt, "<|USER_QUERY_n123|>")
	autoCtxIdx := strings.Index(prompt, "<|AUTO_PROVIDE_CTX_[n123_provider_one]_START key=provider-one|>")
	prevUserInputIdx := strings.Index(prompt, "<|PREV_USER_INPUT_n123|>")

	require.NotEqual(t, -1, traitsIdx)
	require.NotEqual(t, -1, workspaceIdx)
	require.NotEqual(t, -1, timelineIdx)
	require.NotEqual(t, -1, currentTimeIdx)
	require.NotEqual(t, -1, toolInventoryIdx)
	require.NotEqual(t, -1, skillsIdx)
	require.NotEqual(t, -1, persistentIdx)
	require.NotEqual(t, -1, schemaIdx)
	require.NotEqual(t, -1, frozenStartIdx)
	require.NotEqual(t, -1, frozenEndIdx)
	require.NotEqual(t, -1, semiSection1Idx)
	require.NotEqual(t, -1, semiSection2Idx)
	require.NotEqual(t, -1, timelineOpenSectionIdx)
	require.NotEqual(t, -1, userQueryIdx)
	require.NotEqual(t, -1, autoCtxIdx)
	require.NotEqual(t, -1, prevUserInputIdx)

	// 段顺序 (P1.1 拆 semi 后): TRAITS -> AI_CACHE_FROZEN(START) ->
	// Tool/Forge/Timeline-frozen -> AI_CACHE_FROZEN(END) ->
	// PROMPT_SECTION_semi-dynamic-1 (Skills + CacheToolCall) ->
	// PROMPT_SECTION_semi-dynamic-2 (Persistent + Schema + OutputExample) ->
	// PROMPT_SECTION_timeline-open (Timeline open + Time + Workspace +
	// SessionEvidence + PREV_USER_INPUT) -> Dynamic (UserQuery + AutoCtx + ...)
	require.Less(t, traitsIdx, frozenStartIdx)
	require.Less(t, frozenStartIdx, toolInventoryIdx)
	require.Less(t, toolInventoryIdx, frozenEndIdx)
	require.Less(t, frozenEndIdx, semiSection1Idx)
	require.Less(t, semiSection1Idx, skillsIdx)
	require.Less(t, skillsIdx, semiSection2Idx)
	require.Less(t, semiSection2Idx, persistentIdx)
	require.Less(t, persistentIdx, schemaIdx)
	require.Less(t, schemaIdx, timelineOpenSectionIdx)
	// timelineIdx 是首次出现的 "# Timeline Memory", 此用例下 frozen 段无 timeline
	// 内容 (单 timeline 事件全部落在最末桶), 因此首次出现位置必在 timeline_open 段。
	require.Less(t, timelineOpenSectionIdx, timelineIdx)
	require.Less(t, timelineIdx, currentTimeIdx)
	require.Less(t, currentTimeIdx, workspaceIdx)
	// P1-C2: PREV_USER_INPUT 已上移到 timeline-open 段, 排在 workspace 之后,
	// userQuery (dynamic 段) 之前.
	require.Less(t, workspaceIdx, prevUserInputIdx)
	require.Less(t, prevUserInputIdx, userQueryIdx)
	require.Less(t, userQueryIdx, autoCtxIdx)
	require.Contains(t, prompt, "<|PERSISTENT|>")
	require.Contains(t, prompt, "<|OUTPUT_EXAMPLE|>")
	require.Contains(t, prompt, "<|SCHEMA|>")
	require.NotContains(t, prompt, "<|SCHEMA_n123|>")

	// OUTPUT_EXAMPLE 必须出现在 semi-dynamic 段 (schema 之后, timeline-open 之前),
	// 不允许再回到 high-static 段污染缓存边界.
	// 关键词: OUTPUT_EXAMPLE 段位置断言, high-static 反污染验证
	outputExampleIdx := strings.Index(prompt, "<|OUTPUT_EXAMPLE|>")
	require.NotEqual(t, -1, outputExampleIdx)
	require.Less(t, schemaIdx, outputExampleIdx)
	require.Less(t, outputExampleIdx, timelineOpenSectionIdx)

	// frozen_block 段子结构: tool_inventory + forge_inventory (本用例 forge 关闭) +
	// timeline_frozen (本用例为空)。filterIncludedPromptSections 会过滤空段,
	// 故实际只剩 tool_inventory 一个 child。
	// 关键词: frozen_block children Name 去前缀
	require.NotEmpty(t, sections[1].Children)
	require.Equal(t, "section.frozen_block.tool_inventory", sections[1].Children[0].Key)
	require.Equal(t, "Tool Inventory", sections[1].Children[0].Label)
	require.Equal(t, reactloops.PromptSectionRoleFrozenBlock, sections[1].Children[0].Role)

	// semi_dynamic_1 段子结构: skills_context (本用例 RecentToolsCache 为空被过滤).
	// 关键词: semi_dynamic_1 children, skills_context, Name 去前缀
	require.Len(t, sections[2].Children, 1)
	require.Equal(t, "section.semi_dynamic_1.skills_context", sections[2].Children[0].Key)
	require.Equal(t, "Skills Context", sections[2].Children[0].Label)
	require.Equal(t, reactloops.PromptSectionRoleSemiDynamic1, sections[2].Children[0].Role)

	// semi_dynamic_2 段子结构: task_instruction + schema + output_example
	// (TaskInstruction / OutputExample 从 high_static 迁入 semi-dynamic-2,
	// 渲染顺序见 semi_dynamic_section_2.txt)。
	// 关键词: semi_dynamic_2 children, output_example/task_instruction 迁入断言, Name 去前缀
	require.Len(t, sections[3].Children, 3)
	require.Equal(t, "section.semi_dynamic_2.task_instruction", sections[3].Children[0].Key)
	require.Equal(t, "Task Instruction", sections[3].Children[0].Label)
	require.Equal(t, "section.semi_dynamic_2.schema", sections[3].Children[1].Key)
	require.Equal(t, "Schema", sections[3].Children[1].Label)
	require.Equal(t, "section.semi_dynamic_2.output_example", sections[3].Children[2].Key)
	require.Equal(t, "Output Example", sections[3].Children[2].Label)
	require.Equal(t, reactloops.PromptSectionRoleSemiDynamic2, sections[3].Children[0].Role)
	require.Equal(t, reactloops.PromptSectionRoleSemiDynamic2, sections[3].Children[2].Role)

	// timeline_open 段子结构 (P1-C2): timeline_open + current_time + workspace +
	// session_evidence (本用例无 SessionEvidence -> 不出现) + user_history.
	// 关键词: timeline_open children Name 去前缀
	require.GreaterOrEqual(t, len(sections[4].Children), 4)
	require.Equal(t, "section.timeline_open.timeline_open", sections[4].Children[0].Key)
	require.Equal(t, "Timeline (Open Tail)", sections[4].Children[0].Label)
	require.Equal(t, "section.timeline_open.current_time", sections[4].Children[1].Key)
	require.Equal(t, "Current Time", sections[4].Children[1].Label)
	require.Equal(t, "section.timeline_open.workspace", sections[4].Children[2].Key)
	require.Equal(t, "Workspace", sections[4].Children[2].Label)
	// P1-C2: user_history 现在挂在 timeline_open 之下而非 dynamic 之下.
	require.Equal(t, "section.timeline_open.user_history", sections[4].Children[3].Key)
	require.Equal(t, "User History", sections[4].Children[3].Label)
	require.Equal(t, reactloops.PromptSectionRoleTimelineOpen, sections[4].Children[3].Role)

	require.GreaterOrEqual(t, len(sections[5].Children), 2)
	require.Equal(t, "section.dynamic.user_query", sections[5].Children[0].Key)
	require.Equal(t, "User Query", sections[5].Children[0].Label)
	require.Equal(t, "section.dynamic.auto_context", sections[5].Children[1].Key)
	require.Equal(t, "Auto Context", sections[5].Children[1].Label)
	require.Equal(t, reactloops.PromptSectionRoleDynamic, sections[5].Children[0].Role)
	require.Equal(t, reactloops.PromptSectionRoleZHDynamic, sections[5].Children[0].RoleZh)
}

// TestPromptManager_RenderLoopSemiDynamic1Section_Order 验证 SEMI-1 段
// (semi_dynamic_section_1.txt) 仅包含 SkillsContext + RecentToolsCache, 不含
// Schema / Persistent / OutputExample / Tool / Forge.
//
// 关键词: renderLoopSemiDynamic1Section, semi_dynamic_section_1 内容范围, P1.1
func TestPromptManager_RenderLoopSemiDynamic1Section_Order(t *testing.T) {
	react, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"object","next_action":{"type":"directly_answer","answer_payload":"ok"},"cumulative_summary":"ok","human_readable_thought":"ok"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)

	rendered, err := react.promptManager.renderLoopSemiDynamic1Section(&reactloops.PromptPrefixMaterials{
		ToolInventory:    true,
		ToolsCount:       2,
		TopToolsCount:    1,
		TopTools:         []*aitool.Tool{aitool.NewWithoutCallback("tool-a", aitool.WithDescription("tool a desc"))},
		HasMoreTools:     true,
		ForgeInventory:   true,
		AIForgeList:      "* `forge-a`: forge a desc",
		SkillsContext:    "<|SKILLS_CONTEXT_demo|>\nskill body\n<|SKILLS_CONTEXT_END_demo|>",
		RecentToolsCache: "<|CACHE_TOOL_CALL_[current-nonce]|>\ncache body\n<|CACHE_TOOL_CALL_END_[current-nonce]|>",
		Schema:           `{"type":"object","properties":{"@action":{"type":"string"}}}`,
		TaskInstruction:  "follow task rules",
		OutputExample:    "example output",
	})
	require.NoError(t, err)

	skillsIdx := strings.Index(rendered, "<|SKILLS_CONTEXT_demo|>")
	cacheIdx := strings.Index(rendered, "<|CACHE_TOOL_CALL_[current-nonce]|>")
	require.NotEqual(t, -1, skillsIdx)
	require.NotEqual(t, -1, cacheIdx)
	require.Less(t, skillsIdx, cacheIdx)
	// SEMI-1 段绝对不能含 Schema / Persistent / OutputExample / Tool / Forge.
	require.NotContains(t, rendered, "<|SCHEMA|>")
	require.NotContains(t, rendered, "<|PERSISTENT|>")
	require.NotContains(t, rendered, "<|OUTPUT_EXAMPLE|>")
	require.NotContains(t, rendered, "# Tool Inventory")
	require.NotContains(t, rendered, "# AI Blueprint Inventory")
}

// TestPromptManager_RenderLoopSemiDynamic2Section_Order 验证 SEMI-2 段
// (semi_dynamic_section_2.txt) 渲染顺序为 Persistent -> Schema -> OutputExample,
// 且不含 SkillsContext / CacheToolCall / Tool / Forge.
//
// 关键词: renderLoopSemiDynamic2Section, semi_dynamic_section_2 顺序, P1.1
func TestPromptManager_RenderLoopSemiDynamic2Section_Order(t *testing.T) {
	react, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"object","next_action":{"type":"directly_answer","answer_payload":"ok"},"cumulative_summary":"ok","human_readable_thought":"ok"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)

	rendered, err := react.promptManager.renderLoopSemiDynamic2Section(&reactloops.PromptPrefixMaterials{
		SkillsContext:    "<|SKILLS_CONTEXT_demo|>\nskill body\n<|SKILLS_CONTEXT_END_demo|>",
		RecentToolsCache: "<|CACHE_TOOL_CALL_[current-nonce]|>\ncache body\n<|CACHE_TOOL_CALL_END_[current-nonce]|>",
		Schema:           `{"type":"object","properties":{"@action":{"type":"string"}}}`,
		TaskInstruction:  "follow task rules",
		OutputExample:    "example output",
	})
	require.NoError(t, err)

	persistentIdx := strings.Index(rendered, "<|PERSISTENT|>")
	schemaIdx := strings.Index(rendered, "<|SCHEMA|>")
	outputExampleIdx := strings.Index(rendered, "<|OUTPUT_EXAMPLE|>")
	require.NotEqual(t, -1, persistentIdx)
	require.NotEqual(t, -1, schemaIdx)
	require.NotEqual(t, -1, outputExampleIdx)
	require.Less(t, persistentIdx, schemaIdx)
	require.Less(t, schemaIdx, outputExampleIdx)
	// SEMI-2 段绝对不能含 SkillsContext / CacheToolCall / Tool / Forge.
	require.NotContains(t, rendered, "<|SKILLS_CONTEXT_demo|>")
	require.NotContains(t, rendered, "<|CACHE_TOOL_CALL_[current-nonce]|>")
	require.NotContains(t, rendered, "# Tool Inventory")
	require.NotContains(t, rendered, "# AI Blueprint Inventory")
}

// TestPromptManager_RenderLoopFrozenBlockSection_Order 验证 FrozenBlock 段
// (frozen_block_section.txt) 渲染顺序为 Tool Inventory -> Forge Inventory ->
// Timeline (Frozen Prefix), 且不包含 Skills/Schema。
//
// 关键词: renderLoopFrozenBlockSection, Tool/Forge/Timeline-frozen 顺序
func TestPromptManager_RenderLoopFrozenBlockSection_Order(t *testing.T) {
	react, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"object","next_action":{"type":"directly_answer","answer_payload":"ok"},"cumulative_summary":"ok","human_readable_thought":"ok"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)

	rendered, err := react.promptManager.renderLoopFrozenBlockSection(&reactloops.PromptPrefixMaterials{
		ToolInventory:  true,
		ToolsCount:     2,
		TopToolsCount:  1,
		TopTools:       []*aitool.Tool{aitool.NewWithoutCallback("tool-a", aitool.WithDescription("tool a desc"))},
		HasMoreTools:   true,
		ForgeInventory: true,
		AIForgeList:    "* `forge-a`: forge a desc",
		TimelineFrozen: "<|TIMELINE_r1t100|>\nfrozen reducer body\n<|TIMELINE_END_r1t100|>",
	})
	require.NoError(t, err)

	toolIdx := strings.Index(rendered, "# Tool Inventory")
	forgeIdx := strings.Index(rendered, "# AI Blueprint Inventory")
	timelineFrozenIdx := strings.Index(rendered, "# Timeline Memory (Frozen Prefix)")
	require.NotEqual(t, -1, toolIdx)
	require.NotEqual(t, -1, forgeIdx)
	require.NotEqual(t, -1, timelineFrozenIdx)
	require.Less(t, toolIdx, forgeIdx)
	require.Less(t, forgeIdx, timelineFrozenIdx)
	require.NotContains(t, rendered, "<|SKILLS_CONTEXT_")
	require.NotContains(t, rendered, "<|SCHEMA|>")
}

// TestPromptManager_AssemblePromptPrefix 验证 5 段 prefix 输出 (high_static +
// frozen_block + semi_dynamic_1 + semi_dynamic_2 + timeline_open), 且 Prompt
// 字段拼接顺序正确。
//
// 关键词: AssemblePromptPrefix, 5 段, 按稳定性分层, P1.1
func TestPromptManager_AssemblePromptPrefix(t *testing.T) {
	react, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"object","next_action":{"type":"directly_answer","answer_payload":"ok"},"cumulative_summary":"ok","human_readable_thought":"ok"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)

	tool := aitool.NewWithoutCallback("tool-a", aitool.WithDescription("tool a desc"))
	base, err := react.promptManager.GetLoopPromptBaseMaterials([]*aitool.Tool{tool}, "pfx123")
	require.NoError(t, err)

	prefix, err := react.promptManager.AssemblePromptPrefix(
		react.promptManager.NewPromptMaterials(base, &reactloops.LoopPromptAssemblyInput{
			Nonce:           "pfx123",
			TaskInstruction: "follow task rules",
			OutputExample:   "example output",
			Schema:          `{"type":"object","properties":{"@action":{"type":"string"}}}`,
		}),
	)
	require.NoError(t, err)
	require.NotNil(t, prefix)
	require.Len(t, prefix.Sections, 5)
	require.Contains(t, prefix.Prompt, "<|TRAITS|>")
	require.Contains(t, prefix.Prompt, "<|SCHEMA|>")
	require.Equal(t, "section.high_static", prefix.Sections[0].Key)
	require.Equal(t, "section.frozen_block", prefix.Sections[1].Key)
	require.Equal(t, "section.semi_dynamic_1", prefix.Sections[2].Key)
	require.Equal(t, "section.semi_dynamic_2", prefix.Sections[3].Key)
	require.Equal(t, "section.timeline_open", prefix.Sections[4].Key)
}

// TestPromptManager_AssembleLoopPrompt_HijackFiveSegment 验证 aireact 主路径
// 产出的 prompt (SYSTEM + FROZEN + SEMI-1 + SEMI-2 + OPEN + DYNAMIC, 三 cache
// 边界齐全) 经 aicache.Observe 后被 hijacker 切成 5 段:
//   - system: 含 AI_CACHE_SYSTEM_high-static 包装, 主动 cc
//   - user1: 含 AI_CACHE_FROZEN_semi-dynamic 完整闭合块 (Tool/Forge/Timeline-frozen),
//     字节边界稳定, 主动 cc
//   - user2: 含 AI_CACHE_SEMI_semi 完整闭合块 (PROMPT_SECTION_semi-dynamic-1 +
//     Skills + CacheToolCall), 字节边界稳定, *不* 打 cc (string content)
//   - user3: 含 AI_CACHE_SEMI2_semi 完整闭合块 (PROMPT_SECTION_semi-dynamic-2 +
//     TaskInstruction + Schema + OutputExample), 字节边界稳定, 主动 cc
//   - user4: 含 PROMPT_SECTION_timeline-open + Dynamic 段, 不打 cc
//
// 这是 P1.1 三 cache 边界的核心收益: dashscope 同时命中 system 短前缀、
// system+frozen 长前缀、system+frozen+semi-1+semi-2 最长前缀三档候选, semi-1
// 不打 cc 但前缀仍跨过其字节序列直达 semi-2 cc 锚点 (合并 prefix cache).
//
// 关键词: aicache hijack 5 段, AI_CACHE_FROZEN + AI_CACHE_SEMI + AI_CACHE_SEMI2
//
//	三边界, 三 cc 主路径, P1.1 拆 semi
func TestPromptManager_AssembleLoopPrompt_HijackFiveSegment(t *testing.T) {
	// P2.1 阈值合并默认 1024 byte 会把本测试的短 fixture (Tool Inventory 仅
	// 一个 tool, 总字节数 << 1KB) 合并降级到 2 段, 与本测试断言的 5 段 happy
	// path 不符. 显式关闭阈值合并以验证字节边界结构.
	// 关键词: P2.1 阈值合并跨包关闭, aicache test helper, 5 段结构验证
	restore := aicache.SetMinCachableUserSegmentBytesForTest(0)
	defer restore()

	react, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"object","next_action":{"type":"directly_answer","answer_payload":"ok"},"cumulative_summary":"ok","human_readable_thought":"ok"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)

	tool := aitool.NewWithoutCallback("tool-a", aitool.WithDescription("tool a desc"))
	react.AddToTimeline("test", "timeline content")

	result, err := react.promptManager.AssembleLoopPrompt([]*aitool.Tool{tool}, &reactloops.LoopPromptAssemblyInput{
		Nonce:           "hj01",
		UserQuery:       "user query body",
		TaskInstruction: "follow task rules",
		OutputExample:   "example output",
		Schema:          `{"type":"object","properties":{"@action":{"type":"string"}}}`,
		SkillsContext:   "<|SKILLS_CONTEXT_skills_context|>\nloaded skill\n<|SKILLS_CONTEXT_END_skills_context|>",
	})
	require.NoError(t, err)

	hijack := aicache.Observe("test-model", result.Prompt)
	require.NotNil(t, hijack, "loop prompt with high-static + frozen + semi-1 + semi-2 blocks should be hijacked")
	require.True(t, hijack.IsHijacked)
	require.Len(t, hijack.Messages, 5, "expect 5-segment hijack (system + user1 + user2 + user3 + user4)")

	systemMsg := hijack.Messages[0]
	require.Equal(t, "system", systemMsg.Role)
	systemContent := chatDetailContentString(systemMsg)
	require.Contains(t, systemContent, "<|AI_CACHE_SYSTEM_high-static|>")
	require.Contains(t, systemContent, "<|AI_CACHE_SYSTEM_END_high-static|>")
	// system 主动打 ephemeral cc -> Content 类型为 []*ChatContent.
	requireMessageHasCacheControl(t, systemMsg, "system")

	user1 := hijack.Messages[1]
	require.Equal(t, "user", user1.Role)
	user1Content := chatDetailContentString(user1)
	require.Contains(t, user1Content, "<|AI_CACHE_FROZEN_semi-dynamic|>")
	require.Contains(t, user1Content, "<|AI_CACHE_FROZEN_END_semi-dynamic|>")
	require.Contains(t, user1Content, "# Tool Inventory")
	require.True(t, strings.HasSuffix(strings.TrimSpace(user1Content), "<|AI_CACHE_FROZEN_END_semi-dynamic|>"),
		"user1 should end at frozen boundary END tag, got: %q", user1Content)
	requireMessageHasCacheControl(t, user1, "user1 frozen")

	user2 := hijack.Messages[2]
	require.Equal(t, "user", user2.Role)
	user2Content := chatDetailContentString(user2)
	require.Contains(t, user2Content, "<|AI_CACHE_SEMI_semi|>",
		"user2 must contain AI_CACHE_SEMI START tag")
	require.Contains(t, user2Content, "<|AI_CACHE_SEMI_END_semi|>",
		"user2 must contain AI_CACHE_SEMI END tag")
	require.Contains(t, user2Content, "<|PROMPT_SECTION_semi-dynamic-1|>",
		"user2 must contain inner PROMPT_SECTION_semi-dynamic-1 wrapper")
	require.Contains(t, user2Content, "<|SKILLS_CONTEXT_skills_context|>",
		"user2 must contain SkillsContext")
	require.True(t, strings.HasSuffix(strings.TrimSpace(user2Content), "<|AI_CACHE_SEMI_END_semi|>"),
		"user2 should end at semi-1 boundary END tag, got: %q", user2Content)
	require.NotContains(t, user2Content, "<|SCHEMA|>",
		"user2 (semi-1) must NOT contain Schema (belongs to semi-2)")
	require.NotContains(t, user2Content, "<|PERSISTENT|>",
		"user2 (semi-1) must NOT contain Persistent (belongs to semi-2)")
	require.NotContains(t, user2Content, "<|PROMPT_SECTION_timeline-open|>",
		"user2 must NOT contain timeline-open section (belongs to user4)")
	// user2 (semi-1) 物理上不打 cc, Content 类型应为 string.
	requireMessageHasNoCacheControl(t, user2, "user2 semi-1")

	user3 := hijack.Messages[3]
	require.Equal(t, "user", user3.Role)
	user3Content := chatDetailContentString(user3)
	require.Contains(t, user3Content, "<|AI_CACHE_SEMI2_semi|>",
		"user3 must contain AI_CACHE_SEMI2 START tag")
	require.Contains(t, user3Content, "<|AI_CACHE_SEMI2_END_semi|>",
		"user3 must contain AI_CACHE_SEMI2 END tag")
	require.Contains(t, user3Content, "<|PROMPT_SECTION_semi-dynamic-2|>",
		"user3 must contain inner PROMPT_SECTION_semi-dynamic-2 wrapper")
	require.Contains(t, user3Content, "<|SCHEMA|>",
		"user3 must contain Schema")
	require.True(t, strings.HasSuffix(strings.TrimSpace(user3Content), "<|AI_CACHE_SEMI2_END_semi|>"),
		"user3 should end at semi-2 boundary END tag, got: %q", user3Content)
	require.NotContains(t, user3Content, "<|PROMPT_SECTION_timeline-open|>",
		"user3 must NOT contain timeline-open section (belongs to user4)")
	require.NotContains(t, user3Content, "<|USER_QUERY_hj01|>",
		"user3 must NOT contain dynamic user query (belongs to user4)")
	requireMessageHasCacheControl(t, user3, "user3 semi-2")

	user4 := hijack.Messages[4]
	require.Equal(t, "user", user4.Role)
	user4Content := chatDetailContentString(user4)
	require.Contains(t, user4Content, "<|PROMPT_SECTION_timeline-open|>")
	require.Contains(t, user4Content, "<|USER_QUERY_hj01|>")
	require.NotContains(t, user4Content, "<|AI_CACHE_FROZEN_semi-dynamic|>",
		"user4 must NOT contain frozen START tag")
	require.NotContains(t, user4Content, "<|AI_CACHE_SEMI_semi|>",
		"user4 must NOT contain semi-1 START tag")
	require.NotContains(t, user4Content, "<|AI_CACHE_SEMI2_semi|>",
		"user4 must NOT contain semi-2 START tag")
	requireMessageHasNoCacheControl(t, user4, "user4 open+dynamic")
}

func TestPromptManager_AssembleLoopPrompt_EmptySemiDynamic1StillKeepsWrapper(t *testing.T) {
	restore := aicache.SetMinCachableUserSegmentBytesForTest(0)
	defer restore()

	react, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"object","next_action":{"type":"directly_answer","answer_payload":"ok"},"cumulative_summary":"ok","human_readable_thought":"ok"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)

	result, err := react.promptManager.AssembleLoopPrompt(nil, &reactloops.LoopPromptAssemblyInput{
		Nonce:           "emptysemi1",
		UserQuery:       "user query body",
		TaskInstruction: "follow task rules",
		OutputExample:   "example output",
		Schema:          `{"type":"object","properties":{"@action":{"type":"string"}}}`,
	})
	require.NoError(t, err)

	require.Contains(t, result.Prompt, "<|AI_CACHE_SEMI_semi|>")
	require.Contains(t, result.Prompt, "<|AI_CACHE_SEMI_END_semi|>")
	require.Contains(t, result.Prompt, "<|PROMPT_SECTION_semi-dynamic-1|>")
	require.Contains(t, result.Prompt, "<|PROMPT_SECTION_END_semi-dynamic-1|>")

	split := aicache.Split(result.Prompt)
	require.NotNil(t, split)
	sectionsBySection := make(map[string]int)
	for _, c := range split.Chunks {
		sectionsBySection[c.Section]++
	}
	require.Equal(t, 1, sectionsBySection[aicache.SectionSemiDynamic1])
	require.Zero(t, sectionsBySection[aicache.SectionRaw])
}

// requireMessageHasCacheControl 断言 ChatDetail 主动打了 ephemeral cc:
// hijacker 用 wrapTextWithEphemeralCC 把 cc 段挂在 []*ChatContent 上, 字段
// CacheControl 形如 map[string]any{"type": "ephemeral"}; 没有 cc 的消息 Content
// 是 string. 测试通过类型判别校验路由结果.
//
// 关键词: hijack 测试 helper, ephemeral cache_control 类型断言
func requireMessageHasCacheControl(t *testing.T, detail aispec.ChatDetail, label string) {
	t.Helper()
	parts, ok := detail.Content.([]*aispec.ChatContent)
	require.True(t, ok, "%s should have []*ChatContent (cc-wrapped) Content, got %T", label, detail.Content)
	require.NotEmpty(t, parts, "%s ChatContent slice should not be empty", label)
	hasCC := false
	for _, p := range parts {
		if p == nil {
			continue
		}
		if p.CacheControl == nil {
			continue
		}
		ccMap, ok := p.CacheControl.(map[string]any)
		if !ok {
			continue
		}
		if ccMap["type"] == "ephemeral" {
			hasCC = true
			break
		}
	}
	require.True(t, hasCC, "%s should have at least one ChatContent part with ephemeral cache_control", label)
}

// requireMessageHasNoCacheControl 断言 ChatDetail 没有打 cc:
// hijacker 用 aispec.NewUserChatDetail 直接给 string Content, 不带 cc.
// 测试通过类型判别校验路由结果.
//
// 关键词: hijack 测试 helper, no cache_control 类型断言
func requireMessageHasNoCacheControl(t *testing.T, detail aispec.ChatDetail, label string) {
	t.Helper()
	switch v := detail.Content.(type) {
	case string:
		// 裸 string content, 物理上不可能携带 cc, 通过.
	case []*aispec.ChatContent:
		for _, p := range v {
			if p == nil {
				continue
			}
			require.Nil(t, p.CacheControl, "%s ChatContent must not carry CacheControl", label)
		}
	default:
		require.Failf(t, "unexpected ChatDetail.Content type", "%s: got %T", label, detail.Content)
	}
}

// TestPromptManager_AssembleLoopPrompt_RecentToolsCacheInSemiSegment 验证
// CACHE_TOOL_CALL 块 (经 LoopPromptAssemblyInput.RecentToolsCache 透传) 物理位置
// 在 semi-dynamic-1 段 (而不再在 dynamic 段, 也不在 semi-dynamic-2 段),
// 经 hijacker 切割后位于 user2.
//
// 关键词: TestPromptManager, CACHE_TOOL_CALL 物理迁移, semi-dynamic-1 段, P1.1 主路径
func TestPromptManager_AssembleLoopPrompt_RecentToolsCacheInSemiSegment(t *testing.T) {
	react, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"object","next_action":{"type":"directly_answer","answer_payload":"ok"},"cumulative_summary":"ok","human_readable_thought":"ok"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)

	tool := aitool.NewWithoutCallback("tool-a", aitool.WithDescription("tool a desc"))

	// 模拟 reactloops/prompt.go 中给 RecentToolsCache 注入的 CACHE_TOOL_CALL 块,
	// 用占位符字面量 nonce "[current-nonce]" 渲染, 跨轮字节稳定.
	// 关键词: CACHE_TOOL_CALL 物理迁移, [current-nonce] 占位符 nonce
	cacheBlock := strings.Join([]string{
		"<|DIRECT_TOOL_ROUTING_[current-nonce]|>",
		"# Fast Tool Routing",
		"Recent tools available via directly_call_tool.",
		"<|DIRECT_TOOL_ROUTING_END_[current-nonce]|>",
		"",
		"<|CACHE_TOOL_CALL_[current-nonce]|>",
		"<|TOOL_bash_[current-nonce]|>",
		"## Tool: bash",
		"<|TOOL_bash_END_[current-nonce]|>",
		"<|CACHE_TOOL_CALL_END_[current-nonce]|>",
	}, "\n")

	result, err := react.promptManager.AssembleLoopPrompt([]*aitool.Tool{tool}, &reactloops.LoopPromptAssemblyInput{
		Nonce:            "turnA",
		UserQuery:        "user query",
		TaskInstruction:  "task rules",
		Schema:           `{"type":"object"}`,
		SkillsContext:    "<|SKILLS_CONTEXT_demo|>\nskill\n<|SKILLS_CONTEXT_END_demo|>",
		RecentToolsCache: cacheBlock,
	})
	require.NoError(t, err)

	prompt := result.Prompt

	// 1. CACHE_TOOL_CALL 必须出现在 prompt 中 (经过模板渲染)
	cacheStartIdx := strings.Index(prompt, "<|CACHE_TOOL_CALL_[current-nonce]|>")
	cacheEndIdx := strings.Index(prompt, "<|CACHE_TOOL_CALL_END_[current-nonce]|>")
	require.NotEqual(t, -1, cacheStartIdx, "CACHE_TOOL_CALL must appear in prompt")
	require.NotEqual(t, -1, cacheEndIdx)

	// 2. 必须位于 PROMPT_SECTION_semi-dynamic-1 段内 (P1.1: CACHE_TOOL_CALL 落在
	//    semi 第一块, 不在 semi-2 / dynamic 段).
	semi1Start := strings.Index(prompt, "<|PROMPT_SECTION_semi-dynamic-1|>")
	semi1End := strings.Index(prompt, "<|PROMPT_SECTION_END_semi-dynamic-1|>")
	require.NotEqual(t, -1, semi1Start)
	require.NotEqual(t, -1, semi1End)
	require.Greater(t, cacheStartIdx, semi1Start, "CACHE_TOOL_CALL must start AFTER semi-dynamic-1 START")
	require.Less(t, cacheEndIdx, semi1End, "CACHE_TOOL_CALL must end BEFORE semi-dynamic-1 END")

	// 3. semi-dynamic-2 段绝对不能含 CACHE_TOOL_CALL (P1.1 拆分边界)
	semi2Start := strings.Index(prompt, "<|PROMPT_SECTION_semi-dynamic-2|>")
	semi2End := strings.Index(prompt, "<|PROMPT_SECTION_END_semi-dynamic-2|>")
	require.NotEqual(t, -1, semi2Start)
	require.NotEqual(t, -1, semi2End)
	semi2Body := prompt[semi2Start:semi2End]
	require.NotContains(t, semi2Body, "<|CACHE_TOOL_CALL_[current-nonce]|>",
		"CACHE_TOOL_CALL must NOT appear in semi-dynamic-2 (belongs to semi-dynamic-1)")

	// 4. dynamic 段不应该再含 CACHE_TOOL_CALL (历史位置)
	dynamicStart := strings.Index(prompt, "<|PROMPT_SECTION_dynamic_turnA|>")
	require.NotEqual(t, -1, dynamicStart)
	dynamicTail := prompt[dynamicStart:]
	require.NotContains(t, dynamicTail, "<|CACHE_TOOL_CALL_[current-nonce]|>",
		"CACHE_TOOL_CALL must NOT remain in dynamic section after physical migration")

	// 5. semi-1 段必须被 AI_CACHE_SEMI_semi 边界包裹 (P1.1 第一对 cache 边界)
	aiCacheSemiStart := strings.Index(prompt, "<|AI_CACHE_SEMI_semi|>")
	aiCacheSemiEnd := strings.Index(prompt, "<|AI_CACHE_SEMI_END_semi|>")
	require.NotEqual(t, -1, aiCacheSemiStart, "P1.1: prompt must contain AI_CACHE_SEMI_semi START")
	require.NotEqual(t, -1, aiCacheSemiEnd, "P1.1: prompt must contain AI_CACHE_SEMI_semi END")
	require.Less(t, aiCacheSemiStart, semi1Start,
		"AI_CACHE_SEMI START must wrap PROMPT_SECTION_semi-dynamic-1")
	require.Greater(t, aiCacheSemiEnd, semi1End,
		"AI_CACHE_SEMI END must wrap PROMPT_SECTION_semi-dynamic-1")

	// 6. semi-2 段必须被 AI_CACHE_SEMI2_semi 边界包裹 (P1.1 第二对 cache 边界)
	aiCacheSemi2Start := strings.Index(prompt, "<|AI_CACHE_SEMI2_semi|>")
	aiCacheSemi2End := strings.Index(prompt, "<|AI_CACHE_SEMI2_END_semi|>")
	require.NotEqual(t, -1, aiCacheSemi2Start, "P1.1: prompt must contain AI_CACHE_SEMI2_semi START")
	require.NotEqual(t, -1, aiCacheSemi2End, "P1.1: prompt must contain AI_CACHE_SEMI2_semi END")
	require.Less(t, aiCacheSemi2Start, semi2Start,
		"AI_CACHE_SEMI2 START must wrap PROMPT_SECTION_semi-dynamic-2")
	require.Greater(t, aiCacheSemi2End, semi2End,
		"AI_CACHE_SEMI2 END must wrap PROMPT_SECTION_semi-dynamic-2")
}

// TestPromptManager_AssembleLoopPrompt_SemiSegmentByteStableAcrossTurns 验证
// 在 turn nonce 不同的两次 AssembleLoopPrompt 调用中, semi-dynamic-1 与
// semi-dynamic-2 段都跨 turn 字节稳定 (因为 CACHE_TOOL_CALL 已用稳定字面量 nonce
// 渲染, Schema / Persistent / OutputExample 不依赖 turn nonce).
//
// 这是 P1.1 三 cache 边界生效的前提: hijacker 切到 user2 (semi-1) 与 user3
// (semi-2) 的字节流必须跨 turn 一致, 才能命中 dashscope prefix cache.
//
// 关键词: TestPromptManager, semi-dynamic-1/2 字节稳定, 跨 turn 一致, P1.1 cache 命中
func TestPromptManager_AssembleLoopPrompt_SemiSegmentByteStableAcrossTurns(t *testing.T) {
	react, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"object","next_action":{"type":"directly_answer","answer_payload":"ok"},"cumulative_summary":"ok","human_readable_thought":"ok"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)

	tool := aitool.NewWithoutCallback("tool-a", aitool.WithDescription("tool a desc"))

	cacheBlock := strings.Join([]string{
		"<|DIRECT_TOOL_ROUTING_[current-nonce]|>",
		"# Fast Tool Routing",
		"<|DIRECT_TOOL_ROUTING_END_[current-nonce]|>",
		"<|CACHE_TOOL_CALL_[current-nonce]|>",
		"<|TOOL_bash_[current-nonce]|>",
		"## Tool: bash",
		"<|TOOL_bash_END_[current-nonce]|>",
		"<|CACHE_TOOL_CALL_END_[current-nonce]|>",
	}, "\n")

	mk := func(turnNonce string, userQuery string) string {
		result, err := react.promptManager.AssembleLoopPrompt([]*aitool.Tool{tool}, &reactloops.LoopPromptAssemblyInput{
			Nonce:            turnNonce,
			UserQuery:        userQuery,
			TaskInstruction:  "task rules",
			Schema:           `{"type":"object"}`,
			SkillsContext:    "<|SKILLS_CONTEXT_demo|>\nskill\n<|SKILLS_CONTEXT_END_demo|>",
			RecentToolsCache: cacheBlock,
		})
		require.NoError(t, err)
		return result.Prompt
	}

	prompt1 := mk("nonce_round1", "first query")
	prompt2 := mk("nonce_round2_completely_different", "second very different query")

	extractSegment := func(t *testing.T, p, startTag, endTag string) string {
		t.Helper()
		startIdx := strings.Index(p, startTag)
		require.NotEqual(t, -1, startIdx, "must contain %s", startTag)
		endIdx := strings.Index(p, endTag)
		require.NotEqual(t, -1, endIdx, "must contain %s", endTag)
		return p[startIdx : endIdx+len(endTag)]
	}

	// SEMI-1 段跨 turn 字节稳定 (Skills + CacheToolCall, [current-nonce] 占位).
	semi1Round1 := extractSegment(t, prompt1, "<|AI_CACHE_SEMI_semi|>", "<|AI_CACHE_SEMI_END_semi|>")
	semi1Round2 := extractSegment(t, prompt2, "<|AI_CACHE_SEMI_semi|>", "<|AI_CACHE_SEMI_END_semi|>")
	require.Equal(t, semi1Round1, semi1Round2,
		"semi-dynamic-1 segment must be byte-stable across different turn nonces (P1.1 cache prerequisite)")
	require.NotContains(t, semi1Round1, "nonce_round1",
		"semi-1 segment must NOT contain turn nonce (would break byte stability)")
	require.NotContains(t, semi1Round1, "nonce_round2_completely_different")
	require.Contains(t, semi1Round1, "[current-nonce]",
		"semi-1 segment must use placeholder stable nonce '[current-nonce]'")

	// SEMI-2 段跨 turn 字节稳定 (Persistent + Schema + OutputExample, 无 turn nonce).
	semi2Round1 := extractSegment(t, prompt1, "<|AI_CACHE_SEMI2_semi|>", "<|AI_CACHE_SEMI2_END_semi|>")
	semi2Round2 := extractSegment(t, prompt2, "<|AI_CACHE_SEMI2_semi|>", "<|AI_CACHE_SEMI2_END_semi|>")
	require.Equal(t, semi2Round1, semi2Round2,
		"semi-dynamic-2 segment must be byte-stable across different turn nonces (P1.1 cache prerequisite)")
	require.NotContains(t, semi2Round1, "nonce_round1",
		"semi-2 segment must NOT contain turn nonce (would break byte stability)")
	require.NotContains(t, semi2Round1, "nonce_round2_completely_different")
}

// chatDetailContentString 把 ChatDetail.Content (string 或 []*ChatContent)
// 拼成纯文本, 仅供测试断言使用。
//
// 关键词: ChatDetail content 兼容, hijack 测试 helper
func chatDetailContentString(detail aispec.ChatDetail) string {
	switch v := detail.Content.(type) {
	case string:
		return v
	case []*aispec.ChatContent:
		var sb strings.Builder
		for _, c := range v {
			if c == nil {
				continue
			}
			sb.WriteString(c.Text)
		}
		return sb.String()
	default:
		return ""
	}
}

// TestPromptManager_AssembleLoopPrompt_AicacheSplitClassification 验证 aireact
// 新"按稳定性分层"路径产出的 prompt 经 aicache.Split 后:
//   - high-static / semi-dynamic-1 / semi-dynamic-2 / timeline-open / dynamic
//     各自被识别 (P1.1 拆 semi)
//   - 老 SectionSemiDynamic 不再出现 (新路径已拆成 semi-dynamic-1 + semi-dynamic-2)
//   - timeline-open 段独立计入 SectionTimelineOpen, 不与老 SectionTimeline 混淆
//
// 关键词: AssembleLoopPrompt aicache split, 6 段切片, SectionSemiDynamic1/2 识别,
//
//	SectionTimelineOpen 识别, P1.1
func TestPromptManager_AssembleLoopPrompt_AicacheSplitClassification(t *testing.T) {
	react, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"object","next_action":{"type":"directly_answer","answer_payload":"ok"},"cumulative_summary":"ok","human_readable_thought":"ok"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)

	tool := aitool.NewWithoutCallback("tool-a", aitool.WithDescription("tool a desc"))
	react.AddToTimeline("test", "timeline content")

	result, err := react.promptManager.AssembleLoopPrompt([]*aitool.Tool{tool}, &reactloops.LoopPromptAssemblyInput{
		Nonce:           "cls01",
		UserQuery:       "user query body",
		TaskInstruction: "follow task rules",
		OutputExample:   "example output",
		Schema:          `{"type":"object","properties":{"@action":{"type":"string"}}}`,
		SkillsContext:   "<|SKILLS_CONTEXT_skills_context|>\nloaded skill\n<|SKILLS_CONTEXT_END_skills_context|>",
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.Prompt)

	split := aicache.Split(result.Prompt)
	require.NotNil(t, split)
	require.NotEmpty(t, split.Chunks)

	sectionsBySection := make(map[string]int)
	for _, c := range split.Chunks {
		sectionsBySection[c.Section]++
	}
	require.Equal(t, 1, sectionsBySection[aicache.SectionHighStatic],
		"expect exactly one high-static chunk, got: %v", sectionsBySection)
	require.Equal(t, 1, sectionsBySection[aicache.SectionSemiDynamic1],
		"expect exactly one semi-dynamic-1 chunk (P1.1 split), got: %v", sectionsBySection)
	require.Equal(t, 1, sectionsBySection[aicache.SectionSemiDynamic2],
		"expect exactly one semi-dynamic-2 chunk (P1.1 split), got: %v", sectionsBySection)
	require.Equal(t, 1, sectionsBySection[aicache.SectionTimelineOpen],
		"expect exactly one timeline-open chunk, got: %v", sectionsBySection)
	require.Equal(t, 1, sectionsBySection[aicache.SectionDynamic],
		"expect exactly one dynamic chunk, got: %v", sectionsBySection)
	// P1.1 后老 SectionSemiDynamic / SectionTimeline 段名都不应再出现.
	require.Zero(t, sectionsBySection[aicache.SectionSemiDynamic],
		"new path should not emit legacy semi-dynamic section after P1.1 split, got: %v", sectionsBySection)
	require.Zero(t, sectionsBySection[aicache.SectionTimeline],
		"new path should not emit legacy timeline section, got: %v", sectionsBySection)
}

// TestPromptManager_HighStaticSection_TokenBudget 校验 high-static 段渲染后
// token 数 >= 1500. 该阈值来自 dashscope / qwen 系列实测的"显式 prefix cache
// 创建最小窗口", 高静态段 < 1500 token 容易被上游直接放弃缓存, 让 high-static
// 这一对 chunk hash 即便稳定也无法转化为真实计费节省。
//
// 该测试覆盖最坏情况 (4 个条件块全为 false / TaskInstruction 为空), 是否任何
// 后续改动都让高静态段降到 1500 token 以下的回归门闸.
//
// 关键词: high-static token budget, dashscope cache 最小窗口, 1500 阈值,
//
//	TestPromptManager_HighStaticSection_TokenBudget
func TestPromptManager_HighStaticSection_TokenBudget(t *testing.T) {
	react, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"object","next_action":{"type":"directly_answer","answer_payload":"ok"},"cumulative_summary":"ok","human_readable_thought":"ok"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)

	// 最坏情况: AllowToolCall / AllowPlanAndExec / HasLoadCapability 全 false,
	// TaskInstruction 为空. 这种 caller 下 high-static 模板只渲染 TRAITS +
	// 方法论协议三段, 对应当前 prompt 模板的最小尺寸.
	rendered, err := react.promptManager.renderLoopHighStaticSection(&reactloops.PromptPrefixMaterials{
		AllowToolCall:     false,
		AllowPlanAndExec:  false,
		HasLoadCapability: false,
		TaskInstruction:   "",
	})
	require.NoError(t, err)
	require.NotEmpty(t, rendered)

	tokenCount := ytoken.CalcTokenCount(rendered)
	require.GreaterOrEqual(t, tokenCount, 1500,
		"high-static section must keep >= 1500 tokens to survive dashscope prefix cache window; got %d tokens (%d bytes)",
		tokenCount, len(rendered))
}

// TestPromptManager_AssembleLoopPrompt_PriorModelThinkingOrder 验证 PriorModelThinking
// 出现在 AI_CACHE_FROZEN_END 之后、AI_CACHE_SEMI / PROMPT_SECTION_semi-dynamic 之前。
func TestPromptManager_AssembleLoopPrompt_PriorModelThinkingOrder(t *testing.T) {
	react, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"object","next_action":{"type":"directly_answer","answer_payload":"ok"},"cumulative_summary":"ok","human_readable_thought":"ok"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)

	tool := aitool.NewWithoutCallback("tool-a", aitool.WithDescription("tool a desc"))
	result, err := react.promptManager.AssembleLoopPrompt([]*aitool.Tool{tool}, &reactloops.LoopPromptAssemblyInput{
		Nonce:              "mt01",
		UserQuery:          "q",
		TaskInstruction:    "ti",
		PriorModelThinking: "merged thought line 1\nline 2",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	prompt := result.Prompt
	frozenEnd := strings.Index(prompt, "<|AI_CACHE_FROZEN_END_semi-dynamic|>")
	mt := strings.Index(prompt, "<|PROMPT_SECTION_model-thinking|>")
	semi := strings.Index(prompt, "<|PROMPT_SECTION_semi-dynamic|>")
	semiCache := strings.Index(prompt, "<|AI_CACHE_SEMI_semi|>")
	require.NotEqual(t, -1, frozenEnd)
	require.NotEqual(t, -1, mt)
	require.NotEqual(t, -1, semi)
	require.NotEqual(t, -1, semiCache)
	require.Less(t, frozenEnd, mt)
	require.Less(t, mt, semiCache)
	require.Less(t, mt, semi)

	sections := mustLoopPromptSections(t, result.Sections)
	require.Len(t, sections, 6)
	require.Equal(t, "section.model_thinking", sections[2].Key)
}
