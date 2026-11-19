package test

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestScalaSyntaxFlow(t *testing.T) {
	t.Run("basic dataflow reaches literal", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, scalaWithPrintln(`
object Main {
  def main(args: Array[String]): Unit = {
    val a = 7
    println(a)
  }
}
`), `println(* #-> * as $source)`, map[string][]string{
			"source": {"7"},
		}, ssaapi.WithLanguage(ssaconfig.SCALA))
	})

	t.Run("cross function dataflow", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(t, scalaWithPrintln(`
object Main {
  def id(a: Int): Int = {
    return a
  }
  def main(args: Array[String]): Unit = {
    println(id(9))
  }
}
`), `println(* #-> * as $source)`, map[string][]string{
			"source": {"9"},
		}, ssaapi.WithLanguage(ssaconfig.SCALA))
	})

	t.Run("closure free value dataflow", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(t, scalaWithPrintln(`
object Main {
  def main(args: Array[String]): Unit = {
    val n = 10
    val f = { (x: Int) => x + n }
    println(f(1))
  }
}
`), `println(* #-> * as $source)`, map[string][]string{
			"source": {"10"},
		}, ssaapi.WithLanguage(ssaconfig.SCALA))
	})

	t.Run("class field dataflow", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, scalaWithPrintln(`
class Box {
  val value = 42
  def show(): Unit = {
    println(value)
  }
}
object Main {
  def main(args: Array[String]): Unit = {
    show()
  }
}
`), `println(* #-> * as $source)`, map[string][]string{
			"source": {"42"},
		}, ssaapi.WithLanguage(ssaconfig.SCALA))
	})
}
