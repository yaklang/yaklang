package pipeline_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/pipeline"
)

func TestSlotPipeWaitsForReleasedSlotBeforeHandlingNextItem(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var started atomic.Int64
	p := pipeline.NewSlotPipe(ctx, 4, 1, func(item int) (int, error) {
		started.Add(1)
		return item, nil
	}, 4)
	p.FeedSlice([]int{1, 2, 3})

	first := <-p.Out()
	require.NotNil(t, first)

	time.Sleep(50 * time.Millisecond)
	require.Equal(t, int64(1), started.Load(), "next item should wait until the first slot is released")

	first.Release()
	second := <-p.Out()
	require.NotNil(t, second)
	second.Release()
	third := <-p.Out()
	require.NotNil(t, third)
	third.Release()

	var values []int
	values = append(values, first.Value, second.Value, third.Value)
	require.ElementsMatch(t, []int{1, 2, 3}, values)
}
