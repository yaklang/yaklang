package loopinfra

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

func TestPatchFailGuard_BumpAndFallback(t *testing.T) {
	runtime := newTestRuntimeForSingleFile(t)
	loop, factory, _ := newLoopAndFactory(t, runtime, WithActionSuffix("code"))
	actionName := factory.GetActionName("modify")

	assert.False(t, IsPatchFallbackMode(loop, actionName))

	c1, fb1 := bumpPatchApplyFail(loop, actionName)
	assert.Equal(t, 1, c1)
	assert.False(t, fb1)

	c2, fb2 := bumpPatchApplyFail(loop, actionName)
	assert.Equal(t, 2, c2)
	assert.False(t, fb2)

	c3, fb3 := bumpPatchApplyFail(loop, actionName)
	assert.Equal(t, 3, c3)
	assert.True(t, fb3)
	assert.True(t, IsPatchFallbackMode(loop, actionName))
	assert.True(t, IsModifyCodePatchFallbackMode(loop))

	resetPatchApplyFail(loop, actionName)
	assert.False(t, IsPatchFallbackMode(loop, actionName))
	assert.Equal(t, 0, loop.GetInt(patchApplyFailCountKey(actionName)))
}

func TestModifyAction_Patch_ApplyFailed_TripsFallbackAfterThree(t *testing.T) {
	runtime := newTestRuntimeForSingleFile(t)
	loop, factory, task := newLoopAndFactory(t, runtime, WithActionSuffix("code"), WithExitAfterWrite(false))
	filename := filepath.Join(runtime.tmpDir, "patch_spin.yak")
	orig := "a = 1\nb = 2\n"
	loop.Set(factory.GetFilenameVariableName(), filename)
	loop.Set(factory.GetFullCodeVariableName(), orig)
	require.NoError(t, os.WriteFile(filename, []byte(orig), 0o644))

	badPatch := `*** Begin Patch
@@ miss
-missing_line_that_does_not_exist
+x = 1
*** End Patch`
	ac, err := loop.GetActionHandler(factory.GetActionName("modify"))
	require.NoError(t, err)

	for i := 1; i <= maxConsecutivePatchApplyFail; i++ {
		loop.Set(factory.GetCodeVariableName(), badPatch)
		op := reactloops.NewActionHandlerOperator(task)
		ac.ActionHandler(loop, mustBuildAction(t, factory.GetActionName("modify"), map[string]any{
			"modify_code_reason": "will fail",
		}), op)
		assert.True(t, op.IsContinued())
		assert.True(t, runtime.timelineContains("modify_patch_apply_failed"))
		assert.Equal(t, orig, loop.Get(factory.GetFullCodeVariableName()))
		assert.Equal(t, i, loop.GetInt(patchApplyFailCountKey(factory.GetActionName("modify"))))
	}

	assert.True(t, IsPatchFallbackMode(loop, factory.GetActionName("modify")))
	assert.True(t, runtime.timelineContains("modify_patch_fallback_mode"))

	// After fallback, line-range modify must succeed and clear the guard.
	loop.Set(factory.GetCodeVariableName(), "b = 9")
	op := reactloops.NewActionHandlerOperator(task)
	ac.ActionHandler(loop, mustBuildAction(t, factory.GetActionName("modify"), map[string]any{
		"modify_start_line":  2,
		"modify_end_line":    2,
		"modify_code_reason": "line range after fallback",
	}), op)
	assert.Equal(t, "a = 1\nb = 9\n", loop.Get(factory.GetFullCodeVariableName()))
	assert.False(t, IsPatchFallbackMode(loop, factory.GetActionName("modify")))
	assert.Equal(t, 0, loop.GetInt(patchApplyFailCountKey(factory.GetActionName("modify"))))
}

func TestModifyAction_PatchFallback_AllowsLargeLineRangeFunctionBody(t *testing.T) {
	runtime := newTestRuntimeForSingleFile(t)
	loop, factory, task := newLoopAndFactory(t, runtime, WithActionSuffix("code"), WithExitAfterWrite(false))
	filename := filepath.Join(runtime.tmpDir, "fallback_fn.yak")
	orig := "build = func(x) {\n    return x\n}\n"
	loop.Set(factory.GetFilenameVariableName(), filename)
	loop.Set(factory.GetFullCodeVariableName(), orig)
	require.NoError(t, os.WriteFile(filename, []byte(orig), 0o644))

	// Trip fallback without going through real patch (set flags directly).
	actionName := factory.GetActionName("modify")
	loop.Set(patchApplyFailCountKey(actionName), "3")
	loop.Set(patchFallbackModeKey(actionName), "true")

	newBody := `build = func(x) {
    y = x + 1
    z = y + 2
    return z
}`
	loop.Set(factory.GetCodeVariableName(), newBody)
	ac, err := loop.GetActionHandler(actionName)
	require.NoError(t, err)
	op := reactloops.NewActionHandlerOperator(task)
	ac.ActionHandler(loop, mustBuildAction(t, actionName, map[string]any{
		"modify_start_line":  1,
		"modify_end_line":    3,
		"modify_code_reason": "rewrite function under fallback",
	}), op)

	full := loop.Get(factory.GetFullCodeVariableName())
	assert.Contains(t, full, "z = y + 2")
	assert.False(t, IsPatchFallbackMode(loop, actionName))
	assert.True(t, runtime.timelineContains("modify_success"))
}

func TestNearestCodeContextHint_FindsAnchor(t *testing.T) {
	full := "a = 1\nbuildHttpPayload = func(frame) {\n    return frame\n}\n"
	hint := nearestCodeContextHint(full, "-buildHttpPayload = func(frame []byte) {\n")
	assert.Contains(t, hint, "buildHttpPayload")
	assert.Contains(t, hint, "2|")
}
