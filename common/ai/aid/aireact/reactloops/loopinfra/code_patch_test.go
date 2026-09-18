package loopinfra

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

func TestBuildCodePatchLineRange_AbsoluteLines(t *testing.T) {
	patch := BuildCodePatchLineRange("new", 2, 2, "old", 9)
	require.NotNil(t, patch)
	assert.Equal(t, "new", patch.Fragment)
	assert.Equal(t, CodePatchKindLineRange, patch.Meta.Kind)
	assert.Equal(t, 11, patch.Meta.StartLine)
	assert.Equal(t, 11, patch.Meta.EndLine)
	assert.Equal(t, "old", patch.Meta.OldSnippet)
}

func TestBuildCodePatchChangeEvent(t *testing.T) {
	patch := BuildCodePatchSnippet("println(\"x\")", "old()", 0)
	event := BuildCodePatchChangeEvent("/tmp/a.yak", patch, 2, "modify_code", "fix", "yaklang_code")
	assert.Equal(t, CodeEventOpPatch, event.Op)
	assert.Equal(t, "println(\"x\")", event.Code.Content)
	assert.Equal(t, "/tmp/a.yak", event.Code.Path)
	assert.Equal(t, 2, event.Code.Version)
	assert.Equal(t, "modify_code:2", event.Code.ChangeID)
	require.NotNil(t, event.Code.Patch)
	assert.Equal(t, CodePatchKindSnippet, event.Code.Patch.Kind)
	assert.Equal(t, "old()", event.Code.Patch.OldSnippet)
	assert.Equal(t, "modify_code", event.SourceAction)
	assert.Equal(t, "fix", event.Reason)
}

func TestLoopCodeDeliveryPatchRoundTrip(t *testing.T) {
	loop, err := reactloops.NewReActLoop("test", mock.NewMockInvoker(context.Background()))
	require.NoError(t, err)
	assert.Nil(t, GetLoopCodeDeliveryPatch(loop))
	patch := BuildCodePatchInsert("line", 3, 0)
	SetLoopCodeDeliveryPatch(loop, patch)
	got := GetLoopCodeDeliveryPatch(loop)
	require.NotNil(t, got)
	assert.Equal(t, CodePatchKindInsert, got.Meta.Kind)
	assert.Equal(t, 3, got.Meta.InsertLine)
	assert.Equal(t, "line", got.Fragment)
	ClearLoopCodeDeliveryPatch(loop)
	assert.Nil(t, GetLoopCodeDeliveryPatch(loop))
}

func TestBuildCodeChangeIDFallsBackToCode(t *testing.T) {
	assert.Equal(t, "code:1", BuildCodeChangeID("", "", 0))
	assert.Equal(t, "custom:2", BuildCodeChangeID("", "custom", 2))
}

func TestBuildCodePatchKinds(t *testing.T) {
	assert.Equal(t, CodePatchKindFull, BuildCodePatchFull("x").Meta.Kind)
	del := BuildCodePatchDelete(1, 2, "old", 0)
	assert.Equal(t, CodePatchKindDelete, del.Meta.Kind)
	assert.Empty(t, del.Fragment)
}
