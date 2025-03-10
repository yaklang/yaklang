package yakvm

import (
	"container/list"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/yaklib/container"
)

func TestSetIterator(t *testing.T) {
	// set 无序
	set := container.NewSet("q", "w", "e")
	iter, err := NewIterator(context.Background(), set)
	require.NoError(t, err)
	result := make([][]any, 0, 3)
	for {
		data, end := iter.Next()
		if end {
			break
		}
		result = append(result, data)
	}
	require.Len(t, result, 3)

	// Since set is unordered, we can only check:
	// 1. Each result has 2 elements
	// 2. The second element is one of the expected values
	// 3. The first elements are 0, 1, 2 (indices)

	values := map[any]bool{"q": false, "w": false, "e": false}
	indices := map[any]bool{0: false, 1: false, 2: false}

	for _, item := range result {
		require.Len(t, item, 2)
		require.Contains(t, values, item[1])
		require.Contains(t, indices, item[0])
		values[item[1]] = true
		indices[item[0]] = true
	}

	// Ensure all expected values and indices were found
	for k, v := range values {
		require.True(t, v, "Value %v not found in results", k)
	}
	for k, v := range indices {
		require.True(t, v, "Index %v not found in results", k)
	}
}

func TestLinkedListIterator(t *testing.T) {
	l := list.New()
	l.PushBack("q")
	l.PushBack("w")
	l.PushBack("e")
	iter, err := NewIterator(context.Background(), l)
	require.NoError(t, err)
	result := make([][]any, 0, 3)
	for {
		data, end := iter.Next()
		if end {
			break
		}
		result = append(result, data)
	}
	require.Len(t, result, 3)
	require.ElementsMatch(t, []any{0, "q"}, result[0])
	require.ElementsMatch(t, []any{1, "w"}, result[1])
	require.ElementsMatch(t, []any{2, "e"}, result[2])
}
