package cybertunnel

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsBridgeRetryableError(t *testing.T) {
	require.True(t, isBridgeRetryableError(errors.New("context deadline exceeded")))
	require.True(t, isBridgeRetryableError(errors.New("received context error while waiting for new LB policy update")))
	require.False(t, isBridgeRetryableError(errors.New("connection refused")))
}

func TestWrapRandomPortBridgeError(t *testing.T) {
	err := wrapRandomPortBridgeError("ns1.example.com:64333", context.DeadlineExceeded)
	require.Error(t, err)
	require.Contains(t, err.Error(), "random port allocation timed out or unavailable")
	require.Contains(t, err.Error(), "DNSLog on the same bridge may still work")
	require.Contains(t, err.Error(), "ns1.example.com:64333")
}
