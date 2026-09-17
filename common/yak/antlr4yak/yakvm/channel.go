package yakvm

import (
	"context"
	"fmt"
	"reflect"
)

func (f *Frame) sendChannel(channel, value *Value) *Value {
	ch := reflect.ValueOf(channel.Value)
	if !ch.IsValid() || ch.Kind() != reflect.Chan {
		panic(fmt.Sprintf("cannot send to %s", channel.TypeVerbose))
	}
	if ch.Type().ChanDir() == reflect.RecvDir {
		panic("cannot send on receive-only channel")
	}
	item := reflect.ValueOf(value.Value)
	elem := ch.Type().Elem()
	// Sending an assignable value preserves its dynamic type and identity. The
	// FFI converter normalizes numbers in empty interfaces and copies containers
	// of nonidentical types; neither operation belongs to channel assignment.
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
	ctx := f.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	// If send and cancellation become ready during Select, either may win.
	if ctx.Err() != nil {
		return undefined
	}
	reflect.Select([]reflect.SelectCase{
		{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(ctx.Done())},
		{Dir: reflect.SelectSend, Chan: ch, Send: item},
	})
	return undefined
}
