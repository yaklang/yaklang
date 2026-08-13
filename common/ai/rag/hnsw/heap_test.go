package hnsw

import (
	"github.com/stretchr/testify/require"
	"math/rand"
	"slices"
	"testing"
)

type Int int

func (i Int) Less(j Int) bool {
	return i < j
}

func TestHeap(t *testing.T) {
	h := NewHeap[Int]()

	for i := 0; i < 20; i++ {
		h.Push(Int(rand.Int() % 100))
	}

	require.Equal(t, 20, h.Len())

	var inOrder []Int
	for h.Len() > 0 {
		inOrder = append(inOrder, h.Pop())
	}

	if !slices.IsSorted(inOrder) {
		t.Errorf("Heap did not return sorted elements: %+v", inOrder)
	}
}

func TestHeapMax(t *testing.T) {
	h := NewHeap[Int]()
	for _, value := range []Int{1, 100, 2, 3, 4, 5, 6} {
		h.Push(value)
	}
	require.Equal(t, Int(100), h.Max())
}
