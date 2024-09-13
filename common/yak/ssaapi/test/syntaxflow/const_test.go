package syntaxflow

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"strconv"
	"testing"
)

func TestNativeCallConst(t *testing.T) {
	code := `cc = "127.0.0.1"`
	t.Run("test const nativeCall global", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `<const(g="127*")> as $output`, map[string][]string{
			"output": {`"127.0.0.1"`},
		}, ssaapi.WithLanguage(ssaapi.Yak))
	})
	t.Run("test const nativeCall reg", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `<const(r=`+strconv.Quote(`^((0|[1-9]\d?|1\d\d|2[0-4]\d|25[0-5])\.){3}(0|[1-9]\d?|1\d\d|2[0-4]\d|25[0-5])$`)+`)> as $output`,
			map[string][]string{
				"output": {`"127.0.0.1"`},
			},
			ssaapi.WithLanguage(ssaapi.Yak))
	})
	t.Run("test const heredoc", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `<const(<<<CODE
"127.0.0.1"
CODE)> as $output`, map[string][]string{
			"output": {`"127.0.0.1"`},
		}, ssaapi.WithLanguage(ssaapi.Yak))
	})
	t.Run("test const nativeCall ex", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `<const(e="127.0.0.1")> as $output`, map[string][]string{
			"output": {`"127.0.0.1"`},
		}, ssaapi.WithLanguage(ssaapi.Yak))
	})
	t.Run("test const nativeCall error", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `<const(b="127.0.0.1")> as $output`, map[string][]string{}, ssaapi.WithLanguage(ssaapi.Yak))
	})
	t.Run("test const Syntactic sugar global", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `"127*" as $output`,
			map[string][]string{
				"output": {`"127.0.0.1"`},
			},
			ssaapi.WithLanguage(ssaapi.Yak))
	})
	t.Run("test const Syntactic sugar exact", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `"127.0.0.1" as $output`,
			map[string][]string{
				"output": {`"127.0.0.1"`},
			},
			ssaapi.WithLanguage(ssaapi.Yak))
	})
}
func TestSyntacticSugar_HavePrefix(t *testing.T) {
	code := `b = "123.131.11.12"`
	t.Run("prefix const test regexp", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `r`+strconv.Quote(`^((0|[1-9]\d?|1\d\d|2[0-4]\d|25[0-5])\.){3}(0|[1-9]\d?|1\d\d|2[0-4]\d|25[0-5])$`)+` as $output`,
			map[string][]string{
				"output": {`"123.131.11.12"`},
			},
			ssaapi.WithLanguage(ssaapi.Yak))
	})
	t.Run("prefix const test exact", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `e"123.131.11.12" as $output`, map[string][]string{
			"output": {`"123.131.11.12"`},
		}, ssaapi.WithLanguage(ssaapi.Yak))
	})
	t.Run("prefix const test glob", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `g"123*" as $output`, map[string][]string{
			"output": {`"123.131.11.12"`},
		}, ssaapi.WithLanguage(ssaapi.Yak))
	})
	t.Run("prefix heredoc global", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `g<<<CODE
123*
CODE as $output`, map[string][]string{
			"output": {`"123.131.11.12"`},
		}, ssaapi.WithLanguage(ssaapi.Yak))
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
		}, ssaapi.WithLanguage(ssaapi.Yak))
	})
}
func TestSyntacticSugar_ConstInRecursive(t *testing.T) {
	code := `
	a = "abcabcabc"
	c = a+b
	d = test(c)
	query(d)`
	t.Run("const_global", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, "query(* #{until: `\"a*\" as $output`}->*)",
			map[string][]string{
				"output": {`"abcabcabc"`},
			}, ssaapi.WithLanguage(ssaapi.Yak))
	})
	t.Run("const_reg", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, "query(* #{until: `r\"[a-z]+\" as $output`}->*)",
			map[string][]string{
				"output": {`"abcabcabc"`},
			},
			ssaapi.WithLanguage(ssaapi.Yak))
	})
	t.Run("const_exact", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, "query(* #{until: `\"abcabcabc\" as $output`}->*)",
			map[string][]string{
				"output": {`"abcabcabc"`},
			},
			ssaapi.WithLanguage(ssaapi.Yak))
	})
}
