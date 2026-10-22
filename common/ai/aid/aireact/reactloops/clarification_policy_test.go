package reactloops

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func TestSubAgentClarificationPolicySurvivesGlobalHotpatch(t *testing.T) {
	cfg := aicommon.NewConfig(context.Background(), buildSubAgentStrategyOptions()...)
	require.False(t, cfg.GetAllowUserInteraction())
	require.NoError(t, aicommon.WithAllowRequireForUserInteract(true)(cfg))
	require.ErrorContains(t, cfg.CheckClarificationAllowed("child", false), "disabled")
	require.ErrorContains(t, cfg.CheckClarificationAllowed("child", true), "disabled")
}
