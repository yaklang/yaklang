package sessionruntime

import (
	"context"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestStartParamsClarificationSettings(t *testing.T) {
	params := &ypb.AIStartParams{AllowPlanUserInteract: true, PlanUserInteractMaxCount: 4, UserInteractLimit: 2}
	c := aicommon.NewConfig(context.Background(), ConvertStartParamsToReActConfig(params)...)
	require.True(t, c.AllowPlanUserInteract)
	require.EqualValues(t, 4, c.PlanUserInteractMaxCount)
	require.EqualValues(t, 2, c.GetUserInteractiveLimitedTimes())
	copy := aicommon.NewConfig(context.Background(), aicommon.ConvertConfigToOptions(c)...)
	require.True(t, copy.AllowPlanUserInteract)
	require.EqualValues(t, 4, copy.PlanUserInteractMaxCount)
	require.EqualValues(t, 2, copy.GetUserInteractiveLimitedTimes())
}
