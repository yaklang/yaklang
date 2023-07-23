package utils

type (
	Stack[T any] struct {
		top    *node[T]
		length int
	}
	node[T any] struct {
		value T
		prev  *node[T]
	}
)

// Create a new stack
func NewStack[T any]() *Stack[T] {
	return &Stack[T]{nil, 0}
}

// Return the number of items in the stack
func (this *Stack[T]) Len() int {
	return this.length
}

func (this *Stack[T]) Size() int {
	return this.length
}

func (this *Stack[T]) IsEmpty() bool {
	return this.length <= 0
}

// View the top item on the stack
func (this *Stack[T]) Peek() T {
	if this.length == 0 {
		var zero T
		return zero
	}
	return this.top.value
}

// View the top n item on the stack
func (this *Stack[T]) PeekN(n int) T {
	if this.length == 0 {
		var zero T
		return zero
	}

	p := this.top
	for i := 1; i <= n; i++ {
		p = p.prev
	}

	return p.value
}

// Pop the top item of the stack and return it
func (this *Stack[T]) Pop() T {
	if this.length == 0 {
		var zero T
		return zero
	}

	n := this.top
	this.top = n.prev
	this.length--
	return n.value
}

// Push a value onto the top of the stack
func (this *Stack[T]) Push(value T) {
	n := &node[T]{value, this.top}
	this.top = n
	this.length++
}

// CreateShadowStack creates a shadow stack, which can be used to restore the stack to its current state.
// dont pop the top item of the stack.
func (this *Stack[T]) CreateShadowStack() func() {
	top := this.top
	return func() {
		this.top = top
	}
}
