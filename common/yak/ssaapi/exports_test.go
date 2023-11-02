package ssaapi

import (
	"fmt"
	"testing"
)

func TestA(t *testing.T) {
	prog := Parse(
		`
var b = ()=>{return window.location.hostname + "/app/"}()
window.location.href = b + "/login.html?ts=";
window.location.href = "www"
	`,
		WithLanguage(JS),
	)

	// the `Ref` just a filter
	win := prog.Ref("window").Ref("location").Ref("href") // this is values
	win.Show()
	// Values: 1
	//       0: Field: window.location.href

	win.Get(0) // get windows.location.href

	win.ForEach(func(window *Value) {
		// show use-def-chain
		// window.ShowUseDefChain()
		// use def chain [OpField]:
		//     Operand 0       href
		//     Operand 1       window.location
		//     Self            window.location.href
		//     User    0       update(window.location.href, add(yak-main$1(), "/login.html?ts="))
		//     User    1       update(window.location.href, "www")

		// this `GetOperands` return Values, use foreach
		window.GetOperands().ForEach(func(v *Value) {
		})
		window.GetOperand(0) // get href
		window.GetUsers()    // get all users, return a Values
		window.GetUser(0)    // get update(window.location.href, add(yak-main$1(), "/login.html?ts="))

		// get this function :
		window.GetUsers().ForEach(func(v *Value) {
			// v.ShowUseDefChain()
			if v.IsUpdate() {
				target := v.GetOperand(1)
				// target.ShowUseDefChain()
				if target.IsBinOp() {
					// target.ShowUseDefChain()
					call := target.GetOperand(0)
					// call.ShowUseDefChain()
					fun := call.GetOperand(0)
					// fun.ShowUseDefChain()
					_ = fun

					// how do next ??
					// fun.GetReturn()

				}
				if target.IsConst() {
					fmt.Println("windows.location.href set by: ", target)
				}
			}
		})

	})
}
