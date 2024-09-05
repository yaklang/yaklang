package syntaxflow

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestSF_Config_Filter(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		a = 11
		b = f(a,1)
		b= 22
		`,
			"b -{until:`* ?{opcode:const}`}-> * as $result",
			map[string][]string{
				"result": {"22"},
			})
	})

	t.Run("test hook", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		a = 11
		b = f(a,1)
		b= 22
		`,
			"b #{hook:`* as $num`}-> as $result",
			map[string][]string{
				"num":    {"1", "11", "22", "Undefined-f", "Undefined-f(11,1)"},
				"result": {"1", "11", "22", "Undefined-f"},
			})
	})
	t.Run("test exclude", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		a = 11
		b = f(a,1)
		b= 22
		`,
			"b #{exclude:`* ?{opcode:const}`}-> as $result",
			map[string][]string{
				"result": {"Undefined-f", "Undefined-f(11,1)"},
			})
	})
	t.Run("test include", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		a = 11
		b = f(a,1)
		b= 22
		`,
			"b #{include:`* ?{opcode:const}`}-> as $result",
			map[string][]string{
				"result": {"11", "22", "1"},
			})
	})

	t.Run("test data exchange between old and new VMs", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		a = 11
		a = f(a,1)
		b = f(a,2)
		b= 22
		`,
			"b #{hook:`* ?{opcode:const} as $num`}-> as $result",
			map[string][]string{
				"num":    {"1", "11", "2", "22"},
				"result": {"1", "11", "2", "22", "Undefined-f"},
			})
	})

	/*
		f (a)=>{
			b=a
			return b
		}
		c = f(1)
		f(c)
	*/
	t.Run("test data and test more config item", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		a = 11
		a = f(a,1)
		b = f(a,2)
		b= 22
		`,
			"b #{hook:`* ?{opcode:const} as $num`}-> as $result",
			map[string][]string{
				"num":    {"1", "11", "2", "22"},
				"result": {"1", "11", "2", "22", "Undefined-f"},
			})
	})
}
func TestMoreconfig(t *testing.T) {
	code := `
	a = 1
	b = 2
	c(a)
	c(dd)
`
	t.Run("hook and hook", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, "c(*#{hook: `*?{opcode: const} as $const`,hook: `*?{!opcode: const} as $_const`}->)",
			map[string][]string{
				"const":  {"1"},
				"_const": {"Undefined-dd"},
			})
	})
	t.Run("hook and until", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, "c(* #{hook: `*?{opcode: const} as $const`,until: `*?{opcode: const} as $_const`}->)",
			map[string][]string{
				"const":  {"1"},
				"_const": {"1"},
			})
	})
	t.Run("until", func(t *testing.T) {
		codes := `a = 1
b = 2
function aaa(a){
    return a
}
c = aaa(a)
cc = aaab(a)
println(aa)`
		ssatest.CheckSyntaxFlow(t, codes, "a-{until: `*<getCaller><name>?{have: \"aaab\"}`}-> as $result", map[string][]string{
			"result": {"Undefined-aaab(1)"},
		})
	})
}
