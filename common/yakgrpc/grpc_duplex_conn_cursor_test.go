package yakgrpc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDatabaseTableWatchCursorDetectsFirstInsertAfterEmptyBaseline(t *testing.T) {
	cursor := &databaseTableWatchCursor{}

	previous, changed := cursor.advance(0)
	require.Zero(t, previous)
	require.False(t, changed, "the initial snapshot is a baseline, not a change")

	previous, changed = cursor.advance(1)
	require.Zero(t, previous)
	require.True(t, changed, "0 -> 1 must wake the frontend")

	previous, changed = cursor.advance(1)
	require.Equal(t, int64(1), previous)
	require.False(t, changed)

	previous, changed = cursor.advance(0)
	require.Equal(t, int64(1), previous)
	require.True(t, changed, "table reset must also be observable")
}
