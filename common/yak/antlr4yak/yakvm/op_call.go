package yakvm

import (
	"fmt"
	"reflect"
)

func (v *Frame) call(caller *Value, wavy bool, args []*Value) {
	if IsUndefined(caller) {
		panic("runtime error: cannot call undefined(nil) as function")
	}
	if v.vm.callFuncCallback != nil {
		v.vm.callFuncCallback(caller, wavy, args)
	}
	if caller.Callable() {
		v.push(&Value{
			TypeVerbose: "__call_returns__",
			Value:       caller.Call(v, wavy, args...),
		})
		return
	}
	panic(fmt.Sprintf("runtime error: %v cannot be called", reflect.TypeOf(caller.Value)))
}

func (v *Frame) asyncCall(caller *Value, wavy bool, args []*Value) {
	if IsUndefined(caller) {
		panic("runtime error: cannot call undefined(nil) as function")
	}
	if v.vm.callFuncCallback != nil {
		v.vm.callFuncCallback(caller, wavy, args)
	}
	if caller.Callable() {
		v.vm.AsyncStart()
		caller.AsyncCall(v, wavy, args...)
		return
	}
	panic(fmt.Sprintf("runtime error: %v cannot be called", reflect.TypeOf(caller)))
}

func (v *Frame) callLua(caller *Value, args []*Value) {
	if IsUndefined(caller) {
		panic("runtime error: cannot call undefined(nil) as function")
	}
	if caller.Callable() {
		v.push(&Value{
			TypeVerbose: "__call_returns__",
			Value:       caller.CallLua(v, args...),
		})
		return
	}
	panic(fmt.Sprintf("runtime error: %v cannot be called", reflect.TypeOf(caller)))
}
