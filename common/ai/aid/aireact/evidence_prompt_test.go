package aireact

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// Evidence deltas are ordinary Timeline entries; TODO follows the open history.
func TestPromptManager_AssembleLoopPrompt_TodoBlockAfterTimelineEvidenceDelta(t *testing.T) {
	react, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"object"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)

	react.config.ApplySessionEvidenceOps([]aicommon.EvidenceOperation{{Op: "add", ID: "todo-evidence", Content: "Evidence body"}})
	todoSnapshot := strings.Join([]string{
		"<|TODO_LIST_ntodo|>",
		"## 待办清单（TODO）",
		"- [ ]: [id: verify_target]: 复现目标错误码",
		"- [ ]: [id: collect_signal]: 采集响应特征",
		"<|TODO_LIST_END_ntodo|>",
	}, "\n")

	result, err := react.promptManager.AssembleLoopPrompt(
		[]*aitool.Tool{},
		&reactloops.LoopPromptAssemblyInput{
			Nonce:           "ntodo",
			UserQuery:       "current user query",
			TaskInstruction: "follow task rules",
			OutputExample:   "example output",
			Schema:          `{"type":"object","properties":{"@action":{"type":"string"}}}`,
			TodoSnapshot:    todoSnapshot,
		},
	)
	require.NoError(t, err)
	require.NotNil(t, result)

	prompt := result.Prompt
	require.NotContains(t, prompt, "<|SESSION_EVIDENCE_")
	sessionEvidenceIdx := strings.Index(prompt, "[id: todo-evidence]")
	todoListIdx := strings.Index(prompt, "<|TODO_LIST_ntodo|>")
	timelineOpenSectionIdx := strings.Index(prompt, "<|PROMPT_SECTION_timeline-open_")
	workspaceIdx := strings.Index(prompt, "# Workspace Context")

	require.NotEqual(t, -1, sessionEvidenceIdx, "loop prompt must expose evidence deltas in ordinary history")
	require.NotEqual(t, -1, todoListIdx, "loop prompt must expose TODO_LIST block when todo list is non-empty")
	require.NotEqual(t, -1, timelineOpenSectionIdx)
	require.NotEqual(t, -1, workspaceIdx)

	require.Less(t, timelineOpenSectionIdx, sessionEvidenceIdx,
		"evidence delta must live inside the timeline-open section")
	require.Less(t, sessionEvidenceIdx, todoListIdx,
		"TODO_LIST must follow the ordinary Open Timeline")
	require.Less(t, workspaceIdx, timelineOpenSectionIdx,
		"stable workspace must precede Open Timeline")

	require.Contains(t, prompt, "- [ ]: [id: verify_target]: 复现目标错误码")
	require.Contains(t, prompt, "- [ ]: [id: collect_signal]: 采集响应特征")
	require.Contains(t, prompt, "TODO LIST 是 `todo_delta` 累计维护后的只读快照")
	require.Contains(t, prompt, "见任务指令段 `## TODO 状态维护（todo_delta）` 末尾的")
	require.Contains(t, prompt, "只读快照")
}
