package test

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestKotlinSyntaxFlow(t *testing.T) {
	t.Run("basic dataflow reaches literal", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, kotlinWithPrintln(`
fun main() {
    val a = 7
    println(a)
}
`), `println(* #-> * as $source)`, map[string][]string{
			"source": {"7"},
		}, ssaapi.WithLanguage(ssaconfig.KOTLIN))
	})

	t.Run("cross function dataflow", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(t, kotlinWithPrintln(`
fun id(a: Int): Int {
    return a
}
fun main() {
    println(id(9))
}
`), `println(* #-> * as $source)`, map[string][]string{
			"source": {"9"},
		}, ssaapi.WithLanguage(ssaconfig.KOTLIN))
	})

	t.Run("closure free value dataflow", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(t, kotlinWithPrintln(`
fun main() {
    val n = 10
    val f = { x: Int -> x + n }
    println(f(1))
}
`), `println(* #-> * as $source)`, map[string][]string{
			"source": {"10"},
		}, ssaapi.WithLanguage(ssaconfig.KOTLIN))
	})

	t.Run("class field dataflow", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, kotlinWithPrintln(`
class Box {
    val value = 42
    fun show() {
        println(value)
    }
}
fun main() {
    show()
}
`), `println(* #-> * as $source)`, map[string][]string{
			"source": {"42"},
		}, ssaapi.WithLanguage(ssaconfig.KOTLIN))
	})
}
