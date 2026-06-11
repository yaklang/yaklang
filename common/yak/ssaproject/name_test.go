package ssaproject

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBaseProjectNameFromProgramName(t *testing.T) {
	require.Equal(t, "111111", BaseProjectNameFromProgramName("111111(2026-06-11 16:43:50)"))
	require.Equal(t, "demo", BaseProjectNameFromProgramName("demo(2026-06-11 16:43:50)"))
	require.Equal(t, "plain-name", BaseProjectNameFromProgramName("plain-name"))
	require.Equal(t, "not-a-timestamp(x)", BaseProjectNameFromProgramName("not-a-timestamp(x)"))
	require.Equal(t, "", BaseProjectNameFromProgramName(""))
}
