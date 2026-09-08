package tests

import (
	"fmt"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/csharp/csharp2ssa"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func init() {
	test.SetLanguage("csharp", csharp2ssa.CreateBuilder)
}

func csharpErrorKindErrors(prog *ssaapi.Program) []*ssa.SSAError {
	return lo.Filter(prog.GetErrors(), func(err *ssa.SSAError, _ int) bool {
		return err != nil && err.Kind == ssa.Error
	})
}

func requireCSharpCompileErrorFree(t *testing.T, prog *ssaapi.Program) {
	t.Helper()
	require.NotNil(t, prog)
	require.Empty(t, csharpErrorKindErrors(prog), prog.GetErrors().String())
}

func CheckCSharpPrintlnValue(code string, want []string, t *testing.T) {
	code = CreateCSharpProgram(code)
	test.CheckNoError(t, code)
	test.CheckPrintlnValue(code, want, t)
}

func CheckAllCSharpPrintlnValue(code string, want []string, t *testing.T) {
	test.CheckNoError(t, code)
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
	src := `public class Main { public static void Main(string[] args) { int a = 1; println(a); } }`
	prog, err := ssaapi.Parse(src, ssaapi.WithLanguage("csharp"))
	require.NoError(t, err)
	requireCSharpCompileErrorFree(t, prog)
	got := lo.Map(
		prog.Ref("println").GetUsers().Flat(func(v *ssaapi.Value) ssaapi.Values {
			return ssaapi.Values{v.GetOperand(1)}
		}),
		func(v *ssaapi.Value, _ int) string { return v.String() },
	)
	require.Equal(t, []string{"1"}, got)
}
