package yakvm

import (
	"context"
	"fmt"
	"reflect"
)

// Not a source identifier: compiler-generated calls cannot shadow user variables.
// Resolve this name at execution time so bytecode contains no native function.
const SelectBuiltinName = "$yak:select$"

// SelectChannels consumes compiler-built Value lists without the FFI numeric or
// container conversions used for ordinary host-function arguments. It performs
// exactly one communication, with no worker goroutines for losing cases.
func (f *Frame) SelectChannels(operands []*Value) []any {
	// reflect.Select allows 65536 cases; reserve one for execution cancellation.
	if len(operands) > 65535 {
		panic("select has too many cases")
	}
	cases := make([]reflect.SelectCase, 0, len(operands)+1)
	defaultSeen := false
	for _, operand := range operands {
		values := operand.ValueList()
		if len(values) != 3 {
			panic("invalid select operands")
		}
		dir := reflect.SelectDir(values[0].Int())
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
			if dir == reflect.SelectRecv {
				if ch.Type().ChanDir() == reflect.SendDir {
					panic("cannot receive from send-only channel")
				}
			} else {
				if ch.Type().ChanDir() == reflect.RecvDir {
					panic("cannot send on receive-only channel")
				}
				item := reflect.ValueOf(values[2].Value)
				elem := ch.Type().Elem()
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
		}
		cases = append(cases, c)
	}
	ctx := f.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return []any{-1, nil, false}
	}
	cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(ctx.Done())})
	chosen, value, ok := reflect.Select(cases)
	if chosen == len(operands) {
		return []any{-1, nil, false}
	}
	var received any
	// Preserve Yak's existing closed-channel convention: nil, false.
	if cases[chosen].Dir == reflect.SelectRecv && ok {
		received = value.Interface()
	}
	return []any{chosen, received, ok}
}
