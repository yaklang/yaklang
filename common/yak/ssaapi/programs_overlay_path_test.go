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

func TestOverlayRebuildFilePartitions(t *testing.T) {
	t.Parallel()

	overlay := newEmptyOverlay()
	overlay.FileToLayerMap.Set("/unchanged.java", 1)
	overlay.FileToLayerMap.Set("/changed.java", 2)
	overlay.rebuildFilePartitions()

	require.True(t, overlay.IsBaseOnlyFile("/unchanged.java"))
	require.True(t, overlay.IsBaseOnlyFile("unchanged.java")) // ensureOverlayPathSlash
	require.False(t, overlay.IsBaseOnlyFile("/changed.java"))

	overridden := overlay.overriddenFilesList()
	require.Contains(t, overridden, "/changed.java")
	require.NotContains(t, overridden, "/unchanged.java")
}
