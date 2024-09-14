package syntaxflow

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestSF_Config_Filter(t *testing.T) {
	t.Run("until", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		// match until 
		a = 11
		b1 = f(a,1)

		// no match until get undefined 
		b3 = ccc 
		`,
			"b* #{until:`* ?{opcode:call}`}-> * as $result",
			map[string][]string{
				"result": {"Undefined-b3", "Undefined-f(11,1)"},
			})
	})

	t.Run("hook", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		a = 11
		b = f(a,1)
		`,
			"b #{hook:`* as $num`}-> as $result",
			map[string][]string{
				"num":    {"1", "11", "Undefined-f", "Undefined-f(11,1)"},
				"result": {"1", "11", "Undefined-f"},
			})
	})

	t.Run("exclude", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		// match exclude 
		b = f1(a1,1)

		// no match exclude get undefined
		b2 = f2(a2)
		`,
			"b* #{exclude:`* ?{opcode:const}`}-> as $result",
			map[string][]string{
				"result": {"Undefined-a2", "Undefined-f2"},
			})
	})
	t.Run("include", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		// match exclude 
		b = f1(a1,1)

		// no match exclude get undefined
		b2 = f2(a2)
		`,
			"b #{include:`* ?{opcode:const}`}-> as $result",
			map[string][]string{
				"result": {"Undefined-a1", "Undefined-f1", "1"},
			})
	})

	t.Run("test data exchange between old and new VMs", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		a = 11
		a = f(a,1)
		b1 = f(a,2)
		b2 = 22
		`,
			"b* #{hook:`* ?{!opcode:const,call} as $num`}-> as $result",
			map[string][]string{
				"num":    {"Undefined-f"},
				"result": {"1", "11", "2", "22", "Undefined-f"},
			})
	})

}
func TestMoreconfig(t *testing.T) {
	code := `
a = 1
f = (i)=>{
	a = i 
}

f(2)
c = a 
`
	t.Run("hook and hook", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code,
			"c(*#{hook: `*?{opcode: const} as $const`,hook: `*?{!opcode: const} as $_const`}->)",
			map[string][]string{
				"const":  {"1"},
				"_const": {"Undefined-dd"},
			})
	})
	t.Run("hook and until", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code,
			"c(* #{hook: `*?{opcode: const} as $const`,until: `*?{opcode: const} as $_const`}->)",
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
