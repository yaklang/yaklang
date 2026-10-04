package aiforge

import (
	"context"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/coordinator"
	"testing"
)

func TestPlanMocker(t *testing.T) {
	registerFormattingRuntime(t)
	token := uuid.NewString()

	forge := NewForgeBlueprint("test-plan-mocker",
		WithPlanMocker(func(config *coordinator.Session) *coordinator.PlanResponse {
			return &coordinator.PlanResponse{
				RootTask: &coordinator.PlanNode{
					Name: token,
				},
			}
		}),
		WithAIOptions(aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return nil, nil
		})),
	)

	coordinator, err := forge.CreateCoordinator(context.Background(), "")
	require.NoError(t, err)
	t.Cleanup(coordinator.Close)

	require.NotNil(t, forge.PlanMocker)

	planResp := forge.PlanMocker(coordinator.Session)
	require.NotNil(t, planResp)
	require.Equal(t, token, planResp.RootTask.Name)

}
