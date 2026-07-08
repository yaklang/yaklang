package loopinfra

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

func TestBuildYaklangPatchLineRange_AbsoluteLines(t *testing.T) {
	patch := BuildYaklangPatchLineRange("new", 2, 2, "old", 9)
	require.NotNil(t, patch)
	assert.Equal(t, "new", patch.Fragment)
	assert.Equal(t, 9, patch.LineBase)
	assert.Equal(t, YaklangPatchKindLineRange, patch.Meta.Kind)
	assert.Equal(t, 11, patch.Meta.StartLine)
	assert.Equal(t, 11, patch.Meta.EndLine)
	assert.Equal(t, "old", patch.Meta.OldSnippet)
}

func TestBuildYaklangPatchChangeEvent(t *testing.T) {
	patch := BuildYaklangPatchSnippet("println(\"x\")", "old()", 0)
	event := BuildYaklangPatchChangeEvent("/tmp/a.yak", patch, 2, "modify_code", "fix")
	assert.Equal(t, LoopYaklangCodeEventOpPatch, event.Op)
	assert.Equal(t, "println(\"x\")", event.Code.Content)
	assert.Equal(t, "/tmp/a.yak", event.Code.Path)
	assert.Equal(t, 2, event.Code.Version)
	assert.Equal(t, "modify_code:2", event.Code.ChangeID)
	require.NotNil(t, event.Code.Patch)
	assert.Equal(t, YaklangPatchKindSnippet, event.Code.Patch.Kind)
	assert.Equal(t, "old()", event.Code.Patch.OldSnippet)
}

func TestLoopYaklangDeliveryPatchStorage(t *testing.T) {
	runtime := newTestRuntimeForSingleFile(t)
	loop, err := reactloops.NewReActLoop("patch-store-test", runtime)
	require.NoError(t, err)

	assert.Nil(t, GetLoopYaklangDeliveryPatch(loop))
	patch := BuildYaklangPatchInsert("line", 3, 0)
	SetLoopYaklangDeliveryPatch(loop, patch)
	got := GetLoopYaklangDeliveryPatch(loop)
	require.NotNil(t, got)
	assert.Equal(t, "line", got.Fragment)
	assert.Equal(t, 3, got.Meta.InsertLine)

	ClearLoopYaklangDeliveryPatch(loop)
	assert.Nil(t, GetLoopYaklangDeliveryPatch(loop))
}
