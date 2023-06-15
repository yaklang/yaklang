package yakvm

func (v *Frame) push(i *Value) {
	v.stack.Push(i)
}

func (v *Frame) peek() *Value {
	return v.stack.Peek().(*Value)
}

func (v *Frame) peekN(n int) *Value {
	return v.stack.PeekN(n).(*Value)
}

func (v *Frame) peekNextCode() *Code {
	if v.codePointer+1 >= len(v.codes) {
		return nil
	}
	return v.codes[v.codePointer+1]
}

func (v *Frame) pop() *Value {
	return v.stack.Pop().(*Value)
}

func (v *Frame) popN(n int) []*Value {
	var args = make([]*Value, n)
	for i := 0; i < n; i++ {
		args[i] = v.pop()
	}
	return args
}

func (v *Frame) popReverseN(n int) []*Value {
	var args = make([]*Value, n)
	for i := 0; i < n; i++ {
		args[n-1-i] = v.pop()
	}
	return args
}

func (v *Frame) popArgN(n int) []*Value {
	if n <= 0 {
		return nil
	}
	return v.popReverseN(n)
}
