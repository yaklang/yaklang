package reactloops

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

type feedbackPreparationProbeTask struct {
	aicommon.AIStatefulTask
	reads int
}

func (t *feedbackPreparationProbeTask) GetId() string        { t.reads++; return "probe" }
func (t *feedbackPreparationProbeTask) GetUserInput() string { t.reads++; return "probe input" }

func TestValueFeedbackSingleModelSkipsRecordPreparation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := aicommon.NewConfig(ctx, aicommon.WithSingleAIModelMode(true), aicommon.WithDisableAutoSkills(true),
		aicommon.WithFastAICallback(func(aicommon.AICallerConfigIf, *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			t.Error("disabled value feedback must not call AI")
			return nil, nil
		}))
	loop := NewMinimalReActLoop(cfg, nil)
	task := &feedbackPreparationProbeTask{
		AIStatefulTask: aicommon.NewStatefulTaskBase("probe", "probe input", ctx, cfg.GetEmitter(), true),
	}
	loop.SetCurrentTask(task)
	loop.submitValueFeedbackWithTrigger(aicommon.ValueFeedbackTriggerLoopEnd, task, 1)
	loop.SubmitRiskFeedback([]string{"risk-1"}, "test", "low")
	require.Zero(t, task.reads, "disabled AIVE must exit before collecting task data")
}
