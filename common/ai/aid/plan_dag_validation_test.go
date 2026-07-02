package aid

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
)

func TestPlanDAGValidationRepairsSelfDependencyWithAI(t *testing.T) {
	validPlanJSON := `{
		"@action": "plan",
		"main_task": "Repair DAG",
		"main_task_goal": "Repair invalid dependency graph",
		"tasks": [
			{"subtask_name": "Collect input", "subtask_goal": "Collect input"},
			{"subtask_name": "Summarize result", "subtask_goal": "Summarize result", "depends_on": ["Collect input"]}
		]
	}`

	var repairPrompt string
	coordinator, err := NewCoordinatorContext(
		context.Background(),
		"repair invalid DAG",
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {}),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			repairPrompt = request.GetPrompt()
			response := config.NewAIResponse()
			response.EmitOutputStream(strings.NewReader(validPlanJSON))
			response.Close()
			return response, nil
		}),
	)
	require.NoError(t, err)

	root := newGraphTask(coordinator, "Root")
	selfDependent := newGraphTask(coordinator, "Self Dependent")
	selfDependent.DependsOn = []string{"Self Dependent"}
	selfDependent.ParentTask = root
	root.Subtasks = []*AiTask{selfDependent}
	coordinator.standardizeTaskTree(root)

	planReq, err := coordinator.createPlanRequest("repair invalid DAG")
	require.NoError(t, err)

	repaired, err := planReq.ensurePlanExecutableDAG(coordinator.newPlanResponse(root))
	require.NoError(t, err)
	require.NotNil(t, repaired)
	require.NotNil(t, repaired.RootTask)
	require.Len(t, repaired.RootTask.Subtasks, 2)

	require.Contains(t, repairPrompt, "task executable DAG contains self dependency")
	require.Contains(t, repairPrompt, selfDependent.Index)
	require.Contains(t, coordinator.Timeline.Dump(), "plan_dag_validation_failed")

	_, err = buildStrictExecutableTaskGraph(repaired.RootTask)
	require.NoError(t, err)
}
