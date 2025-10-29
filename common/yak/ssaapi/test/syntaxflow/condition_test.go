package syntaxflow

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"

	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestSimple(t *testing.T) {
	t.Run("test get function", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
func a(){}
func b(){}
`, `
*?{opcode: func} as $sink
`, map[string][]string{"sink": {
			"Function-@main",
			"Function-a",
			"Function-b",
		}}, ssaapi.WithLanguage(ssaconfig.Yak))
	})
	t.Run("Test opcode", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		aa = 1 // constant
		ab = b // undefined
		`,
			`
		a* as $target1
		$target1?{opcode: const} as $target2
		`,
			map[string][]string{
				"target1": {"1", "Undefined-ab"},
				"target2": {"1"},
			},
		)
	})

	t.Run("Test multiple opcode", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		aa = 1 // constant
		ab = b // undefined
		f = (i) => {
			ac = i
		}
		`,
			`
		a* as $target1
		$target1?{opcode: const, param} as $target2
		`,
			map[string][]string{
				"target1": {"1", "Undefined-ab", "Parameter-i"},
				"target2": {"1", "Parameter-i"},
			},
		)
	})

	t.Run("string condition", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		aa = "araaa"
		ab = "abcccc"
		`,
			`
		a* as $target1
		$target1?{have: abc} as $target2
		`,
			map[string][]string{
				"target1": {`"araaa"`, `"abcccc"`},
				"target2": {`"abcccc"`},
			},
		)
	})

	t.Run("negative condition", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		aa = "araaa"
		ab = "abcccc"
		ac = b // undefined
		`,
			`
		a* as $target1
		$target1?{not have: abc} as $target2
		$target1?{! opcode: const} as $target3
		`,
			map[string][]string{
				"target1": {`"araaa"`, `"abcccc"`, "Undefined-ac"},
				"target2": {`"araaa"`, "Undefined-ac"},
				"target3": {"Undefined-ac"},
			},
		)
	})

	t.Run("logical condition", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		aa = "araaa"
		ab = abcccc()
		ac = "abcccc"
		`,
			`
		a* as $target1
		$target1?{(have: abc) && (opcode: const)} as $target2
		$target1?{(! have: ara) && ((have: abc) || (opcode: const))} as $target3
		`,
			map[string][]string{
				"target1": {`"araaa"`, `"abcccc"`, "Undefined-abcccc", "Undefined-abcccc()"},
				"target2": {`"abcccc"`},
				"target3": {"Undefined-abcccc()", "Undefined-abcccc", `"abcccc"`},
			},
		)
	})

}

func Test_String_Contain(t *testing.T) {
	t.Run("test string contain have", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		aa = "araaa"
		ab = "abcccc"
		ac = "ccc"
		`,
			`
		a* as $target1
		$target1?{have: abc, ccc} as $target2
		`,
			map[string][]string{
				"target1": {`"araaa"`, `"abcccc"`, `"ccc"`},
				"target2": {`"abcccc"`},
			},
		)
	})

	t.Run("test string contain any", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		aa = "araaa"
		ab = "abcccc"
		ac = "ccc"
		`,
			`
		a* as $target1
		$target1?{any: abc, ccc} as $target2
		`,
			map[string][]string{
				"target1": {`"araaa"`, `"abcccc"`, `"ccc"`},
				"target2": {`"abcccc"`, `"ccc"`},
			},
		)
	})
}

func Test_Condition_FilterExpr(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		f = (a1, a2) => {
			a1.b = 1
		}
		`,
			`
			a* as $target1
			$target1?{.b} as $target2
			a*?{.b} as $target3
			`,
			map[string][]string{
				"target1": {"Parameter-a1", "Parameter-a2"},
				"target2": {"Parameter-a1"},
				"target3": {"Parameter-a1"},
			})
	})

	t.Run("logical", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		f = (a1, a2, a3) => {
			a1.b = 1
			a2.c = 2
		}
		`,
			`
			a* as $target1
			$target1?{(.b) || (.c)} as $target2
			a*?{(.b) || (.c)} as $target3
			`,
			map[string][]string{
				"target1": {"Parameter-a1", "Parameter-a2", "Parameter-a3"},
				"target2": {"Parameter-a1", "Parameter-a2"},
				"target3": {"Parameter-a1", "Parameter-a2"},
			})
	})
}
func TestConditionFilter(t *testing.T) {
	code := `
		f = (a1, a2, a3) => {
			a1 = "abc"
			b2 = "anc123"
			b3 = "anc"
			b4 = "anc1anc"
			a3 = 12
		}
`
	t.Run("test regexp condition", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
a* as $target
$target?{have: /^[0-9]+$/} as $output
`, map[string][]string{
			"output": {`12`},
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})
	t.Run("test global condition", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
b* as $target
$target?{have: anc1*} as $output
`, map[string][]string{
			"output": {`"anc123"`, `"anc1anc"`},
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})
	t.Run("test exact condition", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
a* as $target
$target?{have: abc} as $output
`, map[string][]string{
			"output": {`"abc"`},
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})
	t.Run("test global and exact", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
b* as $target
$target?{have: anc,*123} as $output
`, map[string][]string{
			"output": {`"anc123"`},
		})
	})
	t.Run("test exact and regexp", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
b* as $target
$target?{have: anc,/[0-9]+anc$/} as $output
`, map[string][]string{
			"output": {`"anc1anc"`},
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})
	t.Run("test global and regex", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
b* as $target
$target?{have: anc*,/[0-9]+anc$/} as $output
`, map[string][]string{
			"output": {`"anc1anc"`},
		})
	})
}

func Test_Condition_Filter_Start_With_Program(t *testing.T) {
	t.Run("test CompareString-Have Regex", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		asd = 1
		asdd = 2
		`,
			`
			*?{ have: /^asd$/  } as $target1
			`,
			map[string][]string{
				"target1": {"1"},
			})
	})
	t.Run("test CompareString-Have Global", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		asd = 1
		asdd = 2
		`,
			`
			*?{ have: a* } as $target1
			`,
			map[string][]string{
				"target1": {"1", "2"},
			})
	})
	t.Run("test CompareString-Have with opcode", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		a1 = "abc"
		a2 = ss
		a3 = func() {}
		`,
			`
			*?{ have: 'a' && opcode:const } as $target1
			`,
			map[string][]string{
				"target1": {"\"abc\""},
			})
	})

	t.Run("test CompareString-Have mutli exact", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		aa = 1
		aacc = 2
		cc = 3
		`,
			`
			*?{have:'aa','cc'} as $target1
			`,
			map[string][]string{
				"target1": {"2"},
			})
	})

	t.Run("test CompareString-Have Any 1 ", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		aa = 1
		aacc = 2
		cc = 3
		`,
			`
			*?{any:'aa','cc'} as $target1
			`,
			map[string][]string{
				"target1": {"1", "2", "3"},
			})
	})

	t.Run("test CompareString-Have Any 2", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		exist = 1
		www = 2
		`,
			`
			*?{any:'notExist','exist'} as $target1
			`,
			map[string][]string{
				"target1": {"1"},
			})
	})

	t.Run("test CompareOpcode 1-1", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		a1=11
		a2=undefined
		a3=func(){}
		`,
			`
			*?{opcode:const} as $target1
			`,
			map[string][]string{
				"target1": {"11"},
			})
	})
	t.Run("test compareOpcode", func(t *testing.T) {
		for i := 0; i < 20; i++ {
			code := `
a1 = 11
a2 = 22
`
			ssatest.CheckSyntaxFlow(t, code, `a*?{opcode: const && have: '11'} as $target1`, map[string][]string{
				"target1": {"11"},
			}, ssaapi.WithLanguage(ssaconfig.Yak))
		}
	})
	t.Run("test CompareOpcode 1-2", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		a1 = 11
		b2 = 22
		a2 = undefined
		a3 = func(){}
		`,
			`
			*?{opcode:const && have:'11'} as $target1
			`,
			map[string][]string{
				"target1": {"11"},
			}, ssaapi.WithLanguage(ssaconfig.Yak))
	})

	t.Run("test CompareOpcode 2", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(t, `    public class demo {
        public static void main(String[] args) {
            String str = "hello";
            if (str.contains("he")) {
                System.out.println("ok");
            }
        }
    }`, `*?{opcode:call && have:"contain"} as $output;
alert $output;`, map[string][]string{
			"output": {"contain"},
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})

	t.Run("test muti filter", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		a1 = 11
		b2 = 22
		a2 = undefined
		a3 = func(){}
		`,
			`
			*?{opcode:const && *?{have:'11'}} as $target1
			`,
			map[string][]string{
				"target1": {"11"},
			})
	})
}

func TestCondition_CheckType(t *testing.T) {
	code := `
package org.joychou.config;
class A{
	public void test() {
		Exception e = 11; 
		InvalidClassException e2 = 22; 
		IOException e3 = 33;
		XXX e4 = 44;
	}
}
	`

	t.Run("check normal have is string contain", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
e* as $target 
$target?{<typeName>?{have:Exception}} as $output
	`, map[string][]string{
			"output": {"11", "22", "33"},
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})

	t.Run("check normal have regexp", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
e* as $target
$target?{<typeName>?{have:/^Exception$/}} as $output
`, map[string][]string{
			"output": {"11"},
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})
}

func TestSearch(t *testing.T) {
	code := `function a(){}`
	ssatest.CheckSyntaxFlow(t, code, `*?{opcode: const}`, map[string][]string{}, ssaapi.WithLanguage(ssaconfig.Yak))
}
