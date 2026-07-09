package subagent

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
)

func TestBuildForwardingEmitterForTask_StampsTaskIdAndUUID(t *testing.T) {
	var captured []*schema.AiOutputEvent
	rootEmitter := aicommon.NewEmitter("root", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		captured = append(captured, e)
		return e, nil
	})

	const subTaskID = "parent-phase2-sub-xxe_ssrf-abcd"
	subTask := aicommon.NewSubTaskBaseWithOptions(
		aicommon.NewStatefulTaskBase("orchestrator", "audit", nil, rootEmitter, true),
		subTaskID,
		"scan",
		aicommon.WithStatefulTaskBaseSubAgent(),
		aicommon.WithStatefulTaskBaseSkipTaskStatusChangeEmit(),
	)
	forwarding := BuildForwardingEmitterForTask(rootEmitter, subTask)
	require.NotNil(t, forwarding)

	_, err := forwarding.EmitStatus("read_file", "running")
	require.NoError(t, err)
	require.Len(t, captured, 1)
	require.Equal(t, subTaskID, captured[0].TaskId)
	require.Equal(t, subTask.GetUUID(), captured[0].TaskUUID)
}
