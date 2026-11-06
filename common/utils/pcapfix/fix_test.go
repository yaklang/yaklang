package pcapfix

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFixWithContextCancelledBeforeAuthorization(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, FixWithContext(ctx), context.Canceled)
	require.Error(t, FixWithContext(nil))
}
