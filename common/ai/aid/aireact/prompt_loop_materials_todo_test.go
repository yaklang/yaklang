package aireact

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestPromptManager_AssembleLoopPrompt_EmptyTodoStateVisible(t *testing.T) {
	react, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"object"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)
	emptySnapshot := react.config.GetVerificationTodoRendered(aicommon.VerificationTodoScope{TaskID: "task-empty"})
	require.Contains(t, emptySnapshot, "空清单不代表任务完成")

	result, err := react.promptManager.AssembleLoopPrompt(
		[]*aitool.Tool{},
		&reactloops.LoopPromptAssemblyInput{
			Nonce:           "nempty",
			UserQuery:       "current user query",
			TaskInstruction: "follow task rules",
			OutputExample:   "example output",
			Schema:          `{"type":"object","properties":{"@action":{"type":"string"}}}`,
			TodoSnapshot:    "<|TODO_LIST_nempty|>" + emptySnapshot + "<|TODO_LIST_END_nempty|>",
		},
	)
	require.NoError(t, err)
	require.NotNil(t, result)

	require.Contains(t, result.Prompt, "<|TODO_LIST_nempty|>")
	require.Contains(t, result.Prompt, "空清单不代表任务完成")
}

// TestPromptManager_AssembleLoopPrompt_TodoBlockStaysInTimelineOpenCacheBoundary
// 验证 TODO_LIST 块通过 aiprojection.Split 后落在 timeline-open 段, 不会污染
// frozen / semi-dynamic / high-static 三段 prefix cache。
//
// 关键词: TodoSnapshot 缓存边界, aiprojection.Split timeline-open, prefix cache 保护
func TestPromptManager_AssembleLoopPrompt_TodoBlockStaysInTimelineOpenCacheBoundary(t *testing.T) {
	react, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"object"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)

	todoSnapshot := strings.Join([]string{
		"<|TODO_LIST_ncache|>",
		"## 待办清单（TODO）",
		"- [ ]: [id: verify_target]: 复现目标错误码",
		"<|TODO_LIST_END_ncache|>",
	}, "\n")

	result, err := react.promptManager.AssembleLoopPrompt(
		[]*aitool.Tool{},
		&reactloops.LoopPromptAssemblyInput{
			Nonce:           "ncache",
			UserQuery:       "current user query",
			TaskInstruction: "follow task rules",
			OutputExample:   "example output",
			Schema:          `{"type":"object","properties":{"@action":{"type":"string"}}}`,
			TodoSnapshot:    todoSnapshot,
		},
	)
	require.NoError(t, err)
	require.NotNil(t, result)

	splitRes := aiprojection.Split(result.Prompt)
	require.NotNil(t, splitRes)

	todoLandedInTimelineOpen := false
	for _, chunk := range splitRes.Chunks {
		if !strings.Contains(chunk.Content, "verify_target") {
			continue
		}
		require.Equal(t, aiprojection.SectionTimelineOpen, chunk.Section,
			"TODO_LIST chunk must live in timeline-open section, not %s", chunk.Section)
		todoLandedInTimelineOpen = true
	}
	require.True(t, todoLandedInTimelineOpen, "TODO_LIST chunk must appear after split")
}
