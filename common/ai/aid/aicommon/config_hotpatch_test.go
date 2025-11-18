package aicommon

import (
	"context"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
	"time"
)

func TestHotPatchConfig(t *testing.T) {
	// Setup config with epm stub
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c := NewTestConfig(ctx)
	c.StartEventLoop(ctx)

	require.True(t, c.AllowRequireForUserInteract)
	require.Equal(t, c.AgreePolicy, AgreePolicyManual)

	c.EventInputChan.SafeFeed(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     HotPatchType_AllowRequireForUserInteract,
		Params: &ypb.AIStartParams{
			DisallowRequireForUserPrompt: true,
		},
	})
	time.Sleep(1 * time.Second)
	require.False(t, c.AllowRequireForUserInteract)

	c.EventInputChan.SafeFeed(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     HotPatchType_AgreePolicy,
		Params: &ypb.AIStartParams{
			ReviewPolicy: string(AgreePolicyYOLO),
		},
	})
	time.Sleep(1 * time.Second)
	require.Equal(t, AgreePolicyYOLO, c.AgreePolicy)
}
