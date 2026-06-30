package yakvm

import (
	"fmt"
	"reflect"
)

// undefinedCallErrorMessage is reused by every call path (sync / async / lua) so the
// hint stays consistent. It deliberately spells out the most common real-world causes
// so both humans and AI agents can self-correct from the error text alone.
const undefinedCallErrorMessage = "runtime error: cannot call undefined(nil) as function. " +
	"common cause: the called name is not defined in THIS file/scope " +
	"(e.g. `if YAK_MAIN { runSelfTest() }` without defining `func runSelfTest(){...}` in the same file), " +
	"a typo in the function/variable name, calling something before it is declared, " +
	"or a wrong library/function name (the lib is real but the function does not exist). " +
	"fix: define the function before calling it, or correct the name " +
	"(use grep_yaklang_samples / yakdoc to find the real API)."

// notCallablePanicMessage explains why a value that exists (not undefined) still cannot be called.
func notCallablePanicMessage(callerValue interface{}) string {
	return fmt.Sprintf("runtime error: value of type %v cannot be called as a function. "+
		"common cause: the name resolves to a non-function value -- e.g. a variable/field shadows a function "+
		"with the same name, you indexed into a map/slice/struct that is not callable, or you used `()` on data. "+
		"fix: rename the variable so it stops shadowing the function, or call the right callable value.",
		reflect.TypeOf(callerValue))
}

func (v *Frame) call(caller *Value, wavy bool, args []*Value) {
	if IsUndefined(caller) {
		panic(undefinedCallErrorMessage)
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
	panic(notCallablePanicMessage(caller.Value))
}

func (v *Frame) asyncCall(caller *Value, wavy bool, args []*Value) {
	if IsUndefined(caller) {
		panic(undefinedCallErrorMessage)
	}
	if v.vm.callFuncCallback != nil {
		v.vm.callFuncCallback(caller, wavy, args)
	}
	if caller.Callable() {
		v.vm.AsyncStart()
		caller.AsyncCall(v, wavy, args...)
		return
	}
	panic(notCallablePanicMessage(caller.Value))
}

func (v *Frame) callLua(caller *Value, args []*Value) {
	if IsUndefined(caller) {
		panic(undefinedCallErrorMessage)
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
