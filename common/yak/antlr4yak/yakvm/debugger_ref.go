package yakvm

import "fmt"

type handlesMap struct {
	start       int
	nextHandle  int
	handleToVal map[int]interface{}
	ValToHandle map[interface{}]int
}

func newHandlesMap(start int) *handlesMap {
	return &handlesMap{start, start, make(map[int]interface{}), make(map[interface{}]int)}
}

func (hs *handlesMap) reset() {
	hs.nextHandle = hs.start
	hs.handleToVal = make(map[int]interface{})
}

func (hs *handlesMap) create(value interface{}) int {
	next := hs.nextHandle
	hs.nextHandle++
	hs.handleToVal[next] = value
	addr := fmt.Sprintf("%p", value)
	hs.ValToHandle[addr] = next
	return next
}

func (hs *handlesMap) get(handle int) (interface{}, bool) {
	v, ok := hs.handleToVal[handle]
	return v, ok
}

func (hs *handlesMap) getReverse(v interface{}) (int, bool) {
	addr := fmt.Sprintf("%p", v)
	i, ok := hs.ValToHandle[addr]
	return i, ok
}

type frameHandlesMap struct {
	m *handlesMap
}

func newFrameHandlesMap() *frameHandlesMap {
	return &frameHandlesMap{newHandlesMap(0)}
}

func (hs *frameHandlesMap) create(value *Frame) int {
	return hs.m.create(value)
}

func (hs *frameHandlesMap) get(handle int) (*Frame, bool) {
	v, ok := hs.m.get(handle)
	if !ok {
		return nil, false
	}
	return v.(*Frame), true
}

func (hs *frameHandlesMap) getReverse(v *Frame) (int, bool) {
	return hs.m.getReverse(v)
}

func (hs *frameHandlesMap) reset() {
	hs.m.reset()
}

type breakPointHandlesMap struct {
	m *handlesMap
}

func newBreakPointHandlesMap() *breakPointHandlesMap {
	return &breakPointHandlesMap{newHandlesMap(1)}
}

func (hs *breakPointHandlesMap) create(value *Breakpoint) int {
	return hs.m.create(value)
}

func (hs *breakPointHandlesMap) get(handle int) (*Breakpoint, bool) {
	v, ok := hs.m.get(handle)
	if !ok {
		return nil, false
	}
	return v.(*Breakpoint), true
}

func (hs *breakPointHandlesMap) getReverse(v *Breakpoint) (int, bool) {
	return hs.m.getReverse(v)
}

func (hs *breakPointHandlesMap) reset() {
	hs.m.reset()
}

type Reference struct {
	FrameHM      *frameHandlesMap
	BreakPointHM *breakPointHandlesMap
	VarHM        *handlesMap // 这里会存储Scope, yakvm.Value, 或者golang的value
}

func NewReference() *Reference {
	return &Reference{
		FrameHM:      newFrameHandlesMap(),
		BreakPointHM: newBreakPointHandlesMap(),
		VarHM:        newHandlesMap(1), // 0 is nil
	}
}
