package aicommon

import (
	"encoding/json"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"testing"
)

func TestAIStatefulTaskBase_TaskSemanticAccessors(t *testing.T) {
	task := NewStatefulTaskBase("task-1", "input", nil, nil, true)

	task.SetTaskRetrievalInfo(&AITaskRetrievalInfo{
		Tags:      []string{"java", "rewrite"},
		Questions: []string{"哪些方法需要重写？"},
		Target:    "提升可读性",
	})

	info := task.GetTaskRetrievalInfo()
	if info == nil {
		t.Fatal("unexpected nil retrieval info")
	}
	if len(info.Tags) != 2 || info.Tags[0] != "java" || info.Tags[1] != "rewrite" {
		t.Fatalf("unexpected tags: %#v", info.Tags)
	}
	if len(info.Questions) != 1 || info.Questions[0] != "哪些方法需要重写？" {
		t.Fatalf("unexpected questions: %#v", info.Questions)
	}
	if info.Target != "提升可读性" {
		t.Fatalf("unexpected target: %#v", info.Target)
	}

	info.Tags[0] = "mutated"
	info.Questions[0] = "mutated"
	info.Target = "mutated"

	got := task.GetTaskRetrievalInfo()
	if got.Tags[0] != "java" {
		t.Fatalf("tags should be returned as a copy")
	}
	if got.Questions[0] != "哪些方法需要重写？" {
		t.Fatalf("questions should be returned as a copy")
	}
	if got.Target != "提升可读性" {
		t.Fatalf("target should be returned as a copy")
	}
}

func TestAIStatefulTaskBase_TaskCannotUpdateFinishStatus(t *testing.T) {
	task := NewStatefulTaskBase("task-1", "input", nil, NewEmitter(uuid.NewString(), func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		if e.Type == schema.EVENT_TYPE_STRUCTURED && e.NodeId == "react_task_status_changed" {
			var data map[string]interface{}
			if err := json.Unmarshal(e.Content, data); err != nil {
				return nil, err
			}
			require.NotEqual(t, AITaskState_Completed, data["react_task_now_status"].(string))
		}
		return nil, nil
	}), false)
	task.SetStatus(AITaskState_Aborted)
	require.Equal(t, AITaskState_Aborted, task.GetStatus())
	task.SetStatus(AITaskState_Completed)
	require.Equal(t, AITaskState_Aborted, task.GetStatus(), "status should not change to Finished after being Aborted")
}

func TestAIStatefulTaskBase_ForceSetStatusAllowsRecoveryFromFinishedState(t *testing.T) {
	task := NewStatefulTaskBase("task-1", "input", nil, nil, true)
	task.SetStatus(AITaskState_Aborted)
	require.Equal(t, AITaskState_Aborted, task.GetStatus())

	task.ForceSetStatus(AITaskState_Processing)
	require.Equal(t, AITaskState_Processing, task.GetStatus(), "force set should allow retrying a recovered aborted task")

	task.Finish(nil)
	require.Equal(t, AITaskState_Completed, task.GetStatus(), "task should be able to finish after being forced back to processing")
}
