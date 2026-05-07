package aicache

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAdvice_DynamicSectionOversized: dynamic 段超过阈值时 advice 必须报警。
//
// 关键词: advice, dynamic_section_oversized, 阈值告警
func TestAdvice_DynamicSectionOversized(t *testing.T) {
	gc := newGlobalCache(8)

	bigBody := strings.Repeat("x", dynamicSectionOversizeThreshold+100)
	prompt := "<|PROMPT_SECTION_dynamic_abcdef|>\n" + bigBody + "\n<|PROMPT_SECTION_dynamic_END_abcdef|>"
	split := Split(prompt)
	rep := gc.Record(split, "test-model")

	advices := buildAdvicesWithCache(rep, split, gc)
	hit := false
	for _, a := range advices {
		if strings.Contains(a, "[dynamic_section_oversized]") {
			hit = true
			break
		}
	}
	require.True(t, hit, "expected dynamic_section_oversized advice in: %v", advices)
}

// TestAdvice_DynamicSectionUnderThresholdNoWarning: dynamic 段在阈值以下不报警。
//
// 关键词: advice, dynamic_section_oversized, 阈值边界
func TestAdvice_DynamicSectionUnderThresholdNoWarning(t *testing.T) {
	gc := newGlobalCache(8)

	smallBody := strings.Repeat("x", 1024)
	prompt := "<|PROMPT_SECTION_dynamic_abcdef|>\n" + smallBody + "\n<|PROMPT_SECTION_dynamic_END_abcdef|>"
	split := Split(prompt)
	rep := gc.Record(split, "test-model")

	advices := buildAdvicesWithCache(rep, split, gc)
	for _, a := range advices {
		require.NotContainsf(t, a, "[dynamic_section_oversized]", "should not warn on small dynamic section, got: %v", advices)
	}
}

// TestAdvice_ReusableAITagInDynamic: 同一 (TAG, body) 在多次 prompt 中以不同
// nonce 出现, 跨 turn 累计后 advice 必须报告 reusable_aitag_in_dynamic 漂移。
//
// 关键词: advice, reusable_aitag_in_dynamic, AITag 漂移, RandStringBytes
func TestAdvice_ReusableAITagInDynamic(t *testing.T) {
	gc := newGlobalCache(32)

	body := "stable plan facts content - " + strings.Repeat("a", 64)
	// 让 dynamic chunk 足够大, 避免 dynamic_section_oversized 干扰 (但顶层 nonce
	// 用同一字面量, 让 dynamic chunk 自身 hash 也稳定)
	makeDynamicWithSubtag := func(subtagNonce string) string {
		inner := "<|PARENT_TASK_" + subtagNonce + "|>\n" + body + "\n<|PARENT_TASK_END_" + subtagNonce + "|>"
		return "<|PROMPT_SECTION_dynamic_outerN|>\n" + inner + "\n<|PROMPT_SECTION_dynamic_END_outerN|>"
	}

	// 给同一 body 喂 4 次, 每次内层 nonce 不同 (典型 RandStringBytes 反模式)
	for i, n := range []string{"abcdef", "ghijkl", "mnopqr", "stuvwx"} {
		split := Split(makeDynamicWithSubtag(n))
		rep := gc.Record(split, "test-model")
		_ = buildAdvicesWithCache(rep, split, gc)
		_ = i
	}

	drifts := gc.GetReusableDynamicSubtagDrifts(reusableAITagMinOccurrences)
	require.NotEmptyf(t, drifts, "expected reusable AITag drift for PARENT_TASK")
	found := false
	for _, d := range drifts {
		if d.TagName == "PARENT_TASK" && d.DistinctNonce >= 2 && d.Occurrences >= 3 {
			found = true
			break
		}
	}
	require.Truef(t, found, "expected PARENT_TASK drift with >=2 distinct nonces, got: %+v", drifts)

	// 再 record 一次, advice 必须包含 reusable_aitag_in_dynamic
	split := Split(makeDynamicWithSubtag("yz0123"))
	rep := gc.Record(split, "test-model")
	advices := buildAdvicesWithCache(rep, split, gc)
	hit := false
	for _, a := range advices {
		if strings.Contains(a, "[reusable_aitag_in_dynamic]") && strings.Contains(a, "PARENT_TASK") {
			hit = true
			break
		}
	}
	require.Truef(t, hit, "expected reusable_aitag_in_dynamic advice for PARENT_TASK, got: %v", advices)
}

// TestAdvice_ReusableAITagStableNonceDoesNotTrigger: 同一 (TAG, body) 重复出
// 现但 nonce 也保持稳定时, advice 不应该报漂移 (因为这正是我们期望的优化结果)。
//
// 关键词: advice, reusable_aitag_in_dynamic, 稳定 nonce 不报警
func TestAdvice_ReusableAITagStableNonceDoesNotTrigger(t *testing.T) {
	gc := newGlobalCache(32)

	body := "stable plan facts content - " + strings.Repeat("b", 64)
	stableNonce := "abcdef"
	makeDynamicWithSubtag := func(outerN string) string {
		inner := "<|FACTS_" + stableNonce + "|>\n" + body + "\n<|FACTS_END_" + stableNonce + "|>"
		return "<|PROMPT_SECTION_dynamic_" + outerN + "|>\n" + inner + "\n<|PROMPT_SECTION_dynamic_END_" + outerN + "|>"
	}

	for _, n := range []string{"out001", "out002", "out003", "out004"} {
		split := Split(makeDynamicWithSubtag(n))
		_ = gc.Record(split, "test-model")
	}

	drifts := gc.GetReusableDynamicSubtagDrifts(reusableAITagMinOccurrences)
	for _, d := range drifts {
		require.NotEqualf(t, "FACTS", d.TagName,
			"FACTS subtag uses stable nonce, should not be reported as drift, got: %+v", d)
	}
}

// TestParseDynamicSubtagStartToken_RecognizesValidPatterns: 验证 token 解析器
// 能正确识别合规的起始标签。
//
// 关键词: parseDynamicSubtagStartToken, AITag 解析器
func TestParseDynamicSubtagStartToken_RecognizesValidPatterns(t *testing.T) {
	cases := []struct {
		token   string
		ok      bool
		tagName string
		nonce   string
	}{
		{"PARENT_TASK_abc123", true, "PARENT_TASK", "abc123"},
		{"FACTS_xyz9", true, "FACTS", "xyz9"},
		{"INSTRUCTION_aBcDeF", true, "INSTRUCTION", "aBcDeF"},
		{"PARENT_TASK_END_abc123", false, "", ""},
		{"_abc123", false, "", ""},
		{"NoNonce_", false, "", ""},
		{"BAD-TAG_abc123", false, "", ""},
		{"TAG_with#chars_abc", false, "", ""},
	}
	for _, c := range cases {
		tag, nonce, ok := parseDynamicSubtagStartToken(c.token)
		require.Equalf(t, c.ok, ok, "token %q ok mismatch", c.token)
		if c.ok {
			require.Equal(t, c.tagName, tag, "token %q tagName mismatch", c.token)
			require.Equal(t, c.nonce, nonce, "token %q nonce mismatch", c.token)
		}
	}
}
