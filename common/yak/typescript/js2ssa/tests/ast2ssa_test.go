package tests

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func TestSimplePrint(t *testing.T) {
	ssatest.CheckPrintlnValue(`
let a = 1
println(a)
`, []string{"1"}, t)
}
