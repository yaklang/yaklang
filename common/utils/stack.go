package utils

type (
	Stack[T any] struct {
		top    *node[T]
		length int
		last   *node[T]
	}
	node[T any] struct {
		value T
		prev  *node[T]
	}
)

// Create a new stack
func NewStack[T any]() *Stack[T] {
	return &Stack[T]{nil, 0, nil}
}

// Return the number of items in the stack
func (this *Stack[T]) Len() int {
	return this.length
}

func (this *Stack[T]) Values(sizes ...int) []T {
	size := this.Len()
	if len(sizes) > 0 {
		size = sizes[0]
	}
	ret := make([]T, 0, size)
	for i := 0; i < size; i++ {
		ret = append(ret, this.PeekN(i))
	}
	return ret
}
func (this *Stack[T]) Free() {
	this.top = nil
	this.last = nil
	this.length = 0
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
	if p == nil {
		var zero T
		return zero
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
	this.last = n
	return n.value
}

func (this *Stack[T]) HaveLastStackValue() bool {
	return this.last != nil
}

func (this *Stack[T]) LastStackValue() T {
	if this.last != nil {
		return this.last.value
	}
	var z T
	return z
}

// PopN the top item of the stack and return it
func (this *Stack[T]) PopN(n int) []T {
	if this.length == 0 {
		return nil
	}

	var ret []T
	for i := 0; i < n; i++ {
		ret = append(ret, this.Pop())
	}

	return ret
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
func (this *Stack[T]) ForeachStack(f func(T) bool) {
	for i := 0; i < this.length; i++ {
		if !f(this.PeekN(i)) {
			break
		}
		continue
	}
}
