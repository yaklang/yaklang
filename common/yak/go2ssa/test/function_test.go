package test

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func TestPhiFuncCall(t *testing.T) {
	code := `package main
func A(a int) {
	println(a)
}
func B(a int) {
	println(a)
}
func main() {
	var a = A
	if c {
		a = B
	}
	println(a)
	a(1)
}

`

	//todo: disAsmLine is error,fix it
	ssatest.CheckSyntaxFlow(t, code, `A() as $functionA
B() as $functionB
`, map[string][]string{
		"functionA": {`phi(a)[Function-a,Function-a](1)`},
		"functionB": {`phi(a)[Function-a,Function-a](1)`},
	}, ssaapi.WithLanguage(ssaapi.GO))
}
