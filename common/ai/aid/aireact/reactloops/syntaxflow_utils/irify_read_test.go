package syntaxflow_utils

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func testTaskWithAttached(attachments ...*aicommon.AttachedResource) aicommon.AIStatefulTask {
	tb := aicommon.NewStatefulTaskBase("tid", "只看高危 SQL", context.Background(), nil, true)
	tb.SetAttachedDatas(attachments)
	return tb
}

func TestReadIrifySyntaxFlowTaskIDFromTask(t *testing.T) {
	task := testTaskWithAttached(aicommon.NewAttachedResource(
		IrifyTypeSyntaxFlow,
		IrifyKeyTaskID,
		"550e8400-e29b-41d4-a716-446655440000",
	))
	id, ok := ReadIrifySyntaxFlowTaskIDFromTask(task)
	require.True(t, ok)
	require.Equal(t, "550e8400-e29b-41d4-a716-446655440000", id)
}

func TestReadIrifySSARiskIDFromTask(t *testing.T) {
	task := testTaskWithAttached(aicommon.NewAttachedResource(
		IrifyTypeSSARisk,
		IrifyKeyRiskID,
		"42",
	))
	id, ok := ReadIrifySSARiskIDFromTask(task)
	require.True(t, ok)
	require.Equal(t, int64(42), id)
}

func TestSSARisksFilterFromAttachments_RawFields(t *testing.T) {
	task := testTaskWithAttached(
		aicommon.NewAttachedResource(IrifyTypeSSARisksFilter, IrifyKeyRuntimeID, "rt-1"),
		aicommon.NewAttachedResource(IrifyTypeSSARisksFilter, IrifyKeyProgramName, "myapp"),
	)
	f, ok := buildSSARisksFilterFromTaskAttachments(task)
	require.True(t, ok)
	require.Equal(t, []string{"rt-1"}, f.RuntimeID)
	require.Equal(t, []string{"myapp"}, f.ProgramName)
}

func TestIrifyProgramNamesFromTask(t *testing.T) {
	task := testTaskWithAttached(aicommon.NewAttachedResource(
		IrifyTypeSyntaxFlow,
		IrifyKeyPrograms,
		"a, b",
	))
	require.Equal(t, []string{"a", "b"}, IrifyProgramNamesFromTask(task))
}
