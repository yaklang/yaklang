package minimartian

import (
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsExpectedDownstreamWriteError(t *testing.T) {
	require.True(t, isExpectedDownstreamWriteError(io.EOF))
	require.True(t, isExpectedDownstreamWriteError(errors.New("wsasend: An established connection was aborted by the software in your host machine.")))
	require.True(t, isExpectedDownstreamWriteError(errors.New("write: broken pipe")))
	require.False(t, isExpectedDownstreamWriteError(errors.New("response serialization failed")))
}
