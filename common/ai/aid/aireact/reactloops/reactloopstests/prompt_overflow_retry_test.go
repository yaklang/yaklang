package reactloopstests

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

// Prompt overflow 只收紧下一次 Prompt 的边界；错误写入 Timeline 后继续执行，
// 不把 provider 的 typed overflow 直接作为任务终态返回。
func TestReActLoop_PromptOverflowRecordsLimitAndContinues(t *testing.T) {
	var mainPromptTokens []int
	compressionCalls := 0
	reactIns, err := aireact.NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			if req.GetCallerLabel() == aicommon.CallerLabelContextCompress {
				compressionCalls++
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(strings.NewReader(
					`{"@action":"compress_context","summary":"已删除重复的溢出诊断，保留当前任务。","retain_timeline":"","retire_skill_names":[],"retire_evidence_ids":[]}`,
				))
				rsp.Close()
				return rsp, nil
			}
			mainPromptTokens = append(mainPromptTokens, aicommon.MeasureTokens(req.GetPrompt()))
			rsp := i.NewAIResponse()
			if len(mainPromptTokens) == 1 {
				rsp.SetRawHTTPResponseData(
					[]byte("HTTP/1.1 400 Bad Request\r\nContent-Type: application/json\r\n\r\n"),
					[]byte(`{"error":{"code":"context_length_exceeded","message":"This model's maximum context length is 32000 tokens. However, your messages resulted in 40000 tokens."}}`),
				)
			} else {
				rsp.EmitOutputStream(strings.NewReader(`{"@action":"finish","answer":"recovered"}`))
			}
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)
	cfg, ok := reactIns.GetConfig().(*aicommon.Config)
	require.True(t, ok)
	base := cfg.GetTimelineContentSizeLimit()
	require.NotNil(t, cfg.GetTimeline())

	loop, err := reactloops.NewReActLoop("prompt-overflow-loop", reactIns)
	require.NoError(t, err)
	err = loop.Execute("prompt-overflow-task", context.Background(), "test input")

	require.NoError(t, err)
	require.Len(t, mainPromptTokens, 2, "overflow recovery must continue with a new outer-loop request")
	require.LessOrEqual(t, len(mainPromptTokens), cfg.GetPromptOverflowRetryMax()+2, "main-loop retries must remain bounded")
	require.Zero(t, compressionCalls, "the recorded provider limit still fits the static prompt")
	require.EqualValues(t, 32000*8/10, cfg.GetTimeline().GetTimelineContentLimit(),
		"the loop records a tighter limit for the model that rejected the prompt")
	require.Less(t, cfg.GetTimeline().GetTimelineContentLimit(), base)
}

// 普通 API 失败同样进入 Timeline 自愈，但不会错误地收紧 Prompt 容量限制。
func TestReActLoop_PlainRequestFailureDoesNotTouchTheLimit(t *testing.T) {
	callCount := 0
	reactIns, err := aireact.NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			callCount++
			rsp := i.NewAIResponse()
			if callCount == 1 {
				rsp.SetRawHTTPResponseData(
					[]byte("HTTP/1.1 400 Bad Request\r\nContent-Type: application/json\r\n\r\n"),
					[]byte(`{"error":{"message":"invalid schema for function"}}`),
				)
			} else {
				rsp.EmitOutputStream(strings.NewReader(`{"@action":"finish","answer":"recovered"}`))
			}
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)
	cfg, ok := reactIns.GetConfig().(*aicommon.Config)
	require.True(t, ok)
	base := cfg.GetTimelineContentSizeLimit()

	loop, err := reactloops.NewReActLoop("plain-failure-loop", reactIns)
	require.NoError(t, err)
	err = loop.Execute("plain-failure-task", context.Background(), "test input")

	require.NoError(t, err)
	require.GreaterOrEqual(t, callCount, 2)
	require.Equal(t, base, cfg.GetTimeline().GetTimelineContentLimit())
}
