package aiforge

import (
	"context"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/coordinator_legacy"
	"testing"
)

func TestPlanMocker(t *testing.T) {
	token := uuid.NewString()

	forge := NewForgeBlueprint("test-plan-mocker",
		WithPlanMocker(func(config *coordinator_legacy.Coordinator) *coordinator_legacy.PlanResponse {
			return &coordinator_legacy.PlanResponse{
				RootTask: &coordinator_legacy.AiTask{
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
