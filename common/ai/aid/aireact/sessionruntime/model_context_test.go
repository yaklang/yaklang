package sessionruntime

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestModelContextContractRejectsInvalidSessionStart(t *testing.T) {
	runtime := New(nil).(*reActSessionRuntime)
	// The actual runtime entry must reject the request before creating a ReAct,
	// saving session metadata, or requiring provider configuration.
	for _, tokens := range []int64{-1, 1, 60 * 1024, 100*1024 - 1} {
		result, err := runtime.createRuntime(&reActSessionStart{ctx: context.Background()}, nil, "invalid-context-start",
			&ypb.AIStartParams{RuntimeContextSizeLimit: tokens}, false)
		require.Nil(t, result)
		require.ErrorContains(t, err, "100K")
	}
}
