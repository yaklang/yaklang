package test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

// TestCppFunctionCallValue checks that a direct call keeps its target and its
// arguments in the printed SSA value.
func TestCppFunctionCallValue(t *testing.T) {
	test.CheckPrintlnValue(`
int add(int a, int b) {
    return a + b;
}

int main() {
    println(add(1, 2));
    return 0;
}
`, []string{`Function-add(1,2)`}, t)
}

// TestCppFunctionParameterValue checks that a parameter reaches println as a
// Parameter value.
func TestCppFunctionParameterValue(t *testing.T) {
	test.CheckPrintlnValue(`
void show(int a) {
    println(a);
}

int main() {
    show(7);
    return 0;
}
`, []string{`Parameter-a`}, t)
}

// TestCppFunctionReturnValue checks that the call result, not the callee body,
// is the value used by the caller.
func TestCppFunctionReturnValue(t *testing.T) {
	test.CheckPrintlnValue(`
int twice(int value) {
    return value + value;
}

int main() {
    int result = twice(3);
    println(result);
    return 0;
}
`, []string{`Function-twice(3)`}, t)
}

// TestCppFunctionRecursion asserts a recursive call resolves to the function
// being built.
func TestCppFunctionRecursion(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"recursion.cpp": `
int factorial(int n) {
    if (n <= 1) {
        return 1;
    }
    return n * factorial(n - 1);
}
`,
	})
	fn := funcBySymbol(t, progs[0], "factorial")
	calls := callsTo(t, fn, fn)
	require.NotEmpty(t, calls, "recursive call must resolve to factorial")
	require.Len(t, calls[0].Args, 1, "factorial(n - 1) takes one argument")
}

// TestCppFunctionMultipleParameters asserts parameter order and names.
func TestCppFunctionMultipleParameters(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"params.cpp": `
int combine(int first, int second, int third) {
    return first + second + third;
}
`,
	})
	fn := funcBySymbol(t, progs[0], "combine")
	require.Len(t, fn.Params, 3, "three declared parameters")
	names := make([]string, 0, len(fn.Params))
	for _, id := range fn.Params {
		value, ok := fn.GetValueById(id)
		require.True(t, ok)
		names = append(names, value.GetName())
	}
	require.Equal(t, []string{"first", "second", "third"}, names)
}

// TestCppFunctionTemplate asserts a function template is lowered and callable.
func TestCppFunctionTemplate(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"template_func.cpp": `
template <typename T>
T twice(T value) {
    return value + value;
}

int use() {
    return twice(3);
}
`,
	})
	fn := funcBySymbol(t, progs[0], "twice")
	require.NotNil(t, fn)
	use := funcBySymbol(t, progs[0], "use")
	require.NotEmpty(t, callsTo(t, use, fn), "template call must resolve to the template function")
}

// TestCppFunctionVariadic asserts a C-style variadic declaration keeps its
// fixed parameters and accepts extra arguments.
func TestCppFunctionVariadic(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"variadic.cpp": `
int total(int count, ...) {
    return count;
}

int use() {
    return total(1, 2, 3);
}
`,
	})
	fn := funcBySymbol(t, progs[0], "total")
	require.Len(t, fn.Params, 1, "only the fixed parameter is declared")
	use := funcBySymbol(t, progs[0], "use")
	calls := callsTo(t, use, fn)
	require.NotEmpty(t, calls)
	require.Len(t, calls[0].Args, 3, "all call arguments are kept")
}

// TestCppFunctionDefaultArgument asserts the call site is not padded with the
// default value; the frontend records the declared default separately.
func TestCppFunctionDefaultArgument(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"default_arg.cpp": `
int addDefault(int a, int b = 10) {
    return a + b;
}

int use() {
    return addDefault(5);
}
`,
	})
	fn := funcBySymbol(t, progs[0], "addDefault")
	use := funcBySymbol(t, progs[0], "use")
	calls := callsTo(t, use, fn)
	require.NotEmpty(t, calls)
	require.Len(t, calls[0].Args, 1, "the caller passes only the explicit argument")
}

// TestCppMemberFunctionTarget asserts a member call binds to the class method.
func TestCppMemberFunctionTarget(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"member_call.cpp": `
class Counter {
public:
    int bump(int by) { return by + 1; }
};

int use() {
    Counter counter;
    return counter.bump(2);
}
`,
	})
	blueprint := progs[0].Program.GetBluePrint("Counter")
	require.NotNil(t, blueprint)
	method := blueprint.GetNormalMethod("bump")
	require.NotNil(t, method, "bump must be registered on Counter")
	fn, ok := ssa.ToFunction(method)
	require.True(t, ok)
	use := funcBySymbol(t, progs[0], "use")
	require.NotEmpty(t, callsTo(t, use, fn), "member call must target Counter::bump")
}

// TestCppPointerMemberFunctionTarget asserts -> uses the same method target.
func TestCppPointerMemberFunctionTarget(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"pointer_call.cpp": `
class Service {
public:
    int handle(int value) { return value + 1; }
};

int use() {
    Service service;
    Service* pointer = &service;
    return pointer->handle(1);
}
`,
	})
	blueprint := progs[0].Program.GetBluePrint("Service")
	require.NotNil(t, blueprint)
	method, ok := ssa.ToFunction(blueprint.GetNormalMethod("handle"))
	require.True(t, ok)
	use := funcBySymbol(t, progs[0], "use")
	require.NotEmpty(t, callsTo(t, use, method), "-> call must target Service::handle")
}
// TestCppFunctionOverloadByArity asserts overloaded functions keep distinct
// bodies and a call resolves to the definition with the matching arity.
func TestCppFunctionOverloadByArity(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"overload.cpp": `
int add(int a, int b) {
    return a + b;
}

int add(int a, int b, int c) {
    return a + b + c;
}

int use2() {
    return add(1, 2);
}

int use3() {
    return add(1, 2, 3);
}
`,
	})
	var twoArg, threeArg *ssa.Function
	progs[0].Program.Funcs.ForEach(func(key string, fn *ssa.Function) bool {
		if fn.GetName() != "add" {
			return true
		}
		switch len(fn.Params) {
		case 2:
			twoArg = fn
		case 3:
			threeArg = fn
		}
		return true
	})
	require.NotNil(t, twoArg, "two-argument overload must exist")
	require.NotNil(t, threeArg, "three-argument overload must exist")
	use2 := funcBySymbol(t, progs[0], "use2")
	require.NotEmpty(t, callsTo(t, use2, twoArg), "use2 must call the two-argument overload")
	use3 := funcBySymbol(t, progs[0], "use3")
	require.NotEmpty(t, callsTo(t, use3, threeArg), "use3 must call the three-argument overload")
}

// TestCppFunctionPointer asserts a function pointer variable holds the callee
// and a call through it targets the function.
func TestCppFunctionPointer(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"func_ptr.cpp": `
int inc(int value) {
    return value + 1;
}

int use() {
    int (*fp)(int) = inc;
    return fp(1);
}
`,
	})
	inc := funcBySymbol(t, progs[0], "inc")
	use := funcBySymbol(t, progs[0], "use")
	require.NotEmpty(t, callsTo(t, use, inc), "call through the function pointer must target inc")
}

// TestCppFunctionTemplateValue asserts the template body and parameters are
// lowered even before monomorphization.
func TestCppFunctionTemplateValue(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"template_value.cpp": `
template <typename T>
T twice(T value) {
    return value + value;
}
`,
	})
	fn := funcBySymbol(t, progs[0], "twice")
	require.Len(t, fn.Params, 1)
	require.NotEmpty(t, fn.Blocks)
}

// TestCppFunctionTemplateExplicit asserts an explicit template argument list
// still binds the call to the template function.
func TestCppFunctionTemplateExplicit(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"template_explicit.cpp": `
template <typename T>
T twice(T value) {
    return value + value;
}

int use() {
    return twice<int>(3);
}
`,
	})
	fn := funcBySymbol(t, progs[0], "twice")
	require.NotNil(t, fn)
	use := funcBySymbol(t, progs[0], "use")
	require.NotEmpty(t, callsTo(t, use, fn), "twice<int>(3) must target twice")
}
