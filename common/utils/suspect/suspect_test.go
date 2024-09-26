package suspect

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestStrSuspectFunc(t *testing.T) {
	require.True(t, isAlpha("abc"))
	require.False(t, isAlpha("abc123"))

	require.True(t, isAlphaNum("abc123"))
	require.False(t, isAlphaNum("abc123!"))

	require.True(t, isDigit("123"))
	require.False(t, isDigit("abc123"))
}
