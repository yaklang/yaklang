package utils

import (
	"container/heap"
)

type IntHeap struct {
	intHeap intHeap
}

func (h *IntHeap) Push(x interface{}) {
	heap.Push(&h.intHeap, x)
}

func (h *IntHeap) Pop() interface{} {
	return heap.Pop(&h.intHeap)
}

func (h *IntHeap) Len() int { return len(h.intHeap) }

func (h *IntHeap) Index(i int) int {
	return h.intHeap[i]
}

type intHeap []int

func (h intHeap) Len() int           { return len(h) }
func (h intHeap) Less(i, j int) bool { return h[i] < h[j] }
func (h intHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *intHeap) Push(x interface{}) {
	// Push and Pop use pointer receivers because they modify the slice's length,
	// not just its contents.
	*h = append(*h, x.(int))
}

func (h *intHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

func NewIntHeap(init ...[]int) *IntHeap {
	var h IntHeap
	if len(init) > 0 {
		h = IntHeap{intHeap(init[0])}
	} else {
		h = IntHeap{intHeap{}}
	}

	heap.Init(&h.intHeap)
	return &h
}
