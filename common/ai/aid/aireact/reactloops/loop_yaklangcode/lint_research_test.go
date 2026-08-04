package loop_yaklangcode

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

func TestNeedsYaklangLintResearchGate(t *testing.T) {
	runtime := mock.NewMockInvoker(context.Background())
	loop, err := reactloops.NewReActLoop("lint-gate", runtime)
	require.NoError(t, err)

	require.False(t, needsYaklangLintResearchGate(loop))

	loop.Set("yak_lint_ok", "false")
	require.True(t, needsYaklangLintResearchGate(loop))

	markYaklangLintResearchDone(loop)
	require.False(t, needsYaklangLintResearchGate(loop))

	markYaklangLintResearchNeeded(loop)
	require.True(t, needsYaklangLintResearchGate(loop))

	loop.Set("yak_lint_ok", "true")
	require.False(t, needsYaklangLintResearchGate(loop))
}

func TestYaklangLintResearchActionFilter(t *testing.T) {
	runtime := mock.NewMockInvoker(context.Background())
	loop, err := reactloops.NewReActLoop("lint-filter", runtime)
	require.NoError(t, err)

	box := &yaklangLoopBox{loop: loop}
	filter := newYaklangLintResearchActionFilter(box)

	loop.Set("yak_lint_ok", "false")
	markYaklangLintResearchNeeded(loop)

	assert.False(t, filter(&reactloops.LoopAction{ActionType: "modify_code"}))
	assert.False(t, filter(&reactloops.LoopAction{ActionType: "write_code"}))
	assert.True(t, filter(&reactloops.LoopAction{ActionType: "grep_yaklang_samples"}))
	assert.True(t, filter(&reactloops.LoopAction{ActionType: "yakdoc_function_details"}))

	markYaklangLintResearchDone(loop)
	assert.True(t, filter(&reactloops.LoopAction{ActionType: "modify_code"}))
}
