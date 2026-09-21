package yakvm

import (
	"fmt"
	"sync"
)

type handlesMap struct {
	// Scope registration also runs outside Debugger.lock, in VM goroutines.
	mu          sync.RWMutex
	start       int
	nextHandle  int
	handleToVal map[int]interface{}
	ValToHandle map[interface{}]int
}

func newHandlesMap(start int) *handlesMap {
	return &handlesMap{
		start: start, nextHandle: start,
		handleToVal: make(map[int]interface{}),
		ValToHandle: make(map[interface{}]int),
	}
}

func (hs *handlesMap) reset() {
	hs.mu.Lock()
	defer hs.mu.Unlock()
	hs.nextHandle = hs.start
	hs.handleToVal = make(map[int]interface{})
	hs.ValToHandle = make(map[interface{}]int)
}

func (hs *handlesMap) forceSet(handle int, value interface{}) {
	hs.mu.Lock()
	defer hs.mu.Unlock()
	hs.handleToVal[handle] = value
	addr := fmt.Sprintf("%p", value)
	hs.ValToHandle[addr] = handle
}

func (hs *handlesMap) create(value interface{}) int {
	hs.mu.Lock()
	defer hs.mu.Unlock()
	return hs.createLocked(value)
}

// getOrCreate keeps reverse lookup and allocation in one critical section so
// concurrent registrations of the same object always return the same handle.
func (hs *handlesMap) getOrCreate(value interface{}) int {
	hs.mu.Lock()
	defer hs.mu.Unlock()
	if handle, ok := hs.ValToHandle[fmt.Sprintf("%p", value)]; ok {
		return handle
	}
	return hs.createLocked(value)
}

func (hs *handlesMap) createLocked(value interface{}) int {
	next := hs.nextHandle
	hs.nextHandle++
	hs.handleToVal[next] = value
	addr := fmt.Sprintf("%p", value)
	hs.ValToHandle[addr] = next
	return next
}

func (hs *handlesMap) get(handle int) (interface{}, bool) {
	hs.mu.RLock()
	defer hs.mu.RUnlock()
	v, ok := hs.handleToVal[handle]
	return v, ok
}

func (hs *handlesMap) getReverse(v interface{}) (int, bool) {
	hs.mu.RLock()
	defer hs.mu.RUnlock()
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
