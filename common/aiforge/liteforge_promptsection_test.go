package aiforge

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aicache"
	"github.com/yaklang/yaklang/common/utils"
)

// 验证 LiteForge 模板能被 aicache.Split 切成预期的 4 段
// 关键词: aicache, PROMPT_SECTION, LiteForge 模板, 4 段切片
func TestLiteForgePrompt_SplitsIntoFourSections(t *testing.T) {
	nonce := strings.ToLower(utils.RandStringBytes(6))
	rendered, err := renderLiteForgePrompt(liteForgePromptParams{
		Nonce:            nonce,
		Prompt:           "you are a capability matcher.",
		Params:           "user input value",
		Schema:           `{"type":"object","properties":{"matched":{"type":"array"}}}`,
		PersistentMemory: "remember: prefer chinese keywords",
		TimelineDump:     "tool[1] success at 12:00",
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

	hs := sections[aicache.SectionHighStatic]
	require.Contains(t, hs.Content, "# Preset")
	require.Contains(t, hs.Content, "# Output Formatter")
	require.Contains(t, hs.Content, "# SCHEMA")
	require.Contains(t, hs.Content, "<schema>")
	require.Contains(t, hs.Content, "# Instruction")
	require.Contains(t, hs.Content, "<background>")
	require.Contains(t, hs.Content, "you are a capability matcher.")
	// high-static 段内不应出现任何 nonce 字符串：去 nonce 是核心改造
	// 关键词: aicache, high-static, 去 nonce
	require.NotContains(t, hs.Content, nonce, "high-static section MUST NOT contain nonce")

	sd := sections[aicache.SectionSemiDynamic]
	require.Contains(t, sd.Content, "# 牢记")
	require.Contains(t, sd.Content, "remember: prefer chinese keywords")

	tl := sections[aicache.SectionTimeline]
	require.Contains(t, tl.Content, "<timeline_"+nonce+">")
	require.Contains(t, tl.Content, "tool[1] success at 12:00")

	dy := sections[aicache.SectionDynamic]
	require.Equal(t, aicache.SectionDynamic+"_"+nonce, dy.Nonce)
	require.Contains(t, dy.Content, "<params_"+nonce+">")
	require.Contains(t, dy.Content, "user input value")
}

// 验证去 nonce 的核心目标：相同输入两次渲染（每次 nonce 都不同），
// 生成的 high-static 段字节内容必须完全一致，保证 hash 稳定
// 关键词: aicache, high-static, 跨调用稳定, 去 nonce 核心回归测试
func TestLiteForgePrompt_HighStaticStableAcrossNonces(t *testing.T) {
	const (
		prompt = "you are a capability matcher with strict identifier rules."
		schema = `{"type":"object","properties":{"matched_identifiers":{"type":"array"}}}`
		params = "hostscan"
	)

	nonceA := strings.ToLower(utils.RandStringBytes(6))
	nonceB := strings.ToLower(utils.RandStringBytes(6))
	require.NotEqual(t, nonceA, nonceB, "two random nonces should differ")

	renderA, err := renderLiteForgePrompt(liteForgePromptParams{
		Nonce:  nonceA,
		Prompt: prompt,
		Schema: schema,
		Params: params,
	})
	require.NoError(t, err)
	renderB, err := renderLiteForgePrompt(liteForgePromptParams{
		Nonce:  nonceB,
		Prompt: prompt,
		Schema: schema,
		Params: params,
	})
	require.NoError(t, err)

	splitA := aicache.Split(renderA)
	splitB := aicache.Split(renderB)
	require.NotNil(t, splitA)
	require.NotNil(t, splitB)

	hsA := pickSection(t, splitA, aicache.SectionHighStatic)
	hsB := pickSection(t, splitB, aicache.SectionHighStatic)

	require.Equal(t, hsA.Content, hsB.Content,
		"high-static content must be byte-identical across different nonces")
	require.Equal(t, hsA.Hash, hsB.Hash,
		"high-static hash must be stable across different nonces")

	// dynamic 段反而每次不同（外层 nonce 不同 + inner <params_NONCE> 不同）
	dyA := pickSection(t, splitA, aicache.SectionDynamic)
	dyB := pickSection(t, splitB, aicache.SectionDynamic)
	require.NotEqual(t, dyA.Hash, dyB.Hash,
		"dynamic hash should differ when nonce differs (anti-injection by design)")
}

// 验证 timeline 段为空时整段被省略，aicache 切出 3 段
// 关键词: aicache, timeline, 空段省略
func TestLiteForgePrompt_TimelineEmptyOmitsSection(t *testing.T) {
	nonce := strings.ToLower(utils.RandStringBytes(6))
	rendered, err := renderLiteForgePrompt(liteForgePromptParams{
		Nonce:        nonce,
		Prompt:       "static instruction",
		Schema:       `{"type":"object"}`,
		Params:       "p",
		TimelineDump: "",
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

// 验证 dynamic 段外层 nonce 与内部 <params_NONCE> 是同一字符串
// 关键词: aicache, dynamic, nonce 一致性
func TestLiteForgePrompt_DynamicNonceConsistent(t *testing.T) {
	nonce := strings.ToLower(utils.RandStringBytes(6))
	rendered, err := renderLiteForgePrompt(liteForgePromptParams{
		Nonce:  nonce,
		Prompt: "x",
		Schema: "y",
		Params: "z",
	})
	require.NoError(t, err)
	split := aicache.Split(rendered)
	require.NotNil(t, split)

	dy := pickSection(t, split, aicache.SectionDynamic)
	require.Equal(t, aicache.SectionDynamic+"_"+nonce, dy.Nonce,
		"dynamic chunk nonce should equal section + outer NONCE")
	require.Contains(t, dy.Content, "<params_"+nonce+">")
	require.Contains(t, dy.Content, "</params_"+nonce+">")
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
