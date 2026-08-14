package hnsw

// Lessable is an interface that allows a type to be compared to another of the same type.
// It is used to define the order of elements in the heap.
type Lessable[T any] interface {
	Less(T) bool
}

// innerHeap is a type that represents the heap data structure.
// it implements the std heap interface.
type innerHeap[T Lessable[T]] struct {
	data []T
}

func (h *innerHeap[T]) Len() int {
	return len(h.data)
}

func (h *innerHeap[T]) Less(i, j int) bool {
	return h.data[i].Less(h.data[j])
}

func (h *innerHeap[T]) Swap(i, j int) {
	h.data[i], h.data[j] = h.data[j], h.data[i]
}

// Heap represents the heap data structure using a flat array to store the elements.
// Its generic implementation avoids the interface boxing allocation incurred by
// container/heap on every Push and Pop in the HNSW search hot path.
type Heap[T Lessable[T]] struct {
	inner *innerHeap[T]
}

func NewHeap[T Lessable[T]]() *Heap[T] {
	return &Heap[T]{
		inner: &innerHeap[T]{data: make([]T, 0)},
	}
}

// Init establishes the heap invariants required by the other routines in this package.
// Init is idempotent with respect to the heap invariants
// and may be called whenever the heap invariants may have been invalidated.
// The complexity is O(n) where n = h.Len().
func (h *Heap[T]) Init(d []T) {
	h.inner.data = d
	for i := len(d)/2 - 1; i >= 0; i-- {
		h.siftDown(i)
	}
}

// Len returns the number of elements in the heap.
func (h *Heap[T]) Len() int {
	return h.inner.Len()
}

// Push pushes the element x onto the heap.
// The complexity is O(log n) where n = h.Len().
func (h *Heap[T]) Push(x T) {
	h.inner.data = append(h.inner.data, x)
	h.siftUp(len(h.inner.data) - 1)
}

// Pop removes and returns the minimum element (according to Less) from the heap.
// The complexity is O(log n) where n = h.Len().
// Pop is equivalent to Remove(h, 0).
func (h *Heap[T]) Pop() T {
	return h.Remove(0)
}

func (h *Heap[T]) PopLast() T {
	return h.Remove(h.Len() - 1)
}

// Remove removes and returns the element at index i from the heap.
// The complexity is O(log n) where n = h.Len().
func (h *Heap[T]) Remove(i int) T {
	n := len(h.inner.data)
	removed := h.inner.data[i]
	last := h.inner.data[n-1]
	var zero T
	h.inner.data[n-1] = zero
	h.inner.data = h.inner.data[:n-1]
	if i == n-1 {
		return removed
	}
	h.inner.data[i] = last
	if i > 0 && h.inner.data[i].Less(h.inner.data[(i-1)/2]) {
		h.siftUp(i)
	} else {
		h.siftDown(i)
	}
	return removed
}

// Min returns the minimum element in the heap.
func (h *Heap[T]) Min() T {
	return h.inner.data[0]
}

// Max returns the maximum element in the heap.
func (h *Heap[T]) Max() T {
	max := h.inner.data[0]
	for i := 1; i < len(h.inner.data); i++ {
		if max.Less(h.inner.data[i]) {
			max = h.inner.data[i]
		}
	}
	return max
}

func (h *Heap[T]) Slice() []T {
	return h.inner.data
}

func (h *Heap[T]) siftUp(i int) {
	for i > 0 {
		parent := (i - 1) / 2
		if !h.inner.data[i].Less(h.inner.data[parent]) {
			return
		}
		h.inner.Swap(i, parent)
		i = parent
	}
}

func (h *Heap[T]) siftDown(i int) {
	n := len(h.inner.data)
	for {
		left := i*2 + 1
		if left >= n {
			return
		}
		smallest := left
		right := left + 1
		if right < n && h.inner.data[right].Less(h.inner.data[left]) {
			smallest = right
		}
		if !h.inner.data[smallest].Less(h.inner.data[i]) {
			return
		}
		h.inner.Swap(i, smallest)
		i = smallest
	}
}
