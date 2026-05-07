package aicache

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

// TestMain 关闭 P2.1 阈值合并 (minCachableUserSegmentBytes = 0), 让本文件
// 既有 happy-path 测试 (4/3 段断言) 都使用很短的 fixture 内容也能命中 happy
// path. 新增的 P2.1 降级路径用例必须用 setHijackerThresholdMerge(t, 1024)
// 显式打开阈值合并, 才能验证降级行为.
//
// 设计理由: 现有 16+ 个 happy-path 测试 fixture 用 ~100-300 byte 短内容,
// 默认 1024 byte 阈值下全部会被合并降级, 修改 fixture 工程量大且会污染断言.
// 在 TestMain 关闭阈值是最低成本且 isolation 良好的方案.
//
// 关键词: aicache, P2.1, hijacker test main, 阈值合并默认关闭, 降级路径显式开启
func TestMain(m *testing.M) {
	saved := minCachableUserSegmentBytes
	minCachableUserSegmentBytes = 0
	code := m.Run()
	minCachableUserSegmentBytes = saved
	os.Exit(code)
}

// extractTextContent 把 hijacker 输出的 message.Content 提取成 string,
// 兼容两种形态:
//   - string (2 段退化路径 / 3 段路径下的 user2)
//   - []*aispec.ChatContent (3 段路径下的 system / user1, 带 cache_control)
//
// §7.7.7 后 hijacker 在 3 段路径下会把 system+user1 包成 ChatContent 数组
// 加 ephemeral cache_control 标记, 测试断言 user 内容时需要先用此 helper
// 提取出底层文本再做 contains/equal 判断。
//
// 关键词: aicache, hijacker test helper, extractTextContent, content 形态兼容
func extractTextContent(t *testing.T, content any) string {
	t.Helper()
	switch v := content.(type) {
	case string:
		return v
	case []*aispec.ChatContent:
		var sb strings.Builder
		for _, c := range v {
			if c != nil {
				sb.WriteString(c.Text)
			}
		}
		return sb.String()
	default:
		t.Fatalf("unexpected message.Content type: %T", content)
		return ""
	}
}

// assertHasEphemeralCacheControl 断言 content 是 []*aispec.ChatContent 形态
// 且至少有一个非 nil 元素的 CacheControl == map[string]any{"type":"ephemeral"}.
// 用于验证 3 段路径下 hijacker 主动打的 cc 字段。
//
// 关键词: aicache, hijacker test helper, ephemeral cc 断言
func assertHasEphemeralCacheControl(t *testing.T, content any, label string) {
	t.Helper()
	contents, ok := content.([]*aispec.ChatContent)
	require.True(t, ok, "%s: expected []*aispec.ChatContent, got %T", label, content)
	require.NotEmpty(t, contents, "%s: ChatContent slice must not be empty", label)
	found := false
	for _, c := range contents {
		if c == nil {
			continue
		}
		if cc, ok := c.CacheControl.(map[string]any); ok && cc["type"] == "ephemeral" {
			found = true
			break
		}
	}
	require.True(t, found, "%s: must contain at least one ephemeral cache_control marker", label)
}

// assertNoCacheControl 断言 content 完全不带 cache_control:
// 要么是 string, 要么是 []*ChatContent 但所有元素的 CacheControl 都为 nil.
// 用于验证 user2 (open 段) 不被 hijacker 打 cc。
//
// 关键词: aicache, hijacker test helper, 无 cc 断言
func assertNoCacheControl(t *testing.T, content any, label string) {
	t.Helper()
	switch v := content.(type) {
	case string:
		return
	case []*aispec.ChatContent:
		for i, c := range v {
			if c == nil {
				continue
			}
			require.Nil(t, c.CacheControl,
				"%s: element[%d] must NOT carry cache_control, got %v", label, i, c.CacheControl)
		}
	default:
		t.Fatalf("%s: unexpected content type: %T", label, content)
	}
}

// disableHijackerThresholdMerge 临时把 P2.1 阈值合并阈值
// (minCachableUserSegmentBytes) 设为 0, 关闭 build4/build3 的短 prompt 合并/旁路
// 逻辑, 让现有 happy-path 测试在不修改 fixture 大小的前提下仍能走 4/3 段路径.
// t.Cleanup 自动恢复原值, 串行测试下不会污染其他用例.
//
// 用法:
//
//	func TestXxx(t *testing.T) {
//	    disableHijackerThresholdMerge(t)
//	    // 后续测试断言 4 段 / 3 段 happy path
//	}
//
// 关键词: aicache, hijacker test helper, P2.1 阈值合并 disable, 测试覆盖
func disableHijackerThresholdMerge(t *testing.T) {
	t.Helper()
	saved := minCachableUserSegmentBytes
	minCachableUserSegmentBytes = 0
	t.Cleanup(func() { minCachableUserSegmentBytes = saved })
}

// setHijackerThresholdMerge 临时把 P2.1 阈值合并阈值设为指定值, 用于 P2.1 降级
// 用例中精细控制阈值 (例如设 50 验证 user1=10 byte 时的合并行为). t.Cleanup
// 自动恢复.
//
// 关键词: aicache, hijacker test helper, P2.1 阈值合并 setter, 测试覆盖
func setHijackerThresholdMerge(t *testing.T, threshold int) {
	t.Helper()
	saved := minCachableUserSegmentBytes
	minCachableUserSegmentBytes = threshold
	t.Cleanup(func() { minCachableUserSegmentBytes = saved })
}

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
			// 2 段路径下 system 是 string, 3 段路径下是 []*ChatContent (带 cc)
			systemMsg := res.Messages[0]
			assert.Equal(t, "system", systemMsg.Role)
			systemContent := extractTextContent(t, systemMsg.Content)
			assert.Contains(t, systemContent, "<|AI_CACHE_SYSTEM_high-static|>")
			assert.Contains(t, systemContent, "<|AI_CACHE_SYSTEM_END_high-static|>")

			// 3 段路径下 system 必须自带 ephemeral cc (§7.7.7 hijacker 自管 cc)
			if len(res.Messages) == 3 {
				assertHasEphemeralCacheControl(t, systemMsg.Content,
					"fixture "+name+" system (3-segment)")
				assertHasEphemeralCacheControl(t, res.Messages[1].Content,
					"fixture "+name+" user1 (3-segment)")
				assertNoCacheControl(t, res.Messages[2].Content,
					"fixture "+name+" user2 (3-segment, open part)")
			}

			// 把所有 user 消息内容拼起来再 round-trip 检查 section 完整性
			var allUser strings.Builder
			for i := 1; i < len(res.Messages); i++ {
				assert.Equal(t, "user", res.Messages[i].Role)
				allUser.WriteString(extractTextContent(t, res.Messages[i].Content))
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

	system := extractTextContent(t, res.Messages[0].Content)
	user1 := extractTextContent(t, res.Messages[1].Content)
	user2 := extractTextContent(t, res.Messages[2].Content)

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

	user1 := extractTextContent(t, res.Messages[1].Content)
	user2 := extractTextContent(t, res.Messages[2].Content)

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

	user1 := extractTextContent(t, res.Messages[1].Content)
	user2 := extractTextContent(t, res.Messages[2].Content)
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

// TestHijack_3SegSplit_SystemAndUser1HaveCacheControl 验证 §7.7.7 hijacker
// 自管双 cc: 3 段路径下 system + user1 主动包成 []*aispec.ChatContent 并
// 挂 ephemeral cache_control; user2 (open 段) 保持 string 不打 cc。
// 这是 aibalance 退让协议 (messagesAlreadyHaveCacheControl) 的触发前提。
// 关键词: aicache, hijacker, 双 cc 自管, ephemeral cache_control, §7.7.7
func TestHijack_3SegSplit_SystemAndUser1HaveCacheControl(t *testing.T) {
	prompt := buildPromptWithTimelineInner(
		tlReducer("9", "reducer-frozen"),
		tlInterval("100", "frozen-A"),
		tlInterval("200", "frozen-B"),
		tlInterval("300", "open-tail"),
	)
	res := hijackHighStatic(prompt)
	require.NotNil(t, res)
	require.True(t, res.IsHijacked)
	require.Len(t, res.Messages, 3, "hijacker self-managed dual cc requires 3-segment path")

	// system: 必须是 []*ChatContent 形态, 末元素 (本场景下唯一) 带 ephemeral cc
	sysContents, ok := res.Messages[0].Content.([]*aispec.ChatContent)
	require.True(t, ok, "system content must be []*ChatContent in 3-segment path, got %T", res.Messages[0].Content)
	require.Len(t, sysContents, 1, "hijacker wraps system into single-element ChatContent slice")
	require.Equal(t, "text", sysContents[0].Type)
	require.NotEmpty(t, sysContents[0].Text)
	cc, ok := sysContents[0].CacheControl.(map[string]any)
	require.True(t, ok, "system CacheControl must be map[string]any, got %T", sysContents[0].CacheControl)
	require.Equal(t, "ephemeral", cc["type"], "system cc must be ephemeral")

	// user1: 同上
	user1Contents, ok := res.Messages[1].Content.([]*aispec.ChatContent)
	require.True(t, ok, "user1 content must be []*ChatContent in 3-segment path, got %T", res.Messages[1].Content)
	require.Len(t, user1Contents, 1)
	require.Equal(t, "text", user1Contents[0].Type)
	require.NotEmpty(t, user1Contents[0].Text)
	cc1, ok := user1Contents[0].CacheControl.(map[string]any)
	require.True(t, ok, "user1 CacheControl must be map[string]any, got %T", user1Contents[0].CacheControl)
	require.Equal(t, "ephemeral", cc1["type"], "user1 cc must be ephemeral")

	// user2: 必须是 string, 不带任何 cc 字段 (open 段易变, 不缓存)
	user2Str, ok := res.Messages[2].Content.(string)
	require.True(t, ok, "user2 (open) content must be string, got %T", res.Messages[2].Content)
	require.NotEmpty(t, user2Str)
	require.Contains(t, user2Str, "open-tail", "user2 should carry the open timeline tail")
}

// TestHijack_3SegSplit_2SegFallbackHasNoCC 退化到 2 段时, hijacker 不打 cc
// (system + user 都是 string), 由 aibalance 走"baseline 单 cc 兜底"路径
// 给最末 system 注入 cc。这保证退化路径不破坏 aibalance 现有行为。
// 关键词: aicache, hijacker, 2 段退化路径, 不打 cc, aibalance 兜底
func TestHijack_3SegSplit_2SegFallbackHasNoCC(t *testing.T) {
	// timeline 只有 1 个 interval → 退化 2 段
	prompt := buildPromptWithTimelineInner(
		tlInterval("777", "lone-interval"),
	)
	res := hijackHighStatic(prompt)
	require.NotNil(t, res)
	require.Len(t, res.Messages, 2, "single interval should fallback to 2 segments")

	// 2 段路径下 system 与 user 都应该是简单 string, 不带 cc
	sysStr, ok := res.Messages[0].Content.(string)
	require.True(t, ok, "2-seg fallback system must be string, got %T", res.Messages[0].Content)
	require.NotEmpty(t, sysStr)
	userStr, ok := res.Messages[1].Content.(string)
	require.True(t, ok, "2-seg fallback user must be string, got %T", res.Messages[1].Content)
	require.NotEmpty(t, userStr)
}

// TestHijack_3SegSplit_PrefixStable 同一 system + 同一 frozen-timeline 段在 2 次
// hijack 中, system 消息与 user1 消息字节级一致 (open 段不同), 这是双 cc 命中
// 的核心前置条件。注意: r1/r2 之间 ChatContent 指针虽不同但 reflect.DeepEqual
// 仍通过 (DeepEqual 对指针递归比较指向的值)。
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

// ---------------------------------------------------------------------------
// frozen boundary 切割专项测试 (§7.7.8 主路径)
// 验证 hijacker 优先用 <|AI_CACHE_FROZEN_semi-dynamic|>...
// <|AI_CACHE_FROZEN_END_semi-dynamic|> 边界标签做切割, 当边界存在时不再
// 进入 timeline 内部解析的退化路径。
// ---------------------------------------------------------------------------

// TestHijack_FrozenBoundary_UserExampleCase 用户给的 4-block 切割期望:
//
//	A-system  (high-static)
//	B-semi-static
//	<|AI_CACHE_FROZEN_semi-dynamic|>
//	<Timeline-Reducer>
//	<Timeline-ITEM1>
//	<Timeline-ITEM2>
//	<|AI_CACHE_FROZEN_END_semi-dynamic|>
//	<Timeline-ITEM3-Open>
//	D
//	E
//	F
//
// 期望切分:
//
//	system: A-system + cc
//	user1:  B-semi-static + frozen-block-content (含 START + END 标签自身) + cc
//	user2:  Timeline-ITEM3-Open + DEF
//
// 关键词: hijacker, frozen boundary, 用户案例, §7.7.8 主路径
func TestHijack_FrozenBoundary_UserExampleCase(t *testing.T) {
	prompt := strings.Join([]string{
		"<|AI_CACHE_SYSTEM_high-static|>\nA-system\n<|AI_CACHE_SYSTEM_END_high-static|>",
		"<|PROMPT_SECTION_semi-dynamic|>\nB-semi-static\n<|PROMPT_SECTION_END_semi-dynamic|>",
		"<|PROMPT_SECTION_timeline|>\n" +
			"<|AI_CACHE_FROZEN_semi-dynamic|>\n" +
			"<|TIMELINE_r1t1|>\nTimeline-Reducer-content\n<|TIMELINE_END_r1t1|>\n" +
			"<|TIMELINE_b3t100|>\nTimeline-ITEM1-content\n<|TIMELINE_END_b3t100|>\n" +
			"<|TIMELINE_b3t200|>\nTimeline-ITEM2-content\n<|TIMELINE_END_b3t200|>\n" +
			"<|AI_CACHE_FROZEN_END_semi-dynamic|>\n" +
			"<|TIMELINE_b3t300|>\nTimeline-ITEM3-Open-content\n<|TIMELINE_END_b3t300|>\n" +
			"<|PROMPT_SECTION_END_timeline|>",
		"<|PROMPT_SECTION_dynamic_userQuery|>\nD-E-F-content\n<|PROMPT_SECTION_dynamic_END_userQuery|>",
	}, "\n\n")

	res := hijackHighStatic(prompt)
	require.NotNil(t, res, "user-case should hijack")
	require.True(t, res.IsHijacked)
	require.Len(t, res.Messages, 3, "user-case should split into 3 segments via frozen boundary")

	system := extractTextContent(t, res.Messages[0].Content)
	user1 := extractTextContent(t, res.Messages[1].Content)
	user2 := extractTextContent(t, res.Messages[2].Content)

	// system 必须含 A-system, 不含 user 段任何内容
	require.Contains(t, system, "A-system", "system must contain high-static body")
	require.NotContains(t, system, "B-semi-static")
	require.NotContains(t, system, "Timeline-")
	require.NotContains(t, system, "D-E-F")

	// user1 必须含 B-semi-static + frozen 段所有内容 + 边界标签自身
	// (user1 包含 END 标签是字节边界稳定性的关键, 见 splitByFrozenBoundary 文档)
	require.Contains(t, user1, "B-semi-static", "user1 should carry semi-static prefix")
	require.Contains(t, user1, "Timeline-Reducer-content", "user1 should carry frozen reducer")
	require.Contains(t, user1, "Timeline-ITEM1-content")
	require.Contains(t, user1, "Timeline-ITEM2-content")
	require.Contains(t, user1, "<|AI_CACHE_FROZEN_END_semi-dynamic|>",
		"user1 must INCLUDE the boundary END tag itself for byte-stable prefix")
	require.NotContains(t, user1, "Timeline-ITEM3-Open-content",
		"open bucket must NOT be in user1")
	require.NotContains(t, user1, "D-E-F-content", "dynamic must NOT be in user1")

	// user2 必须含 open + dynamic, 不含 frozen 任何内容
	require.Contains(t, user2, "Timeline-ITEM3-Open-content", "user2 should carry open bucket")
	require.Contains(t, user2, "D-E-F-content", "user2 should carry dynamic")
	require.NotContains(t, user2, "Timeline-Reducer-content")
	require.NotContains(t, user2, "Timeline-ITEM1-content")
	require.NotContains(t, user2, "<|AI_CACHE_FROZEN_semi-dynamic|>",
		"user2 must NOT carry the boundary START tag (it belongs to user1)")

	// system + user1 必须自带 ephemeral cc (§7.7.7 hijacker 自管双 cc)
	assertHasEphemeralCacheControl(t, res.Messages[0].Content, "user-case system")
	assertHasEphemeralCacheControl(t, res.Messages[1].Content, "user-case user1")
	assertNoCacheControl(t, res.Messages[2].Content, "user-case user2")
}

// TestHijack_FrozenBoundary_NoTimelineSection_StillSplits frozen boundary
// 不依赖 PROMPT_SECTION_timeline 包装, 直接出现在 user 区块的任意位置都能切割。
// 这验证了边界标签的"通用切割锚点"语义 — 任何 caller 都能用它声明缓存边界,
// 不需要走 timeline 渲染。
// 关键词: hijacker, frozen boundary, 无 timeline section, 通用切割锚点
func TestHijack_FrozenBoundary_NoTimelineSection_StillSplits(t *testing.T) {
	prompt := strings.Join([]string{
		"<|AI_CACHE_SYSTEM_high-static|>\nstatic-system\n<|AI_CACHE_SYSTEM_END_high-static|>",
		"<|PROMPT_SECTION_semi-dynamic|>\nsemi-content\n" +
			"<|AI_CACHE_FROZEN_semi-dynamic|>\n" +
			"frozen-prefix-block\n" +
			"<|AI_CACHE_FROZEN_END_semi-dynamic|>\n" +
			"open-tail-after-boundary\n" +
			"<|PROMPT_SECTION_END_semi-dynamic|>",
	}, "\n\n")

	res := hijackHighStatic(prompt)
	require.NotNil(t, res)
	require.Len(t, res.Messages, 3,
		"frozen boundary inside semi-dynamic (no timeline) should still split to 3 segments")

	user1 := extractTextContent(t, res.Messages[1].Content)
	user2 := extractTextContent(t, res.Messages[2].Content)

	require.Contains(t, user1, "semi-content")
	require.Contains(t, user1, "frozen-prefix-block")
	require.Contains(t, user1, "<|AI_CACHE_FROZEN_END_semi-dynamic|>")
	require.NotContains(t, user1, "open-tail-after-boundary")

	require.Contains(t, user2, "open-tail-after-boundary")
	require.NotContains(t, user2, "frozen-prefix-block")
}

// TestHijack_FrozenBoundary_PrefixStableAcrossOpenChange 前缀稳定: open
// 段内容变化不影响 user1 字节序列 (与 §7.7.7 双 cc 命中前提对齐)。
// 关键词: hijacker, frozen boundary, 前缀字节稳定
func TestHijack_FrozenBoundary_PrefixStableAcrossOpenChange(t *testing.T) {
	mk := func(openContent string) string {
		return strings.Join([]string{
			"<|AI_CACHE_SYSTEM_high-static|>\nfixed-system\n<|AI_CACHE_SYSTEM_END_high-static|>",
			"<|PROMPT_SECTION_semi-dynamic|>\nsd-content\n<|PROMPT_SECTION_END_semi-dynamic|>",
			"<|PROMPT_SECTION_timeline|>\n" +
				"<|AI_CACHE_FROZEN_semi-dynamic|>\n" +
				"<|TIMELINE_b3t100|>\nfrozen-A\n<|TIMELINE_END_b3t100|>\n" +
				"<|TIMELINE_b3t200|>\nfrozen-B\n<|TIMELINE_END_b3t200|>\n" +
				"<|AI_CACHE_FROZEN_END_semi-dynamic|>\n" +
				"<|TIMELINE_b3t999|>\n" + openContent + "\n<|TIMELINE_END_b3t999|>\n" +
				"<|PROMPT_SECTION_END_timeline|>",
		}, "\n\n")
	}
	r1 := hijackHighStatic(mk("open-r1-payload"))
	r2 := hijackHighStatic(mk("open-r2-completely-different"))
	require.NotNil(t, r1)
	require.NotNil(t, r2)
	require.Len(t, r1.Messages, 3)
	require.Len(t, r2.Messages, 3)

	assert.Equal(t, r1.Messages[0].Content, r2.Messages[0].Content,
		"system must be byte-stable when high-static unchanged")
	assert.Equal(t, r1.Messages[1].Content, r2.Messages[1].Content,
		"user1 must be byte-stable when frozen prefix unchanged (boundary cut)")
	assert.NotEqual(t, r1.Messages[2].Content, r2.Messages[2].Content,
		"user2 should differ when open bucket changes")
}

// TestHijack_FrozenBoundary_OnlyStartTag_FallsBack 只有 START 没有 END 的
// 残缺边界 -> hijacker 应当退化到 timeline 内部解析路径, 不在残缺位置乱切。
// 关键词: hijacker, frozen boundary, 残缺边界退化
func TestHijack_FrozenBoundary_OnlyStartTag_FallsBack(t *testing.T) {
	prompt := strings.Join([]string{
		"<|AI_CACHE_SYSTEM_high-static|>\nstatic\n<|AI_CACHE_SYSTEM_END_high-static|>",
		"<|PROMPT_SECTION_timeline|>\n" +
			"<|AI_CACHE_FROZEN_semi-dynamic|>\n" + // 只有 START 没有 END
			"<|TIMELINE_r1t1|>\nreducer-X\n<|TIMELINE_END_r1t1|>\n" +
			"<|TIMELINE_b3t100|>\nfrozen-A\n<|TIMELINE_END_b3t100|>\n" +
			"<|TIMELINE_b3t200|>\nopen-tail\n<|TIMELINE_END_b3t200|>\n" +
			"<|PROMPT_SECTION_END_timeline|>",
	}, "\n\n")

	res := hijackHighStatic(prompt)
	require.NotNil(t, res, "should still hijack via fallback timeline parse")
	require.Len(t, res.Messages, 3,
		"residual START-only boundary should NOT trigger boundary split, must fall back to timeline parse (which still gives 3 segments)")

	// 退化路径走 timeline 内部解析: 末 b* 是 open, 前面 reducer + 非末 b* 是 frozen
	user1 := extractTextContent(t, res.Messages[1].Content)
	user2 := extractTextContent(t, res.Messages[2].Content)
	require.Contains(t, user1, "reducer-X")
	require.Contains(t, user1, "frozen-A")
	require.Contains(t, user2, "open-tail")
	require.NotContains(t, user1, "open-tail")
}

// TestHijack_FrozenBoundary_EndedBeforeStart 出现 END 但 START 在 END 之后
// (病态顺序) -> hijacker 退化到 timeline 内部解析。
// 关键词: hijacker, frozen boundary, START 在 END 之后退化
func TestHijack_FrozenBoundary_EndedBeforeStart(t *testing.T) {
	// 注意: splitByFrozenBoundary 用 strings.Index 找第一个 START, 然后从 START 后
	// 找第一个 END。此处构造的是"END 在前, START 在后", 第一个 START 后没有 END,
	// 所以应当退化。
	prompt := strings.Join([]string{
		"<|AI_CACHE_SYSTEM_high-static|>\nstatic\n<|AI_CACHE_SYSTEM_END_high-static|>",
		"<|PROMPT_SECTION_timeline|>\n" +
			"<|AI_CACHE_FROZEN_END_semi-dynamic|>\n" + // END 在前
			"<|TIMELINE_r1t1|>\nreducer-Y\n<|TIMELINE_END_r1t1|>\n" +
			"<|TIMELINE_b3t100|>\nfrozen-A2\n<|TIMELINE_END_b3t100|>\n" +
			"<|TIMELINE_b3t200|>\nopen-tail2\n<|TIMELINE_END_b3t200|>\n" +
			"<|AI_CACHE_FROZEN_semi-dynamic|>\n" + // START 在后
			"<|PROMPT_SECTION_END_timeline|>",
	}, "\n\n")

	res := hijackHighStatic(prompt)
	require.NotNil(t, res, "should fall back gracefully")
	require.Len(t, res.Messages, 3, "fall back to timeline parse should still yield 3 segments")
}

// TestHijack_FrozenBoundary_TimelineDumpFormat 端到端格式验证: 用 aicommon
// Timeline.Dump 实际输出格式构造 prompt (手写复刻 Dump 输出, 不依赖真实
// Timeline 时间戳), 验证 hijacker 可以正确切割。
// 关键词: hijacker, frozen boundary, Timeline.Dump 输出格式兼容
func TestHijack_FrozenBoundary_TimelineDumpFormat(t *testing.T) {
	// 复刻 aicommon Timeline.Dump 在 frozen+open 混合场景下的输出格式:
	//   <|AI_CACHE_FROZEN_semi-dynamic|>
	//   <|TIMELINE_b3tXXXX|>...<|TIMELINE_END_b3tXXXX|>   (frozen 桶)
	//   <|TIMELINE_b3tYYYY|>...<|TIMELINE_END_b3tYYYY|>   (frozen 桶)
	//   <|AI_CACHE_FROZEN_END_semi-dynamic|>
	//   <|TIMELINE_b3tZZZZ|>...<|TIMELINE_END_b3tZZZZ|>   (open 末桶)
	timelineDump := strings.Join([]string{
		"<|AI_CACHE_FROZEN_semi-dynamic|>",
		"<|TIMELINE_b3t1746180000|>",
		"# bucket=2026/05/02 10:00:00-10:03:00 interval=3m",
		"10:00:30 [tool/scan ok]",
		"data-A",
		"<|TIMELINE_END_b3t1746180000|>",
		"<|TIMELINE_b3t1746180180|>",
		"# bucket=2026/05/02 10:03:00-10:06:00 interval=3m",
		"10:04:00 [tool/scan ok]",
		"data-B",
		"<|TIMELINE_END_b3t1746180180|>",
		"<|AI_CACHE_FROZEN_END_semi-dynamic|>",
		"<|TIMELINE_b3t1746180360|>",
		"# bucket=2026/05/02 10:06:00-10:09:00 interval=3m",
		"10:07:00 [tool/scan ok]",
		"data-C-OPEN",
		"<|TIMELINE_END_b3t1746180360|>",
	}, "\n")

	prompt := strings.Join([]string{
		"<|AI_CACHE_SYSTEM_high-static|>\nfixed-static\n<|AI_CACHE_SYSTEM_END_high-static|>",
		"<|PROMPT_SECTION_timeline|>\n" + timelineDump + "\n<|PROMPT_SECTION_END_timeline|>",
		"<|PROMPT_SECTION_dynamic_q|>\nfinal-user-question\n<|PROMPT_SECTION_dynamic_END_q|>",
	}, "\n\n")

	res := hijackHighStatic(prompt)
	require.NotNil(t, res)
	require.Len(t, res.Messages, 3, "Timeline.Dump-format prompt should split via boundary")

	user1 := extractTextContent(t, res.Messages[1].Content)
	user2 := extractTextContent(t, res.Messages[2].Content)
	require.Contains(t, user1, "<|AI_CACHE_FROZEN_END_semi-dynamic|>",
		"user1 must include END tag for byte-stable prefix")
	require.Contains(t, user1, "data-A")
	require.Contains(t, user1, "data-B")
	require.NotContains(t, user1, "data-C-OPEN", "open bucket data must NOT be in user1")
	require.NotContains(t, user1, "final-user-question")

	require.Contains(t, user2, "data-C-OPEN", "open bucket should be in user2")
	require.Contains(t, user2, "final-user-question",
		"dynamic question must end up in user2 (open tail)")
}

// ---------------------------------------------------------------------------
// Semi boundary 测试 (P1 双 cache 边界 4 段切分)
// ---------------------------------------------------------------------------

// TestHijack_SemiBoundary_HappyPath4Segments 验证 frozen + semi 双边界齐全时
// hijacker 切成 4 段消息: [system+cc, user1+cc, user2+cc, user3].
//
// 端到端 prompt 形态:
//   SYSTEM (high-static)        -> system + cc
//   AI_CACHE_FROZEN ... END     -> user1 + cc
//   AI_CACHE_SEMI   ... END     -> user2 + cc (内含 PROMPT_SECTION_semi-dynamic + 内容)
//   timeline-open + dynamic     -> user3 (无 cc)
//
// 关键词: TestHijack_SemiBoundary, 4 段切分, P1 双 cache 边界, 双 cc 主路径
func TestHijack_SemiBoundary_HappyPath4Segments(t *testing.T) {
	prompt := strings.Join([]string{
		"<|AI_CACHE_SYSTEM_high-static|>\nA-system\n<|AI_CACHE_SYSTEM_END_high-static|>",
		"<|AI_CACHE_FROZEN_semi-dynamic|>\n" +
			"frozen-tool-inventory\n" +
			"<|TIMELINE_r1t1|>\nfrozen-reducer\n<|TIMELINE_END_r1t1|>\n" +
			"<|AI_CACHE_FROZEN_END_semi-dynamic|>",
		"<|AI_CACHE_SEMI_semi|>\n" +
			"<|PROMPT_SECTION_semi-dynamic|>\n" +
			"skills-context-block\n" +
			"<|SCHEMA|>\nschema-content\n<|SCHEMA|>\n" +
			"<|CACHE_TOOL_CALL_[current-nonce]|>\ncache-tool-call-content\n<|CACHE_TOOL_CALL_END_[current-nonce]|>\n" +
			"<|PROMPT_SECTION_END_semi-dynamic|>\n" +
			"<|AI_CACHE_SEMI_END_semi|>",
		"<|PROMPT_SECTION_timeline-open|>\nopen-timeline-bucket\n<|PROMPT_SECTION_END_timeline-open|>",
		"<|PROMPT_SECTION_dynamic_q|>\nuser-query-and-dynamic\n<|PROMPT_SECTION_dynamic_END_q|>",
	}, "\n\n")

	res := hijackHighStatic(prompt)
	require.NotNil(t, res, "happy-path should hijack")
	require.True(t, res.IsHijacked)
	require.Len(t, res.Messages, 4, "double-boundary prompt should split into 4 segments")

	system := extractTextContent(t, res.Messages[0].Content)
	user1 := extractTextContent(t, res.Messages[1].Content)
	user2 := extractTextContent(t, res.Messages[2].Content)
	user3 := extractTextContent(t, res.Messages[3].Content)

	require.Contains(t, system, "A-system")
	require.NotContains(t, system, "frozen-tool-inventory")

	// user1: frozen 段, 含 frozen END 标签自身, 不含 semi 任何内容
	require.Contains(t, user1, "frozen-tool-inventory")
	require.Contains(t, user1, "frozen-reducer")
	require.Contains(t, user1, "<|AI_CACHE_FROZEN_END_semi-dynamic|>",
		"user1 must include frozen END tag for byte-stable prefix")
	require.NotContains(t, user1, "skills-context-block")
	require.NotContains(t, user1, "cache-tool-call-content")
	require.NotContains(t, user1, "open-timeline-bucket")

	// user2: semi 段, 必须包含 START 与 END 边界, 含 SkillsContext + Schema + CacheToolCall
	require.Contains(t, user2, "<|AI_CACHE_SEMI_semi|>",
		"user2 must include semi START tag")
	require.Contains(t, user2, "<|AI_CACHE_SEMI_END_semi|>",
		"user2 must include semi END tag for byte-stable prefix")
	require.Contains(t, user2, "skills-context-block")
	require.Contains(t, user2, "schema-content")
	require.Contains(t, user2, "cache-tool-call-content")
	require.NotContains(t, user2, "open-timeline-bucket")
	require.NotContains(t, user2, "user-query-and-dynamic")

	// user3: open + dynamic, 不含 semi 边界与内容
	require.Contains(t, user3, "open-timeline-bucket")
	require.Contains(t, user3, "user-query-and-dynamic")
	require.NotContains(t, user3, "<|AI_CACHE_SEMI_semi|>")
	require.NotContains(t, user3, "skills-context-block")

	// system + user1 + user2 都必须自带 ephemeral cc, user3 不带
	assertHasEphemeralCacheControl(t, res.Messages[0].Content, "4seg system")
	assertHasEphemeralCacheControl(t, res.Messages[1].Content, "4seg user1")
	assertHasEphemeralCacheControl(t, res.Messages[2].Content, "4seg user2")
	assertNoCacheControl(t, res.Messages[3].Content, "4seg user3")
}

// TestHijack_SemiBoundary_WithPriorModelThinking 在 frozen 与 semi 之间插入
// PROMPT_SECTION_model-thinking 时, hijacker 应剥出独立 assistant 消息,
// semi 段仍单独一条 user (带 cc).
func TestHijack_SemiBoundary_WithPriorModelThinking(t *testing.T) {
	mt := "<|PROMPT_SECTION_model-thinking|>\nstep-A\nstep-B\n<|PROMPT_SECTION_END_model-thinking|>"
	prompt := strings.Join([]string{
		"<|AI_CACHE_SYSTEM_high-static|>\nA-system\n<|AI_CACHE_SYSTEM_END_high-static|>",
		"<|AI_CACHE_FROZEN_semi-dynamic|>\n" +
			"frozen-tool-inventory\n" +
			"<|AI_CACHE_FROZEN_END_semi-dynamic|>",
		mt,
		"<|AI_CACHE_SEMI_semi|>\n" +
			"<|PROMPT_SECTION_semi-dynamic|>\n" +
			"skills-context-block\n" +
			"<|PROMPT_SECTION_END_semi-dynamic|>\n" +
			"<|AI_CACHE_SEMI_END_semi|>",
		"<|PROMPT_SECTION_timeline-open|>\nopen-timeline-bucket\n<|PROMPT_SECTION_END_timeline-open|>",
		"<|PROMPT_SECTION_dynamic_q|>\nuser-query\n<|PROMPT_SECTION_dynamic_END_q|>",
	}, "\n\n")

	res := hijackHighStatic(prompt)
	require.NotNil(t, res)
	require.True(t, res.IsHijacked)
	require.Len(t, res.Messages, 5, "expect system + user1 + assistant(thinking) + user2(semi) + user3")

	require.Equal(t, "system", res.Messages[0].Role)
	require.Equal(t, "user", res.Messages[1].Role)
	require.Equal(t, "assistant", res.Messages[2].Role)
	require.Equal(t, "user", res.Messages[3].Role)
	require.Equal(t, "user", res.Messages[4].Role)

	require.Equal(t, priorAssistantMessageSurface, extractTextContent(t, res.Messages[2].Content))
	require.Contains(t, res.Messages[2].ReasoningContent, "step-A")
	require.Contains(t, res.Messages[2].ReasoningContent, "step-B")
	raw, err := json.Marshal(res.Messages[2])
	require.NoError(t, err)
	require.Contains(t, string(raw), `"reasoning_content"`)
	require.Contains(t, string(raw), `"role":"assistant"`)

	require.NotContains(t, extractTextContent(t, res.Messages[2].Content), "PROMPT_SECTION_model-thinking")

	user2 := extractTextContent(t, res.Messages[3].Content)
	require.Contains(t, user2, "<|AI_CACHE_SEMI_semi|>")
	require.Contains(t, user2, "skills-context-block")
	require.NotContains(t, user2, "step-A")
	require.NotContains(t, user2, "PROMPT_SECTION_model-thinking")

	user3 := extractTextContent(t, res.Messages[4].Content)
	require.Contains(t, user3, "user-query")
}

// TestHijack_SemiBoundary_MissingSemi_FallsBackTo3Segments 验证仅有 frozen
// 边界, 没有 semi 边界时, hijacker 退化到 3 段 (与场景 A 等价).
// 关键词: TestHijack_SemiBoundary, 缺 semi 边界退化 3 段
func TestHijack_SemiBoundary_MissingSemi_FallsBackTo3Segments(t *testing.T) {
	prompt := strings.Join([]string{
		"<|AI_CACHE_SYSTEM_high-static|>\nA-system\n<|AI_CACHE_SYSTEM_END_high-static|>",
		"<|AI_CACHE_FROZEN_semi-dynamic|>\n" +
			"frozen-tool-inventory\n" +
			"<|AI_CACHE_FROZEN_END_semi-dynamic|>",
		"<|PROMPT_SECTION_semi-dynamic|>\n" +
			"skills-no-semi-boundary\n" +
			"<|PROMPT_SECTION_END_semi-dynamic|>",
		"<|PROMPT_SECTION_dynamic_q|>\ndynamic-q\n<|PROMPT_SECTION_dynamic_END_q|>",
	}, "\n\n")

	res := hijackHighStatic(prompt)
	require.NotNil(t, res)
	require.Len(t, res.Messages, 3,
		"missing semi boundary should fall back to 3-segment frozen-only path")

	user1 := extractTextContent(t, res.Messages[1].Content)
	user2 := extractTextContent(t, res.Messages[2].Content)
	require.Contains(t, user1, "frozen-tool-inventory")
	require.Contains(t, user1, "<|AI_CACHE_FROZEN_END_semi-dynamic|>")
	require.Contains(t, user2, "skills-no-semi-boundary")
	require.Contains(t, user2, "dynamic-q")

	// 3 段路径下: system + user1 自带 cc, user2 不带 cc
	assertHasEphemeralCacheControl(t, res.Messages[0].Content, "3seg fallback system")
	assertHasEphemeralCacheControl(t, res.Messages[1].Content, "3seg fallback user1")
	assertNoCacheControl(t, res.Messages[2].Content, "3seg fallback user2")
}

// TestHijack_SemiBoundary_SemiBeforeFrozen_FallsBack 病态顺序: semi 边界
// 出现在 frozen END 之前. splitBySemiBoundary 从 frozenEnd 之后找 semi START,
// 找不到就退化, hijacker 仍能切 3 段.
// 关键词: TestHijack_SemiBoundary, 病态顺序 semi 在前 frozen 在后退化
func TestHijack_SemiBoundary_SemiBeforeFrozen_FallsBack(t *testing.T) {
	prompt := strings.Join([]string{
		"<|AI_CACHE_SYSTEM_high-static|>\nA-system\n<|AI_CACHE_SYSTEM_END_high-static|>",
		// semi 在前 (异常)
		"<|AI_CACHE_SEMI_semi|>\nillegal-semi\n<|AI_CACHE_SEMI_END_semi|>",
		"<|AI_CACHE_FROZEN_semi-dynamic|>\nfrozen-content\n<|AI_CACHE_FROZEN_END_semi-dynamic|>",
		"<|PROMPT_SECTION_dynamic_q|>\nq\n<|PROMPT_SECTION_dynamic_END_q|>",
	}, "\n\n")

	res := hijackHighStatic(prompt)
	require.NotNil(t, res)
	require.Len(t, res.Messages, 3,
		"semi-before-frozen should NOT trigger 4-segment split, must fall back to 3-segment frozen-only path")
}

// TestHijack_SemiBoundary_SemiNestedInsideFrozen_FallsBack semi 边界完全嵌套
// 在 frozen 边界内 (frozen END 在 semi END 之后). splitBySemiBoundary 从
// frozenEnd 后找 semi START, 找不到就退化. 这与"semi 必须在 frozen END 之后"
// 的契约对齐.
// 关键词: TestHijack_SemiBoundary, semi 嵌套于 frozen 内退化
func TestHijack_SemiBoundary_SemiNestedInsideFrozen_FallsBack(t *testing.T) {
	prompt := strings.Join([]string{
		"<|AI_CACHE_SYSTEM_high-static|>\nA-system\n<|AI_CACHE_SYSTEM_END_high-static|>",
		"<|AI_CACHE_FROZEN_semi-dynamic|>\n" +
			"frozen-prefix\n" +
			"<|AI_CACHE_SEMI_semi|>\nnested-semi\n<|AI_CACHE_SEMI_END_semi|>\n" +
			"frozen-suffix\n" +
			"<|AI_CACHE_FROZEN_END_semi-dynamic|>",
		"<|PROMPT_SECTION_dynamic_q|>\nq\n<|PROMPT_SECTION_dynamic_END_q|>",
	}, "\n\n")

	res := hijackHighStatic(prompt)
	require.NotNil(t, res)
	require.Len(t, res.Messages, 3,
		"semi nested inside frozen should NOT trigger 4-segment split (semi START is before frozen END), must fall back")
}

// TestHijack_SemiBoundary_User3Empty_FallsBack 4 段切分要求 user3 (semi END
// 之后到末尾) 非空; 否则退化到 3 段 (避免空消息).
// 关键词: TestHijack_SemiBoundary, user3 空退化 3 段
func TestHijack_SemiBoundary_User3Empty_FallsBack(t *testing.T) {
	prompt := strings.Join([]string{
		"<|AI_CACHE_SYSTEM_high-static|>\nA-system\n<|AI_CACHE_SYSTEM_END_high-static|>",
		"<|AI_CACHE_FROZEN_semi-dynamic|>\nfrozen-content\n<|AI_CACHE_FROZEN_END_semi-dynamic|>",
		"<|AI_CACHE_SEMI_semi|>\nsemi-content\n<|AI_CACHE_SEMI_END_semi|>",
		// 故意没有任何 user3 内容
	}, "\n\n")

	res := hijackHighStatic(prompt)
	require.NotNil(t, res)
	// user3 为空时不应该走 4 段, 退化到 3 段或 2 段都可
	require.NotEqual(t, 4, len(res.Messages),
		"empty user3 must NOT trigger 4-segment split")
}

// TestHijack_SemiBoundary_PrefixStableAcrossDynamicChange 4 段切分下,
// 改变 user3 (open + dynamic) 内容时, system / user1 / user2 必须字节稳定
// (这是 P1 双 cache 边界双 cc 命中前提).
// 关键词: TestHijack_SemiBoundary, 字节稳定, system/user1/user2 跨调用一致
func TestHijack_SemiBoundary_PrefixStableAcrossDynamicChange(t *testing.T) {
	mk := func(dyn string) string {
		return strings.Join([]string{
			"<|AI_CACHE_SYSTEM_high-static|>\nfixed-system\n<|AI_CACHE_SYSTEM_END_high-static|>",
			"<|AI_CACHE_FROZEN_semi-dynamic|>\nfixed-frozen\n<|AI_CACHE_FROZEN_END_semi-dynamic|>",
			"<|AI_CACHE_SEMI_semi|>\n" +
				"<|PROMPT_SECTION_semi-dynamic|>\nfixed-semi-content\n<|PROMPT_SECTION_END_semi-dynamic|>\n" +
				"<|AI_CACHE_SEMI_END_semi|>",
			"<|PROMPT_SECTION_dynamic_q|>\n" + dyn + "\n<|PROMPT_SECTION_dynamic_END_q|>",
		}, "\n\n")
	}

	r1 := hijackHighStatic(mk("dynamic-r1-payload"))
	r2 := hijackHighStatic(mk("dynamic-r2-completely-different"))
	require.NotNil(t, r1)
	require.NotNil(t, r2)
	require.Len(t, r1.Messages, 4)
	require.Len(t, r2.Messages, 4)

	assert.Equal(t, r1.Messages[0].Content, r2.Messages[0].Content,
		"system must be byte-stable when high-static unchanged")
	assert.Equal(t, r1.Messages[1].Content, r2.Messages[1].Content,
		"user1 must be byte-stable when frozen prefix unchanged")
	assert.Equal(t, r1.Messages[2].Content, r2.Messages[2].Content,
		"user2 must be byte-stable when semi prefix unchanged (P1 二级 cache 边界)")
	assert.NotEqual(t, r1.Messages[3].Content, r2.Messages[3].Content,
		"user3 should differ when dynamic content changes")
}

// ---------------------------------------------------------------------------
// P2.1 短 prompt 阈值合并 / 旁路用例
// 验证 build4SegmentMessages / build3SegmentMessages 在 user 段过短时按
// minCachableUserSegmentBytes (默认 1024 byte) 自动降级:
//   - 4 段 → 3 段 (user1 < 阈值, user1+user2 >= 阈值)
//   - 4 段 → 2 段 (user1+user2 < 阈值)
//   - 3 段 → 2 段 (user1 < 阈值)
//   - happy path 仍保留 (user1, user2 都 >= 阈值)
//   - 阈值边界 (user1 == 阈值) 不触发合并
// 这些用例显式 setHijackerThresholdMerge(t, 1024) 打开默认阈值, 与 TestMain
// 默认关闭阈值的 isolation 配合.
// ---------------------------------------------------------------------------

// largeFiller 生成大于 minBytes 的 ASCII 字符串, 用于撑大 fixture 让 user 段
// 达到 P2.1 阈值。内容是稳定 pattern, 不破坏字节稳定性测试断言.
//
// 关键词: aicache test helper, P2.1, fixture filler
func largeFiller(label string, minBytes int) string {
	unit := "[" + label + "-padding-line-" + strings.Repeat("x", 32) + "]\n"
	var sb strings.Builder
	for sb.Len() < minBytes {
		sb.WriteString(unit)
	}
	return sb.String()
}

// TestHijack_P2_FourSegment_FallbackToThree_WhenUser1TooSmall 验证 P2.1
// 阶段 1: user1 (frozen 段) 内容很短 (< 1KB) 但 user2 (semi 段) 很大,
// hijacker 把 user1 合并到 user2, 退化为 3 段消息: [sys+cc, u12+cc, u3].
// 关键词: P2.1, 阈值合并, 4 段 → 3 段降级
func TestHijack_P2_FourSegment_FallbackToThree_WhenUser1TooSmall(t *testing.T) {
	setHijackerThresholdMerge(t, 1024)

	semiBody := largeFiller("semi-body", 4096) // user2 ≥ 4KB 远超阈值
	prompt := strings.Join([]string{
		"<|AI_CACHE_SYSTEM_high-static|>\nA-system\n<|AI_CACHE_SYSTEM_END_high-static|>",
		"<|AI_CACHE_FROZEN_semi-dynamic|>\n" +
			"tiny-frozen\n" +
			"<|AI_CACHE_FROZEN_END_semi-dynamic|>",
		"<|AI_CACHE_SEMI_semi|>\n" +
			"<|PROMPT_SECTION_semi-dynamic|>\n" +
			"semi-marker-X\n" +
			semiBody +
			"<|PROMPT_SECTION_END_semi-dynamic|>\n" +
			"<|AI_CACHE_SEMI_END_semi|>",
		"<|PROMPT_SECTION_dynamic_q|>\nuser-query-payload\n<|PROMPT_SECTION_dynamic_END_q|>",
	}, "\n\n")

	res := hijackHighStatic(prompt)
	require.NotNil(t, res)
	require.True(t, res.IsHijacked)
	require.Len(t, res.Messages, 3,
		"user1 < 1KB but user1+user2 >= 1KB should merge to 3 segments")

	system := extractTextContent(t, res.Messages[0].Content)
	merged := extractTextContent(t, res.Messages[1].Content)
	user3 := extractTextContent(t, res.Messages[2].Content)

	require.Contains(t, system, "A-system")
	// merged 必须同时包含原 user1 (frozen) 与 user2 (semi) 内容, 字节连续无截断
	require.Contains(t, merged, "tiny-frozen", "merged user12 must carry user1 content")
	require.Contains(t, merged, "<|AI_CACHE_FROZEN_END_semi-dynamic|>",
		"merged user12 must keep frozen END tag (字节边界)")
	require.Contains(t, merged, "semi-marker-X", "merged user12 must carry user2 content")
	require.Contains(t, merged, "<|AI_CACHE_SEMI_END_semi|>",
		"merged user12 must keep semi END tag")
	require.NotContains(t, merged, "user-query-payload")

	require.Contains(t, user3, "user-query-payload")
	require.NotContains(t, user3, "tiny-frozen")
	require.NotContains(t, user3, "semi-marker-X")

	// system + user12 主动打 cc, user3 不带 cc
	assertHasEphemeralCacheControl(t, res.Messages[0].Content, "P2 4to3 system")
	assertHasEphemeralCacheControl(t, res.Messages[1].Content, "P2 4to3 user12")
	assertNoCacheControl(t, res.Messages[2].Content, "P2 4to3 user3")
}

// TestHijack_P2_FourSegment_FallbackToTwo_WhenUserTooSmall 验证 P2.1
// 阶段 2: user1 + user2 合计仍 < 1KB, 进一步合并 user3, 退化为 2 段消息:
// [sys+cc, all_user 不带 cc]. 仅 system 段缓存, user 段透传.
// 关键词: P2.1, 阈值合并, 4 段 → 2 段降级, user 段透传
func TestHijack_P2_FourSegment_FallbackToTwo_WhenUserTooSmall(t *testing.T) {
	setHijackerThresholdMerge(t, 1024)

	prompt := strings.Join([]string{
		"<|AI_CACHE_SYSTEM_high-static|>\nA-system-static\n<|AI_CACHE_SYSTEM_END_high-static|>",
		"<|AI_CACHE_FROZEN_semi-dynamic|>\n" +
			"tiny-frozen-1\n" +
			"<|AI_CACHE_FROZEN_END_semi-dynamic|>",
		"<|AI_CACHE_SEMI_semi|>\n" +
			"<|PROMPT_SECTION_semi-dynamic|>\n" +
			"tiny-semi-2\n" +
			"<|PROMPT_SECTION_END_semi-dynamic|>\n" +
			"<|AI_CACHE_SEMI_END_semi|>",
		"<|PROMPT_SECTION_dynamic_q|>\ntiny-dynamic-3\n<|PROMPT_SECTION_dynamic_END_q|>",
	}, "\n\n")

	res := hijackHighStatic(prompt)
	require.NotNil(t, res)
	require.True(t, res.IsHijacked)
	require.Len(t, res.Messages, 2,
		"user1+user2 < 1KB should fall back to 2 segments (sys cc + all_user no cc)")

	system := extractTextContent(t, res.Messages[0].Content)
	allUser := extractTextContent(t, res.Messages[1].Content)

	require.Contains(t, system, "A-system-static")

	// allUser 必须依次包含 frozen + semi + dynamic 三段原内容
	require.Contains(t, allUser, "tiny-frozen-1")
	require.Contains(t, allUser, "<|AI_CACHE_FROZEN_END_semi-dynamic|>")
	require.Contains(t, allUser, "tiny-semi-2")
	require.Contains(t, allUser, "<|AI_CACHE_SEMI_END_semi|>")
	require.Contains(t, allUser, "tiny-dynamic-3")

	// 仅 system 打 cc, user 不打 cc
	assertHasEphemeralCacheControl(t, res.Messages[0].Content, "P2 4to2 system")
	assertNoCacheControl(t, res.Messages[1].Content, "P2 4to2 all_user")
}

// TestHijack_P2_ThreeSegment_FallbackToTwo_WhenUser1TooSmall 验证 P2.1
// 3 段路径降级: 仅有 frozen 边界 (无 semi 边界), user1 < 1KB 时 hijacker
// 退化到 build2SegmentMessages, 整段 user 透传不打 cc.
// 关键词: P2.1, 阈值合并, 3 段 → 2 段降级, build3 阈值检查
func TestHijack_P2_ThreeSegment_FallbackToTwo_WhenUser1TooSmall(t *testing.T) {
	setHijackerThresholdMerge(t, 1024)

	prompt := strings.Join([]string{
		"<|AI_CACHE_SYSTEM_high-static|>\nA-system\n<|AI_CACHE_SYSTEM_END_high-static|>",
		"<|AI_CACHE_FROZEN_semi-dynamic|>\n" +
			"tiny-frozen-only\n" +
			"<|AI_CACHE_FROZEN_END_semi-dynamic|>",
		"<|PROMPT_SECTION_dynamic_q|>\ndynamic-tail\n<|PROMPT_SECTION_dynamic_END_q|>",
	}, "\n\n")

	res := hijackHighStatic(prompt)
	require.NotNil(t, res)
	require.True(t, res.IsHijacked)
	require.Len(t, res.Messages, 2,
		"3-segment path with user1 < 1KB should fall back to 2 segments")

	// build3 阈值合并 → build2SegmentMessages 兜底, system/user 都是 string 不打 cc,
	// 由 aibalance 走 baseline 单 cc 兜底 (与原 2 段退化路径一致).
	sysStr, ok := res.Messages[0].Content.(string)
	require.True(t, ok, "P2 3to2 system must be string (build2 fallback), got %T", res.Messages[0].Content)
	require.Contains(t, sysStr, "A-system")

	allUser, ok := res.Messages[1].Content.(string)
	require.True(t, ok, "P2 3to2 user must be string, got %T", res.Messages[1].Content)
	require.Contains(t, allUser, "tiny-frozen-only")
	require.Contains(t, allUser, "<|AI_CACHE_FROZEN_END_semi-dynamic|>")
	require.Contains(t, allUser, "dynamic-tail")
}

// TestHijack_P2_FourSegment_HappyPathPreserved_WhenBothLarge 验证当 user1 与
// user2 都 >= 1KB 时, P2.1 阈值合并不应触发, hijacker 走原 4 段 happy path.
// 关键词: P2.1, 阈值合并 happy 不影响, 4 段保留
func TestHijack_P2_FourSegment_HappyPathPreserved_WhenBothLarge(t *testing.T) {
	setHijackerThresholdMerge(t, 1024)

	frozenBody := largeFiller("frozen-body", 2048)
	semiBody := largeFiller("semi-body", 2048)

	prompt := strings.Join([]string{
		"<|AI_CACHE_SYSTEM_high-static|>\nA-system\n<|AI_CACHE_SYSTEM_END_high-static|>",
		"<|AI_CACHE_FROZEN_semi-dynamic|>\n" +
			frozenBody +
			"<|AI_CACHE_FROZEN_END_semi-dynamic|>",
		"<|AI_CACHE_SEMI_semi|>\n" +
			"<|PROMPT_SECTION_semi-dynamic|>\n" +
			semiBody +
			"<|PROMPT_SECTION_END_semi-dynamic|>\n" +
			"<|AI_CACHE_SEMI_END_semi|>",
		"<|PROMPT_SECTION_dynamic_q|>\nuser-q\n<|PROMPT_SECTION_dynamic_END_q|>",
	}, "\n\n")

	res := hijackHighStatic(prompt)
	require.NotNil(t, res)
	require.True(t, res.IsHijacked)
	require.Len(t, res.Messages, 4,
		"both user1 and user2 above threshold should keep 4-segment happy path")

	assertHasEphemeralCacheControl(t, res.Messages[0].Content, "P2 happy system")
	assertHasEphemeralCacheControl(t, res.Messages[1].Content, "P2 happy user1")
	assertHasEphemeralCacheControl(t, res.Messages[2].Content, "P2 happy user2")
	assertNoCacheControl(t, res.Messages[3].Content, "P2 happy user3")
}

// TestHijack_P2_FourSegment_BoundaryEdge_AtThreshold 验证阈值边界 (用 `<` 不
// 是 `<=`): user1 字节数恰好等于阈值时, 不触发合并, 仍走 happy 4 段.
// 关键词: P2.1, 阈值合并边界, < 严格小于
func TestHijack_P2_FourSegment_BoundaryEdge_AtThreshold(t *testing.T) {
	// 用一个较小阈值便于精确控制 user1 字节数
	setHijackerThresholdMerge(t, 200)

	// 构造一个 frozen 段, 让其 user1 (含 START + body + END 标签) 字节数
	// 恰好等于 200. 先估出 START + END 标签字节, 再用 padding 补到 200 整.
	startTag := "<|AI_CACHE_FROZEN_semi-dynamic|>\n"
	endTag := "\n<|AI_CACHE_FROZEN_END_semi-dynamic|>"
	overhead := len(startTag) + len(endTag)
	// 还要算上 splitBySemiBoundary TrimSpace 后剩余的字节. 这里直接用大于
	// 阈值一点的 body 让 user1 >= 200. 用 200 - overhead + 32 个填充确保严格 >= 阈值
	bodyBytes := 200 - overhead + 32
	if bodyBytes < 0 {
		bodyBytes = 32
	}
	frozenBody := strings.Repeat("a", bodyBytes)

	semiBody := largeFiller("semi", 400)

	prompt := strings.Join([]string{
		"<|AI_CACHE_SYSTEM_high-static|>\nsys\n<|AI_CACHE_SYSTEM_END_high-static|>",
		startTag + frozenBody + endTag,
		"<|AI_CACHE_SEMI_semi|>\n" +
			"<|PROMPT_SECTION_semi-dynamic|>\n" +
			semiBody +
			"<|PROMPT_SECTION_END_semi-dynamic|>\n" +
			"<|AI_CACHE_SEMI_END_semi|>",
		"<|PROMPT_SECTION_dynamic_q|>\nq\n<|PROMPT_SECTION_dynamic_END_q|>",
	}, "\n\n")

	res := hijackHighStatic(prompt)
	require.NotNil(t, res)
	require.True(t, res.IsHijacked)
	require.Len(t, res.Messages, 4,
		"user1 >= threshold (strict less-than check) should keep 4-segment happy path")
}
