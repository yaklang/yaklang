package test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

// TestCppCastStatic asserts static_cast lowers to a dedicated typecast
// instruction instead of passing the operand through.
func TestCppCastStatic(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"cast.cpp": `
int use(int value) {
    return static_cast<int>(value);
}
`,
	})
	fn := funcBySymbol(t, progs[0], "use")
	casts := typeCastsIn(t, fn)
	require.Len(t, casts, 1, "static_cast must emit one typecast")
	require.Equal(t, ssa.NumberTypeKind, casts[0].GetType().GetTypeKind())
	operand, ok := fn.GetValueById(casts[0].Value)
	require.True(t, ok)
	require.Equal(t, "value", operand.GetName())
}

// TestCppCastCoversNamedForms asserts all four named casts and the C-style cast
// share the typecast instruction.
func TestCppCastCoversNamedForms(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"casts.cpp": `
int use(int value) {
    int a = static_cast<int>(value);
    int b = const_cast<int>(value);
    int c = reinterpret_cast<int>(value);
    int d = dynamic_cast<int>(value);
    int e = (int)value;
    return a + b + c + d + e;
}
`,
	})
	fn := funcBySymbol(t, progs[0], "use")
	casts := typeCastsIn(t, fn)
	require.Len(t, casts, 5, "four named casts and one C-style cast")
	for _, cast := range casts {
		require.Equal(t, ssa.NumberTypeKind, cast.GetType().GetTypeKind())
	}
}

// TestCppCastFunctional asserts the functional form `int(value)` is also a
// typecast and keeps the value visible to println.
func TestCppCastFunctional(t *testing.T) {
	test.CheckPrintlnValue(`
int main() {
    int value = 3;
    int cast = int(value);
    println(cast);
    return 0;
}
`, []string{"castType(number, 3)"}, t)
}

// TestCppCastKeepsValue asserts a cast does not lose the operand value.
func TestCppCastKeepsValue(t *testing.T) {
	test.CheckPrintlnValue(`
int main() {
    double value = 2.5;
    println(static_cast<int>(value));
    return 0;
}
`, []string{"castType(number, 2.5)"}, t)
}

// TestCppCastRegexp covers the regexp form of the println assertions: every
// cast prints through the typecast value.
func TestCppCastRegexp(t *testing.T) {
	test.CheckPrintlnValueRegexp(`
int main() {
    int value = 1;
    println(static_cast<long>(value));
    return 0;
}
`, []string{`castType\(.*, 1\)`}, t)
}
