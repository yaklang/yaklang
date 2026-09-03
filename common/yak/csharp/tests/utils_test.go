package tests

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/csharp/csharp2ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func init() {
	test.SetLanguage("csharp", csharp2ssa.CreateBuilder)
}

func CheckCSharpPrintlnValue(code string, want []string, t *testing.T) {
	code = CreateCSharpProgram(code)
	test.CheckPrintlnValue(code, want, t)
}

func CheckAllCSharpPrintlnValue(code string, want []string, t *testing.T) {
	test.CheckPrintlnValue(code, want, t)
}

func CreateCSharpProgram(code string) string {
	template := `
public class Main {
    public static void Main(string[] args) {
        %s
    }
}`
	return fmt.Sprintf(template, code)
}

func TestCSharpLanguageRegistration(t *testing.T) {
	prog, err := ssaapi.Parse(`public class Main { public static void Main(string[] args) { int a = 1; println(a); } }`, ssaapi.WithLanguage("csharp"))
	require.NoError(t, err)
	require.NotNil(t, prog)
}
