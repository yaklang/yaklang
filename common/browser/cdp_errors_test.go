package browser

import (
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsBrokenCDPError(t *testing.T) {
	require.True(t, isBrokenCDPError(errors.New("write tcp 127.0.0.1:60224->127.0.0.1:60221: use of closed network connection")))
	require.True(t, isBrokenCDPError(errors.New("wait page load: EOF")))
	require.True(t, isBrokenCDPError(io.EOF))
	require.False(t, isBrokenCDPError(errors.New("element not found")))
	require.False(t, isBrokenCDPError(nil))
}
