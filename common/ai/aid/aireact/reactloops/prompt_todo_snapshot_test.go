package reactloops

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWithDisableTodoSnapshot(t *testing.T) {
	t.Run("default renders todo snapshot", func(t *testing.T) {
		loop := &ReActLoop{}
		require.True(t, loop.shouldRenderTodoSnapshot())
	})

	t.Run("opt-out skips todo snapshot", func(t *testing.T) {
		loop := &ReActLoop{}
		WithDisableTodoSnapshot(true)(loop)
		require.False(t, loop.shouldRenderTodoSnapshot())
	})

	t.Run("explicit false keeps todo snapshot enabled", func(t *testing.T) {
		loop := &ReActLoop{}
		WithDisableTodoSnapshot(false)(loop)
		require.True(t, loop.shouldRenderTodoSnapshot())
	})
}
