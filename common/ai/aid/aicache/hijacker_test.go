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
// 兼容 2 段（timeline 不可拆 frozen/open）与 3 段（timeline 含多 interval 或
// reducer+interval）两种结果。
// 关键词: aicache, hijacker, fixture 真实数据, 2 段 3 段兼容
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
			require.True(t, len(res.Messages) == 2 || len(res.Messages) == 3,
				"fixture %s should produce 2 or 3 messages, got %d", name, len(res.Messages))

			// system 中必须包含完整的 high-static 内容
			systemMsg := res.Messages[0]
			assert.Equal(t, "system", systemMsg.Role)
			systemContent := systemMsg.Content.(string)
			assert.Contains(t, systemContent, "<|AI_CACHE_SYSTEM_high-static|>")
			assert.Contains(t, systemContent, "<|AI_CACHE_SYSTEM_END_high-static|>")

			// 把所有 user 消息内容拼起来再 round-trip 检查 section 完整性
			var allUser strings.Builder
			for i := 1; i < len(res.Messages); i++ {
				assert.Equal(t, "user", res.Messages[i].Role)
				allUser.WriteString(res.Messages[i].Content.(string))
				allUser.WriteString("\n")
			}
			combined := allUser.String()

			for _, sec := range meta.Sections {
				if sec.Section == SectionHighStatic {
					continue
				}
				switch sec.Section {
				case SectionSemiDynamic:
					assert.Contains(t, combined, "<|PROMPT_SECTION_semi-dynamic|>")
				case SectionTimeline:
					assert.Contains(t, combined, "<|PROMPT_SECTION_timeline|>")
				case SectionDynamic:
					assert.Contains(t, combined, "<|PROMPT_SECTION_dynamic_")
				}
			}

			// round-trip：所有 user 消息合并后再 Split 应少 1 chunk（high-static 被搬走）
			// 3 段路径下 timeline 会被拆成两个独立 PROMPT_SECTION_timeline 块，
			// 因此 chunks 数会比"原 chunks - 1"再多 1。
			roundtrip := Split(combined)
			expectChunks := meta.DeclaredChunks - 1
			if len(res.Messages) == 3 {
				expectChunks++
			}
			assert.Equal(t, expectChunks, len(roundtrip.Chunks),
				"user msgs should round-trip to %d chunks (mode=%d-segment)",
				expectChunks, len(res.Messages))
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

// ---------------------------------------------------------------------------
// 3 段拆分专项测试 (§7.7 Timeline frozen/open 边界识别)
// ---------------------------------------------------------------------------

// buildPromptWithTimelineInner 构造一个含 high-static + semi-dynamic +
// timeline + dynamic 4 段的 prompt, timeline 段内嵌指定的 TIMELINE block raw
// 序列。每一项 timelineBlocks 都是已经包好 <|TIMELINE_xxx|>...<|TIMELINE_END_xxx|>
// 的字符串。
//
// 关键词: aicache, hijacker, 测试构造, timeline 内嵌
func buildPromptWithTimelineInner(timelineBlocks ...string) string {
	timelineInner := strings.Join(timelineBlocks, "\n")
	return strings.Join([]string{
		"<|AI_CACHE_SYSTEM_high-static|>\nstatic-CONTENT\n<|AI_CACHE_SYSTEM_END_high-static|>",
		"<|PROMPT_SECTION_semi-dynamic|>\nsd-CONTENT\n<|PROMPT_SECTION_END_semi-dynamic|>",
		"<|PROMPT_SECTION_timeline|>\n" + timelineInner + "\n<|PROMPT_SECTION_END_timeline|>",
		"<|PROMPT_SECTION_dynamic_nonceX|>\nuser query X\n<|PROMPT_SECTION_dynamic_END_nonceX|>",
	}, "\n\n")
}

func tlInterval(nonce, body string) string {
	return "<|TIMELINE_b3t" + nonce + "|>\n" + body + "\n<|TIMELINE_END_b3t" + nonce + "|>"
}

func tlReducer(nonce, body string) string {
	return "<|TIMELINE_r" + nonce + "t1|>\n" + body + "\n<|TIMELINE_END_r" + nonce + "t1|>"
}

// TestHijack_3SegSplit_MultiInterval timeline 内含 3 个 interval block
// 期望切成 3 段, 前 2 个 interval 进 user1 (frozen), 末 1 个 interval 进 user2 (open)
// 关键词: aicache, hijacker, 3 段拆分, 多 interval
func TestHijack_3SegSplit_MultiInterval(t *testing.T) {
	prompt := buildPromptWithTimelineInner(
		tlInterval("100", "bucket-A-content"),
		tlInterval("200", "bucket-B-content"),
		tlInterval("300", "bucket-C-content"),
	)
	res := hijackHighStatic(prompt)
	require.NotNil(t, res)
	require.True(t, res.IsHijacked)
	require.Len(t, res.Messages, 3, "should split to system + user1(frozen) + user2(open)")

	assert.Equal(t, "system", res.Messages[0].Role)
	assert.Equal(t, "user", res.Messages[1].Role)
	assert.Equal(t, "user", res.Messages[2].Role)

	system := res.Messages[0].Content.(string)
	user1 := res.Messages[1].Content.(string)
	user2 := res.Messages[2].Content.(string)

	assert.Contains(t, system, "static-CONTENT")
	assert.NotContains(t, user1, "static-CONTENT")
	assert.NotContains(t, user2, "static-CONTENT")

	// user1 = semi-dynamic + frozen-timeline (含 b100, b200)
	assert.Contains(t, user1, "<|PROMPT_SECTION_semi-dynamic|>")
	assert.Contains(t, user1, "sd-CONTENT")
	assert.Contains(t, user1, "<|PROMPT_SECTION_timeline|>")
	assert.Contains(t, user1, "bucket-A-content")
	assert.Contains(t, user1, "bucket-B-content")
	assert.NotContains(t, user1, "bucket-C-content", "C is the open bucket, must be in user2")
	assert.NotContains(t, user1, "PROMPT_SECTION_dynamic", "dynamic should be in user2")

	// user2 = open-timeline (含 b300) + dynamic
	assert.Contains(t, user2, "<|PROMPT_SECTION_timeline|>")
	assert.Contains(t, user2, "bucket-C-content")
	assert.NotContains(t, user2, "bucket-A-content")
	assert.NotContains(t, user2, "bucket-B-content")
	assert.Contains(t, user2, "<|PROMPT_SECTION_dynamic_nonceX|>")
	assert.Contains(t, user2, "user query X")
}

// TestHijack_3SegSplit_ReducerPlusInterval timeline 内含 reducer + 多 interval
// 期望 reducer + 前 N-1 interval 进 user1, 末 interval 进 user2
// 关键词: aicache, hijacker, 3 段拆分, reducer 与 interval 混合
func TestHijack_3SegSplit_ReducerPlusInterval(t *testing.T) {
	prompt := buildPromptWithTimelineInner(
		tlReducer("42", "compressed-summary"),
		tlInterval("400", "bucket-D-content"),
		tlInterval("500", "bucket-E-content"),
	)
	res := hijackHighStatic(prompt)
	require.NotNil(t, res)
	require.Len(t, res.Messages, 3)

	user1 := res.Messages[1].Content.(string)
	user2 := res.Messages[2].Content.(string)

	assert.Contains(t, user1, "compressed-summary", "reducer is frozen, should be in user1")
	assert.Contains(t, user1, "bucket-D-content", "non-last interval is frozen, should be in user1")
	assert.NotContains(t, user1, "bucket-E-content")

	assert.Contains(t, user2, "bucket-E-content", "last interval is open, should be in user2")
	assert.NotContains(t, user2, "compressed-summary")
	assert.NotContains(t, user2, "bucket-D-content")
}

// TestHijack_3SegSplit_ReducerPlusSingleInterval timeline 内含 1 reducer + 1 interval
// reducer 一定 frozen 进 user1, 单个 interval 是 open 进 user2 → 仍切 3 段
// 关键词: aicache, hijacker, 3 段拆分, 单 interval 加 reducer
func TestHijack_3SegSplit_ReducerPlusSingleInterval(t *testing.T) {
	prompt := buildPromptWithTimelineInner(
		tlReducer("11", "reducer-only"),
		tlInterval("999", "single-interval"),
	)
	res := hijackHighStatic(prompt)
	require.NotNil(t, res)
	require.Len(t, res.Messages, 3,
		"reducer (frozen) + 1 interval (open) should still split to 3 segments")

	user1 := res.Messages[1].Content.(string)
	user2 := res.Messages[2].Content.(string)
	assert.Contains(t, user1, "reducer-only")
	assert.NotContains(t, user1, "single-interval")
	assert.Contains(t, user2, "single-interval")
}

// TestHijack_3SegSplit_OnlyOneInterval timeline 内只有 1 个 interval block 且无 reducer
// → 没有 frozen 部分, 应退化到 2 段 (与 fixture 000010 行为一致)
// 关键词: aicache, hijacker, 退化 2 段, 单 interval
func TestHijack_3SegSplit_OnlyOneInterval(t *testing.T) {
	prompt := buildPromptWithTimelineInner(
		tlInterval("777", "lone-interval"),
	)
	res := hijackHighStatic(prompt)
	require.NotNil(t, res)
	require.Len(t, res.Messages, 2,
		"single interval without reducer should fallback to 2 segments")

	user := res.Messages[1].Content.(string)
	assert.Contains(t, user, "lone-interval")
	assert.Contains(t, user, "<|PROMPT_SECTION_timeline|>")
	assert.Contains(t, user, "<|PROMPT_SECTION_dynamic_nonceX|>")
}

// TestHijack_3SegSplit_OnlyReducer timeline 内只有 reducer 没 interval
// → 没有 open 段, 应退化到 2 段
// 关键词: aicache, hijacker, 退化 2 段, 全 reducer
func TestHijack_3SegSplit_OnlyReducer(t *testing.T) {
	prompt := buildPromptWithTimelineInner(
		tlReducer("1", "reducer-A"),
		tlReducer("2", "reducer-B"),
	)
	res := hijackHighStatic(prompt)
	require.NotNil(t, res)
	require.Len(t, res.Messages, 2,
		"only reducers without interval should fallback to 2 segments")

	user := res.Messages[1].Content.(string)
	assert.Contains(t, user, "reducer-A")
	assert.Contains(t, user, "reducer-B")
}

// TestHijack_3SegSplit_NoTimelineSection 完全无 timeline 段 → 自然 2 段
// 关键词: aicache, hijacker, 退化 2 段, 无 timeline
func TestHijack_3SegSplit_NoTimelineSection(t *testing.T) {
	prompt := strings.Join([]string{
		"<|AI_CACHE_SYSTEM_high-static|>\nstatic\n<|AI_CACHE_SYSTEM_END_high-static|>",
		"<|PROMPT_SECTION_semi-dynamic|>\nsd\n<|PROMPT_SECTION_END_semi-dynamic|>",
		"<|PROMPT_SECTION_dynamic_x|>\nq\n<|PROMPT_SECTION_dynamic_END_x|>",
	}, "\n\n")
	res := hijackHighStatic(prompt)
	require.NotNil(t, res)
	require.Len(t, res.Messages, 2, "no timeline section should produce 2 segments")
}

// TestHijack_3SegSplit_PrefixStable 同一 system + 同一 frozen-timeline 段在 2 次
// hijack 中, system 消息与 user1 消息字节级一致 (open 段不同), 这是双 cc 命中
// 的核心前置条件。
// 关键词: aicache, hijacker, 字节稳定, 3 段拆分前缀稳定
func TestHijack_3SegSplit_PrefixStable(t *testing.T) {
	frozen := []string{
		tlReducer("9", "reducer-frozen"),
		tlInterval("100", "frozen-A"),
		tlInterval("200", "frozen-B"),
	}
	prompt1 := buildPromptWithTimelineInner(append(frozen, tlInterval("300", "open-r1"))...)
	prompt2 := buildPromptWithTimelineInner(append(frozen, tlInterval("301", "open-r2"))...)

	r1 := hijackHighStatic(prompt1)
	r2 := hijackHighStatic(prompt2)
	require.NotNil(t, r1)
	require.NotNil(t, r2)
	require.Len(t, r1.Messages, 3)
	require.Len(t, r2.Messages, 3)

	assert.Equal(t, r1.Messages[0].Content, r2.Messages[0].Content,
		"system message must be byte-identical when high-static is unchanged")
	assert.Equal(t, r1.Messages[1].Content, r2.Messages[1].Content,
		"user1 (frozen prefix) must be byte-identical when frozen timeline is unchanged")
	assert.NotEqual(t, r1.Messages[2].Content, r2.Messages[2].Content,
		"user2 (open part) should differ across rounds when open bucket grows")
}
