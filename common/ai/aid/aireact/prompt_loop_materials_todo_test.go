package aireact

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicache"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// TestPromptManager_AssembleLoopPrompt_TodoBlockAfterSessionEvidence 验证
// loop prompt timeline-open 段中 TODO 块紧跟 SESSION_EVIDENCE 之后, 与
// SessionEvidence 并列暴露给模型。这是用户明确要求的物理位置, 让 loop 任何
// 一次 iteration 都能看到 TODO 列表, 不再受限于 Verify 调用时机。
//
// 关键词: TodoSnapshot 段顺序, SESSION_EVIDENCE 之后, timeline-open
func TestPromptManager_AssembleLoopPrompt_TodoBlockAfterSessionEvidence(t *testing.T) {
	react, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"object"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)

	sessionEvidence := "<|SESSION_EVIDENCE_ntodo|>\n# Evidence body\n<|SESSION_EVIDENCE_END_ntodo|>"
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
			SessionEvidence: sessionEvidence,
			TodoSnapshot:    todoSnapshot,
		},
	)
	require.NoError(t, err)
	require.NotNil(t, result)

	prompt := result.Prompt
	sessionEvidenceIdx := strings.Index(prompt, "<|SESSION_EVIDENCE_ntodo|>")
	todoListIdx := strings.Index(prompt, "<|TODO_LIST_ntodo|>")
	timelineOpenSectionIdx := strings.Index(prompt, "<|PROMPT_SECTION_timeline-open|>")
	workspaceIdx := strings.Index(prompt, "# Workspace Context")

	require.NotEqual(t, -1, sessionEvidenceIdx, "loop prompt must expose SESSION_EVIDENCE block when evidence is non-empty")
	require.NotEqual(t, -1, todoListIdx, "loop prompt must expose TODO_LIST block when todo list is non-empty")
	require.NotEqual(t, -1, timelineOpenSectionIdx)
	require.NotEqual(t, -1, workspaceIdx)

	require.Less(t, timelineOpenSectionIdx, sessionEvidenceIdx,
		"SESSION_EVIDENCE block must live inside the timeline-open section, not above it")
	require.Less(t, sessionEvidenceIdx, todoListIdx,
		"TODO_LIST block must come AFTER SESSION_EVIDENCE block (the user-requested physical layout)")
	require.Less(t, todoListIdx, workspaceIdx,
		"TODO_LIST block must come before Workspace section")

	require.Contains(t, prompt, "- [ ]: [id: verify_target]: 复现目标错误码")
	require.Contains(t, prompt, "- [ ]: [id: collect_signal]: 采集响应特征")
}

// TestPromptManager_AssembleLoopPrompt_TodoBlockSkippedWhenEmpty 验证当
// SessionPromptState 中 TODO 列表为空时, loop prompt 不渲染 TODO_LIST 块,
// 不引入空段污染 prompt。
//
// 关键词: TodoSnapshot 空块过滤, 模板 if 跳过
func TestPromptManager_AssembleLoopPrompt_TodoBlockSkippedWhenEmpty(t *testing.T) {
	react, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"object"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)

	result, err := react.promptManager.AssembleLoopPrompt(
		[]*aitool.Tool{},
		&reactloops.LoopPromptAssemblyInput{
			Nonce:           "nempty",
			UserQuery:       "current user query",
			TaskInstruction: "follow task rules",
			OutputExample:   "example output",
			Schema:          `{"type":"object","properties":{"@action":{"type":"string"}}}`,
		},
	)
	require.NoError(t, err)
	require.NotNil(t, result)

	require.NotContains(t, result.Prompt, "<|TODO_LIST_",
		"empty TODO state should NOT render the TODO_LIST block")
}

// TestPromptManager_AssembleLoopPrompt_TodoBlockStaysInTimelineOpenCacheBoundary
// 验证 TODO_LIST 块通过 aicache.Split 后落在 timeline-open 段, 不会污染
// frozen / semi-dynamic / high-static 三段 prefix cache。
//
// 关键词: TodoSnapshot 缓存边界, aicache.Split timeline-open, prefix cache 保护
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

	splitRes := aicache.Split(result.Prompt)
	require.NotNil(t, splitRes)

	todoLandedInTimelineOpen := false
	for _, chunk := range splitRes.Chunks {
		if !strings.Contains(chunk.Content, "<|TODO_LIST_ncache|>") {
			continue
		}
		require.Equal(t, aicache.SectionTimelineOpen, chunk.Section,
			"TODO_LIST chunk must live in timeline-open section, not %s", chunk.Section)
		todoLandedInTimelineOpen = true
	}
	require.True(t, todoLandedInTimelineOpen, "TODO_LIST chunk must appear after split")
}
