package syntaxflow

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestSF_Config_Until(t *testing.T) {
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

}

func TestSF_Config_HOOK(t *testing.T) {
	t.Run("hook", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(t, `
		a = 11
		b = f(a,1)
		`,
			"b #{hook:`* as $num`}-> as $result",
			map[string][]string{
				"num": {"Undefined-f(11,1)"},
			})
	})

}

func TestSF_Config_Exclude(t *testing.T) {
	t.Run("exclude in top value", func(t *testing.T) {
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

	t.Run("exclude in dataflow path ", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		b = f1(1 + d)

		b2 = 11 + c 
		`, "b* #{exclude: `* ?{opcode:call}`}-> as $result", map[string][]string{
			"result": {"Undefined-c", "11"},
		})
	})
}

func TestSF_Config_Include(t *testing.T) {
	t.Run("include", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		b0 = f1 + 0 
		b1 = f1(1)
		b2 = f2(2)
		b3 = f3(3)
		`,
			"b* #{include:`* ?{have:f1}`}-> as $result",
			map[string][]string{
				"result": {"Undefined-f1", "1", "0"},
			})
	})

	t.Run("include in dataflow path", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		b0 = f1 + 0 
		b1 = f1(1)
		b2 = f2(2)
		b3 = f3(3)
		`,
			"b* #{include:`* ?{have:f1 && opcode:call}`}-> as $result; ",
			map[string][]string{
				"result": {"Undefined-f1", "1"},
			})
	})
}

func TestSF_config_WithNameVariableInner(t *testing.T) {
	check := func(t *testing.T, code string) {
		ssatest.CheckSyntaxFlow(t, `
		b0 = f1(1)

		b1 = f2 + 22
		`,
			code, map[string][]string{
				"result": {"Undefined-f2", "22", "Undefined-f1(1)"},
			})
	}
	t.Run("check no name", func(t *testing.T) {
		check(t, "b* #{until:`* ?{opcode:call}`}-> as $result")
	})

	t.Run("check only one name", func(t *testing.T) {
		check(t, "b* #{until:`* ?{opcode:call} as $name`}-> as $result")
	})

	t.Run("check magic name", func(t *testing.T) {
		check(t, `
b* #{until: <<<UNTIL
	* as $value;
	* ?{opcode:call} as $__next__
UNTIL
}-> as $result`)
	})
}

func TestSF_Config_MultipleConfig(t *testing.T) {
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
