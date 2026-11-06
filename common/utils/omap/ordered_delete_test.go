package omap

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOrderedMapDeleteKeepsOrderAtEdges(t *testing.T) {
	o := NewEmptyOrderedMap[string, int]()
	o.Set("a", 1)
	o.Set("b", 2)
	o.Set("c", 3)

	snapshot := o.Keys()
	o.Delete("c")
	o.Set("d", 4)
	require.Equal(t, []string{"a", "b", "c"}, snapshot, "a held Keys snapshot must not change on append")
	require.Equal(t, []string{"a", "b", "d"}, o.Keys())

	o.Delete("a")
	require.Equal(t, []string{"b", "d"}, o.Keys())

	o.Delete("d")
	require.Equal(t, []string{"b"}, o.Keys())
}
