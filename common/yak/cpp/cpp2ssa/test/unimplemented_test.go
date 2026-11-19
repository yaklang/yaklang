package test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

// Every test in this file is an executable specification for behaviour the C++
// frontend does not implement yet. The body states the expected SSA, and t.Skip
// records the gap so the suite stays green without hiding the missing feature.
// Remove t.Skip together with the matching TODO(ssa-cpp) in the frontend.

// TestCppUnimplementedPointerWriteThrough asserts a write through a pointer
// updates the pointee.
func TestCppUnimplementedPointerWriteThrough(t *testing.T) {
	t.Skip("TODO(ssa-cpp): pointer write-through is not modelled; *p = 7 does not update x")
	test.CheckPrintlnValue(`
int main() {
    int x = 5;
    int* p = &x;
    *p = 7;
    println(x);
    return 0;
}
`, []string{"7"}, t)
}

// TestCppUnimplementedPointerParameterWrite asserts a write through a pointer
// parameter reaches the caller's variable.
func TestCppUnimplementedPointerParameterWrite(t *testing.T) {
	t.Skip("TODO(ssa-cpp): a write through a pointer parameter does not propagate to the caller")
	test.CheckPrintlnValue(`
void set(int* target) {
    *target = 9;
}

int main() {
    int value = 1;
    set(&value);
    println(value);
    return 0;
}
`, []string{"9"}, t)
}

// TestCppUnimplementedVirtualDispatch asserts a virtual call through a base-typed
// parameter binds the override, not the base declaration.
func TestCppUnimplementedVirtualDispatch(t *testing.T) {
	t.Skip("TODO(ssa-cpp): virtual dispatch is not resolved; the call keeps the static target")
	progs := compileCppProject(t, map[string]string{
		"virtual_target.cpp": `
class Shape {
public:
    virtual int area() { return 0; }
};

class Rectangle : public Shape {
public:
    int area() override { return 4; }
};

int areaOf(Shape* shape) {
    return shape->area();
}

int use() {
    Rectangle rectangle;
    return areaOf(&rectangle);
}
`,
	})
	areaOf := funcBySymbol(t, progs[0], "areaOf")
	derived := findBlueprint(t, progs, "Rectangle")
	require.NotNil(t, derived)
	override, ok := ssa.ToFunction(derived.GetNormalMethod("area"))
	require.True(t, ok, "Rectangle::area must be a function")
	require.NotEmpty(t, callsTo(t, areaOf, override), "shape->area() must reach Rectangle::area")
}

// TestCppUnimplementedLambdaDefaultCapture asserts a default capture creates
// the free values the lambda body reads.
func TestCppUnimplementedLambdaDefaultCapture(t *testing.T) {
	t.Skip("TODO(ssa-cpp): default captures [&], [=] and this are not expanded into free values")
	progs := compileCppProject(t, map[string]string{
		"lambda_default.cpp": `
int main() {
    int x = 1;
    auto bump = [&](int by) { x = x + by; return x; };
    return bump(2);
}
`,
	})
	lambda := findLambdaFunction(t, progs[0])
	require.Equal(t, []string{"x"}, freeValueNames(t, lambda), "[&] must capture x as a free value")
}

// TestCppUnimplementedTemplateMonomorphization asserts an explicit template
// call specializes the parameter type.
func TestCppUnimplementedTemplateMonomorphization(t *testing.T) {
	t.Skip("TODO(ssa-cpp): template calls keep the template function; no instantiation is built")
	progs := compileCppProject(t, map[string]string{
		"template_instantiate.cpp": `
template <typename T>
T twice(T value) {
    return value + value;
}

int use() {
    return twice<int>(3);
}
`,
	})
	twice := funcBySymbol(t, progs[0], "twice")
	require.NotEmpty(t, twice.Params)
	parameter, ok := twice.GetValueById(twice.Params[0])
	require.True(t, ok)
	require.Equal(t, ssa.NumberTypeKind, parameter.GetType().GetTypeKind(),
		"twice<int> must specialize T to a number")
}

// TestCppUnimplementedSizeofLayout asserts sizeof uses the real object layout.
func TestCppUnimplementedSizeofLayout(t *testing.T) {
	t.Skip("TODO(ssa-cpp): sizeof/alignof evaluate to 0 because object layout is not modelled")
	test.CheckPrintlnValue(`
int main() {
    println(sizeof(int));
    println(sizeof(double));
    return 0;
}
`, []string{"4", "8"}, t)
}

// TestCppUnimplementedMemberAssignmentRead asserts a member write is visible to
// a later read of the same member.
func TestCppUnimplementedMemberAssignmentRead(t *testing.T) {
	t.Skip("TODO(ssa-cpp): a member write does not reach a later read; box.value stays undefined")
	test.CheckPrintlnValue(`
class Box {
public:
    int value;
};

int main() {
    Box box;
    box.value = 5;
    println(box.value);
    return 0;
}
`, []string{"5"}, t)
}

// TestCppUnimplementedOverloadByType asserts overloads with the same arity but
// different parameter types resolve from the argument type.
func TestCppUnimplementedOverloadByType(t *testing.T) {
	t.Skip("TODO(ssa-cpp): overload resolution compares parameter count only, not types")
	progs := compileCppProject(t, map[string]string{
		"overload_type.cpp": `
int pick(int value) {
    return 1;
}

int pick(double other) {
    return 2;
}

int use() {
    return pick(3);
}
`,
	})
	// The parameter names keep the two same-arity overloads distinguishable
	// without a type-kind check: int and double share NumberTypeKind.
	var intOverload *ssa.Function
	progs[0].Program.Funcs.ForEach(func(key string, fn *ssa.Function) bool {
		if fn.GetName() != "pick" || len(fn.Params) != 1 {
			return true
		}
		parameter, ok := fn.GetValueById(fn.Params[0])
		if ok && parameter.GetName() == "value" {
			intOverload = fn
		}
		return true
	})
	require.NotNil(t, intOverload, "the int overload must exist")
	use := funcBySymbol(t, progs[0], "use")
	require.NotEmpty(t, callsTo(t, use, intOverload), "pick(3) must call the int overload")
}

// TestCppUnimplementedOperatorOverloadCall asserts an operator expression on
// class operands binds the operator function.
func TestCppUnimplementedOperatorOverloadCall(t *testing.T) {
	t.Skip("TODO(ssa-cpp): operator expressions stay BinOp; operator overloads are not called")
	progs := compileCppProject(t, map[string]string{
		"operator_call.cpp": `
struct Vec2 {
    int x;
    int y;
};

Vec2 operator+(const Vec2& left, const Vec2& right) {
    Vec2 result;
    result.x = left.x + right.x;
    result.y = left.y + right.y;
    return result;
}

int use() {
    Vec2 a;
    Vec2 b;
    Vec2 sum = a + b;
    return sum.x;
}
`,
	})
	operator := funcBySymbol(t, progs[0], "operator+")
	use := funcBySymbol(t, progs[0], "use")
	require.NotEmpty(t, callsTo(t, use, operator), "a + b must call operator+")
}

// TestCppUnimplementedDefaultArgument asserts a default argument fills the
// missing trailing parameter at the call site.
func TestCppUnimplementedDefaultArgument(t *testing.T) {
	t.Skip("TODO(ssa-cpp): default arguments are not recorded; a missing argument stays undefined")
	test.CheckPrintlnValue(`
int add(int a, int b = 2) {
    return a + b;
}

int main() {
    println(add(3));
    return 0;
}
`, []string{"5"}, t)
}

// TestCppUnimplementedStdlibBehavior asserts a standard-library call carries
// real behaviour instead of an undefined placeholder result.
func TestCppUnimplementedStdlibBehavior(t *testing.T) {
	t.Skip("TODO(ssa-cpp): stdlib members are placeholders and return undefined")
	test.CheckPrintlnValue(`
#include <string>

int main() {
    std::string name = "cpp";
    println(name.size());
    return 0;
}
`, []string{"3"}, t)
}
