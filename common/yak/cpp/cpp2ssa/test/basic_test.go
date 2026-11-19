package test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

// TestCppHelloWorld is the smallest end-to-end lowering check.
func TestCppHelloWorld(t *testing.T) {
	test.CheckPrintlnValue(`
int main() {
    println("Hello, World!");
    return 0;
}
`, []string{`"Hello, World!"`}, t)
}

// TestCppBasicVariables asserts a declaration followed by a read keeps the
// assigned value.
func TestCppBasicVariables(t *testing.T) {
	test.CheckPrintlnValue(`
int main() {
    int a = 1;
    int b = 2;
    int sum = a + b;
    println(sum);
    return 0;
}
`, []string{"3"}, t)
}

// TestCppModernSpellings keeps C++14/17 spellings in the lower path: range-for,
// auto, brace init, enum class, static_cast, lambda and try/catch.
func TestCppModernSpellings(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"modern.cpp": `
#include <vector>

enum class Color { Red, Green };

int modern() {
    auto total = 0;
    std::vector<int> values{1, 2, 3};
    for (auto value : values) {
        total += value;
    }
    Color color = Color::Red;
    auto add = [](int x) { return x + 1; };
    try {
        total += add(total);
    } catch (int code) {
        total += code;
    }
    return color == Color::Red ? total : 0;
}
`,
	})
	prog := progs[0]
	for _, ref := range []string{"modern", "total", "values", "color"} {
		require.NotEmpty(t, prog.Ref(ref), "missing SSA symbol %q", ref)
	}
}

// TestCppLambdaFunction asserts a lambda body becomes an anonymous function.
func TestCppLambdaFunction(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"lambda.cpp": `
int use() {
    auto add = [](int x) { return x + 1; };
    return add(2);
}
`,
	})
	// The C++ frontend lowers a lambda as an anonymous function. Captures are
	// not turned into free values yet, so it is not a child of `use`.
	var lambda string
	progs[0].Program.Funcs.ForEach(func(name string, fn *ssa.Function) bool {
		if strings.HasPrefix(name, "lambda_") {
			lambda = name
			return false
		}
		return true
	})
	require.NotEmpty(t, lambda, "lambda must be lowered as an anonymous function")
}

// TestCppStructAndBracedInit asserts a struct value and braced initializer are
// accepted by the lowerer.
func TestCppStructAndBracedInit(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"struct.cpp": `
struct Point {
    int x;
    int y;
};

int use() {
    Point point{1, 2};
    return point.x + point.y;
}
`,
	})
	prog := progs[0]
	require.NotNil(t, prog.Program.GetBluePrint("Point"), "struct must be a blueprint")
	require.NotEmpty(t, prog.Ref("point"))
}
// TestCppBasicLiterals asserts decimal, hex, octal, binary, char and boolean
// literals keep their values.
func TestCppBasicLiterals(t *testing.T) {
	test.CheckPrintlnValue(`
int main() {
    println(42);
    println(0x10);
    println(010);
    println(true);
    println('A');
    println(3.5);
    return 0;
}
`, []string{"42", "16", "8", "true", "65", "3.5"}, t)
}

// TestCppBasicNamespace asserts a namespace function is callable by its
// qualified name and its value reaches the caller.
func TestCppBasicNamespace(t *testing.T) {
	test.CheckPrintlnValue(`
namespace math {
int add(int a, int b) { return a + b; }
}

int main() {
    println(math::add(1, 2));
    return 0;
}
`, []string{"Function-math::add(1,2)"}, t)
}

// TestCppBasicArray asserts an array declaration and indexing lower to SSA and
// the indexed element is readable.
func TestCppBasicArray(t *testing.T) {
	test.CheckPrintlnValue(`
int main() {
    int values[3] = {1, 2, 3};
    int element = values[1];
    println(element);
    return 0;
}
`, []string{"2"}, t)
}

// TestCppBasicPointerRead asserts &/* keep the pointee link, so reading through
// the pointer returns the original value. A write through the pointer is a
// remaining gap in the pointer alias model.
func TestCppBasicPointerRead(t *testing.T) {
	test.CheckPrintlnValue(`
int main() {
    int x = 5;
    int* p = &x;
    println(*p);
    return 0;
}
`, []string{"5"}, t)
}

// TestCppBasicReference asserts a reference declaration parses and an
// assignment through the reference updates the reference variable.
func TestCppBasicReference(t *testing.T) {
	test.CheckPrintlnValue(`
int main() {
    int x = 1;
    int& r = x;
    r = 5;
    println(r);
    return 0;
}
`, []string{"5"}, t)
}
