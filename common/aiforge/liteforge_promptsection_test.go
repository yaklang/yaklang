package aiforge

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aicache"
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

	// high-static 段：含 Preset / Output Formatter / SCHEMA / Instruction(StaticInstruction)
	hs := sections[aicache.SectionHighStatic]
	require.Contains(t, hs.Content, "# Preset")
	require.Contains(t, hs.Content, "# Output Formatter")
	require.Contains(t, hs.Content, "# SCHEMA")
	require.Contains(t, hs.Content, "<schema>")
	require.Contains(t, hs.Content, "# Instruction")
	require.Contains(t, hs.Content, "<instruction>")
	require.Contains(t, hs.Content, "you are a capability matcher.",
		"StaticInstruction content must appear inside high-static section")
	// high-static 段不应含 Prompt 内容（已经移到 dynamic 段）
	require.NotContains(t, hs.Content, "user_query=hostscan",
		"Prompt content must NOT appear in high-static section anymore (B-tier moved Prompt to dynamic)")
	// high-static 段内不应出现任何 nonce 字符串
	require.NotContains(t, hs.Content, nonce, "high-static section MUST NOT contain nonce")

	// semi-dynamic 段：persistent memory
	sd := sections[aicache.SectionSemiDynamic]
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

// TestLiteForgePrompt_TimelineEmptyOmitsSection 验证 timeline 段为空时整段被省略
// 关键词: aicache, timeline, 空段省略
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
	require.NotContains(t, rendered, "<|PROMPT_SECTION_timeline|>")

	split := aicache.Split(rendered)
	require.NotNil(t, split)
	require.Len(t, split.Chunks, 3, "expect 3 sections when timeline is empty")

	for _, c := range split.Chunks {
		require.NotEqual(t, aicache.SectionTimeline, c.Section)
	}
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
	require.Contains(t, hs.Content, "<schema>")
	require.NotContains(t, hs.Content, "# Instruction",
		"# Instruction block should be omitted when StaticInstruction is empty")
	require.NotContains(t, hs.Content, "<instruction>",
		"<instruction> tag should be omitted when StaticInstruction is empty")
	require.NotContains(t, hs.Content, "old caller pattern: everything in Prompt",
		"Prompt content must NOT leak into high-static when StaticInstruction is empty")

	// 老调用方传入的 Prompt 现在出现在 dynamic 段
	dy := pickSection(t, split, aicache.SectionDynamic)
	require.Contains(t, dy.Content, "old caller pattern: everything in Prompt")
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
