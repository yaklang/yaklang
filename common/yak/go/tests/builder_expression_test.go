package tests

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssatest"
	"testing"
)

func TestVariableDeclare(t *testing.T) {
	code := `package main
var a = 1
func main(){
var b = a
println(b)
}`
	ssatest.CheckPrintlnValue(code, []string{"1"}, t)
}
