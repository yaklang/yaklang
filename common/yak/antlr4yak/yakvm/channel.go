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
	if err := f.AutoConvertReflectValueByType(&item, ch.Type().Elem()); err != nil {
		panic(err)
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
