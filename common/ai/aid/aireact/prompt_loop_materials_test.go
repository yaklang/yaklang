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
// 下 5 段顺序: high_static -> frozen_block -> semi_dynamic (Skills + Schema) ->
// timeline_open -> dynamic; 以及 frozen_block 段被 AI_CACHE_FROZEN_semi-dynamic
// 标签包裹。
//
// 关键词: AssembleLoopPrompt section order, 5 段顺序, frozen-block, AI_CACHE_FROZEN
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
	require.Len(t, sections, 5)

	require.Equal(t, "section.high_static", sections[0].Key)
	require.Equal(t, "section.frozen_block", sections[1].Key)
	require.Equal(t, "section.semi_dynamic", sections[2].Key)
	require.Equal(t, "section.timeline_open", sections[3].Key)
	require.Equal(t, "section.dynamic", sections[4].Key)
	require.Equal(t, reactloops.PromptSectionRoleHighStatic, sections[0].Role)
	require.Equal(t, reactloops.PromptSectionRoleZHHighStatic, sections[0].RoleZh)
	require.Equal(t, reactloops.PromptSectionRoleFrozenBlock, sections[1].Role)
	require.Equal(t, reactloops.PromptSectionRoleSemiDynamic, sections[2].Role)
	require.Equal(t, reactloops.PromptSectionRoleTimelineOpen, sections[3].Role)
	require.Equal(t, reactloops.PromptSectionRoleDynamic, sections[4].Role)

	prompt := result.Prompt
	traitsIdx := strings.Index(prompt, "<|TRAITS|>")
	workspaceIdx := strings.Index(prompt, "# Workspace Context")
	timelineIdx := strings.Index(prompt, "# Timeline Memory")
	currentTimeIdx := strings.Index(prompt, "# Current Time")
	toolInventoryIdx := strings.Index(prompt, "# Tool Inventory")
	skillsIdx := strings.Index(prompt, "<|SKILLS_CONTEXT_skills_context|>")
	schemaIdx := strings.Index(prompt, "<|SCHEMA|>")
	frozenStartIdx := strings.Index(prompt, "<|AI_CACHE_FROZEN_semi-dynamic|>")
	frozenEndIdx := strings.Index(prompt, "<|AI_CACHE_FROZEN_END_semi-dynamic|>")
	semiSectionIdx := strings.Index(prompt, "<|PROMPT_SECTION_semi-dynamic|>")
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
	require.NotEqual(t, -1, schemaIdx)
	require.NotEqual(t, -1, frozenStartIdx)
	require.NotEqual(t, -1, frozenEndIdx)
	require.NotEqual(t, -1, semiSectionIdx)
	require.NotEqual(t, -1, timelineOpenSectionIdx)
	require.NotEqual(t, -1, userQueryIdx)
	require.NotEqual(t, -1, autoCtxIdx)
	require.NotEqual(t, -1, prevUserInputIdx)

	// 段顺序 (P1-C2 调整后): TRAITS -> AI_CACHE_FROZEN(START) ->
	// Tool/Forge/Timeline-frozen -> AI_CACHE_FROZEN(END) ->
	// PROMPT_SECTION_semi-dynamic(Skills + Schema) ->
	// PROMPT_SECTION_timeline-open(Timeline open + Time + Workspace +
	// SessionEvidence + PREV_USER_INPUT) -> Dynamic(UserQuery + AutoCtx + ...)
	require.Less(t, traitsIdx, frozenStartIdx)
	require.Less(t, frozenStartIdx, toolInventoryIdx)
	require.Less(t, toolInventoryIdx, frozenEndIdx)
	require.Less(t, frozenEndIdx, semiSectionIdx)
	require.Less(t, semiSectionIdx, skillsIdx)
	require.Less(t, skillsIdx, schemaIdx)
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
	require.NotEmpty(t, sections[1].Children)
	require.Equal(t, "section.frozen_block.tool_inventory", sections[1].Children[0].Key)
	require.Equal(t, reactloops.PromptSectionRoleFrozenBlock, sections[1].Children[0].Role)

	// semi_dynamic 段子结构: skills_context + schema + output_example +
	// task_instruction (Tool/Forge 已迁出, OutputExample 从 high_static 迁入并紧跟
	// Schema 之后, TaskInstruction 同样从 high_static 迁入并紧跟 OutputExample 之后)。
	// 关键词: semi_dynamic children, output_example 迁入断言, task_instruction 迁入断言
	require.Len(t, sections[2].Children, 4)
	require.Equal(t, "section.semi_dynamic.skills_context", sections[2].Children[0].Key)
	require.Equal(t, "section.semi_dynamic.schema", sections[2].Children[1].Key)
	require.Equal(t, reactloops.PromptSectionRoleSemiDynamic, sections[2].Children[0].Role)
	require.Equal(t, reactloops.PromptSectionRoleSemiDynamic, sections[2].Children[1].Role)
	require.Equal(t, "section.semi_dynamic.output_example", sections[2].Children[2].Key)
	require.Equal(t, "section.semi_dynamic.task_instruction", sections[2].Children[3].Key)
	require.Equal(t, reactloops.PromptSectionRoleSemiDynamic, sections[2].Children[3].Role)

	// timeline_open 段子结构 (P1-C2): timeline_open + current_time + workspace +
	// session_evidence (本用例无 SessionEvidence -> 不出现) + user_history.
	require.GreaterOrEqual(t, len(sections[3].Children), 4)
	require.Equal(t, "section.timeline_open.timeline_open", sections[3].Children[0].Key)
	require.Equal(t, "section.timeline_open.current_time", sections[3].Children[1].Key)
	require.Equal(t, "section.timeline_open.workspace", sections[3].Children[2].Key)
	// P1-C2: user_history 现在挂在 timeline_open 之下而非 dynamic 之下.
	require.Equal(t, "section.timeline_open.user_history", sections[3].Children[3].Key)
	require.Equal(t, reactloops.PromptSectionRoleTimelineOpen, sections[3].Children[3].Role)

	require.GreaterOrEqual(t, len(sections[4].Children), 2)
	require.Equal(t, "section.dynamic.user_query", sections[4].Children[0].Key)
	require.Equal(t, "section.dynamic.auto_context", sections[4].Children[1].Key)
	require.Equal(t, reactloops.PromptSectionRoleDynamic, sections[4].Children[0].Role)
	require.Equal(t, reactloops.PromptSectionRoleZHDynamic, sections[4].Children[0].RoleZh)
}

// TestPromptManager_RenderLoopSemiDynamicSection_Order 验证 SEMI 残留段
// (semi_dynamic_section.txt) 仅包含 Skills Context + Schema; Tool/Forge 已迁出
// 到 frozen_block_section.txt。
//
// 关键词: renderLoopSemiDynamicSection, Skills + Schema 残留段
func TestPromptManager_RenderLoopSemiDynamicSection_Order(t *testing.T) {
	react, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"object","next_action":{"type":"directly_answer","answer_payload":"ok"},"cumulative_summary":"ok","human_readable_thought":"ok"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)

	rendered, err := react.promptManager.renderLoopSemiDynamicSection(&reactloops.PromptPrefixMaterials{
		ToolInventory:  true,
		ToolsCount:     2,
		TopToolsCount:  1,
		TopTools:       []*aitool.Tool{aitool.NewWithoutCallback("tool-a", aitool.WithDescription("tool a desc"))},
		HasMoreTools:   true,
		ForgeInventory: true,
		AIForgeList:    "* `forge-a`: forge a desc",
		SkillsContext:  "<|SKILLS_CONTEXT_demo|>\nskill body\n<|SKILLS_CONTEXT_END_demo|>",
		Schema:         `{"type":"object","properties":{"@action":{"type":"string"}}}`,
	})
	require.NoError(t, err)

	skillsIdx := strings.Index(rendered, "<|SKILLS_CONTEXT_demo|>")
	schemaIdx := strings.Index(rendered, "<|SCHEMA|>")
	require.NotEqual(t, -1, skillsIdx)
	require.NotEqual(t, -1, schemaIdx)
	require.Less(t, skillsIdx, schemaIdx)
	// Tool / Forge 不在 SEMI 残留段里, 已迁到 frozen_block_section.txt
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

// TestPromptManager_AssemblePromptPrefix 验证 4 段 prefix 输出 (high_static +
// frozen_block + semi_dynamic + timeline_open), 且 Prompt 字段拼接顺序正确。
//
// 关键词: AssemblePromptPrefix, 4 段, 按稳定性分层
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
		react.promptManager.NewPromptPrefixMaterials(base, &reactloops.LoopPromptAssemblyInput{
			Nonce:           "pfx123",
			TaskInstruction: "follow task rules",
			OutputExample:   "example output",
			Schema:          `{"type":"object","properties":{"@action":{"type":"string"}}}`,
		}),
	)
	require.NoError(t, err)
	require.NotNil(t, prefix)
	require.Len(t, prefix.Sections, 4)
	require.Contains(t, prefix.Prompt, "<|TRAITS|>")
	require.Contains(t, prefix.Prompt, "<|SCHEMA|>")
	require.Equal(t, "section.high_static", prefix.Sections[0].Key)
	require.Equal(t, "section.frozen_block", prefix.Sections[1].Key)
	require.Equal(t, "section.semi_dynamic", prefix.Sections[2].Key)
	require.Equal(t, "section.timeline_open", prefix.Sections[3].Key)
}

// TestPromptManager_AssembleLoopPrompt_HijackFourSegment 验证 aireact 主路径
// 产出的 prompt (SYSTEM + FROZEN + SEMI + OPEN + DYNAMIC, 双 cache 边界齐全)
// 经 aicache.Observe 后被 hijacker 切成 4 段:
//   - system: 含 AI_CACHE_SYSTEM_high-static 包装, 主动 cc
//   - user1: 含 AI_CACHE_FROZEN_semi-dynamic 完整闭合块 (Tool/Forge/Timeline-frozen),
//     字节边界稳定, 主动 cc
//   - user2: 含 AI_CACHE_SEMI_semi 完整闭合块 (PROMPT_SECTION_semi-dynamic +
//     Skills + Schema + CacheToolCall), 字节边界稳定, 主动 cc
//   - user3: 含 PROMPT_SECTION_timeline-open + Dynamic 段, 不打 cc
//
// 这是 P1 双 cache 边界的核心收益: dashscope 同时命中 system 短前缀、
// system+frozen 长前缀、system+frozen+semi 更长前缀三档候选 (实际命中前 N 档
// 由 dashscope 决定), 比单 frozen 边界 (P0) 多一档.
//
// 关键词: aicache hijack 4 段, AI_CACHE_FROZEN + AI_CACHE_SEMI 双边界,
//
//	三 cc 主路径, P1 双 cache 边界
func TestPromptManager_AssembleLoopPrompt_HijackFourSegment(t *testing.T) {
	// P2.1 阈值合并默认 1024 byte 会把本测试的短 fixture (Tool Inventory 仅
	// 一个 tool, 总字节数 << 1KB) 合并降级到 2 段, 与本测试断言的 4 段 happy
	// path 不符. 显式关闭阈值合并以验证字节边界结构.
	// 关键词: P2.1 阈值合并跨包关闭, aicache test helper, 4 段结构验证
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
	require.NotNil(t, hijack, "loop prompt with high-static + frozen + semi block should be hijacked")
	require.True(t, hijack.IsHijacked)
	require.Len(t, hijack.Messages, 4, "expect 4-segment hijack (system + user1 + user2 + user3)")

	systemMsg := hijack.Messages[0]
	require.Equal(t, "system", systemMsg.Role)
	systemContent := chatDetailContentString(systemMsg)
	require.Contains(t, systemContent, "<|AI_CACHE_SYSTEM_high-static|>")
	require.Contains(t, systemContent, "<|AI_CACHE_SYSTEM_END_high-static|>")

	user1 := hijack.Messages[1]
	require.Equal(t, "user", user1.Role)
	user1Content := chatDetailContentString(user1)
	require.Contains(t, user1Content, "<|AI_CACHE_FROZEN_semi-dynamic|>")
	require.Contains(t, user1Content, "<|AI_CACHE_FROZEN_END_semi-dynamic|>")
	require.Contains(t, user1Content, "# Tool Inventory")
	// user1 末尾必须以 frozen END 标签作为字节边界, 让 dashscope 缓存命中
	require.True(t, strings.HasSuffix(strings.TrimSpace(user1Content), "<|AI_CACHE_FROZEN_END_semi-dynamic|>"),
		"user1 should end at frozen boundary END tag, got: %q", user1Content)

	user2 := hijack.Messages[2]
	require.Equal(t, "user", user2.Role)
	user2Content := chatDetailContentString(user2)
	require.Contains(t, user2Content, "<|AI_CACHE_SEMI_semi|>",
		"user2 must contain AI_CACHE_SEMI START tag")
	require.Contains(t, user2Content, "<|AI_CACHE_SEMI_END_semi|>",
		"user2 must contain AI_CACHE_SEMI END tag")
	require.Contains(t, user2Content, "<|PROMPT_SECTION_semi-dynamic|>",
		"user2 must contain inner PROMPT_SECTION_semi-dynamic wrapper")
	require.Contains(t, user2Content, "<|SKILLS_CONTEXT_skills_context|>",
		"user2 must contain SkillsContext")
	require.Contains(t, user2Content, "<|SCHEMA|>",
		"user2 must contain Schema")
	require.True(t, strings.HasSuffix(strings.TrimSpace(user2Content), "<|AI_CACHE_SEMI_END_semi|>"),
		"user2 should end at semi boundary END tag, got: %q", user2Content)
	require.NotContains(t, user2Content, "<|PROMPT_SECTION_timeline-open|>",
		"user2 must NOT contain timeline-open section (belongs to user3)")
	require.NotContains(t, user2Content, "<|USER_QUERY_hj01|>",
		"user2 must NOT contain dynamic user query (belongs to user3)")

	user3 := hijack.Messages[3]
	require.Equal(t, "user", user3.Role)
	user3Content := chatDetailContentString(user3)
	require.Contains(t, user3Content, "<|PROMPT_SECTION_timeline-open|>")
	require.Contains(t, user3Content, "<|USER_QUERY_hj01|>")
	require.NotContains(t, user3Content, "<|AI_CACHE_FROZEN_semi-dynamic|>",
		"user3 must NOT contain frozen START tag")
	require.NotContains(t, user3Content, "<|AI_CACHE_SEMI_semi|>",
		"user3 must NOT contain semi START tag")
}

// TestPromptManager_AssembleLoopPrompt_RecentToolsCacheInSemiSegment 验证
// CACHE_TOOL_CALL 块 (经 LoopPromptAssemblyInput.RecentToolsCache 透传) 物理位置
// 在 semi-dynamic 段 (而不再在 dynamic 段), 经 hijacker 切割后位于 user2.
//
// 关键词: TestPromptManager, CACHE_TOOL_CALL 物理迁移, semi-dynamic 段, P1 主路径
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

	// 2. 必须位于 PROMPT_SECTION_semi-dynamic 段内 (而不在 dynamic 段)
	semiStart := strings.Index(prompt, "<|PROMPT_SECTION_semi-dynamic|>")
	semiEnd := strings.Index(prompt, "<|PROMPT_SECTION_END_semi-dynamic|>")
	require.NotEqual(t, -1, semiStart)
	require.NotEqual(t, -1, semiEnd)
	require.Greater(t, cacheStartIdx, semiStart, "CACHE_TOOL_CALL must start AFTER semi-dynamic START")
	require.Less(t, cacheEndIdx, semiEnd, "CACHE_TOOL_CALL must end BEFORE semi-dynamic END")

	// 3. dynamic 段不应该再含 CACHE_TOOL_CALL (历史位置)
	dynamicStart := strings.Index(prompt, "<|PROMPT_SECTION_dynamic_turnA|>")
	require.NotEqual(t, -1, dynamicStart)
	dynamicTail := prompt[dynamicStart:]
	require.NotContains(t, dynamicTail, "<|CACHE_TOOL_CALL_[current-nonce]|>",
		"CACHE_TOOL_CALL must NOT remain in dynamic section after physical migration")

	// 4. semi 段必须被 AI_CACHE_SEMI_semi 边界包裹 (P1)
	aiCacheSemiStart := strings.Index(prompt, "<|AI_CACHE_SEMI_semi|>")
	aiCacheSemiEnd := strings.Index(prompt, "<|AI_CACHE_SEMI_END_semi|>")
	require.NotEqual(t, -1, aiCacheSemiStart, "P1: prompt must contain AI_CACHE_SEMI_semi START")
	require.NotEqual(t, -1, aiCacheSemiEnd, "P1: prompt must contain AI_CACHE_SEMI_semi END")
	require.Less(t, aiCacheSemiStart, semiStart,
		"AI_CACHE_SEMI START must wrap PROMPT_SECTION_semi-dynamic")
	require.Greater(t, aiCacheSemiEnd, semiEnd,
		"AI_CACHE_SEMI END must wrap PROMPT_SECTION_semi-dynamic")
}

// TestPromptManager_AssembleLoopPrompt_SemiSegmentByteStableAcrossTurns 验证
// 在 turn nonce 不同的两次 AssembleLoopPrompt 调用中, semi-dynamic 段
// (含 SkillsContext + Schema + CACHE_TOOL_CALL) 字节稳定 (因为 CACHE_TOOL_CALL
// 已用稳定字面量 nonce 渲染).
//
// 这是 P1 双 cache 边界生效的前提: hijacker 切到 user2 的字节流必须跨 turn
// 一致, 才能命中 dashscope prefix cache.
//
// 关键词: TestPromptManager, semi-dynamic 字节稳定, 跨 turn 一致, P1 cache 命中
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

	// 提取每次 prompt 的 semi-dynamic 段 (从 AI_CACHE_SEMI START 到 END)
	extractSemiSegment := func(t *testing.T, p string) string {
		t.Helper()
		startTag := "<|AI_CACHE_SEMI_semi|>"
		endTag := "<|AI_CACHE_SEMI_END_semi|>"
		startIdx := strings.Index(p, startTag)
		require.NotEqual(t, -1, startIdx, "must contain AI_CACHE_SEMI START")
		endIdx := strings.Index(p, endTag)
		require.NotEqual(t, -1, endIdx, "must contain AI_CACHE_SEMI END")
		return p[startIdx : endIdx+len(endTag)]
	}

	semi1 := extractSemiSegment(t, prompt1)
	semi2 := extractSemiSegment(t, prompt2)

	require.Equal(t, semi1, semi2,
		"semi-dynamic segment must be byte-stable across different turn nonces (P1 cache prerequisite)")

	// 双重保险: semi 段内绝对不能含两个 turn nonce 任意一个
	require.NotContains(t, semi1, "nonce_round1",
		"semi segment must NOT contain turn nonce (would break byte stability)")
	require.NotContains(t, semi1, "nonce_round2_completely_different")
	// 但应该含占位符字面量 nonce
	require.Contains(t, semi1, "[current-nonce]",
		"semi segment must use placeholder stable nonce '[current-nonce]'")
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
//   - high-static / semi-dynamic / timeline-open / dynamic 各自被识别
//   - timeline-open 段独立计入 SectionTimelineOpen, 不与老 SectionTimeline 混淆
//
// 关键词: AssembleLoopPrompt aicache split, 5 段切片, SectionTimelineOpen 识别
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
	require.Equal(t, 1, sectionsBySection[aicache.SectionSemiDynamic],
		"expect exactly one semi-dynamic chunk, got: %v", sectionsBySection)
	require.Equal(t, 1, sectionsBySection[aicache.SectionTimelineOpen],
		"expect exactly one timeline-open chunk, got: %v", sectionsBySection)
	require.Equal(t, 1, sectionsBySection[aicache.SectionDynamic],
		"expect exactly one dynamic chunk, got: %v", sectionsBySection)
	// 老 timeline 段名不应该出现 (新路径已迁移到 timeline-open)
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
//        TestPromptManager_HighStaticSection_TokenBudget
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
