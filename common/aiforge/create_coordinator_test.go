package aiforge

import (
	"context"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"testing"
)

func TestPlanMocker(t *testing.T) {
	token := uuid.NewString()

	forge := NewForgeBlueprint("test-plan-mocker",
		WithPlanMocker(func(config *aid.Coordinator) *aid.PlanResponse {
			return &aid.PlanResponse{
				RootTask: &aid.AiTask{
					Name: token,
				},
			}
		}),
		WithAIOptions(aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return nil, nil
		})),
	)

	coordinator, err := forge.CreateCoordinator(context.Background(), "")
	if err != nil {
		return
	}

	require.NotNil(t, coordinator.PlanMocker)

	planResp := coordinator.PlanMocker(coordinator)
	require.NotNil(t, planResp)
	require.Equal(t, token, planResp.RootTask.Name)

}
