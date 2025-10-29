package syntaxflow

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestNativeCallConst(t *testing.T) {
	code := `cc = "127.0.0.1"`
	t.Run("test const nativeCall global", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `<const(g="127*")> as $output`, map[string][]string{
			"output": {`"127.0.0.1"`},
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})
	t.Run("test const nativeCall reg", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `<const(r=`+strconv.Quote(`^((0|[1-9]\d?|1\d\d|2[0-4]\d|25[0-5])\.){3}(0|[1-9]\d?|1\d\d|2[0-4]\d|25[0-5])$`)+`)> as $output`,
			map[string][]string{
				"output": {`"127.0.0.1"`},
			},
			ssaapi.WithLanguage(ssaconfig.Yak))
	})
	t.Run("test const heredoc", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `<const(<<<CODE
"127.0.0.1"
CODE)> as $output`, map[string][]string{
			"output": {`"127.0.0.1"`},
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})
	t.Run("test const nativeCall ex", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `<const(e="127.0.0.1")> as $output`, map[string][]string{
			"output": {`"127.0.0.1"`},
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})
	t.Run("test const nativeCall error", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `<const(b="127.0.0.1")> as $output`, map[string][]string{}, ssaapi.WithLanguage(ssaconfig.Yak))
	})
	t.Run("test const Syntactic sugar global", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `"127*" as $output`,
			map[string][]string{
				"output": {`"127.0.0.1"`},
			},
			ssaapi.WithLanguage(ssaconfig.Yak))
	})
	t.Run("test const Syntactic sugar exact", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `"127.0.0.1" as $output`,
			map[string][]string{
				"output": {`"127.0.0.1"`},
			},
			ssaapi.WithLanguage(ssaconfig.Yak))
	})
}
func TestSyntacticSugar_HavePrefix(t *testing.T) {
	code := `b = "123.131.11.12"`
	t.Run("prefix const test regexp", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `r`+strconv.Quote(`^((0|[1-9]\d?|1\d\d|2[0-4]\d|25[0-5])\.){3}(0|[1-9]\d?|1\d\d|2[0-4]\d|25[0-5])$`)+` as $output`,
			map[string][]string{
				"output": {`"123.131.11.12"`},
			},
			ssaapi.WithLanguage(ssaconfig.Yak))
	})
	t.Run("prefix const test exact", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `e"123.131.11.12" as $output`, map[string][]string{
			"output": {`"123.131.11.12"`},
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})
	t.Run("prefix const test glob", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `g"123*" as $output`, map[string][]string{
			"output": {`"123.131.11.12"`},
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})
	t.Run("prefix heredoc global", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `g<<<CODE
123*
CODE as $output`, map[string][]string{
			"output": {`"123.131.11.12"`},
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})
	t.Run("prefix heredoc exact", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `e<<<CODE
123.131.11.12
CODE as $output`, map[string][]string{
			"output": {`"123.131.11.12"`},
		})
	})
	t.Run("prefix heredoc regexp", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `r<<<CODE
^((0|[1-9]\d?|1\d\d|2[0-4]\d|25[0-5])\.){3}(0|[1-9]\d?|1\d\d|2[0-4]\d|25[0-5])$
CODE as $output`, map[string][]string{
			"output": {`"123.131.11.12"`},
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})
}
func TestSyntacticSugar_ConstInRecursive(t *testing.T) {
	code := `
	a = "abcabcabc"
	c = a+b
	d = test(c)
	query(d)`
	t.Run("const_global", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
query(* #{until: <<<UNTIL
	"a*" as $output 
UNTIL}->*)`,
			map[string][]string{
				"output": {`"abcabcabc"`},
			}, ssaapi.WithLanguage(ssaconfig.Yak))
	})
	t.Run("const_reg", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, "query(* #{until: `r\"[a-z]+\" as $output`}->*)",
			map[string][]string{
				"output": {`"abcabcabc"`},
			},
			ssaapi.WithLanguage(ssaconfig.Yak))
	})
	t.Run("const_exact", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, "query(* #{until: `\"abcabcabc\" as $output`}->*)",
			map[string][]string{
				"output": {`"abcabcabc"`},
			},
			ssaapi.WithLanguage(ssaconfig.Yak))
	})
}

func TestConstsSearchPlaceholderConst(t *testing.T) {
	t.Run("const as member call key should not be searched", func(t *testing.T) {
		code := `
a = {"hello":"world"}
a.b = "test"
`
		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			vals, err := prog.SyntaxFlowWithError(`e"b" as $result`)
			require.NoError(t, err)
			result := vals.GetValues("result")
			result.Show()
			require.Equal(t, 0, result.Len())
			return nil
		})
	})

	t.Run("const as blue print container key should not be searched", func(t *testing.T) {
		code := `
package org.example;
class Test {
	public void foo() {
	System.out.println("Hello, World!");
}
}
`
		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			vals, err := prog.SyntaxFlowWithError(`g"fo*" as $result`)
			require.NoError(t, err)
			result := vals.GetValues("result")
			result.Show()
			require.Equal(t, 0, result.Len())
			return nil
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})
}

func TestConstCompare(t *testing.T) {
	t.Run("test compare const equal:simple ", func(t *testing.T) {
		code := ` a = 1;`
		ssatest.CheckSyntaxFlow(t, code, `a?{= 1} as $result`, map[string][]string{
			"result": {"1"},
		})
	})

	t.Run("test compare const equal:native call getActualParamLen", func(t *testing.T) {
		code := ` a("param1","param2")
a("param1","param2","param3")
`
		ssatest.CheckSyntaxFlow(t, code, `a?(*<len>?{==2}) as $result`, map[string][]string{
			"result": {`Undefined-a("param1","param2")`},
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})
	t.Run("test get a have 2 param", func(t *testing.T) {
		code := `a("param1","param2")
a("param1","param2","param3")
`
		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			result, err := prog.SyntaxFlowWithError(`a()`)
			require.NoError(t, err)
			result.Show()
			return nil
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})

	t.Run("test compare const equal:string", func(t *testing.T) {
		code := `a("param1","param2")`
		ssatest.CheckSyntaxFlow(t, code, `a?{*(*?{=="param1"})} as $result`, map[string][]string{
			"result": {"Undefined-a"},
		})
	})

	t.Run("test compare const equal:value list", func(t *testing.T) {
		code := ` a1("param1","param2")
	a2("param")
`
		ssatest.CheckSyntaxFlow(t, code, `a*?{*?(<len>?{==1})} as $result`, map[string][]string{
			"result": {"Undefined-a2"},
		})
	})

	t.Run("test compare const not equal 1 ", func(t *testing.T) {
		code := `
a1 = 1
a2 = 11
a3 = true 
`
		ssatest.CheckSyntaxFlow(t, code, `a*?{!=1} as $result`, map[string][]string{
			"result": {"11", "true"},
		})
	})

	t.Run("test compare const not equal 2 ", func(t *testing.T) {
		code := `
a1 = "hello"
a2 = "world"
a3 = true 
`
		ssatest.CheckSyntaxFlow(t, code, `a*?{!="hello"} as $result`, map[string][]string{
			"result": {"\"world\"", "true"},
		})
	})

	t.Run("test compare number const lt and gt ", func(t *testing.T) {
		code := `
	a1=  66 ; 
	a2 = 999;
`
		ssatest.CheckSyntaxFlow(t, code, `a*?{ > 66 } as $result`, map[string][]string{
			"result": {"999"},
		})
		ssatest.CheckSyntaxFlow(t, code, `a*?{ >= 66 } as $result`, map[string][]string{
			"result": {"66", "999"},
		})
		ssatest.CheckSyntaxFlow(t, code, `a*?{ < 999 } as $result`, map[string][]string{
			"result": {"66"},
		})
		ssatest.CheckSyntaxFlow(t, code, `a*?{ <= 999 } as $result`, map[string][]string{
			"result": {"66", "999"},
		})
	})

	t.Run("test compare string const lt and gt ", func(t *testing.T) {
		code := `
	a1 =  "hello" ; 
	a2 =  "helloworld";
`
		ssatest.CheckSyntaxFlow(t, code, `a*?{ > "hello" } as $result`, map[string][]string{
			"result": {"\"helloworld\""},
		})
		ssatest.CheckSyntaxFlow(t, code, `a*?{ >=  "hello" } as $result`, map[string][]string{
			"result": {"\"hello\"", "\"helloworld\""},
		})
	})
}
