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
	filter := newYaklangLoopActionFilter(box)

	loop.Set("full_code", "println(1)")
	loop.Set("yak_lint_ok", "false")
	markYaklangLintResearchNeeded(loop)

	assert.False(t, filter(&reactloops.LoopAction{ActionType: "modify_code"}))
	assert.False(t, filter(&reactloops.LoopAction{ActionType: "write_code"}))
	assert.True(t, filter(&reactloops.LoopAction{ActionType: "grep_yaklang_samples"}))
	assert.True(t, filter(&reactloops.LoopAction{ActionType: "yakdoc_function_details"}))
	assert.False(t, filter(&reactloops.LoopAction{ActionType: "finish"}))

	markYaklangLintResearchDone(loop)
	assert.True(t, filter(&reactloops.LoopAction{ActionType: "modify_code"}))
}

type stubLoopEarlyExitKV struct {
	data map[string]string
}

func (s *stubLoopEarlyExitKV) Get(key string) string {
	if s == nil || s.data == nil {
		return ""
	}
	return s.data[key]
}

func TestNeedsBlockYaklangEarlyExit(t *testing.T) {
	loop := &stubLoopEarlyExitKV{data: map[string]string{}}
	assert.True(t, needsBlockYaklangEarlyExit(loop))

	loop.data["full_code"] = "println(1)"
	assert.False(t, needsBlockYaklangEarlyExit(loop))

	loop.data["yak_lint_ok"] = "false"
	assert.True(t, needsBlockYaklangEarlyExit(loop))
}

func TestYaklangLoopActionFilter_EarlyExitBlocked(t *testing.T) {
	runtime := mock.NewMockInvoker(context.Background())
	loop, err := reactloops.NewReActLoop("lint-filter-exit", runtime)
	require.NoError(t, err)

	box := &yaklangLoopBox{loop: loop}
	filter := newYaklangLoopActionFilter(box)

	assert.False(t, filter(&reactloops.LoopAction{ActionType: "finish"}))
	assert.False(t, filter(&reactloops.LoopAction{ActionType: "directly_answer"}))
	assert.True(t, filter(&reactloops.LoopAction{ActionType: "write_code"}))

	loop.Set("full_code", "println(1)")
	loop.Set("yak_lint_ok", "true")
	assert.True(t, filter(&reactloops.LoopAction{ActionType: "finish"}))
}

func TestYaklangLoopActionFilter_LintAllowsModifyAfterResearch(t *testing.T) {
	runtime := mock.NewMockInvoker(context.Background())
	loop, err := reactloops.NewReActLoop("lint-filter-modify", runtime)
	require.NoError(t, err)

	box := &yaklangLoopBox{loop: loop}
	filter := newYaklangLoopActionFilter(box)

	loop.Set("full_code", "bad")
	loop.Set("yak_lint_ok", "false")
	markYaklangLintResearchDone(loop)

	assert.True(t, filter(&reactloops.LoopAction{ActionType: "modify_code"}))
	assert.True(t, filter(&reactloops.LoopAction{ActionType: "write_code"}))
}

func TestBuildPinnedDSLSection(t *testing.T) {
	section := BuildPinnedDSLSection()
	require.Contains(t, section, "func(x string)")
	require.Contains(t, section, "poc.Post")
	require.Contains(t, section, "append(a, b...)")
	require.Contains(t, section, "跨行")
}
