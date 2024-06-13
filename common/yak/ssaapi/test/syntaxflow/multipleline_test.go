package syntaxflow

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestMultipleLine(t *testing.T) {
	t.Run("test start with identifier", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		f({
			"a": 1, 
		})
		`,
			`f(* as $obj)
			$obj.a as $a`,
			map[string][]string{
				"a": {"1"},
			})
	})

	t.Run("test start with primary", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		f = {
			"a": (i)=>{return i},
			"b": (i)=>{return i},
		}
		f.a(1)
		f.b(2)
		`,
			`f.a(* as $a)
			f.b(* as $b)
			`,
			map[string][]string{
				"a": {"1"},
				"b": {"2"},
			})
	})

}

func TestVariable(t *testing.T) {
	t.Run("multiple assign variable", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		a = 1
		b = 2
		c = 3
		`,
			`a as $res
			b as $res
			`,
			map[string][]string{
				"res": {"1", "2"},
			})
	})
}

func Test_Statement_Error(t *testing.T) {
	t.Run("test not found error in multiple statement", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		a = 1
		b = 2
		`,
			`a as $res
			unknown as $res 
			b as $res
			`,
			map[string][]string{
				"res": {"1", "2"},
			})
	})
}
