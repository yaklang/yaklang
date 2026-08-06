package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeOverlayFilePath(t *testing.T) {
	t.Parallel()

	require.Equal(t, "/Main.java", normalizeOverlayFilePath("/prog/Main.java", "prog"))
	require.Equal(t, "/src/Main.java", normalizeOverlayFilePath("/prog(2026-01-01)/src/Main.java", "prog"))
	require.Equal(t, "/Main.java", normalizeOverlayFilePath("Main.java", ""))
	require.Equal(t, "/Main.java", normalizeOverlayFilePath("/Main.java", ""))
	require.Equal(t, "/a/b.java", ensureOverlayPathSlash("a/b.java"))
	require.Equal(t, "/a/b.java", ensureOverlayPathSlash("/a/b.java"))
}

func TestOverlayAggregatedFSPathRoundTrip(t *testing.T) {
	t.Parallel()

	require.Equal(t, "Main.java", overlayAggregatedFSPath("/Main.java"))
	require.Equal(t, "src/Main.java", overlayAggregatedFSPath("/src/Main.java"))
	require.Equal(t, "/Main.java", overlayPathFromAggregatedFS("Main.java"))
	require.Equal(t, "/src/Main.java", overlayPathFromAggregatedFS("src/Main.java"))
}

func TestOverlayExcludeAndDiffOwnership(t *testing.T) {
	t.Parallel()

	overlay := newEmptyOverlay()
	overlay.Base = &Program{}
	overlay.Diff = []*ProgramLayer{
		{File: []string{"/changed.java"}},
	}
	overlay.setExcludeFile(map[string]struct{}{"/gone.java": {}})

	require.Contains(t, overlay.Diff[0].File, "/changed.java")
	require.True(t, overlay.IsExcludedPath("/changed.java"))
	require.True(t, overlay.IsExcludedPath("/gone.java"))
	require.False(t, overlay.IsExcludedPath("/unchanged.java"))
	require.Contains(t, overlay.ExcludeFile, "/changed.java")
	require.Contains(t, overlay.ExcludeFile, "/gone.java")
	owner, ok := overlay.ownerDiffIndex("/changed.java")
	require.True(t, ok)
	require.Equal(t, 0, owner)
	_, ownedGone := overlay.ownerDiffIndex("/gone.java")
	require.False(t, ownedGone)
}
