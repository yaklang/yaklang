package aicache

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 关键词: aicache, splitter, 完整 4 段切片
func TestSplit_FourSections(t *testing.T) {
	prompt := buildFourSectionPrompt("nonceA", "user query A", "tools=A", "static-A", "timeline-A", "memory-A")

	split := Split(prompt)
	require.NotNil(t, split)
	require.Len(t, split.Chunks, 4)

	assert.Equal(t, SectionHighStatic, split.Chunks[0].Section)
	assert.Equal(t, SectionHighStatic, split.Chunks[0].Nonce)

	assert.Equal(t, SectionSemiDynamic, split.Chunks[1].Section)
	assert.Equal(t, SectionSemiDynamic, split.Chunks[1].Nonce)

	assert.Equal(t, SectionTimeline, split.Chunks[2].Section)
	assert.Equal(t, SectionTimeline, split.Chunks[2].Nonce)

	assert.Equal(t, SectionDynamic, split.Chunks[3].Section)
	assert.Equal(t, "dynamic_nonceA", split.Chunks[3].Nonce)

	for _, ch := range split.Chunks {
		assert.NotEmpty(t, ch.Hash)
		assert.Equal(t, len(ch.Content), ch.Bytes)
	}
}

// 关键词: aicache, splitter, 仅含一段, 老 PROMPT_SECTION 兼容
func TestSplit_OnlyHighStatic(t *testing.T) {
	prompt := "<|PROMPT_SECTION_high-static|>\nstatic body\n<|PROMPT_SECTION_END_high-static|>"

	split := Split(prompt)
	require.Len(t, split.Chunks, 1)
	assert.Equal(t, SectionHighStatic, split.Chunks[0].Section)
	assert.Equal(t, "static body", split.Chunks[0].Content)
}

// TestSplit_OnlyHighStatic_AICacheSystemTag 验证新形态
// <|AI_CACHE_SYSTEM_high-static|> 也能被切到 high-static section。
// 关键词: aicache, splitter, AI_CACHE_SYSTEM, 新标签形态
func TestSplit_OnlyHighStatic_AICacheSystemTag(t *testing.T) {
	prompt := "<|AI_CACHE_SYSTEM_high-static|>\nstatic body\n<|AI_CACHE_SYSTEM_END_high-static|>"

	split := Split(prompt)
	require.Len(t, split.Chunks, 1)
	assert.Equal(t, SectionHighStatic, split.Chunks[0].Section)
	assert.Equal(t, SectionHighStatic, split.Chunks[0].Nonce)
	assert.Equal(t, "static body", split.Chunks[0].Content)
}

// TestSplit_HighStaticTagEquivalence 验证新老两种 high-static tagName
// 切出来的 chunk hash 完全一致（同 Section + 同 Content => 同 Hash），
// 这是"老服务器 dump 与新服务器 dump 在 aicache 表里复用同一缓存槽位"
// 这一关键属性的回归测试。
// 关键词: aicache, splitter, AI_CACHE_SYSTEM, PROMPT_SECTION 等价 hash
func TestSplit_HighStaticTagEquivalence(t *testing.T) {
	body := "static body"
	oldStyle := "<|PROMPT_SECTION_high-static|>\n" + body + "\n<|PROMPT_SECTION_END_high-static|>"
	newStyle := "<|AI_CACHE_SYSTEM_high-static|>\n" + body + "\n<|AI_CACHE_SYSTEM_END_high-static|>"

	a := Split(oldStyle)
	b := Split(newStyle)
	require.Len(t, a.Chunks, 1)
	require.Len(t, b.Chunks, 1)
	assert.Equal(t, a.Chunks[0].Section, b.Chunks[0].Section)
	assert.Equal(t, a.Chunks[0].Hash, b.Chunks[0].Hash, "old and new high-static tags must hash to same value")
}

// 关键词: aicache, splitter, 无标签退化为 raw
func TestSplit_NoTag(t *testing.T) {
	prompt := "this prompt is just plain text without any PROMPT_SECTION wrapper"

	split := Split(prompt)
	require.Len(t, split.Chunks, 1)
	assert.Equal(t, SectionRaw, split.Chunks[0].Section)
	assert.Equal(t, prompt, split.Chunks[0].Content)
}

// 关键词: aicache, splitter, 空字符串
func TestSplit_Empty(t *testing.T) {
	split := Split("")
	require.NotNil(t, split)
	assert.Empty(t, split.Chunks)
	assert.Equal(t, 0, split.Bytes)
}

// 关键词: aicache, splitter, 哈希稳定性
func TestSplit_HashStability(t *testing.T) {
	prompt1 := buildFourSectionPrompt("nonceA", "q", "tools", "static", "timeline", "memory")
	prompt2 := buildFourSectionPrompt("nonceB", "q", "tools", "static", "timeline", "memory")

	s1 := Split(prompt1)
	s2 := Split(prompt2)
	require.Len(t, s1.Chunks, 4)
	require.Len(t, s2.Chunks, 4)

	// 内层 dynamic nonce 不同，但其它三段哈希应当相同
	assert.Equal(t, s1.Chunks[0].Hash, s2.Chunks[0].Hash, "high-static hash should be stable")
	assert.Equal(t, s1.Chunks[1].Hash, s2.Chunks[1].Hash, "semi-dynamic hash should be stable")
	assert.Equal(t, s1.Chunks[2].Hash, s2.Chunks[2].Hash, "timeline hash should be stable")
	// dynamic 段内容相同（USER_QUERY 内的 nonce 不同），所以 hash 会变
	// 这里仅断言 dynamic chunk 存在且 nonce 反映出来
	assert.NotEqual(t, s1.Chunks[3].Nonce, s2.Chunks[3].Nonce)
}

// buildFourSectionPrompt 还原 aireact 真实拼接结构
// 注意：动态段的结束标签顺序与 wrapPromptMessageSection 保持一致，
// 即 <|PROMPT_SECTION_dynamic_<nonce>|>...<|PROMPT_SECTION_dynamic_END_<nonce>|>
// 关键词: aicache, test helper, 四段 prompt 构造
func buildFourSectionPrompt(innerNonce, userQuery, tools, staticBody, timelineBody, memoryBody string) string {
	dynamicInner := "<|USER_QUERY_" + innerNonce + "|>\n" + userQuery + "\n<|USER_QUERY_END_" + innerNonce + "|>\n" +
		"<|INJECTED_MEMORY_" + innerNonce + "|>\n" + memoryBody + "\n<|INJECTED_MEMORY_END_" + innerNonce + "|>"

	parts := []string{
		"<|PROMPT_SECTION_high-static|>\n" + staticBody + "\n<|PROMPT_SECTION_END_high-static|>",
		"<|PROMPT_SECTION_semi-dynamic|>\n" + tools + "\n<|PROMPT_SECTION_END_semi-dynamic|>",
		"<|PROMPT_SECTION_timeline|>\n" + timelineBody + "\n<|PROMPT_SECTION_END_timeline|>",
		"<|PROMPT_SECTION_dynamic_" + innerNonce + "|>\n" + dynamicInner + "\n<|PROMPT_SECTION_dynamic_END_" + innerNonce + "|>",
	}
	return strings.Join(parts, "\n\n")
}
