package tests

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/java/java2ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func init() {
	test.SetLanguage("java", java2ssa.CreateBuilder)
}

func CheckJavaTestCase(t *testing.T, tc test.TestCase) {
	prog, err := ssaapi.Parse(tc.Code, ssaapi.WithLanguage("java"))
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

func CheckJavaPrintlnValue(code string, want []string, t *testing.T) {
	code = CreateJavaProgram(code)
	test.CheckPrintlnValue(code, want, t)
}

func CheckAllJavaPrintlnValue(code string, want []string, t *testing.T) {
	test.CheckPrintlnValue(code, want, t)
}

func CheckJavaCode(code string, t *testing.T) {
	code = CreateJavaProgram(code)
	CheckJavaTestCase(t, test.TestCase{Code: code})
}

func CheckAllJavaCode(code string, t *testing.T) {
	CheckJavaTestCase(t, test.TestCase{Code: code})
}

func CreateJavaProgram(code string) string {
	template := `

public class Main {
    public static void main(String[] args) {
        %s
    }
}`
	code = fmt.Sprintf(template, code)
	return code
}
