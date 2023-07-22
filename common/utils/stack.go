package utils

type (
	Stack struct {
		top    *node
		length int
	}
	node struct {
		value interface{}
		prev  *node
	}
)

// Create a new stack
func NewStack() *Stack {
	return &Stack{nil, 0}
}

// Return the number of items in the stack
func (this *Stack) Len() int {
	return this.length
}
func (this *Stack) Size() int {
	return this.length
}
func (this *Stack) IsEmpty() bool {
	return this.length <= 0
}

// View the top item on the stack
func (this *Stack) Peek() interface{} {
	if this.length == 0 {
		return nil
	}
	return this.top.value
}

// View the top n item on the stack
func (this *Stack) PeekN(n int) interface{} {
	if this.length == 0 {
		return nil
	}

	p := this.top
	for i := 1; i <= n; i++ {
		p = p.prev
	}

	return p.value
}

// Pop the top item of the stack and return it
func (this *Stack) Pop() interface{} {
	if this.length == 0 {
		return nil
	}

	n := this.top
	this.top = n.prev
	this.length--
	return n.value
}

// Push a value onto the top of the stack
func (this *Stack) Push(value interface{}) {
	n := &node{value, this.top}
	this.top = n
	this.length++
}

// CreateShadowStack creates a shadow stack, which can be used to restore the stack to its current state.
// dont pop the top item of the stack.
func (this *Stack) CreateShadowStack() func() {
	top := this.top
	return func() {
		this.top = top
	}
}
