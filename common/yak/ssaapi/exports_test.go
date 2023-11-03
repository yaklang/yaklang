package ssaapi

import (
	"fmt"
	"strings"
	"testing"
)

func TestA(t *testing.T) {

	prog := Parse(
		`
() => {
	window.location.href = "11"
}
a = 1 
if (a > 8){
	setTimeout(() => {
		window.location.href = "22"
	})
}
if (checkFunc(a)) {
	setTimeout(() => {
		window.location.href = "33"
	})
}
setTimeout(() => {
	window.location.href = "44"
})

a = {}
a["b"] = window.location
b = window.location
a.b.href = "5555"
window.location.href = "6666"
b.href = "7777"

a["c"] = window 
a.c.location.href = "8888"
a["d"] = a.c
a.d.location.href = "9999"
a["e"] = a.c.location
a.e.href = "1010"


var b = ()=>{return window.location.hostname + "/app/"}()
window.location.href = b + "/login.html?ts=";
window.location.href = "www"
	`,
		WithLanguage(JS),
	)

	// prog.Show()
	// the `Ref` just a filter
	// window := prog.Ref("window")
	// window.Show()
	// fmt.Println("windows : ")
	// window.ForEach(func(v *Value) {
	// 	v.ShowUseDefChain()
	// })

	win := prog.Ref("window").Ref("location").Ref("href")
	// win.Show()
	// Values: 1
	//       0: Field: window.location.href

	result := make([]string, 0)

	// win.Get(0) // get windows.location.href
	checkValueReachable := func(v *Value) bool {
		reach := v.IsReachable()
		if reach == -1 {
			return false
		}
		if reach == 0 {
			fmt.Printf("in condition %s, this value %s can reachable\n", v.GetReachable(), v)
		}
		return true
	}

	checkFunctionReachable := func(fun *Value) bool {
		if !fun.HasUsers() {
			return false
		}
		// fun.ShowUseDefChain()
		ret := false
		fun.GetUsers().ForEach(func(v *Value) {
			// v.ShowUseDefChain()
			if !checkValueReachable(v) {
				return
			}
			if v.IsCall() {
				callee := v.GetOperand(0)
				if callee == fun {
					ret = true
				}
				if callee.String() == "setTimeout" {
					ret = true
				}
			}
			// fmt.Println(v)
		})
		return ret
	}

	win.ForEach(func(window *Value) {
		window.ShowWithSource()
		if !window.InMainFunction() {
			fun := window.GetFunction()
			if !checkFunctionReachable(fun) {
				fmt.Println("this value in unreachable sub-function,skip")
				return
			}
		} else {
			if !checkValueReachable(window) {
				fmt.Println("this value is unreachable,skip")
				return
			}
		}

		// show use-def-chain
		window.ShowUseDefChain()
		// use-def chain:  |Type   |index  |Opcode |Value
		// 					Operand 0       Field   window.location
		// 					Operand 1       Const   href
		// 					Self            Field   window.location.href
		// 					User    0       Update  update(window.location.href, "6666")
		// 					User    1       Update  update(window.location.href, "7777")
		// 					User    2       Update  update(window.location.href, add(yak-main$5(, binding(window)), "/login.html?ts="))
		// 					User    3       Update  update(window.location.href, "www")

		// this `GetOperands` return Values, use foreach
		// window.GetOperands().ForEach(func(v *Value) {
		// })
		// window.GetOperand(0) // get href
		// window.GetUsers()    // get all users, return a Values
		// window.GetUser(0)    // get update(window.location.href, add(yak-main$1(), "/login.html?ts="))

		// get this function :
		window.GetUsers().ForEach(func(v *Value) {
			// v.ShowUseDefChain()
			if !v.IsUpdate() {
				return
			}
			// check this value reachable

			target := v.GetOperand(1)
			// target.ShowUseDefChain()
			if target.IsBinOp() {
				// target.ShowUseDefChain()
				call := target.GetOperand(0)
				// call.ShowUseDefChain()
				fun := call.GetOperand(0)
				// fun.ShowUseDefChain()
				_ = fun
				if fun.IsFunction() {
					ret := fun.GetReturn()
					// ret.Show()
					ret.ForEach(func(v *Value) {
						// v.ShowUseDefChain()
						v1 := v.GetOperand(0)
						// v1.ShowUseDefChain()
						str := strings.Replace(target.String(), target.GetOperand(0).String(), v1.String(), -1)
						// fmt.Println("windows.location.href set by:", str)
						result = append(result, str)
					})
				}

				// how do next ??
				// fun.GetReturn()

			}
			if target.IsConst() {
				// fmt.Println("windows.location.href set by: ", target)
				result = append(result, target.String())
			}
		})
	})

	for _, res := range result {
		fmt.Println("window.location.href = ", res)
	}
}
