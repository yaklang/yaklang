package yakgrpc

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestManualLargeRequestReplacementStore_MultipartPart(t *testing.T) {
	store := newManualLargeRequestReplacementStore()
	t.Cleanup(store.close)
	target, err := newManualLargeRequestReplacementTarget(false, 2)
	require.NoError(t, err)

	require.NoError(t, store.consume(false, 2, []byte("first-"), true, false, false))
	activePath := store.active[target].path
	require.FileExists(t, activePath)
	require.True(t, store.hasActive())
	require.False(t, store.hasCompleted())

	require.NoError(t, store.consume(false, 2, []byte("second"), false, true, false))
	require.False(t, store.hasActive())
	require.True(t, store.hasCompleted())
	completedPath := store.multipartPaths()[2]
	require.Equal(t, activePath, completedPath)
	content, err := os.ReadFile(completedPath)
	require.NoError(t, err)
	require.Equal(t, []byte("first-second"), content)

	// Starting another upload for the same part discards the prior completed
	// file and supports an empty replacement file in one message.
	require.NoError(t, store.consume(false, 2, nil, true, true, false))
	require.NoFileExists(t, completedPath)
	emptyPath := store.multipartPaths()[2]
	require.FileExists(t, emptyPath)
	info, err := os.Stat(emptyPath)
	require.NoError(t, err)
	require.Zero(t, info.Size())

	require.NoError(t, store.consume(false, 2, nil, false, false, true))
	require.False(t, store.hasCompleted())
	require.NoFileExists(t, emptyPath)
}

func TestManualLargeRequestReplacementStore_RequestBody(t *testing.T) {
	store := newManualLargeRequestReplacementStore()
	t.Cleanup(store.close)

	require.NoError(t, store.consume(true, 0, []byte("raw-body"), true, true, false))
	bodyPath := store.bodyPath()
	require.FileExists(t, bodyPath)
	content, err := os.ReadFile(bodyPath)
	require.NoError(t, err)
	require.Equal(t, []byte("raw-body"), content)
	require.Empty(t, store.multipartPaths())
}

func TestManualLargeRequestReplacementStoreRejectsChunkBeforeStart(t *testing.T) {
	store := newManualLargeRequestReplacementStore()
	t.Cleanup(store.close)

	err := store.consume(false, 0, []byte("orphan"), false, false, false)
	require.ErrorContains(t, err, "was not started")
}
