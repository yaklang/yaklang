package reactloops

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	aicommonmock "github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/schema"
)

func TestExecuteDeepIntentRecognition_SkipsIntentTaskLifecycleEvents(t *testing.T) {
	var events []*schema.AiOutputEvent
	baseInvoker := aicommonmock.NewMockInvoker(context.Background())
	cfg := baseInvoker.GetConfig().(*aicommonmock.MockedAIConfig)
	cfg.Emitter = aicommon.NewEmitter("deep-intent-lifecycle", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		events = append(events, e)
		return e, nil
	})

	invoker := &deepIntentLifecycleInvoker{MockInvoker: baseInvoker}
	loop := NewMinimalReActLoop(cfg, invoker)
	task := aicommon.NewStatefulTaskBase("main-task", "main user input", cfg.GetContext(), cfg.GetEmitter(), true)
	loop.SetCurrentTask(task)

	result := ExecuteDeepIntentRecognition(invoker, loop, task)

	require.NotNil(t, result)
	require.Equal(t, "intent analysis", result.IntentAnalysis)
	require.NotNil(t, invoker.intentTask)
	require.Equal(t, "main-task_intent", invoker.intentTask.GetId())
	require.NotContains(t, deepIntentLifecycleNodeIDs(events), "react_task_created")
	require.NotContains(t, deepIntentLifecycleNodeIDs(events), "react_task_status_changed")
}

type deepIntentLifecycleInvoker struct {
	*aicommonmock.MockInvoker

	intentTask aicommon.AIStatefulTask
}

func (d *deepIntentLifecycleInvoker) ExecuteLoopTaskIF(taskTypeName string, task aicommon.AIStatefulTask, options ...any) (bool, error) {
	d.intentTask = task
	task.SetStatus(aicommon.AITaskState_Processing)
	task.SetStatus(aicommon.AITaskState_Completed)

	intentLoop := NewMinimalReActLoop(d.GetConfig(), d)
	intentLoop.SetCurrentTask(task)
	for _, option := range options {
		if loopOption, ok := option.(ReActLoopOption); ok {
			loopOption(intentLoop)
		}
	}
	if intentLoop.onLoopInstanceCreated != nil {
		intentLoop.onLoopInstanceCreated(intentLoop)
	}

	intentLoop.Set("intent_analysis", "intent analysis")
	return true, nil
}

func deepIntentLifecycleNodeIDs(events []*schema.AiOutputEvent) []string {
	nodeIDs := make([]string, 0, len(events))
	for _, event := range events {
		nodeIDs = append(nodeIDs, event.NodeId)
	}
	return nodeIDs
}
