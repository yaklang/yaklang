package aireact

import (
	"bytes"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils"
)

// Verification 保持纯观测，schema 不包含 todo_delta。TodoSnapshot 的只读
// 渲染由 TestGenerateVerificationPrompt_TruncatesLongTodoSnapshotButKeepsFocus 覆盖。

func TestGenerateVerificationPrompt_IncludesEvidenceJSONArrayGuidance(t *testing.T) {
	react, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "test"}}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create ReAct instance: %v", err)
	}

	prompt, _, err := react.promptManager.GenerateVerificationPrompt("请继续验证接口行为", true, "tool executed: continue")
	if err != nil {
		t.Fatalf("Failed to generate verification prompt: %v", err)
	}

	if !utils.MatchAllOfSubString(
		prompt,
		"`evidence` 不是必填字段",
		"JSON 对象数组",
		"`op`",
		"`id`",
		"`content`",
	) {
		t.Fatalf("verification prompt should contain evidence JSON array guidance. Got:\n%s", prompt)
	}

	if strings.Contains(prompt, "<|EVIDENCE_") {
		t.Fatalf("verification prompt should NOT contain AITAG EVIDENCE blocks anymore. Got:\n%s", prompt)
	}
}
