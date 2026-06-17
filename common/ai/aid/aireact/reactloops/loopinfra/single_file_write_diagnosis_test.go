package loopinfra

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

func TestDiagnoseMissingWriteCode_MarkdownWithoutTag(t *testing.T) {
	factory := NewSingleFileModificationSuiteFactory(
		mock.NewMockInvoker(context.Background()),
		WithActionSuffix("code"),
		WithAITagConfig("GEN_CODE", "yak_code", "yaklang-code", "code/yaklang"),
	)
	loop, err := reactloops.NewReActLoop("diag-test", factory.GetRuntime())
	assert.NoError(t, err)
	loop.Set(LoopVarLastAIDecisionResponse, `{"@action":"write_code"}
`+"```yak\nprintln(1)\n```")

	msg := factory.DiagnoseMissingWriteCode(loop)
	assert.Contains(t, msg, "write_code")
	assert.Contains(t, msg, "markdown code fences")
	assert.Contains(t, msg, "GEN_CODE")
}

func TestDiagnoseMissingWriteCode_MissingTagBlock(t *testing.T) {
	factory := NewSingleFileModificationSuiteFactory(
		mock.NewMockInvoker(context.Background()),
		WithActionSuffix("code"),
		WithAITagConfig("GEN_CODE", "yak_code", "yaklang-code", "code/yaklang"),
	)
	loop, err := reactloops.NewReActLoop("diag-test", factory.GetRuntime())
	assert.NoError(t, err)
	loop.Set(LoopVarLastAIDecisionResponse, `{"@action":"write_code","human_readable_thought":"hi"}`)

	msg := factory.DiagnoseMissingWriteCode(loop)
	assert.Contains(t, msg, "missing")
	assert.Contains(t, msg, "GEN_CODE")
}

func TestDiagnoseMissingWriteCode_EmptyTagBlock(t *testing.T) {
	factory := NewSingleFileModificationSuiteFactory(
		mock.NewMockInvoker(context.Background()),
		WithActionSuffix("code"),
		WithAITagConfig("GEN_CODE", "yak_code", "yaklang-code", "code/yaklang"),
	)
	loop, err := reactloops.NewReActLoop("diag-test", factory.GetRuntime())
	assert.NoError(t, err)
	loop.Set(LoopVarLastAIDecisionResponse, `{"@action":"write_code"}
<|GEN_CODE_abcd|>
<|GEN_CODE_END_abcd|>`)

	msg := factory.DiagnoseMissingWriteCode(loop)
	assert.Contains(t, msg, "yak_code")
	assert.Contains(t, msg, "wrong nonce")
}
