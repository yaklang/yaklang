package syntaxflow_utils

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

// testVars implements VarSource for unit tests.
type testVars map[string]any

func (m testVars) GetVariable(k string) any {
	if m == nil {
		return nil
	}
	return m[k]
}

func testTask(attached ...*aicommon.AttachedResource) aicommon.AIStatefulTask {
	tb := aicommon.NewStatefulTaskBase("tid", "只看高危 SQL", context.Background(), nil, true)
	tb.SetAttachedDatas(attached)
	return tb
}

func TestSyntaxFlowTaskID_Attachment(t *testing.T) {
	task := testTask(aicommon.NewAttachedResource(
		AttachedTypeSyntaxFlow,
		AttachedKeyTaskID,
		"550e8400-e29b-41d4-a716-446655440000",
	))
	id, ok := SyntaxFlowTaskID(task, nil)
	require.True(t, ok)
	require.Equal(t, "550e8400-e29b-41d4-a716-446655440000", id)
}

func TestSyntaxFlowTaskID_LoopVarWinsOverAttachment(t *testing.T) {
	vars := testVars{LoopVarSyntaxFlowTaskID: "from-loop"}
	task := testTask(aicommon.NewAttachedResource(
		AttachedTypeSyntaxFlow,
		AttachedKeyTaskID,
		"from-attach",
	))
	id, ok := SyntaxFlowTaskID(task, vars)
	require.True(t, ok)
	require.Equal(t, "from-loop", id)
}

func TestSSARiskID_Attachment(t *testing.T) {
	task := testTask(aicommon.NewAttachedResource(
		AttachedTypeSSARisk,
		AttachedKeyRiskID,
		"42",
	))
	id, ok := SSARiskID(task, nil)
	require.True(t, ok)
	require.Equal(t, int64(42), id)
}

func TestSSARisksFilterForOverview_FuzzySearchOnly(t *testing.T) {
	task := aicommon.NewStatefulTaskBase("x", "只看高危 SQL", context.Background(), nil, true)
	f := SSARisksFilterForOverview(task, nil, task.GetUserInput())
	require.Equal(t, "只看高危 SQL", f.Search)
	require.Empty(t, f.RuntimeID)
}

func TestSSARisksFilterForOverview_AttachmentFields(t *testing.T) {
	task := testTask(
		aicommon.NewAttachedResource(AttachedTypeSSARisksFilter, AttachedKeyRuntimeID, "rt-1"),
		aicommon.NewAttachedResource(AttachedTypeSSARisksFilter, AttachedKeyProgramName, "myapp"),
	)
	f := SSARisksFilterForOverview(task, nil, task.GetUserInput())
	require.Equal(t, []string{"rt-1"}, f.RuntimeID)
	require.Equal(t, []string{"myapp"}, f.ProgramName)
	require.Empty(t, f.Search)
}

func TestSyntaxFlowScanSessionMode_Attachment(t *testing.T) {
	task := testTask(aicommon.NewAttachedResource(
		AttachedTypeSyntaxFlow,
		AttachedKeySessionMode,
		SessionModeStart,
	))
	require.Equal(t, SessionModeStart, SyntaxFlowScanSessionMode(task, nil))
}

func TestSyntaxFlowRuleFullQuality_Attachment(t *testing.T) {
	task := testTask(aicommon.NewAttachedResource(
		AttachedTypeSyntaxFlowRule,
		AttachedKeyFullQuality,
		"true",
	))
	require.True(t, SyntaxFlowRuleFullQuality(task, nil))
}

func TestProgramNamesHint(t *testing.T) {
	task := testTask(aicommon.NewAttachedResource(
		AttachedTypeSyntaxFlow,
		AttachedKeyPrograms,
		"a, b",
	))
	require.Equal(t, []string{"a", "b"}, ProgramNamesHint(task))
}
