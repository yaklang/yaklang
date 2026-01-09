package test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/python/python2ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func init() {
	test.SetLanguage("python", python2ssa.CreateBuilder)
}

// CheckPythonTestCase checks a Python test case.
func CheckPythonTestCase(t *testing.T, tc test.TestCase) {
	prog, err := ssaapi.Parse(tc.Code, ssaapi.WithLanguage("python"))
	require.Nil(t, err, "parse error")
	prog.Show()
	fmt.Println(prog.GetErrors().String())
	if tc.Want == nil {
		tc.Want = make([]string, 0)
	}
	if tc.Check != nil {
		tc.Check(prog, tc.Want)
	}
}

// CheckPythonPrintlnValue checks println values in Python code.
// This wraps the code in a main function for testing.
func CheckPythonPrintlnValue(code string, want []string, t *testing.T) {
	code = CreatePythonProgram(code)
	test.CheckPrintlnValue(code, want, t)
}

// CheckAllPythonPrintlnValue checks println values in complete Python code.
func CheckAllPythonPrintlnValue(code string, want []string, t *testing.T) {
	test.CheckPrintlnValue(code, want, t)
}

// CheckPythonCode checks Python code can be parsed and built.
func CheckPythonCode(code string, t *testing.T) {
	code = CreatePythonProgram(code)
	CheckPythonTestCase(t, test.TestCase{Code: code})
}

// CheckAllPythonCode checks complete Python code can be parsed and built.
func CheckAllPythonCode(code string, t *testing.T) {
	CheckPythonTestCase(t, test.TestCase{Code: code})
}

// CreatePythonProgram wraps Python code in a main function for testing.
// For Python, we can use a simple script format or a main function.
func CreatePythonProgram(code string) string {
	// Python doesn't need a wrapper like Java, but we can add one if needed
	// For now, just return the code as-is
	return code
}

