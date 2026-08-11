package antlr4yak

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/antlr/v4"
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakast"
)

func formatYakCompat(code string) (formatted string, errCount int) {
	input := antlr.NewInputStream(code)
	lex := yak.NewYaklangLexer(input)
	ts := antlr.NewCommonTokenStream(lex, antlr.TokenDefaultChannel)
	p := yak.NewYaklangParser(ts)
	vt := yakast.NewYakCompiler()
	vt.AntlrTokenStream = ts
	p.AddErrorListener(vt.GetParserErrorListener())
	vt.VisitProgram(p.Program().(*yak.ProgramContext))
	return vt.GetFormattedCode(), len(vt.GetErrors())
}

func TestGoTypedFuncCompat_ParseAndExec(t *testing.T) {
	cases := []struct {
		name string
		code string
	}{
		{
			name: "multi_param_types",
			code: `
f = func(a string, b int) {
	return a
}
assert f("hi", 1) == "hi"
`,
		},
		{
			name: "return_slice_type",
			code: `
f = func(x string) []byte {
	return x
}
assert f("ab") == "ab"
`,
		},
		{
			name: "map_interface_param",
			code: `
f = func(m map[string]interface{}) {
	return m
}
v = f({"k": 1})
assert v["k"] == 1
`,
		},
		{
			name: "untyped_still_works",
			code: `
f = func(a, b) {
	return a + b
}
assert f(1, 2) == 3
`,
		},
		{
			name: "named_func_with_types",
			code: `
func add(a int, b int) int {
	return a + b
}
assert add(2, 3) == 5
`,
		},
		{
			name: "error_return_type",
			code: `
f = func(x string) error {
	return nil
}
assert f("x") == nil
`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			formatted, errCount := formatYakCompat(tc.code)
			if errCount > 0 {
				t.Fatalf("parse/compile errors=%d code:\n%s\nformatted:\n%s", errCount, tc.code, formatted)
			}
			eng := New()
			if err := eng.SafeEval(context.Background(), tc.code); err != nil {
				t.Fatalf("exec failed: %v\ncode:\n%s", err, tc.code)
			}
		})
	}
}

func TestLogErrorMemberCallStillWorks(t *testing.T) {
	// error 不能是关键字，否则 log.error 无法解析
	code := `
called = false
log = {"error": func(msg) { called = true }}
log.error("设置内存或cpu检测")
assert called
`
	if _, errCount := formatYakCompat(code); errCount > 0 {
		t.Fatalf("log.error must parse; errors=%d", errCount)
	}
	eng := New()
	if err := eng.SafeEval(context.Background(), code); err != nil {
		t.Fatalf("exec log.error failed: %v", err)
	}
}

func TestGoTypedFuncCompat_FormatterKeepsTypes(t *testing.T) {
	code := `f = func(a string, b int) []byte {
    return a
}`
	formatted, errCount := formatYakCompat(code)
	if errCount > 0 {
		t.Fatalf("format errors=%d formatted=%#v", errCount, formatted)
	}
	for _, want := range []string{"string", "int", "[]byte"} {
		if !strings.Contains(formatted, want) {
			t.Fatalf("formatted should keep type %q, got:\n%s", want, formatted)
		}
	}
	// re-parse formatted output
	if _, errCount = formatYakCompat(formatted); errCount > 0 {
		t.Fatalf("re-parse formatted failed errors=%d:\n%s", errCount, formatted)
	}
}
