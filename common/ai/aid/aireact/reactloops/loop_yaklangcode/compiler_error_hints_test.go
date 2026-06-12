package loop_yaklangcode

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAllBuiltinCompilerErrorMessagesHaveHints(t *testing.T) {
	for _, tc := range allBuiltinCompilerErrorMessages {
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

func TestExtractCoreCompilerMessage(t *testing.T) {
	raw := `[Error]: Value undefined:foo in [1:1 -- 1:4] from SSA:TypeCheck`
	assert.Equal(t, "Value undefined:foo", extractCoreCompilerMessage(raw))
}

func TestCheckCodeAndFormatErrors_CommonCasesHaveHints(t *testing.T) {
	cases := map[string]string{
		"undefined":     "undefinedFunc()",
		"invalid_field": "x=1\nx.foo",
		"multi_assign":  "a, b = 1, 2, 3",
		"break":         "break",
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) {
			errorMsg, hasBlocking := checkCodeAndFormatErrors(code)
			require.True(t, hasBlocking, "expected blocking errors")
			require.Contains(t, errorMsg, "AI助手提示:", "expected AI hint in output")
		})
	}
}

func TestCheckCodeAndFormatErrors_FunctionParameterTypesIntegration(t *testing.T) {
	code := `bruteTask.SetResultHandler(func(result map[string]interface{}) {
    println(result)
})`
	errorMsg, hasBlocking := checkCodeAndFormatErrors(code)
	require.True(t, hasBlocking)
	require.Contains(t, errorMsg, "AI助手提示:")
	require.True(t, strings.Contains(errorMsg, "函数参数不允许有类型声明") ||
		strings.Contains(errorMsg, "语法解析失败") ||
		strings.Contains(errorMsg, "编译器/静态分析报错"))
}
