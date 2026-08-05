package yakgrpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGetAIReActRecommendedSkills(t *testing.T) {
	response, err := (&Server{}).GetAIReActRecommendedSkills(context.Background(), &ypb.Empty{})
	require.NoError(t, err)
	require.Len(t, response.GetData(), 2)

	require.Equal(t, []string{
		"pentest-task-design",
		"code-review",
	}, []string{
		response.GetData()[0].GetName(),
		response.GetData()[1].GetName(),
	})
	for _, skill := range response.GetData() {
		require.Equal(t, aicommon.EnabledCapabilityTypeSkill, skill.GetType())
		require.NotEmpty(t, skill.GetDisplayNameZhCN())
		require.NotEmpty(t, skill.GetDescription())
	}

	// The response fields are deliberately shaped so the frontend can pass
	// them straight back in the StartAIReAct start event.
	params := &ypb.AIStartParams{}
	for _, skill := range response.GetData() {
		params.EnabledCapabilities = append(params.EnabledCapabilities, &ypb.AIEnabledCapability{
			Name: skill.GetName(), Type: skill.GetType(),
		})
	}
	config := aicommon.NewConfig(context.Background(), ConvertYPBAIStartParamsToReActConfig(params)...)
	require.Equal(t, []string{
		"pentest-task-design",
		"code-review",
	}, config.GetEnabledSkillNames())
}
