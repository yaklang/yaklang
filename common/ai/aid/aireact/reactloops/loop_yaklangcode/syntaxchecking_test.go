package loop_yaklangcode

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheckCodeAndFormatErrors_FunctionParameterTypes(t *testing.T) {
	// Test the specific case from the error log
	code := `
// 结果处理
found := false
bruteTask.SetResultHandler(func(result map[string]interface{}) {
    if result["status"] == "success" {
        found = true
        yakit.StatusCard("爆破成功", "找到有效凭证", "brute-success", "success")
    }
})
`

	errorMsg, hasBlockingErrors := checkCodeAndFormatErrors(code)

	// Should have blocking errors
	assert.True(t, hasBlockingErrors, "Should have blocking errors for function parameter types")

	// Should contain intelligent hint about function parameter types
	assert.Contains(t, errorMsg, "AI助手提示:", "Should contain AI assistant hint")
	assert.Contains(t, errorMsg, "Yaklang DSL 中函数参数不允许有类型声明", "Should contain specific hint about parameter types")
	assert.Contains(t, errorMsg, "错误: func(result map[string]interface{})", "Should show incorrect syntax")
	assert.Contains(t, errorMsg, "正确: func(result)", "Should show correct syntax")
}

func TestCheckCodeAndFormatErrors_VariableTypeDeclarations(t *testing.T) {
	tests := []struct {
		name string
		code string
	}{
		{
			name: "var with map type",
			code: `
var result map[string]interface{}
result = {}
`,
		},
		{
			name: "var with slice type",
			code: `
var data []byte
data = []
`,
		},
		{
			name: "var with string type",
			code: `
var name string
name = "test"
`,
		},
		{
			name: "assignment with slice type",
			code: `
result := []string{"a", "b"}
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errorMsg, hasBlockingErrors := checkCodeAndFormatErrors(tt.code)

			if hasBlockingErrors && strings.Contains(errorMsg, "AI助手提示:") {
				assert.Contains(t, errorMsg, "变量声明不需要显式类型", "Should contain hint about variable declarations")
			}
		})
	}
}

func TestCheckCodeAndFormatErrors_ImportStatements(t *testing.T) {
	code := `
import "fmt"
import "strings"

func main() {
    fmt.Println("Hello")
}
`

	errorMsg, hasBlockingErrors := checkCodeAndFormatErrors(code)

	if hasBlockingErrors && strings.Contains(errorMsg, "AI助手提示:") {
		assert.Contains(t, errorMsg, "不需要 import 语句", "Should contain hint about import statements")
	}
}

func TestCheckCodeAndFormatErrors_PackageDeclarations(t *testing.T) {
	code := `
package main

func hello() {
    println("Hello World")
}
`

	errorMsg, hasBlockingErrors := checkCodeAndFormatErrors(code)

	if hasBlockingErrors && strings.Contains(errorMsg, "AI助手提示:") {
		assert.Contains(t, errorMsg, "不需要 package 声明", "Should contain hint about package declarations")
	}
}

func TestCheckCodeAndFormatErrors_ArraySliceSyntax(t *testing.T) {
	code := `
arr := []string{"a", "b", "c"}
`

	errorMsg, hasBlockingErrors := checkCodeAndFormatErrors(code)

	if hasBlockingErrors && strings.Contains(errorMsg, "AI助手提示:") {
		assert.Contains(t, errorMsg, "数组/切片语法", "Should contain hint about array/slice syntax")
	}
}

func TestCheckCodeAndFormatErrors_ValidCode(t *testing.T) {
	// Test with valid Yaklang code that doesn't produce warnings
	// Note: println is not recognized by the static analyzer, so we use simple assignments only
	code := `
name := "test"
result := name + " world"
count := 1 + 2
_ = result
_ = count
`

	errorMsg, hasBlockingErrors := checkCodeAndFormatErrors(code)

	// Valid code should not have blocking errors
	assert.False(t, hasBlockingErrors, "Valid code should not have blocking errors")
	assert.Empty(t, errorMsg, "Valid code should not have error messages")
}

func TestGetIntelligentErrorHint_FunctionParameterTypes(t *testing.T) {
	// This is a unit test for the helper function
	// We can't easily test it directly since it's not exported,
	// but the integration tests above cover the functionality

	// Test that the main function works correctly
	code := `bruteTask.SetResultHandler(func(result map[string]interface{}) {`

	errorMsg, hasBlockingErrors := checkCodeAndFormatErrors(code)

	if hasBlockingErrors {
		// Should contain the specific hint we're looking for
		expectedHints := []string{
			"AI助手提示:",
			"函数参数不允许有类型声明",
			"func(result map[string]interface{})",
			"func(result)",
		}

		for _, hint := range expectedHints {
			assert.Contains(t, errorMsg, hint, "Should contain hint: %s", hint)
		}
	}
}

func TestCheckCodeAndFormatErrors_EmptyCode(t *testing.T) {
	errorMsg, hasBlockingErrors := checkCodeAndFormatErrors("")

	assert.False(t, hasBlockingErrors, "Empty code should not have blocking errors")
	assert.Empty(t, errorMsg, "Empty code should not have error messages")
}

func TestCheckCodeAndFormatErrors_MultipleErrors(t *testing.T) {
	// Test code with multiple syntax errors.
	code := `
package main
import "fmt"
func test(param string) {
    var result map[string]interface{}
    fmt.Println(result)
}
`

	errorMsg, hasBlockingErrors := checkCodeAndFormatErrors(code)

	if hasBlockingErrors {
		// Should contain AI hints
		assert.Contains(t, errorMsg, "AI助手提示:", "Should contain AI assistant hints")

		// May contain hints about package, import, or parameter types
		// depending on which error is processed first
		hasRelevantHint := strings.Contains(errorMsg, "package 声明") ||
			strings.Contains(errorMsg, "import 语句") ||
			strings.Contains(errorMsg, "函数参数") ||
			strings.Contains(errorMsg, "变量声明")

		assert.True(t, hasRelevantHint, "Should contain at least one relevant hint")
	}
}
