package loopinfra

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

func newFactoryForSuiteTest(t *testing.T, opts ...SingleFileModificationOption) *SingleFileModificationSuiteFactory {
	t.Helper()
	runtime := mock.NewMockInvoker(context.Background())
	return NewSingleFileModificationSuiteFactory(runtime, opts...)
}

func TestNewSingleFileModificationSuiteFactory_DefaultValues(t *testing.T) {
	f := newFactoryForSuiteTest(t)
	assert.Equal(t, "content", f.prefix)
	assert.Equal(t, "content", f.actionSuffix)
	assert.Equal(t, ".txt", f.fileExtension)
	assert.Equal(t, "GEN_CONTENT", f.aiTagName)
	assert.Equal(t, "content", f.aiTagVariable)
	assert.Equal(t, "content", f.aiNodeId)
	assert.Equal(t, "text/plain", f.contentType)
	assert.Equal(t, "yaklang_code_editor", f.eventType)
	assert.True(t, f.ShouldExitAfterWrite())
	assert.NotNil(t, f.GetRuntime())
}

func TestFactoryOptionsAndGetters(t *testing.T) {
	f := newFactoryForSuiteTest(t,
		WithLoopVarsPrefix("report"),
		WithActionSuffix("section"),
		WithFileExtension(".md"),
		WithAITagConfig("GEN_REPORT", "report_content", "report-content", "text/markdown"),
		WithEventType("report_editor"),
		WithExitAfterWrite(false),
	)

	assert.Equal(t, "report_section", f.GetActionName("report"))
	assert.Equal(t, "report_content", f.GetCodeVariableName())
	assert.Equal(t, "full_report_code", f.GetFullCodeVariableName())
	assert.Equal(t, "report_filename", f.GetFilenameVariableName())
	assert.Equal(t, ".md", f.GetFileExtension())
	assert.Equal(t, "report_editor", f.GetEventType())
	assert.False(t, f.ShouldExitAfterWrite())
}

func TestFactoryGetFullCodeAndFilename_BackwardCompat(t *testing.T) {
	fYak := newFactoryForSuiteTest(t, WithLoopVarsPrefix("yak"))
	assert.Equal(t, "full_code", fYak.GetFullCodeVariableName())
	assert.Equal(t, "filename", fYak.GetFilenameVariableName())

	fCode := newFactoryForSuiteTest(t, WithLoopVarsPrefix("code"))
	assert.Equal(t, "full_code", fCode.GetFullCodeVariableName())
	assert.Equal(t, "filename", fCode.GetFilenameVariableName())
}

func TestGetActions_ReturnsFourActions(t *testing.T) {
	f := newFactoryForSuiteTest(t, WithActionSuffix("code"))
	opts := f.GetActions()
	assert.Len(t, opts, 4)

	// Use a full loop so options can register actions on action map.
	fullLoop, err := reactloops.NewReActLoop("suite-test-loop", mock.NewMockInvoker(context.Background()))
	assert.NoError(t, err)
	for _, opt := range opts {
		opt(fullLoop)
	}
	_, err = fullLoop.GetActionHandler("write_code")
	assert.NoError(t, err)
	_, err = fullLoop.GetActionHandler("modify_code")
	assert.NoError(t, err)
	_, err = fullLoop.GetActionHandler("insert_code")
	assert.NoError(t, err)
	_, err = fullLoop.GetActionHandler("delete_code")
	assert.NoError(t, err)
}

func TestOnFileChanged_DefaultAndConfigured(t *testing.T) {
	f := newFactoryForSuiteTest(t)
	msg, blocking := f.OnFileChanged("abc", nil)
	assert.Equal(t, "", msg)
	assert.False(t, blocking)

	f2 := newFactoryForSuiteTest(t, WithFileChanged(func(content string, operator *reactloops.LoopActionHandlerOperator) (string, bool) {
		return "lint failed", true
	}))
	msg, blocking = f2.OnFileChanged("abc", nil)
	assert.Equal(t, "lint failed", msg)
	assert.True(t, blocking)
}

func TestPrettifyCode_DefaultAndConfigured(t *testing.T) {
	f := newFactoryForSuiteTest(t)
	start, end, code, fixed := f.PrettifyCode("line1")
	assert.Equal(t, 0, start)
	assert.Equal(t, 0, end)
	assert.Equal(t, "line1", code)
	assert.False(t, fixed)

	f2 := newFactoryForSuiteTest(t, WithCodePrettify(func(code string) (int, int, string, bool) {
		return 3, 5, "cleaned", true
	}))
	start, end, code, fixed = f2.PrettifyCode("raw")
	assert.Equal(t, 3, start)
	assert.Equal(t, 5, end)
	assert.Equal(t, "cleaned", code)
	assert.True(t, fixed)
}

func TestDetectSpinningAndReflectionPrompt_DefaultAndConfigured(t *testing.T) {
	f := newFactoryForSuiteTest(t)
	spin, reason := f.DetectSpinning(nil, 1, 2)
	assert.False(t, spin)
	assert.Equal(t, "", reason)
	assert.Equal(t, "", f.GetReflectionPrompt(1, 2, "r"))

	f2 := newFactoryForSuiteTest(t,
		WithSpinDetection(func(loop *reactloops.ReActLoop, startLine, endLine int) (bool, string) {
			return true, "repeat edits"
		}),
		WithReflectionPrompt(func(startLine, endLine int, reason string) string {
			return "please rethink"
		}),
	)
	spin, reason = f2.DetectSpinning(nil, 1, 2)
	assert.True(t, spin)
	assert.Equal(t, "repeat edits", reason)
	assert.Equal(t, "please rethink", f2.GetReflectionPrompt(1, 2, "repeat edits"))
}

func TestDefaultPrettifyAITagCode_EmptyAndInvalidInputs(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{name: "empty", in: ""},
		{name: "all empty lines", in: "\n\n\t"},
		{name: "no line number", in: "plain text"},
		{name: "non consecutive", in: "1 | a\n3 | b"},
		{name: "invalid second line format", in: "1 | a\nb"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end, result, fixed := defaultPrettifyAITagCode(tt.in)
			assert.Equal(t, 0, start)
			assert.Equal(t, 0, end)
			assert.Equal(t, tt.in, result)
			assert.False(t, fixed)
		})
	}
}

func TestDefaultPrettifyAITagCode_ValidCases(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		start, end, result, fixed := defaultPrettifyAITagCode("1 | a\n2 | b")
		assert.True(t, fixed)
		assert.Equal(t, 1, start)
		assert.Equal(t, 2, end)
		assert.Equal(t, "a\nb", result)
	})

	t.Run("leading and trailing empty lines", func(t *testing.T) {
		start, end, result, fixed := defaultPrettifyAITagCode("\n\n10 | x\n11 | y\n\n")
		assert.True(t, fixed)
		assert.Equal(t, 10, start)
		assert.Equal(t, 11, end)
		assert.Equal(t, "x\ny", result)
	})

	t.Run("keep empty line in middle", func(t *testing.T) {
		start, end, result, fixed := defaultPrettifyAITagCode("7 | line1\n\n8 | line2")
		assert.True(t, fixed)
		assert.Equal(t, 7, start)
		assert.Equal(t, 8, end)
		assert.Equal(t, "line1\n\nline2", result)
	})
}

