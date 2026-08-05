package format

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAllBuiltinCompilerErrorMessagesHaveHints(t *testing.T) {
	for _, tc := range AllBuiltinCompilerErrorMessages {
		t.Run(tc.name, func(t *testing.T) {
			hint := lookupCompilerErrorHint(tc.message, "")
			require.NotEmpty(t, hint, "expected hint for %q", tc.message)
		})
	}
}

func TestLookupCompilerErrorHint_ExternLibEnrichment(t *testing.T) {
	msg := `ExternLib [poc] don't has [appendHeade], maybe you meant appendHeader ?`
	hint := lookupCompilerErrorHint(msg, "")
	require.Contains(t, hint, "已自动附加 YakDocument")
	require.Contains(t, hint, "appendHeader")
}

func TestLookupCompilerErrorHint_ExternTypeFallback(t *testing.T) {
	msg := `ExternType [[]number] don't has [CCCCC], maybe you meant Cap ?`
	hint := lookupCompilerErrorHint(msg, "")
	require.NotEmpty(t, hint)
	require.Contains(t, hint, "不存在此成员")
}

func TestLookupCompilerErrorHint_Fallback(t *testing.T) {
	hint := lookupCompilerErrorHint("totally unknown compiler message xyz", "")
	require.NotEmpty(t, hint)
	require.Contains(t, hint, "编译器/静态分析报错")
}

func TestLookupCompilerErrorHint_PocPostArity(t *testing.T) {
	msg := `The function call returns (lowhttp.LowhttpResponse, http.Request, error) type, but 2 variables on the left side.`
	hint := lookupCompilerErrorHint(msg, `rsp, err := poc.Post(url)`)
	require.Contains(t, hint, "rsp, req, err")
	require.Contains(t, hint, "poc.Post")
}

func TestExtractCoreCompilerMessage(t *testing.T) {
	raw := `[Error]: Value undefined:foo in [1:1 -- 1:4] from SSA:TypeCheck`
	assert.Equal(t, "Value undefined:foo", ExtractCoreCompilerMessage(raw))
}

func TestCheckAndFormat_CommonCasesHaveHints(t *testing.T) {
	cases := map[string]string{
		"undefined":     "undefinedFunc()",
		"invalid_field": "x=1\nx.foo",
		"multi_assign":  "a, b = 1, 2, 3",
		"break":         "break",
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) {
			errorMsg, hasBlocking, _ := CheckAndFormat(code, YakRunnerDefaults(0)...)
			require.True(t, hasBlocking, "expected blocking errors")
			require.Contains(t, errorMsg, "AI助手提示:", "expected AI hint in output")
		})
	}
}

func TestFormatSingleForCopy_IncludesLocation(t *testing.T) {
	code := "undefinedFunc()\n"
	_, hasBlocking, results := CheckAndFormat(code, CopyAllDefaults("yak")...)
	require.True(t, hasBlocking)
	require.NotEmpty(t, results)

	single := FormatSingleForCopy(code, results[0], CopyAllDefaults("yak")...)
	require.NotEmpty(t, single)
	require.Contains(t, single, "修改建议:")
	require.Contains(t, single, "in [")
}

func TestLookupCompilerErrorHint_ByteLiteral(t *testing.T) {
	hint := lookupCompilerErrorHint("T should be byte, but got number", `body := []byte{172, 237}`)
	require.Contains(t, hint, "0x")
	require.Contains(t, hint, "byte")
	require.NotContains(t, hint, "append(a, b...)")
}

func TestLookupCompilerErrorHint_AppendBytes(t *testing.T) {
	hint := lookupCompilerErrorHint("T should be byte, but got bytes|[]any", `frame = append(lengthPrefix, rawPayload...)`)
	require.Contains(t, hint, "append(a, b...)")
	require.NotContains(t, hint, "[]byte{172")

	hint2 := lookupCompilerErrorHint("T should be byte, but got number", `defaultPayload = append(defaultPayload, tcNull...)`)
	require.Contains(t, hint2, "append")
}

func TestLookupCompilerErrorHint_FuncReturnType(t *testing.T) {
	hint := lookupCompilerErrorHint("mismatched input '[' expecting ')'", `build = func(frame) []byte {`)
	require.Contains(t, hint, "返回类型")
}
