package tests

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssatest"
	"testing"
)

func TestVariableDeclare(t *testing.T) {
	code := `package main

func main(){
var a = 1
println(a)
}`
	ssatest.MockSSA(t, code)
}
