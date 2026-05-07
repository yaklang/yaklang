package aiforge

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aicache"
	"github.com/yaklang/yaklang/common/ai/ytoken"
	"github.com/yaklang/yaklang/common/utils"
)

// TestLiteForgePrompt_SplitsIntoFourSections 验证 LiteForge 模板能被 aicache.Split 切成预期的 4 段
// B 档语义：StaticInstruction -> high-static 段；Prompt -> dynamic 段
// 关键词: aicache, PROMPT_SECTION, LiteForge 模板, 4 段切片, B 档
func TestLiteForgePrompt_SplitsIntoFourSections(t *testing.T) {
	nonce := strings.ToLower(utils.RandStringBytes(6))
	rendered, err := renderLiteForgePrompt(liteForgePromptParams{
		Nonce:             nonce,
		StaticInstruction: "you are a capability matcher.",
		Prompt:            "user_query=hostscan; tags=[a,b,c]",
		Params:            "user input value",
		Schema:            `{"type":"object","properties":{"matched":{"type":"array"}}}`,
		PersistentMemory:  "remember: prefer chinese keywords",
		TimelineDump:      "tool[1] success at 12:00",
	})
	require.NoError(t, err)

	split := aicache.Split(rendered)
	require.NotNil(t, split)
	require.Len(t, split.Chunks, 4, "expect 4 sections when all fields present")

	sections := make(map[string]*aicache.Chunk, 4)
	for _, c := range split.Chunks {
		sections[c.Section] = c
	}
	require.Contains(t, sections, aicache.SectionHighStatic)
	require.Contains(t, sections, aicache.SectionSemiDynamic)
	require.Contains(t, sections, aicache.SectionTimeline)
	require.Contains(t, sections, aicache.SectionDynamic)

	// high-static 段：仅含 Preset / Output Formatter 通用文案 (P0-B1: SCHEMA 与
	// Instruction 已下移到 semi-dynamic 段, 让 high-static 跨 forge byte-stable)
	hs := sections[aicache.SectionHighStatic]
	require.Contains(t, hs.Content, "# Preset")
	require.Contains(t, hs.Content, "# Output Formatter")
	require.NotContains(t, hs.Content, "# SCHEMA",
		"P0-B1: SCHEMA must NOT appear in high-static anymore (moved to semi-dynamic)")
	require.NotContains(t, hs.Content, "<schema>",
		"P0-B1: <schema> tag must NOT appear in high-static anymore")
	require.NotContains(t, hs.Content, "# Instruction",
		"P0-B1: # Instruction must NOT appear in high-static anymore")
	require.NotContains(t, hs.Content, "<instruction>",
		"P0-B1: <instruction> tag must NOT appear in high-static anymore")
	require.NotContains(t, hs.Content, "you are a capability matcher.",
		"P0-B1: StaticInstruction content must NOT appear in high-static anymore")
	require.NotContains(t, hs.Content, "user_query=hostscan",
		"Prompt content must NOT appear in high-static section (B-tier moved Prompt to dynamic)")
	require.NotContains(t, hs.Content, nonce, "high-static section MUST NOT contain nonce")

	// semi-dynamic 段：含 SCHEMA + Instruction (P0-B1 下移) + persistent memory
	sd := sections[aicache.SectionSemiDynamic]
	require.Contains(t, sd.Content, "# SCHEMA",
		"P0-B1: SCHEMA must appear in semi-dynamic now")
	require.Contains(t, sd.Content, "<schema>")
	require.Contains(t, sd.Content, "# Instruction",
		"P0-B1: # Instruction must appear in semi-dynamic now")
	require.Contains(t, sd.Content, "<instruction>")
	require.Contains(t, sd.Content, "you are a capability matcher.",
		"P0-B1: StaticInstruction content must appear in semi-dynamic")
	require.Contains(t, sd.Content, "# 牢记")
	require.Contains(t, sd.Content, "remember: prefer chinese keywords")

	// timeline 段
	tl := sections[aicache.SectionTimeline]
	require.Contains(t, tl.Content, "<timeline_"+nonce+">")
	require.Contains(t, tl.Content, "tool[1] success at 12:00")

	// dynamic 段：含调用方动态上下文（Prompt） + 用户参数（Params）
	dy := sections[aicache.SectionDynamic]
	require.Equal(t, aicache.SectionDynamic+"_"+nonce, dy.Nonce)
	require.Contains(t, dy.Content, "<context_"+nonce+">",
		"Prompt should be wrapped by <context_NONCE>...</context_NONCE> in dynamic section")
	require.Contains(t, dy.Content, "user_query=hostscan; tags=[a,b,c]",
		"Prompt content must appear in dynamic section")
	require.Contains(t, dy.Content, "<params_"+nonce+">")
	require.Contains(t, dy.Content, "user input value")
}

// TestLiteForgePrompt_HighStaticStableAcrossNonces 跨调用 high-static 哈希稳定性回归测试
// 这是 B 档改造的核心收益验证：相同 StaticInstruction + Schema 时，high-static 段必须 byte-identical
// 即便两次调用 nonce 不同、Prompt（动态内容）不同
// 关键词: aicache, high-static, 跨调用稳定, B 档核心回归测试
func TestLiteForgePrompt_HighStaticStableAcrossNonces(t *testing.T) {
	const (
		staticInstruction = "you are a capability matcher with strict identifier rules."
		schema            = `{"type":"object","properties":{"matched_identifiers":{"type":"array"}}}`
	)

	nonceA := strings.ToLower(utils.RandStringBytes(6))
	nonceB := strings.ToLower(utils.RandStringBytes(6))
	require.NotEqual(t, nonceA, nonceB, "two random nonces should differ")

	// 故意让两次调用的 Prompt（动态内容）不同，模拟真实负载
	renderA, err := renderLiteForgePrompt(liteForgePromptParams{
		Nonce:             nonceA,
		StaticInstruction: staticInstruction,
		Prompt:            "user_query=hostscan",
		Schema:            schema,
		Params:            "p1",
	})
	require.NoError(t, err)
	renderB, err := renderLiteForgePrompt(liteForgePromptParams{
		Nonce:             nonceB,
		StaticInstruction: staticInstruction,
		Prompt:            "user_query=portscan",
		Schema:            schema,
		Params:            "p2",
	})
	require.NoError(t, err)

	splitA := aicache.Split(renderA)
	splitB := aicache.Split(renderB)
	require.NotNil(t, splitA)
	require.NotNil(t, splitB)

	hsA := pickSection(t, splitA, aicache.SectionHighStatic)
	hsB := pickSection(t, splitB, aicache.SectionHighStatic)

	require.Equal(t, hsA.Content, hsB.Content,
		"high-static content must be byte-identical across different nonces and different Prompt content")
	require.Equal(t, hsA.Hash, hsB.Hash,
		"high-static hash must be stable across different nonces and different Prompt content")

	dyA := pickSection(t, splitA, aicache.SectionDynamic)
	dyB := pickSection(t, splitB, aicache.SectionDynamic)
	require.NotEqual(t, dyA.Hash, dyB.Hash,
		"dynamic hash should differ when nonce / Prompt / Params differ (anti-injection by design)")
}

// TestLiteForgePrompt_TimelineEmptyOmitsSection 验证 timeline 内容为空时
// 仍按 4 段对齐, 输出空 timeline-open 占位, 防止 LiteForge 路径在 timeline
// 缺失时让 dynamic 段紧贴 semi-dynamic, 破坏 4 段哈希对齐.
// 关键词: aicache, timeline-open empty placeholder, 4 段对齐
func TestLiteForgePrompt_TimelineEmptyOmitsSection(t *testing.T) {
	nonce := strings.ToLower(utils.RandStringBytes(6))
	rendered, err := renderLiteForgePrompt(liteForgePromptParams{
		Nonce:             nonce,
		StaticInstruction: "static instruction",
		Prompt:            "dynamic content",
		Schema:            `{"type":"object"}`,
		Params:            "p",
		TimelineDump:      "",
	})
	require.NoError(t, err)
	require.NotContains(t, rendered, "<|PROMPT_SECTION_timeline|>",
		"老 timeline 段在没有 frozen / dump 时不应出现")
	require.Contains(t, rendered, "<|PROMPT_SECTION_timeline-open|>",
		"timeline-open 段必须无条件输出 (即便为空), 保证 4 段对齐")
	require.Contains(t, rendered, "<|PROMPT_SECTION_END_timeline-open|>")

	split := aicache.Split(rendered)
	require.NotNil(t, split)
	require.Len(t, split.Chunks, 4,
		"expect 4 sections (high-static, semi-dynamic, timeline-open empty placeholder, dynamic) when timeline content is empty")

	for _, c := range split.Chunks {
		require.NotEqual(t, aicache.SectionTimeline, c.Section,
			"老 timeline 段不应出现在 chunks 中")
	}

	tlOpen := pickSection(t, split, aicache.SectionTimelineOpen)
	require.NotNil(t, tlOpen, "timeline-open 段应作为空占位存在")
	require.Empty(t, strings.TrimSpace(tlOpen.Content),
		"timeline-open 占位 chunk 内容应为空 (仅含起止标签之间的空内容)")
}

// TestLiteForgePrompt_DynamicNonceConsistent 验证 dynamic 段外层 nonce 与内部 wrapper nonce 一致
// B 档新增：内层除了 <params_NONCE>，还应有 <context_NONCE>
// 关键词: aicache, dynamic, nonce 一致性, context wrapper
func TestLiteForgePrompt_DynamicNonceConsistent(t *testing.T) {
	nonce := strings.ToLower(utils.RandStringBytes(6))
	rendered, err := renderLiteForgePrompt(liteForgePromptParams{
		Nonce:             nonce,
		StaticInstruction: "static",
		Prompt:            "dyn",
		Schema:            "y",
		Params:            "z",
	})
	require.NoError(t, err)
	split := aicache.Split(rendered)
	require.NotNil(t, split)

	dy := pickSection(t, split, aicache.SectionDynamic)
	require.Equal(t, aicache.SectionDynamic+"_"+nonce, dy.Nonce,
		"dynamic chunk nonce should equal section + outer NONCE")
	require.Contains(t, dy.Content, "<context_"+nonce+">")
	require.Contains(t, dy.Content, "</context_"+nonce+">")
	require.Contains(t, dy.Content, "<params_"+nonce+">")
	require.Contains(t, dy.Content, "</params_"+nonce+">")
}

// TestLiteForgePrompt_OnlyStaticInstructionEmpty 验证 StaticInstruction 为空时 # Instruction 块整体省略
// 兼容旧调用方：未设置 StaticInstruction 时，high-static 段不出现 instruction 标签
// 关键词: aicache, high-static, StaticInstruction 兼容
func TestLiteForgePrompt_OnlyStaticInstructionEmpty(t *testing.T) {
	nonce := strings.ToLower(utils.RandStringBytes(6))
	rendered, err := renderLiteForgePrompt(liteForgePromptParams{
		Nonce:  nonce,
		Prompt: "old caller pattern: everything in Prompt",
		Schema: `{"type":"object"}`,
		Params: "p",
	})
	require.NoError(t, err)

	split := aicache.Split(rendered)
	require.NotNil(t, split)

	hs := pickSection(t, split, aicache.SectionHighStatic)
	require.Contains(t, hs.Content, "# Preset")
	require.NotContains(t, hs.Content, "<schema>",
		"P0-B1: <schema> moved to semi-dynamic, must NOT appear in high-static")
	require.NotContains(t, hs.Content, "# Instruction",
		"# Instruction block should be omitted when StaticInstruction is empty")
	require.NotContains(t, hs.Content, "<instruction>",
		"<instruction> tag should be omitted when StaticInstruction is empty")
	require.NotContains(t, hs.Content, "old caller pattern: everything in Prompt",
		"Prompt content must NOT leak into high-static when StaticInstruction is empty")

	// P0-B1: schema 现在在 semi-dynamic 段
	sd := pickSection(t, split, aicache.SectionSemiDynamic)
	require.Contains(t, sd.Content, "<schema>",
		"P0-B1: schema must appear in semi-dynamic when present")
	require.NotContains(t, sd.Content, "# Instruction",
		"# Instruction block must remain omitted when StaticInstruction is empty")
	require.NotContains(t, sd.Content, "<instruction>",
		"<instruction> tag must remain omitted when StaticInstruction is empty")

	// 老调用方传入的 Prompt 现在出现在 dynamic 段
	dy := pickSection(t, split, aicache.SectionDynamic)
	require.Contains(t, dy.Content, "old caller pattern: everything in Prompt")
}

// TestLiteForgePrompt_TimelineFrozenOpen_RendersBothSections 验证 P0-B3 改动:
// LiteForge 模板支持 TimelineFrozenBlock + TimelineOpen 两段独立渲染, frozen 段
// 进 <|AI_CACHE_FROZEN_semi-dynamic|> 块, open 段进 <|PROMPT_SECTION_timeline-open|> 块。
//
// 跨调用稳定性: 仅 TimelineOpen 内容变化时, frozen 段输出必须 byte-stable。
//
// 关键词: aicache, LiteForge timeline 拆分, AI_CACHE_FROZEN_semi-dynamic,
//
//	PROMPT_SECTION_timeline-open, P0-B3
func TestLiteForgePrompt_TimelineFrozenOpen_RendersBothSections(t *testing.T) {
	nonceA := strings.ToLower(utils.RandStringBytes(6))
	nonceB := strings.ToLower(utils.RandStringBytes(6))

	frozen := "<|TIMELINE_frozen-1|>reducer summary L1<|TIMELINE_END_frozen-1|>"
	openA := "<|TIMELINE_open-1|>tool result A<|TIMELINE_END_open-1|>"
	openB := "<|TIMELINE_open-2|>tool result B (different)<|TIMELINE_END_open-2|>"

	renderA, err := renderLiteForgePrompt(liteForgePromptParams{
		Nonce:               nonceA,
		Prompt:              "p1",
		StaticInstruction:   "instr",
		Schema:              `{"type":"object"}`,
		Params:              "x",
		TimelineFrozenBlock: frozen,
		TimelineOpen:        openA,
	})
	require.NoError(t, err)
	renderB, err := renderLiteForgePrompt(liteForgePromptParams{
		Nonce:               nonceB,
		Prompt:              "p2",
		StaticInstruction:   "instr",
		Schema:              `{"type":"object"}`,
		Params:              "y",
		TimelineFrozenBlock: frozen,
		TimelineOpen:        openB,
	})
	require.NoError(t, err)

	require.Contains(t, renderA, "<|AI_CACHE_FROZEN_semi-dynamic|>",
		"timeline frozen wrap tag must appear when TimelineFrozenBlock is non-empty")
	require.Contains(t, renderA, "<|AI_CACHE_FROZEN_END_semi-dynamic|>")
	require.Contains(t, renderA, "<|PROMPT_SECTION_timeline-open|>",
		"timeline-open section tag must appear when TimelineOpen is non-empty")
	require.Contains(t, renderA, "<|PROMPT_SECTION_END_timeline-open|>")
	require.NotContains(t, renderA, "<|PROMPT_SECTION_timeline|>",
		"legacy single-timeline tag must NOT appear when frozen+open path is used")

	splitA := aicache.Split(renderA)
	splitB := aicache.Split(renderB)
	require.NotNil(t, splitA)
	require.NotNil(t, splitB)

	tlOpenA := pickSection(t, splitA, aicache.SectionTimelineOpen)
	tlOpenB := pickSection(t, splitB, aicache.SectionTimelineOpen)
	require.Contains(t, tlOpenA.Content, "tool result A")
	require.Contains(t, tlOpenB.Content, "tool result B (different)")
	require.NotEqual(t, tlOpenA.Hash, tlOpenB.Hash,
		"timeline-open hash should differ when open content differs")

	// frozen 段被 splitter 归类到 semi-dynamic / timeline 等 cacheable 前缀段中:
	// 跨调用 frozen 内容相同时, 至少有一个 cacheable section chunk 的 hash 字节稳定。
	stableSectionFound := false
	for _, secName := range []string{aicache.SectionSemiDynamic, aicache.SectionTimeline} {
		var hashesA, hashesB []string
		for _, c := range splitA.Chunks {
			if c.Section == secName {
				hashesA = append(hashesA, c.Hash)
			}
		}
		for _, c := range splitB.Chunks {
			if c.Section == secName {
				hashesB = append(hashesB, c.Hash)
			}
		}
		if len(hashesA) > 0 && len(hashesA) == len(hashesB) {
			same := true
			for i := range hashesA {
				if hashesA[i] != hashesB[i] {
					same = false
					break
				}
			}
			if same {
				stableSectionFound = true
				break
			}
		}
	}
	require.True(t, stableSectionFound,
		"at least one cacheable section (semi-dynamic / timeline) must be byte-stable when only TimelineOpen differs")
}

// TestLiteForgePrompt_TimelineLegacyDumpFallback 验证当只填 TimelineDump (兼容字段)
// 时仍走老 PROMPT_SECTION_timeline 路径, 与历史调用方保持兼容。
//
// 关键词: aicache, LiteForge legacy timeline, TimelineDump 兼容, P0-B3
func TestLiteForgePrompt_TimelineLegacyDumpFallback(t *testing.T) {
	nonce := strings.ToLower(utils.RandStringBytes(6))
	rendered, err := renderLiteForgePrompt(liteForgePromptParams{
		Nonce:        nonce,
		Prompt:       "p",
		Schema:       `{"type":"object"}`,
		Params:       "x",
		TimelineDump: "legacy timeline dump body",
	})
	require.NoError(t, err)

	require.Contains(t, rendered, "<|PROMPT_SECTION_timeline|>",
		"legacy TimelineDump fallback must render PROMPT_SECTION_timeline tag")
	require.Contains(t, rendered, "legacy timeline dump body")
	require.NotContains(t, rendered, "<|AI_CACHE_FROZEN_semi-dynamic|>",
		"frozen wrap must not appear when TimelineFrozenBlock is empty")
	require.NotContains(t, rendered, "<|PROMPT_SECTION_timeline-open|>",
		"timeline-open section must not appear when TimelineOpen is empty")
}

// TestLiteForgePrompt_HighStaticTokenBudget 防止 LiteForge high-static 段后续被
// 误改瘦身, 跌破 dashscope/qwen 显式 prefix cache 的最小窗口。
//
// 关键词: aicache, LiteForge high-static, token budget, ytoken, prefix cache 阈值
//
// 历史背景: cachebench-20260507-221527.md 报告里, [high_static_too_short] advice
// 在 50-prompt 跑里告警 28 次, 全部来自 LiteForge 调用 (基线 199 token, 远低于
// dashscope 显式 prefix cache 的最小窗口 ~1500 token). 阈值取 1200 token 作为
// 硬下限 (用户调研值), 实际渲染应稳定在 1500+ token 一档, 双倍冗余防退化.
func TestLiteForgePrompt_HighStaticTokenBudget(t *testing.T) {
	const minTokens = 1200
	rendered, err := renderLiteForgePrompt(liteForgePromptParams{
		Nonce:             "fixed1",
		StaticInstruction: "instr",
		Prompt:            "p",
		Schema:            `{"type":"object"}`,
		Params:            "x",
	})
	require.NoError(t, err)

	split := aicache.Split(rendered)
	require.NotNil(t, split)
	hs := pickSection(t, split, aicache.SectionHighStatic)
	got := ytoken.CalcTokenCount(hs.Content)
	require.GreaterOrEqual(t, got, minTokens,
		"LiteForge high-static section must keep >= %d tokens to be cacheable by dashscope/qwen explicit prefix cache; got %d",
		minTokens, got)
}

func pickSection(t *testing.T, split *aicache.PromptSplit, section string) *aicache.Chunk {
	t.Helper()
	for _, c := range split.Chunks {
		if c.Section == section {
			return c
		}
	}
	t.Fatalf("section %q not found in split: chunks=%v", section, split.Chunks)
	return nil
}
