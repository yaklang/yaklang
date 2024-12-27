package suspect

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStrSuspectFunc(t *testing.T) {
	require.True(t, isAlpha("abc"))
	require.False(t, isAlpha("abc123"))

	require.True(t, isAlphaNum("abc123"))
	require.False(t, isAlphaNum("abc123!"))

	require.True(t, isDigit("123"))
	require.False(t, isDigit("abc123"))
}

func TestIsBase64Password(t *testing.T) {
	require.True(t, IsBase64Password("cXdlcg=="))
	require.False(t, IsBase64Password("e8eaafd604440b7dea70188472c2e5b8"))
}
