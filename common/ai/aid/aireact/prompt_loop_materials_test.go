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

	// 段顺序: TRAITS -> AI_CACHE_FROZEN(START) -> Tool/Forge/Timeline-frozen ->
	// AI_CACHE_FROZEN(END) -> PROMPT_SECTION_semi-dynamic(Skills + Schema) ->
	// PROMPT_SECTION_timeline-open(Timeline open + Time + Workspace) -> Dynamic
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
	require.Less(t, workspaceIdx, userQueryIdx)
	require.Less(t, userQueryIdx, autoCtxIdx)
	require.Less(t, autoCtxIdx, prevUserInputIdx)
	require.Contains(t, prompt, "<|PERSISTENT|>")
	require.Contains(t, prompt, "<|OUTPUT_EXAMPLE|>")
	require.Contains(t, prompt, "<|SCHEMA|>")
	require.NotContains(t, prompt, "<|SCHEMA_n123|>")

	// frozen_block 段子结构: tool_inventory + forge_inventory (本用例 forge 关闭) +
	// timeline_frozen (本用例为空)。filterIncludedPromptSections 会过滤空段,
	// 故实际只剩 tool_inventory 一个 child。
	require.NotEmpty(t, sections[1].Children)
	require.Equal(t, "section.frozen_block.tool_inventory", sections[1].Children[0].Key)

	// semi_dynamic 段子结构: skills_context + schema (Tool/Forge 已迁出)。
	require.Len(t, sections[2].Children, 2)
	require.Equal(t, "section.semi_dynamic.skills_context", sections[2].Children[0].Key)
	require.Equal(t, "section.semi_dynamic.schema", sections[2].Children[1].Key)

	// timeline_open 段子结构: timeline_open + current_time + workspace。
	require.GreaterOrEqual(t, len(sections[3].Children), 3)
	require.Equal(t, "section.timeline_open.timeline_open", sections[3].Children[0].Key)
	require.Equal(t, "section.timeline_open.current_time", sections[3].Children[1].Key)
	require.Equal(t, "section.timeline_open.workspace", sections[3].Children[2].Key)

	require.GreaterOrEqual(t, len(sections[4].Children), 3)
	require.Equal(t, "section.dynamic.user_query", sections[4].Children[0].Key)
	require.Equal(t, "section.dynamic.auto_context", sections[4].Children[1].Key)
	require.Equal(t, "section.dynamic.user_history", sections[4].Children[2].Key)
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

// TestPromptManager_AssembleLoopPrompt_HijackThreeSegment 验证 aireact 新路径
// 产出的 prompt 经 aicache.Observe 后被 hijacker 切成 3 段:
//   - system: 含 AI_CACHE_SYSTEM_high-static 包装, 主动 cc
//   - user1: 含 AI_CACHE_FROZEN_semi-dynamic 完整闭合块 (Tool/Forge/Timeline-frozen),
//     字节边界稳定, 主动 cc
//   - user2: 含 PROMPT_SECTION_semi-dynamic + PROMPT_SECTION_timeline-open + Dynamic
//     段, 不打 cc
//
// 这是按稳定性分层后的核心收益: dashscope 双 cc 同时命中 system 短前缀和
// system + frozen 长前缀, 显著提升复用率 (CACHE_BOUNDARY_GUIDE.md §7.7.7)。
//
// 关键词: aicache hijack 3 段, AI_CACHE_FROZEN 边界切片, 双 cc 命中
func TestPromptManager_AssembleLoopPrompt_HijackThreeSegment(t *testing.T) {
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
	require.NotNil(t, hijack, "loop prompt with high-static + frozen block should be hijacked")
	require.True(t, hijack.IsHijacked)
	require.Len(t, hijack.Messages, 3, "expect 3-segment hijack (system + user1 + user2)")

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
	require.Contains(t, user2Content, "<|PROMPT_SECTION_semi-dynamic|>")
	require.Contains(t, user2Content, "<|PROMPT_SECTION_timeline-open|>")
	require.Contains(t, user2Content, "<|USER_QUERY_hj01|>")
	require.NotContains(t, user2Content, "<|AI_CACHE_FROZEN_semi-dynamic|>",
		"user2 must not contain frozen START tag")
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
