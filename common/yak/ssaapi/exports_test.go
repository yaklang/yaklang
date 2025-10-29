package ssaapi

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestA(t *testing.T) {

	prog, err := Parse(
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


var b = (()=>{return window.location.hostname + "/app/"})()
window.location.href = b + "/login.html?ts=";
window.location.href = "www"
	`,
		WithLanguage(ssaconfig.JS),
	)
	if err != nil {
		t.Fatal("prog parse error", err)
	}
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

	win.Show()
	win.ShowWithSource()
	win.ForEach(func(window *Value) {
		window.ShowWithRange()
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
		// window.ShowUseDefChain()
		// use-def:  |Type   |index  |Opcode |Value
		// 			Operand 0       Field   window.location
		// 			Operand 1       Const   href
		// 			Self            Field   window.location.href
		// 			User    0       Update  update(window.location.href, "6666")
		// 			User    1       Update  update(window.location.href, "7777")
		// 			User    2       Update  update(window.location.href, add(yak-main$5(, binding(window)), "/login.html?ts="))
		// 			User    3       Update  update(window.location.href, "www")

		// this `GetOperands` return Values, use foreach
		// window.GetOperands().ForEach(func(v *Value) {
		// })
		// window.GetOperand(0) // get href
		// window.GetUsers()    // get all users, return a Values
		// window.GetUser(0)    // get update(window.location.href, add(yak-main$1(), "/login.html?ts="))

		// get this function :
		window.GetUsers().ForEach(func(v *Value) {
			// v.ShowUseDefChain()
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
			if target.IsConstInst() {
				// fmt.Println("windows.location.href set by: ", target)
				result = append(result, target.String())
			}
		})
	})

	for _, res := range result {
		fmt.Println("window.location.href = ", res)
	}
}

func TestB(t *testing.T) {
	prog, err := Parse(`
	$(document).ready(function(){
		$("button").click(function(){
		  $.get("/example/jquery/demo_test.asp",function(data,status){
			alert("数据：" + data + "\n状态：" + status);
		  });
		});
	  });
	`, WithLanguage(ssaconfig.JS))
	if err != nil {
		t.Fatal("prog parse error", err)
	}

	values := prog.Ref("$")
	values.Show()
	u := values.GetUsers()
	u.Show()
	u2 := u.Filter(func(v *Value) bool { return v.IsCall() })
	fmt.Println(reflect.TypeOf(u2))
}

func TestFeedCode(t *testing.T) {

	code1 := `
	a = 1 
	b = "first line" 
	c = 1
	defer println("defer 1")
	`

	code2 := `
	if a == 1 {
		b = "second line"
	}

	defer println("defer 2")
	f = (a) => {
		println(c) // FreeValue
		println(a)
	}
	`

	code3 := `
	send(b)
	f()
	defer println("defer 3")
	`

	prog, err := Parse(code1)
	if err != nil {
		t.Fatal(err)
	}
	prog.Feed(strings.NewReader(code2))
	prog.Feed(strings.NewReader(code3))
	// prog.Finish()
	prog.Show()

	prog.Ref("f").Show()

}

func TestAAA(t *testing.T) {
	test := assert.New(t)
	prog, err := Parse(`
	a = {} 
	a[0] = 1
	target = a[0]
	`)
	test.Nil(err)
	prog.Ref("a").ForEach(func(v *Value) {
		v.ShowWithRange()
		v.ShowUseDefChain()
	})

	prog.Ref("target").ForEach(func(v *Value) {
		v.ShowWithRange()
		v.ShowUseDefChain()
	})
}
