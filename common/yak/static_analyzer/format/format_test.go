package format_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/format"
)

func TestCheckAndFormat_FunctionParameterTypes(t *testing.T) {
	code := `
bruteTask.SetResultHandler(func(result map[string]interface{}) {
    if result["status"] == "success" {
        found = true
    }
})
`
	errorMsg, hasBlockingErrors, _ := format.CheckAndFormat(code, format.YakRunnerDefaults(0)...)
	assert.True(t, hasBlockingErrors)
	assert.Contains(t, errorMsg, "AI助手提示:")
}

func TestCheckAndFormat_EmptyCode(t *testing.T) {
	errorMsg, hasBlockingErrors, results := format.CheckAndFormat("")
	assert.False(t, hasBlockingErrors)
	assert.Empty(t, errorMsg)
	assert.Nil(t, results)
}

func TestCheckAndFormat_CodeLineBaseOffsetsDisplayedLines(t *testing.T) {
	code := "func bad(x string) {\n}\n"
	baseMsg, blocking, _ := format.CheckAndFormat(code, format.YakRunnerDefaults(0)...)
	assert.True(t, blocking)
	assert.NotEmpty(t, baseMsg)

	offsetMsg, offsetBlocking, _ := format.CheckAndFormat(code, format.YakRunnerDefaults(27)...)
	assert.Equal(t, blocking, offsetBlocking)
	assert.NotEmpty(t, offsetMsg)
	assert.Contains(t, offsetMsg, "from compiler")
	assert.NotEqual(t, baseMsg, offsetMsg)
}

func TestCopyAllDefaults_IncludesAllIssues(t *testing.T) {
	code := `
package main
import "fmt"
func test(param string) {
    var result map[string]interface{}
    fmt.Println(result)
}
`
	limited, _, _ := format.CheckAndFormat(code, format.YakRunnerDefaults(0)...)
	all, _, _ := format.CheckAndFormat(code, format.CopyAllDefaults("yak")...)
	assert.True(t, len(all) >= len(limited))
}
