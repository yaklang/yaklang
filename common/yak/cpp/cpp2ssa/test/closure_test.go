package test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

// findLambdaFunction returns the first lowered lambda in prog.
func findLambdaFunction(t *testing.T, prog *ssaapi.Program) *ssa.Function {
	t.Helper()
	var found *ssa.Function
	prog.Program.Funcs.ForEach(func(key string, fn *ssa.Function) bool {
		if strings.HasPrefix(key, "lambda_") || strings.HasPrefix(fn.GetName(), "lambda_") {
			found = fn
			return false
		}
		return true
	})
	require.NotNil(t, found, "lambda function was not lowered")
	return found
}

// TestCppLambdaCaptureByValue asserts an explicit by-value capture becomes a
// free-value parameter.
func TestCppLambdaCaptureByValue(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"lambda.cpp": `
int main() {
    int x = 2;
    auto add = [x](int y) { return x + y; };
    println(add(3));
    return 0;
}
`,
	})
	prog := progs[0]
	mainFn := funcBySymbol(t, prog, "main")
	lambda := findLambdaFunction(t, prog)
	require.Equal(t, []string{"x"}, freeValueNames(t, lambda), "capture list must produce free value x")
	require.NotEmpty(t, callsTo(t, mainFn, lambda), "add(3) must call the lambda")
}

// TestCppLambdaCaptureByReference asserts `[&x]` also produces a free value and
// that a non-captured variable is not present.
func TestCppLambdaCaptureByReference(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"lambda_ref.cpp": `
int main() {
    int x = 1;
    int other = 9;
    auto bump = [&x](int by) { x = x + by; return x; };
    bump(2);
    println(x);
    return 0;
}
`,
	})
	prog := progs[0]
	lambda := findLambdaFunction(t, prog)
	require.Equal(t, []string{"x"}, freeValueNames(t, lambda), "only the explicit capture becomes a free value")
	_ = funcBySymbol(t, prog, "main")
}

// TestCppLambdaWithoutCapture asserts a capture-less lambda still has its
// declared parameters and no free values.
func TestCppLambdaWithoutCapture(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"lambda_empty.cpp": `
int main() {
    auto twice = [](int value) { return value + value; };
    println(twice(2));
    return 0;
}
`,
	})
	lambda := findLambdaFunction(t, progs[0])
	require.Empty(t, freeValueNames(t, lambda))
	require.Len(t, lambda.Params, 1, "lambda parameters must be SSA parameters")
	param, ok := lambda.GetValueById(lambda.Params[0])
	require.True(t, ok)
	require.Equal(t, "value", param.GetName())
}
