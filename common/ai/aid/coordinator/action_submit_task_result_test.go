package coordinator

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

func TestCoordinatorActionSubmitTaskResult(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(map[bool]string{false: "text", true: "native"}[native], func(t *testing.T) {
			f := newActionFixture(t, false)
			worker, err := NewWorkerLoop(f.loop.GetInvoker(), reactloops.WithFunctionCallMode(native))
			require.NoError(t, err)
			f.loop = worker
			worker.SetCurrentTask(f.task)
			worker.Set("coordinator_worker_attempt", workerAttemptRef{TaskID: "a", AttemptID: 2})
			f.invoke("submit_task_result", map[string]any{"summary": " "}, true)
			require.Nil(t, worker.GetVariable("coordinator_task_result"))
			args := map[string]any{"summary": "confirmed source", "artifacts": []string{"report.md"}, "evidence_ids": []string{"source.1"}}
			f.invoke("submit_task_result", args, false)
			f.invoke("submit_task_result", map[string]any{"summary": " "}, true)
			require.Equal(t, Result{Summary: "confirmed source", Artifacts: []string{"report.md"}, EvidenceIDs: []string{"source.1"}}, worker.GetVariable("coordinator_task_result"))
			worker.Set("coordinator_worker_attempt", workerAttemptRef{TaskID: "b", AttemptID: 3})
			f.invoke("submit_task_result", map[string]any{"summary": "second result"}, false)
			f.cfg.Timeline.FreezeAll()
			semi := aicommon.BuildPromptFrozenOpenMaterials(f.cfg).SessionEvidenceSemiDynamic
			require.NotContains(t, semi, "confirmed source", "only Controller settlement publishes the canonical result")
			require.NotContains(t, semi, "second result")
			require.Contains(t, semi, `"action":"submit_task_result"`)
		})
	}
}
