package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
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

func TestOverlayGetScanFilePartition(t *testing.T) {
	t.Parallel()

	overlay := newEmptyOverlay()
	layer1 := &ProgramLayer{
		LayerIndex:  1,
		FileHashMap: utils.NewSafeMap[int](),
		FileSet:     utils.NewSafeMap[struct{}](),
	}
	layer2 := &ProgramLayer{
		LayerIndex:  2,
		FileHashMap: utils.NewSafeMap[int](),
		FileSet:     utils.NewSafeMap[struct{}](),
	}
	layer1.FileHashMap.Set("/Keep.java", 1)
	layer1.FileHashMap.Set("/A.java", 1)
	layer1.FileHashMap.Set("/Gone.java", 1)
	layer2.FileHashMap.Set("/A.java", 0)
	layer2.FileHashMap.Set("/Gone.java", -1)
	layer2.FileHashMap.Set("/New.java", 1)

	overlay.Layers = []*ProgramLayer{layer1, layer2}
	overlay.FileToLayerMap.Set("/Keep.java", 1)
	overlay.FileToLayerMap.Set("/A.java", 2)
	overlay.FileToLayerMap.Set("/New.java", 2)
	overlay.rebuildFilePartitions()

	part := overlay.GetScanFilePartition()
	require.Equal(t, 3, part.AggregatedCount) // Keep, A, New
	require.Contains(t, part.Overridden, "/A.java")
	require.Contains(t, part.Overridden, "/New.java")
	require.Contains(t, part.Unchanged, "/Keep.java")
	require.Contains(t, part.Deleted, "/Gone.java")
	require.False(t, overlay.IsPresentInAggregatedView("/Gone.java"))
	require.True(t, overlay.IsPresentInAggregatedView("/Keep.java"))
	owner, ok := overlay.GetFileOwnerLayer("/A.java")
	require.True(t, ok)
	require.Equal(t, 2, owner)
	require.Contains(t, overlay.PathsOwnedByLayer(2), "/A.java")
	require.Contains(t, overlay.PathsOwnedByLayer(1), "/Keep.java")
}
