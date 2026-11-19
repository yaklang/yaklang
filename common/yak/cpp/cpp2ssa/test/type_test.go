package test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/cpp/cpp2ssa"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// TestCppPrimitiveTypes asserts the scalar keyword types map to the SSA basic
// type kinds.
func TestCppPrimitiveTypes(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"types.cpp": `
int counter;
double ratio;
bool enabled;
char letter;
`,
	})
	prog := progs[0]
	require.Equal(t, ssa.NumberTypeKind, firstValue(t, prog, "counter").GetTypeKind())
	require.Equal(t, ssa.NumberTypeKind, firstValue(t, prog, "ratio").GetTypeKind())
	require.Equal(t, ssa.BooleanTypeKind, firstValue(t, prog, "enabled").GetTypeKind())
	require.Equal(t, ssa.ByteTypeKind, firstValue(t, prog, "letter").GetTypeKind())
}

// TestCppAutoType asserts auto keeps the initializer's type.
func TestCppAutoType(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"auto.cpp": `
int infer() {
    auto target = 3;
    return target;
}
`,
	})
	require.True(t, anyRefKind(progs[0], "target", ssa.NumberTypeKind),
		"auto must take the initializer type")
}

// TestCppPointerType asserts a pointer declarator wraps the element type.
func TestCppPointerType(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"pointer.cpp": `
int deref(int* pointer) {
    return *pointer;
}
`,
	})
	fn := funcBySymbol(t, progs[0], "deref")
	require.Len(t, fn.Params, 1)
	parameter, ok := fn.GetValueById(fn.Params[0])
	require.True(t, ok)
	object, ok := ssa.ToObjectType(parameter.GetType())
	require.True(t, ok, "pointer parameter must be an object type, got %s", parameter.GetType())
	require.Equal(t, ssa.PointerKind, object.Kind, "pointer parameter must keep its level")
}

// TestCppTypedefAlias asserts a typedef name resolves to the alias target.
func TestCppTypedefAlias(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"alias.cpp": `
typedef int Number;
using Score = int;

int use() {
    Number count = 1;
    Score score = 2;
    return count + score;
}
`,
	})
	prog := progs[0]
	require.Equal(t, ssa.NumberTypeKind, firstValue(t, prog, "count").GetTypeKind())
	require.Equal(t, ssa.NumberTypeKind, firstValue(t, prog, "score").GetTypeKind())
}

// TestCppClassTypeKind asserts an object of a class type is typed by the
// blueprint, not by any.
func TestCppClassTypeKind(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"class_type.cpp": `
class Point {
public:
    int x;
};

int use() {
    Point target;
    return 0;
}
`,
	})
	require.True(t, anyRefKind(progs[0], "target", ssa.ClassBluePrintTypeKind),
		"class object must keep the blueprint type")
}

// TestCppEnumClassSymbol asserts a scoped enum keeps its enumerator symbols.
func TestCppEnumClassSymbol(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"enum.cpp": `
enum class Color { Red, Green };

int use() {
    Color color = Color::Red;
    return color == Color::Red ? 1 : 0;
}
`,
	})
	prog := progs[0]
	require.NotEmpty(t, prog.Ref("color"))
	require.NotEmpty(t, prog.Ref("Color"))
}

// TestCppStdlibPlaceholders keeps the standard-library baseline: the container
// types exist and their members are callable without the real headers.
func TestCppStdlibPlaceholders(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"stdlib.cpp": `
#include <string>
#include <map>
#include <vector>

int use() {
    std::string name = "cpp";
    std::vector<int> numbers;
    numbers.push_back(name.size());
    std::map<std::string, int> counters;
    counters["a"] = 1;
    return numbers.size() + counters.size();
}
`,
	})
	prog := progs[0]
	for _, ref := range []string{"use", "name", "numbers", "counters"} {
		require.NotEmpty(t, prog.Ref(ref), "missing SSA symbol %q", ref)
	}
	require.NotNil(t, prog.Program.GetBluePrint("std::string"), "std::string placeholder must reach the program")
	require.Contains(t, cpp2ssa.StdlibHeaders(), "vector")
	require.Contains(t, cpp2ssa.StdlibTypeNames(), "string")
}

// TestCppHeaderExtensionOwnership states the .h policy in executable form: .h
// stays with C, C++ headers use .hpp/.hxx/.hh.
func TestCppHeaderExtensionOwnership(t *testing.T) {
	require.False(t, cpp2ssa.IsCppHeaderExtension("include/legacy.h"))
	require.True(t, cpp2ssa.IsCppHeaderExtension("include/modern.hpp"))
	require.True(t, cpp2ssa.IsCppHeaderExtension("include/modern.hxx"))
	require.True(t, cpp2ssa.IsCppHeaderExtension("include/modern.hh"))
	require.True(t, strings.HasSuffix(ssaconfig.CPP.GetFileExt(), ".cpp"))
}

func firstValue(t *testing.T, prog *ssaapi.Program, name string) *ssaapi.Value {
	t.Helper()
	values := prog.Ref(name)
	require.NotEmptyf(t, values, "missing SSA symbol %q", name)
	return values[0]
}

// anyRefKind reports whether one value of name has the requested type kind.
func anyRefKind(prog *ssaapi.Program, name string, kind ssa.TypeKind) bool {
	for _, value := range prog.Ref(name) {
		if value.GetTypeKind() == kind {
			return true
		}
	}
	return false
}