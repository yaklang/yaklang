package aicommon

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestIsFastNoopContinueReview(t *testing.T) {
	now := time.Now()
	ep := NewEndpointManager().CreateEndpoint()

	ep.createdAtMs = now.Add(-100 * time.Millisecond).UnixMilli()
	require.True(t, isFastNoopContinueReview(ep, aitool.InvokeParams{
		"suggestion": "continue",
	}, now))

	require.False(t, isFastNoopContinueReview(ep, aitool.InvokeParams{
		"suggestion":   "continue",
		"extra_prompt": "change the command",
	}, now), "user-authored review input must remain in the timeline")

	ep.createdAtMs = now.Add(-501 * time.Millisecond).UnixMilli()
	require.False(t, isFastNoopContinueReview(ep, aitool.InvokeParams{
		"suggestion": "continue",
	}, now), "slow/manual reviews must remain in the timeline")

	ep.createdAtMs = now.Add(-100 * time.Millisecond).UnixMilli()
	require.False(t, isFastNoopContinueReview(ep, aitool.InvokeParams{
		"suggestion": "wrong_params",
	}, now), "non-continue decisions must remain in the timeline")
}
