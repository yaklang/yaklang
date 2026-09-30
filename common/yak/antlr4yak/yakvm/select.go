package yakvm

import (
	"context"
	"fmt"
	"reflect"
)

// Not a source identifier: compiler-generated calls cannot shadow user variables.
// Resolve this name at execution time so bytecode contains no native function.
const SelectBuiltinName = "$yak:select$"

// Only this private function type receives the frame from native call dispatch.
// Ordinary Go functions, even with the same signature, keep their FFI behavior.
type selectBuiltinFunc func(*Frame, []*Value) []any

// SelectBuiltin is stateless: registering it needs no per-engine bound method,
// and execution uses the actual caller rather than looking up a goroutine frame.
func SelectBuiltin() any {
	return selectBuiltinFunc((*Frame).SelectChannels)
}

func (f *Frame) callSelectBuiltin(fn selectBuiltinFunc, async bool, args []*Value) []any {
	if async || len(args) != 1 || args[0] == nil {
		panic("invalid select call")
	}
	operands, ok := args[0].Value.([]*Value)
	if !ok {
		// The existing OpNewSlice constructs []any for an empty select. All
		// nonempty case lists are produced by OpList and contain raw *Values.
		empty, isEmptySlice := args[0].Value.([]any)
		if !isEmptySlice || len(empty) != 0 {
			panic("invalid select operands")
		}
	}
	return fn(f, operands)
}

// SelectChannels consumes compiler-built Value lists without the FFI numeric or
// container conversions used for ordinary host-function arguments. It performs
// exactly one communication, with no worker goroutines for losing cases.
func (f *Frame) SelectChannels(operands []*Value) []any {
	// reflect.Select allows 65536 cases; reserve one for execution cancellation.
	if len(operands) > 65535 {
		panic("select has too many cases")
	}
	// Most selects have few active channels. Nil channels must still be
	// validated, but do not need reflect.Select's per-receive scratch storage.
	var caseStorage [8]reflect.SelectCase
	cases := caseStorage[:]
	active := 0
	// Before the first disabled case, compact and source indexes are identical.
	// Only active cases after it need a mapping; dense selects need none.
	firstDisabled := -1
	var indexStorage [8]int
	indexes := indexStorage[:]
	defaultSeen := false
	for sourceIndex, operand := range operands {
		values := operand.ValueList()
		if len(values) != 3 {
			panic("invalid select operands")
		}
		// Compiler-generated directions are int; retain the existing numeric
		// conversion for callers constructing operands directly.
		if values[0] == nil {
			panic("invalid select direction")
		}
		direction, isInt := values[0].Value.(int)
		if !isInt {
			direction = values[0].Int()
		}
		dir := reflect.SelectDir(direction)
		c := reflect.SelectCase{Dir: dir}
		if dir == reflect.SelectDefault {
			if defaultSeen {
				panic("select has multiple default clauses")
			}
			defaultSeen = true
		} else {
			if dir != reflect.SelectSend && dir != reflect.SelectRecv {
				panic("invalid select direction")
			}
			ch := reflect.ValueOf(values[1].Value)
			if !ch.IsValid() || ch.Kind() != reflect.Chan {
				panic("select case requires a channel")
			}
			c.Chan = ch
			typ := ch.Type()
			if dir == reflect.SelectRecv {
				if typ.ChanDir() == reflect.SendDir {
					panic("cannot receive from send-only channel")
				}
			} else {
				if typ.ChanDir() == reflect.RecvDir {
					panic("cannot send on receive-only channel")
				}
				item := reflect.ValueOf(values[2].Value)
				elem := typ.Elem()
				if !item.IsValid() {
					switch elem.Kind() {
					case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice, reflect.UnsafePointer:
						item = reflect.Zero(elem)
					default:
						panic(fmt.Sprintf("cannot send nil to channel of %s", elem))
					}
				} else if !item.Type().AssignableTo(elem) {
					panic(fmt.Sprintf("cannot send %s to channel of %s", item.Type(), elem))
				}
				c.Send = item
			}
			if ch.IsNil() {
				if firstDisabled < 0 {
					firstDisabled = active
				}
				continue
			}
		}
		if firstDisabled >= 0 {
			if active-firstDisabled == len(indexes) {
				grown := make([]int, len(operands)-firstDisabled)
				copy(grown, indexes)
				indexes = grown
			}
			indexes[active-firstDisabled] = sourceIndex
		}
		if active == len(cases) {
			grown := make([]reflect.SelectCase, len(operands)+1)
			copy(grown, cases)
			cases = grown
		}
		cases[active] = c
		active++
	}
	ctx := f.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return []any{-1, nil, false}
	}
	cancelIndex := active
	if active == len(cases) {
		grown := make([]reflect.SelectCase, len(operands)+1)
		copy(grown, cases)
		cases = grown
	}
	cases[active] = reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(ctx.Done())}
	cases = cases[:active+1]
	chosen, value, ok := reflect.Select(cases)
	if chosen == cancelIndex {
		return []any{-1, nil, false}
	}
	var received any
	// Preserve Yak's existing closed-channel convention: nil, false.
	if cases[chosen].Dir == reflect.SelectRecv && ok {
		received = value.Interface()
	}
	if firstDisabled >= 0 && chosen >= firstDisabled {
		chosen = indexes[chosen-firstDisabled]
	}
	return []any{chosen, received, ok}
}
