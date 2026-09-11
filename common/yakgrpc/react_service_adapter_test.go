package yakgrpc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultServersShareProcessReActService(t *testing.T) {
	first := &Server{}
	second := &Server{}
	require.Same(t, first.getReActService(), second.getReActService())
	require.Equal(t, first.getReActSessionRuntime(), second.getReActSessionRuntime())
}
