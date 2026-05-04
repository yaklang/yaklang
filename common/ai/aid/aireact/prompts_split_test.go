package aireact

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicache"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// 关键词: aicache.Split 单测, P0-A5, 模板段稳定性回归
//
// 这些测试覆盖 P0-A 阶段重构后的 4 大 prompt 模板, 用于:
//  1. 保证模板被 aicache splitter 识别为多段 chunk (不再出 raw/noise)
//  2. 至少包含 high-static 与 dynamic 两段, 防止退化为单一散文 prompt
//  3. high-static / semi-dynamic 段 hash 在跨调用 (不同 nonce) 下保持稳定,
//     是真实命中上游 prefix cache 的前提

// chunkSections 把 splitter 输出的 chunks 收集成 section -> chunks 映射,
// 方便断言。
func chunkSections(t *testing.T, prompt string) map[string][]*aicache.Chunk {
	t.Helper()
	require.NotEmpty(t, prompt, "split target prompt should not be empty")
	res := aicache.Split(prompt)
	require.NotNil(t, res, "Split result should not be nil")
	out := make(map[string][]*aicache.Chunk)
	for _, c := range res.Chunks {
		require.NotNil(t, c)
		out[c.Section] = append(out[c.Section], c)
	}
	return out
}

func newSplitTestReact(t *testing.T) *ReAct {
	t.Helper()
	react, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)
	return react
}

// TestSplit_VerificationPrompt_FourSections 验证 verification.txt 在重构后:
//   - 至少切出 high-static 和 dynamic 两段
//   - 不出现 raw/noise chunk
//   - 跨调用下 high-static 段 hash 稳定
//
// 关键词: P0-A5, verification.txt 4 段断言, aicache.Split
func TestSplit_VerificationPrompt_FourSections(t *testing.T) {
	react := newSplitTestReact(t)

	prompt1, _, err := react.promptManager.GenerateVerificationPrompt(
		"check os type", true, "uname -s -> Darwin",
	)
	require.NoError(t, err)
	prompt2, _, err := react.promptManager.GenerateVerificationPrompt(
		"check os type", true, "uname -s -> Darwin",
	)
	require.NoError(t, err)

	sec1 := chunkSections(t, prompt1)
	sec2 := chunkSections(t, prompt2)

	require.NotEmpty(t, sec1[aicache.SectionHighStatic], "verification prompt must expose high-static chunk")
	require.NotEmpty(t, sec1[aicache.SectionDynamic], "verification prompt must expose dynamic chunk")
	require.Empty(t, sec1[aicache.SectionRaw], "verification prompt should not produce raw/noise chunk; rendered output:\n%s", prompt1)

	require.Equal(t, sec1[aicache.SectionHighStatic][0].Hash, sec2[aicache.SectionHighStatic][0].Hash,
		"high-static hash must be byte-stable across calls")
}

// TestSplit_IntervalReviewPrompt_FourSections 验证 interval-review.txt 在重构后:
//   - 至少切出 high-static 和 dynamic 两段
//   - 不出现 raw/noise chunk
//   - 跨调用下 high-static 段 hash 稳定
//
// 关键词: P0-A5, interval-review.txt 4 段断言, aicache.Split
func TestSplit_IntervalReviewPrompt_FourSections(t *testing.T) {
	react := newSplitTestReact(t)

	tool := aitool.NewWithoutCallback(
		"network_diagnose",
		aitool.WithStringParam("target"),
	)
	prompt1, err := react.promptManager.GenerateIntervalReviewPromptWithContext(
		tool,
		aitool.InvokeParams{"target": "127.0.0.1"},
		[]byte("partial output"),
		nil,
		time.Unix(0, 0),
		1,
		"expect structured diagnostics",
	)
	require.NoError(t, err)
	prompt2, err := react.promptManager.GenerateIntervalReviewPromptWithContext(
		tool,
		aitool.InvokeParams{"target": "127.0.0.1"},
		[]byte("partial output"),
		nil,
		time.Unix(0, 0),
		1,
		"expect structured diagnostics",
	)
	require.NoError(t, err)

	sec1 := chunkSections(t, prompt1)
	sec2 := chunkSections(t, prompt2)

	require.NotEmpty(t, sec1[aicache.SectionHighStatic], "interval-review prompt must expose high-static chunk")
	require.NotEmpty(t, sec1[aicache.SectionDynamic], "interval-review prompt must expose dynamic chunk")
	require.Empty(t, sec1[aicache.SectionRaw], "interval-review prompt should not produce raw/noise chunk; rendered:\n%s", prompt1)

	require.Equal(t, sec1[aicache.SectionHighStatic][0].Hash, sec2[aicache.SectionHighStatic][0].Hash,
		"high-static hash must be byte-stable across calls")
}

// TestSplit_AIReviewToolCallPrompt_FourSections 验证 aireact 内置的
// ai-review-tool-call.txt (与 aicommon 副本同步) 在重构后:
//   - 至少切出 high-static 和 dynamic 两段
//   - 不出现 raw/noise chunk
//   - 跨调用下 high-static 段 hash 稳定
//
// 关键词: P0-A5, ai-review-tool-call.txt 4 段断言, aicache.Split
func TestSplit_AIReviewToolCallPrompt_FourSections(t *testing.T) {
	react := newSplitTestReact(t)

	prompt1, err := react.promptManager.GenerateAIReviewPrompt(
		"verify file exists", "bash", `{"command":"ls /tmp"}`,
	)
	require.NoError(t, err)
	prompt2, err := react.promptManager.GenerateAIReviewPrompt(
		"verify file exists", "bash", `{"command":"ls /tmp"}`,
	)
	require.NoError(t, err)

	sec1 := chunkSections(t, prompt1)
	sec2 := chunkSections(t, prompt2)

	require.NotEmpty(t, sec1[aicache.SectionHighStatic], "ai-review-tool-call prompt must expose high-static chunk")
	require.NotEmpty(t, sec1[aicache.SectionDynamic], "ai-review-tool-call prompt must expose dynamic chunk")
	require.Empty(t, sec1[aicache.SectionRaw], "ai-review-tool-call prompt should not produce raw/noise chunk; rendered:\n%s", prompt1)

	require.Equal(t, sec1[aicache.SectionHighStatic][0].Hash, sec2[aicache.SectionHighStatic][0].Hash,
		"high-static hash must be byte-stable across calls")
}
