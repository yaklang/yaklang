package loop_syntaxflow_rule

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loopinfra"
)

func TestBuildSyntaxFlowFullChangeEvent(t *testing.T) {
	event := BuildSyntaxFlowFullChangeEvent("create", "/tmp/a.sf", "desc(title: \"t\")", 1, "write_rule", "init")
	assert.Equal(t, "create", event.Op)
	assert.Equal(t, "desc(title: \"t\")", event.Code.Content)
	assert.Equal(t, "/tmp/a.sf", event.Code.Path)
	assert.Equal(t, 1, event.Code.Version)
	assert.Equal(t, "write_rule:1", event.Code.ChangeID)
	assert.Equal(t, "write_rule", event.SourceAction)
	assert.Equal(t, "init", event.Reason)
}

func TestBuildSyntaxFlowPatchChangeEvent(t *testing.T) {
	patch := loopinfra.BuildCodePatchSnippet("alert $x;", "old", 0)
	event := BuildSyntaxFlowPatchChangeEvent("/tmp/a.sf", patch, 3, "modify_rule", "fix")
	assert.Equal(t, loopinfra.CodeEventOpPatch, event.Op)
	assert.Equal(t, "alert $x;", event.Code.Content)
	assert.Equal(t, "/tmp/a.sf", event.Code.Path)
	assert.Equal(t, 3, event.Code.Version)
	assert.Equal(t, "modify_rule:3", event.Code.ChangeID)
	require.NotNil(t, event.Code.Patch)
	assert.Equal(t, loopinfra.CodePatchKindSnippet, event.Code.Patch.Kind)
	assert.Equal(t, "old", event.Code.Patch.OldSnippet)
}
