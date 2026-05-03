package aicache

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHijack_FourSectionPrompt 验证 4 段完整 prompt 被切成 [system, user]，
// system 包含 AI_CACHE_SYSTEM 包装，user 含其余 3 段且能被 Split 重新识别。
// 关键词: aicache, hijacker, 4 段切分
func TestHijack_FourSectionPrompt(t *testing.T) {
	prompt := buildFourSectionPrompt("nonceA", "user query A", "tools=A", "static-A", "timeline-A", "memory-A")

	res := hijackHighStatic(prompt)
	require.NotNil(t, res)
	require.True(t, res.IsHijacked)
	require.Len(t, res.Messages, 2)

	system := res.Messages[0]
	user := res.Messages[1]
	assert.Equal(t, "system", system.Role)
	assert.Equal(t, "user", user.Role)

	systemContent, ok := system.Content.(string)
	require.True(t, ok)
	assert.Contains(t, systemContent, "<|AI_CACHE_SYSTEM_high-static|>")
	assert.Contains(t, systemContent, "<|AI_CACHE_SYSTEM_END_high-static|>")
	assert.Contains(t, systemContent, "static-A")

	userContent, ok := user.Content.(string)
	require.True(t, ok)
	// user 中不应再出现 high-static 段
	assert.NotContains(t, userContent, "static-A")
	// 其余 3 段都应保留原 PROMPT_SECTION 标签
	assert.Contains(t, userContent, "<|PROMPT_SECTION_semi-dynamic|>")
	assert.Contains(t, userContent, "<|PROMPT_SECTION_timeline|>")
	assert.Contains(t, userContent, "<|PROMPT_SECTION_dynamic_nonceA|>")

	// round-trip：Split(user) 应得到 3 个 chunk（semi-dynamic / timeline / dynamic）
	roundtrip := Split(userContent)
	require.Len(t, roundtrip.Chunks, 3)
	assert.Equal(t, SectionSemiDynamic, roundtrip.Chunks[0].Section)
	assert.Equal(t, SectionTimeline, roundtrip.Chunks[1].Section)
	assert.Equal(t, SectionDynamic, roundtrip.Chunks[2].Section)
}

// TestHijack_OnlyHighStatic 仅有 high-static 段时，user 内容为空，
// 透传不能只发 system 单条，必须返回 nil 让 ChatBase 走默认路径。
// 关键词: aicache, hijacker, 仅 high-static 透传
func TestHijack_OnlyHighStatic(t *testing.T) {
	prompt := "<|PROMPT_SECTION_high-static|>\nstatic only\n<|PROMPT_SECTION_END_high-static|>"
	res := hijackHighStatic(prompt)
	assert.Nil(t, res, "only high-static should not produce hijack")
}

// TestHijack_NoPromptSection 完全无 PROMPT_SECTION 标签的 prompt 必须透传
// 关键词: aicache, hijacker, 无标签透传
func TestHijack_NoPromptSection(t *testing.T) {
	prompt := "this prompt is just plain text without any PROMPT_SECTION wrapper"
	res := hijackHighStatic(prompt)
	assert.Nil(t, res, "no PROMPT_SECTION should not produce hijack")
}

// TestHijack_NoHighStaticButOtherSections 有 semi-dynamic / dynamic 但没有
// high-static 时也应透传：缺少需要"搬运到 system"的内容
// 关键词: aicache, hijacker, 无 high-static 透传
func TestHijack_NoHighStaticButOtherSections(t *testing.T) {
	prompt := strings.Join([]string{
		"<|PROMPT_SECTION_semi-dynamic|>\ntools=A\n<|PROMPT_SECTION_END_semi-dynamic|>",
		"<|PROMPT_SECTION_dynamic_nonceA|>\nuser query\n<|PROMPT_SECTION_dynamic_END_nonceA|>",
	}, "\n\n")
	res := hijackHighStatic(prompt)
	assert.Nil(t, res, "no high-static should not produce hijack")
}

// TestHijack_MultipleHighStaticBlocks 多个 high-static 块按出现顺序拼到同
// 一个 system 消息；user 段保留剩余 block.Raw
// 关键词: aicache, hijacker, 多 high-static 拼接
func TestHijack_MultipleHighStaticBlocks(t *testing.T) {
	prompt := strings.Join([]string{
		"<|PROMPT_SECTION_high-static|>\nfirst static\n<|PROMPT_SECTION_END_high-static|>",
		"<|PROMPT_SECTION_semi-dynamic|>\nsd content\n<|PROMPT_SECTION_END_semi-dynamic|>",
		"<|PROMPT_SECTION_high-static|>\nsecond static\n<|PROMPT_SECTION_END_high-static|>",
		"<|PROMPT_SECTION_dynamic_nonceA|>\nuser query\n<|PROMPT_SECTION_dynamic_END_nonceA|>",
	}, "\n\n")
	res := hijackHighStatic(prompt)
	require.NotNil(t, res)
	require.True(t, res.IsHijacked)
	require.Len(t, res.Messages, 2)

	systemContent := res.Messages[0].Content.(string)
	// 两段都要按出现顺序拼进 system
	firstAt := strings.Index(systemContent, "first static")
	secondAt := strings.Index(systemContent, "second static")
	assert.True(t, firstAt >= 0 && secondAt > firstAt, "two high-static blocks should appear in order")

	userContent := res.Messages[1].Content.(string)
	assert.Contains(t, userContent, "<|PROMPT_SECTION_semi-dynamic|>")
	assert.Contains(t, userContent, "<|PROMPT_SECTION_dynamic_nonceA|>")
	assert.NotContains(t, userContent, "first static")
	assert.NotContains(t, userContent, "second static")
}

// TestHijack_EmptyMsg 空 prompt 应直接透传
// 关键词: aicache, hijacker, 空 prompt
func TestHijack_EmptyMsg(t *testing.T) {
	assert.Nil(t, hijackHighStatic(""))
	assert.Nil(t, hijackHighStatic("   \n  \t\n"))
}

// TestHijack_PrefixStable 多次 hijack 同一 high-static 段，system 消息字节
// 完全一致，是隐式缓存命中所需的字节稳定性。
// 关键词: aicache, hijacker, 字节稳定
func TestHijack_PrefixStable(t *testing.T) {
	prompt1 := buildFourSectionPrompt("n1", "u1", "tools", "S", "tl", "mem")
	prompt2 := buildFourSectionPrompt("n2", "u2", "tools", "S", "tl", "mem")

	r1 := hijackHighStatic(prompt1)
	r2 := hijackHighStatic(prompt2)
	require.NotNil(t, r1)
	require.NotNil(t, r2)
	assert.Equal(t, r1.Messages[0].Content, r2.Messages[0].Content,
		"system message should be byte-identical across calls when high-static is unchanged")
}

// TestHijack_NewTagFourSectionPrompt 验证新形态 AI_CACHE_SYSTEM_high-static
// 也能被 hijack 切出来；其他段保持 PROMPT_SECTION（与生产实际一致）。
// 关键词: aicache, hijacker, AI_CACHE_SYSTEM, 新标签
func TestHijack_NewTagFourSectionPrompt(t *testing.T) {
	prompt := strings.Join([]string{
		"<|AI_CACHE_SYSTEM_high-static|>\nstatic-A\n<|AI_CACHE_SYSTEM_END_high-static|>",
		"<|PROMPT_SECTION_semi-dynamic|>\ntools=A\n<|PROMPT_SECTION_END_semi-dynamic|>",
		"<|PROMPT_SECTION_timeline|>\ntimeline-A\n<|PROMPT_SECTION_END_timeline|>",
		"<|PROMPT_SECTION_dynamic_nonceA|>\nuser query A\n<|PROMPT_SECTION_dynamic_END_nonceA|>",
	}, "\n\n")

	res := hijackHighStatic(prompt)
	require.NotNil(t, res)
	require.True(t, res.IsHijacked)
	require.Len(t, res.Messages, 2)

	systemContent := res.Messages[0].Content.(string)
	assert.Contains(t, systemContent, "<|AI_CACHE_SYSTEM_high-static|>")
	assert.Contains(t, systemContent, "static-A")

	userContent := res.Messages[1].Content.(string)
	assert.NotContains(t, userContent, "static-A")
	assert.Contains(t, userContent, "<|PROMPT_SECTION_semi-dynamic|>")
	assert.Contains(t, userContent, "<|PROMPT_SECTION_dynamic_nonceA|>")

	// user 段 round-trip 应得到 3 个 chunk（无 high-static）
	roundtrip := Split(userContent)
	require.Len(t, roundtrip.Chunks, 3)
}

// TestHijack_NewAndOldTagsCoexist 老服务器与新服务器混合时（dump 里既有
// AI_CACHE_SYSTEM_high-static 也有 PROMPT_SECTION_high-static），两段都应该
// 被 hijack 收进 system 消息。
// 关键词: aicache, hijacker, 双标签共存
func TestHijack_NewAndOldTagsCoexist(t *testing.T) {
	prompt := strings.Join([]string{
		"<|PROMPT_SECTION_high-static|>\nold-style static\n<|PROMPT_SECTION_END_high-static|>",
		"<|PROMPT_SECTION_semi-dynamic|>\nsd content\n<|PROMPT_SECTION_END_semi-dynamic|>",
		"<|AI_CACHE_SYSTEM_high-static|>\nnew-style static\n<|AI_CACHE_SYSTEM_END_high-static|>",
		"<|PROMPT_SECTION_dynamic_nonceA|>\nuser query\n<|PROMPT_SECTION_dynamic_END_nonceA|>",
	}, "\n\n")

	res := hijackHighStatic(prompt)
	require.NotNil(t, res)
	require.True(t, res.IsHijacked)

	systemContent := res.Messages[0].Content.(string)
	firstAt := strings.Index(systemContent, "old-style static")
	secondAt := strings.Index(systemContent, "new-style static")
	assert.True(t, firstAt >= 0, "old-style high-static should be in system")
	assert.True(t, secondAt > firstAt, "new-style high-static should follow old-style in source order")

	userContent := res.Messages[1].Content.(string)
	assert.NotContains(t, userContent, "old-style static")
	assert.NotContains(t, userContent, "new-style static")
}

// TestHijack_FixtureFourSection 用真实生产 dump 验证 hijacker 行为
// 关键词: aicache, hijacker, fixture 真实数据
func TestHijack_FixtureFourSection(t *testing.T) {
	cases := []string{"000005.txt", "000010.txt", "000060.txt"}
	for _, name := range cases {
		name := name
		t.Run(name, func(t *testing.T) {
			meta := loadFixtureRawPrompt(t, name)
			split := Split(meta.Raw)
			require.Equal(t, meta.DeclaredChunks, len(split.Chunks))

			res := hijackHighStatic(meta.Raw)
			require.NotNil(t, res, "fixture %s has high-static, should hijack", name)
			require.True(t, res.IsHijacked)
			require.Len(t, res.Messages, 2)

			// system 中必须包含完整的 high-static 内容
			systemContent := res.Messages[0].Content.(string)
			assert.Contains(t, systemContent, "<|AI_CACHE_SYSTEM_high-static|>")
			assert.Contains(t, systemContent, "<|AI_CACHE_SYSTEM_END_high-static|>")

			// user 中其他 section 必须仍然存在
			userContent := res.Messages[1].Content.(string)
			for _, sec := range meta.Sections {
				if sec.Section == SectionHighStatic {
					continue
				}
				switch sec.Section {
				case SectionSemiDynamic:
					assert.Contains(t, userContent, "<|PROMPT_SECTION_semi-dynamic|>")
				case SectionTimeline:
					assert.Contains(t, userContent, "<|PROMPT_SECTION_timeline|>")
				case SectionDynamic:
					// dynamic section 标签里带 nonce，截前缀就够了
					assert.Contains(t, userContent, "<|PROMPT_SECTION_dynamic_")
				}
			}

			// round-trip：user 再 Split 应少 1 chunk（high-static 被搬走）
			roundtrip := Split(userContent)
			assert.Equal(t, meta.DeclaredChunks-1, len(roundtrip.Chunks),
				"user msg should round-trip to %d chunks", meta.DeclaredChunks-1)
		})
	}
}

// TestHijack_FixtureRawNoSection 真实 dump 中的 raw prompt（无任何
// PROMPT_SECTION 标签）应透传
// 关键词: aicache, hijacker, raw fixture 透传
func TestHijack_FixtureRawNoSection(t *testing.T) {
	meta := loadFixtureRawPrompt(t, "000045.txt")
	require.Equal(t, 1, meta.DeclaredChunks)
	require.Equal(t, SectionRaw, meta.Sections[0].Section)

	res := hijackHighStatic(meta.Raw)
	assert.Nil(t, res, "raw prompt without PROMPT_SECTION should not be hijacked")
}

// TestHijack_FixtureThreeSection 3 段 prompt（无 timeline）也能被 hijack
// 关键词: aicache, hijacker, 3 段 fixture
func TestHijack_FixtureThreeSection(t *testing.T) {
	meta := loadFixtureRawPrompt(t, "000001.txt")
	require.Equal(t, 3, meta.DeclaredChunks)

	res := hijackHighStatic(meta.Raw)
	require.NotNil(t, res)
	require.True(t, res.IsHijacked)
	require.Len(t, res.Messages, 2)

	roundtrip := Split(res.Messages[1].Content.(string))
	assert.Equal(t, 2, len(roundtrip.Chunks), "3-section fixture should round-trip user to 2 chunks")
}
