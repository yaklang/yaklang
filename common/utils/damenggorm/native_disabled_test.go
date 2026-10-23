//go:build !dameng

package damenggorm

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNativeDriverDisabledDoesNotFallBack(t *testing.T) {
	require.False(t, NativeEnabled())
	db, err := OpenDBContext(context.Background(), "dm://user:private-password@127.0.0.1:1?schema=SSA_IR")
	require.Nil(t, db)
	require.ErrorIs(t, err, ErrNativeUnavailable)
	require.NotContains(t, err.Error(), "private-password")
}
