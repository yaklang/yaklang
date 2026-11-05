package entityrepos

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestYieldKHopExDrainsResultsBeforeInternalCancellation(t *testing.T) {
	previous := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(previous)
	db := setupTestDB(t)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	repo := createMockData(t, db)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	starts := make(chan string, 1)
	starts <- "entity-a"
	close(starts)
	results := repo.YieldKHopEx(ctx, starts, WithKHopK(2))
	// Hold the reader while the finite producer finishes. Internal pipeline
	// cancellation cannot turn its already queued results into partial output.
	time.Sleep(50 * time.Millisecond)
	var got []string
	for path := range results {
		require.Equal(t, 2, path.K)
		h := path.Hops
		require.NotNil(t, h)
		require.NotNil(t, h.Next)
		require.Nil(t, h.Next.Next)
		got = append(got, h.Src.Uuid+"/"+h.Dst.Uuid+"/"+h.Next.Dst.Uuid)
	}
	require.NoError(t, ctx.Err(), "the caller did not cancel the observation")
	require.ElementsMatch(t, []string{
		"entity-a/entity-b/entity-c",
		"entity-a/entity-b/entity-f",
		"entity-a/entity-e/entity-c",
	}, got)
}
