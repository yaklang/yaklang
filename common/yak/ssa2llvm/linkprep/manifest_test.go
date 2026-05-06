package linkprep

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildManifestSeedTooShort(t *testing.T) {
	_, err := BuildManifest([]byte("short"))
	require.Error(t, err)
}

func TestBuildManifestStableAndUnique(t *testing.T) {
	seed := []byte("0123456789abcdef")
	m1, err := BuildManifest(seed)
	require.NoError(t, err)
	m2, err := BuildManifest(seed)
	require.NoError(t, err)
	require.Equal(t, m1, m2)

	syms := CanonicalRuntimeSymbols()
	require.Len(t, m1, len(syms))
	seen := make(map[string]struct{}, len(m1))
	for _, orig := range syms {
		newName, ok := m1[orig]
		require.True(t, ok, orig)
		require.True(t, strings.HasPrefix(newName, "rt_"))
		require.Len(t, newName, len("rt_")+16)
		seen[newName] = struct{}{}
	}
	require.Len(t, seen, len(syms), "renamed symbols must be unique")
}

func TestBuildManifestDiffersBySeed(t *testing.T) {
	a, err := BuildManifest([]byte("0123456789abcdef"))
	require.NoError(t, err)
	b, err := BuildManifest([]byte("fedcba9876543210"))
	require.NoError(t, err)
	require.NotEqual(t, a, b)
}
