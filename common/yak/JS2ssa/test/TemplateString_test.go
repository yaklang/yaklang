package test

import (
	_ "embed"
	_ "net/http/pprof"
	"testing"

	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestTemplateString1(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		test.CheckPrintlnValue("var a = `hello world`; println(a)", []string{`"hello world"`}, t)
	})
}

func TestTemplateString2(t *testing.T) {

	t.Run("expr", func(t *testing.T) {
		test.CheckPrintlnValue("var b = 123; var a = `b = ${b}`; println(a)", []string{`add("b = ", castType(string, 123))`}, t)
	})
}

func TestTemplateString3(t *testing.T) {
	t.Run("expr add", func(t *testing.T) {
		test.CheckPrintlnValue("var a = `hello ${5 + 10} world`; println(a)", []string{`add(add("hello ", castType(string, 15)), " world")`}, t)
	})
}