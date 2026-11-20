package aimedia

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestImageCatalogRetainsResourcesWithoutPromptState(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	catalog, err := OpenCatalog(root)
	require.NoError(t, err)
	require.NoError(t, catalog.BindOwner("session-a"))
	img, err := FromBytes(ctx, testPNG(t), "image/png")
	require.NoError(t, err)
	first, err := catalog.Register(ctx, "tool-1", img)
	require.NoError(t, err)
	second, err := catalog.Register(ctx, "tool-2", img)
	require.NoError(t, err)
	require.NotEqual(t, first.ID, second.ID)
	again, err := catalog.Register(ctx, "tool-1", img)
	require.NoError(t, err)
	require.Equal(t, first.ID, again.ID)
	require.Len(t, catalog.Snapshot(), 2)
	restored, err := OpenCatalog(root)
	require.NoError(t, err)
	require.Equal(t, "session-a", restored.Owner())
	require.Error(t, restored.BindOwner("other-session"))
	ref, err := restored.Find(first.ID)
	require.NoError(t, err)
	raw, err := ref.Image.Bytes(ctx)
	require.NoError(t, err)
	require.Equal(t, testPNG(t), raw)
	_, err = restored.Find("unrelated-image")
	require.Error(t, err)
	index, err := os.ReadFile(filepath.Join(root, "index.json"))
	require.NoError(t, err)
	for _, field := range []string{"active", "pinned", "summary", "calibrations", "rejected"} {
		require.NotContains(t, string(index), `"`+field+`"`)
	}
	fork, err := restored.Fork(ctx, filepath.Join(t.TempDir(), "child"))
	require.NoError(t, err)
	require.NoError(t, fork.BindOwner("session-a"))
	_, err = fork.Register(ctx, "child-image", img)
	require.NoError(t, err)
	require.Len(t, restored.Snapshot(), 2)
	require.Len(t, fork.Snapshot(), 3)
	require.NoError(t, os.Remove(first.Image.Path))
	_, err = fork.Snapshot()[0].Image.Bytes(ctx)
	require.NoError(t, err, "child retains its own resource reference")
}

func TestImageCatalogConcurrentRegistrationAndSnapshot(t *testing.T) {
	ctx := context.Background()
	catalog, err := OpenCatalog(t.TempDir())
	require.NoError(t, err)
	img, err := FromBytes(ctx, testPNG(t), "image/png")
	require.NoError(t, err)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, regErr := catalog.Register(ctx, fmt.Sprintf("tool-%d", i), img)
			require.NoError(t, regErr)
			_ = catalog.Snapshot()
		}()
	}
	wg.Wait()
	require.Len(t, catalog.Snapshot(), 8)
}

func TestImageCatalogIgnoresLegacyVisibilityAndSummaries(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	catalog, err := OpenCatalog(root)
	require.NoError(t, err)
	img, err := FromBytes(ctx, testPNG(t), "image/png")
	require.NoError(t, err)
	ref, err := catalog.Register(ctx, "legacy", img)
	require.NoError(t, err)
	legacy, err := json.Marshal(map[string]any{"items": []any{map[string]any{"id": ref.ID, "image": ref.Image, "active": true, "pinned": true, "summary": "old prototype summary"}}, "calibrations": map[string]int{"model": 10}})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, "index.json"), legacy, 0o600))
	restored, err := OpenCatalog(root)
	require.NoError(t, err)
	require.Len(t, restored.Snapshot(), 1)
	serialized, err := json.Marshal(restored.Snapshot())
	require.NoError(t, err)
	require.NotContains(t, string(serialized), "summary")
}

func TestImageDerivationKeepsOriginalCoordinates(t *testing.T) {
	ctx := context.Background()
	img, err := FromBytes(ctx, testPNG(t), "image/png")
	require.NoError(t, err)
	view, err := Derive(ctx, img, image.Rect(1, 1, 5, 4), 64)
	require.NoError(t, err)
	x, y, err := view.OriginalPoint(2, 1)
	require.NoError(t, err)
	require.Equal(t, float64(3), x)
	require.Equal(t, float64(2), y)
	_, err = Derive(ctx, img, image.Rect(-1, 0, 1, 1), 64)
	require.Error(t, err)
}

func TestImageTokensUsePixelsNotCompressedBytes(t *testing.T) {
	require.Equal(t, int64(256), EstimateImageTokens(512, 512, "patch32"))
	require.Equal(t, int64(255), EstimateImageTokens(512, 512, "tile512"))
	require.Greater(t, EstimateImageTokens(2048, 1024, "conservative"), EstimateImageTokens(512, 256, "conservative"))
}
