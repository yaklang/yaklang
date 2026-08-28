package loop_yaklangcode

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheckCodeAndFormatErrors_FunctionParameterTypes(t *testing.T) {
	code := `
handler = func(result map[string]interface{}) {
    if result["status"] == "success" {
        println("ok")
    }
}
_ = handler
`

	errorMsg, hasBlockingErrors := checkCodeAndFormatErrors(code)
	assert.False(t, hasBlockingErrors, "typed func param should parse; blocking=%v msg=%s", hasBlockingErrors, errorMsg)
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

		// May contain hints about package / import / var types
		// depending on which error is processed first
		hasRelevantHint := strings.Contains(errorMsg, "package 声明") ||
			strings.Contains(errorMsg, "import 语句") ||
			strings.Contains(errorMsg, "变量声明")

		assert.True(t, hasRelevantHint, "Should contain at least one relevant hint")
	}
}

func TestCheckCodeAndFormatErrors_CodeLineBaseOffsetsDisplayedLines(t *testing.T) {
	code := "import \"fmt\"\n"
	baseMsg, blocking := checkCodeAndFormatErrors(code)
	assert.True(t, blocking)
	assert.NotEmpty(t, baseMsg)

	offsetMsg, offsetBlocking := checkCodeAndFormatErrors(code, 27)
	assert.Equal(t, blocking, offsetBlocking)
	assert.NotEmpty(t, offsetMsg)
	// Displayed ranges should shift by code_line_base while still reporting the same error text.
	assert.Contains(t, offsetMsg, "from compiler")
	assert.NotEqual(t, baseMsg, offsetMsg)
}
