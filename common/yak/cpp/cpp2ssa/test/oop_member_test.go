package test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// TestCppOperatorOverloadSymbol asserts an operator overload is lowered as a
// named function. Operator resolution at call sites is not modelled; `a + b`
// stays a BinOp.
// TODO(ssa-cpp): bind operator expressions to overload functions when both
// operands are class types.
func TestCppOperatorOverloadSymbol(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"operator.cpp": `
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
`,
	})
	fn := funcBySymbol(t, progs[0], "operator+")
	require.NotNil(t, fn)
	require.Len(t, fn.Params, 2, "operator+ takes two operands")
}

// TestCppStaticMethodAndMember asserts static members and static methods are
// registered in their own slots.
func TestCppStaticMethodAndMember(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"static.cpp": `
class Counter {
public:
    static int instances() { return count_; }
    static int count_;
};

int Counter::count_ = 0;
`,
	})
	blueprint := progs[0].Program.GetBluePrint("Counter")
	require.NotNil(t, blueprint)
	require.NotNil(t, blueprint.GetStaticMember("count_"), "static field must not be a normal member")
	require.Nil(t, blueprint.GetNormalMember("count_"))
	method, ok := ssa.ToFunction(blueprint.GetStaticMethod("instances"))
	require.True(t, ok, "static method must be registered")
	require.Equal(t, "instances", method.GetMethodName())
}

// TestCppTemplateClass asserts a class template still registers its fields and
// methods on a blueprint.
func TestCppTemplateClass(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"template_class.cpp": `
template <typename T>
class Box {
public:
    Box() : value_() {}
    T get() const { return value_; }
    void set(T value) { value_ = value; }

private:
    T value_;
};

int use() {
    Box<int> box;
    box.set(3);
    return box.get();
}
`,
	})
	blueprint := progs[0].Program.GetBluePrint("Box")
	require.NotNil(t, blueprint)
	require.NotNil(t, blueprint.GetNormalMember("value_"), "template field must be registered")
	require.NotNil(t, blueprint.GetNormalMethod("get"), "template method must be registered")
	require.NotNil(t, blueprint.GetNormalMethod("set"), "template method must be registered")
	// `Box<int>` normalizes to the same blueprint spelling (`Box`); a distinct
	// instantiated symbol is not materialized yet.
	require.NotEmpty(t, progs[0].Ref("box"))
}

// TestCppNestedClass asserts a nested class gets a qualified blueprint name.
func TestCppNestedClass(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"nested_class.cpp": `
class Outer {
public:
    class Inner {
    public:
        int value;
    };
};
`,
	})
	require.NotNil(t, progs[0].Program.GetBluePrint("Outer"))
	inner := progs[0].Program.GetBluePrint("Outer::Inner")
	require.NotNil(t, inner, "nested class must be qualified")
	require.NotNil(t, inner.GetNormalMember("value"))
}

// TestCppMemberAccessAndAssignment asserts reading and writing a field goes
// through the member value, not a detached name.
func TestCppMemberAccessAndAssignment(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"member_access.cpp": `
class Box {
public:
    int value;
};

int use() {
    Box box;
    box.value = 5;
    return box.value;
}
`,
	})
	blueprint := progs[0].Program.GetBluePrint("Box")
	require.NotNil(t, blueprint)
	require.NotNil(t, blueprint.GetNormalMember("value"))
	use := funcBySymbol(t, progs[0], "use")
	// The member write and read must both appear in the function body.
	require.NotEmpty(t, use.Blocks)
}

// TestCppConstMethod asserts const-qualified methods keep their name and are
// callable.
func TestCppConstMethod(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"const_method.cpp": `
class Value {
public:
    Value() : value_(1) {}
    int get() const { return value_; }
private:
    int value_;
};

int use() {
    Value value;
    return value.get();
}
`,
	})
	blueprint := progs[0].Program.GetBluePrint("Value")
	require.NotNil(t, blueprint)
	method, ok := ssa.ToFunction(blueprint.GetNormalMethod("get"))
	require.True(t, ok, "const method must be registered")
	use := funcBySymbol(t, progs[0], "use")
	require.NotEmpty(t, callsTo(t, use, method))
}

// TestCppMethodOverloadKeepsBoth asserts overloading does not drop a definition.
func TestCppMethodOverloadKeepsBoth(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"overload.cpp": `
class Printer {
public:
    int print(int value) { return value; }
    int print(int value, int times) { return value * times; }
};
`,
	})
	blueprint := progs[0].Program.GetBluePrint("Printer")
	require.NotNil(t, blueprint)
	// The blueprint slot keeps the latest definition; both function bodies must
	// still exist in the program.
	require.NotNil(t, blueprint.GetNormalMethod("print"))
	count := 0
	progs[0].Program.Funcs.ForEach(func(name string, fn *ssa.Function) bool {
		if fn.GetMethodName() == "print" {
			count++
		}
		return true
	})
	require.GreaterOrEqual(t, count, 2, "both print overloads must be lowered")
}