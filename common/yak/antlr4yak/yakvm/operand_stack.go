package yakvm

// operandStack belongs to one frame. Other VM stacks keep their existing
// linked representation and shadow-stack contract.
type operandStack struct{ values []*Value }

func (s *operandStack) Len() int          { return len(s.values) }
func (s *operandStack) Push(value *Value) { s.values = append(s.values, value) }
func (s *operandStack) Peek() *Value      { return s.PeekN(0) }
func (s *operandStack) PeekN(n int) *Value {
	if n < 0 || n >= len(s.values) {
		return nil
	}
	return s.values[len(s.values)-1-n]
}
func (s *operandStack) Pop() *Value {
	n := len(s.values)
	if n == 0 {
		panic("operand stack underflow")
	}
	v := s.values[n-1]
	s.values[n-1] = nil
	s.values = s.values[:n-1]
	return v
}
func (s *operandStack) Clear() {
	clear(s.values)
	if cap(s.values) > 4096 {
		s.values = nil
	} else {
		s.values = s.values[:0]
	}
}

// Pop2 returns operands in evaluation order: left first, right second.
func (v *Frame) pop2() (left, right *Value) {
	right = v.pop()
	left = v.pop()
	return
}
