package yakvm

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeferPreservesNativeCallCancellation(t *testing.T) {
	owner, cancel := context.WithCancel(context.Background())
	defer cancel()
	frame := &Frame{ctx: owner}
	exit := frame.enterDeferredExecution()
	defer exit()
	native := frame.NativeCallContext()
	require.NoError(t, native.Err())
	cancel()
	require.NoError(t, frame.GetContext().Err(), "Yak cleanup still has its grace period")
	require.ErrorIs(t, native.Err(), context.Canceled)
	require.ErrorIs(t, frame.NativeCallContext().Err(), context.Canceled)
}
