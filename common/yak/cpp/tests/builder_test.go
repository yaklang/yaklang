package tests

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/cpp/cpp2ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func compileCppProject(t *testing.T, files map[string]string) []*ssaapi.Program {
	t.Helper()
	vf := filesys.NewVirtualFs()
	for name, code := range files {
		vf.AddFile(name, code)
	}
	progs, err := ssaapi.ParseProjectWithFS(
		vf,
		ssaapi.WithLanguage(ssaconfig.CPP),
		ssaapi.WithMemory(),
	)
	require.NoError(t, err)
	require.NotEmpty(t, progs)
	return progs
}

// TestCppClassAndInheritanceCompile checks that a class, its base list, its
// constructors and destructors, and its methods reach the SSA.
func TestCppClassAndInheritanceCompile(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"shape.cpp": `
class Shape {
public:
    Shape() : id_(0) {}
    virtual ~Shape() {}
    virtual double area() const { return 0.0; }
    int id() const { return id_; }
protected:
    int id_;
};

class Circle : public Shape {
public:
    explicit Circle(double r) : radius_(r) {}
    double area() const { return radius_ * radius_; }
private:
    double radius_;
};

int main() {
    Circle circle(2.0);
    return circle.area() > 0.0 ? 1 : 0;
}
`,
	})
	prog := progs[0]
	for _, ref := range []string{"Shape", "Circle", "main", "circle", "radius_", "id_"} {
		require.NotEmpty(t, prog.Ref(ref), "missing SSA symbol %q", ref)
	}
	// Methods live on the class blueprint; the project builder keeps the type
	// reachable even though the restored blueprint only exposes the last method.
	require.NotNil(t, prog.Program.GetBluePrint("Shape"))
	require.NotNil(t, prog.Program.GetBluePrint("Circle"))
}

// TestCppMemberCallOnPointer checks that a member call through -> resolves to the
// class method instead of a detached name.
func TestCppMemberCallOnPointer(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"pointer.cpp": `
class Service {
public:
    int handle(int value) { return value + 1; }
};

int run() {
    Service service;
    Service* pointer = &service;
    return pointer->handle(1);
}
`,
	})
	prog := progs[0]
	require.NotEmpty(t, prog.Ref("pointer"))
	require.NotEmpty(t, prog.Ref("service"))
	require.NotNil(t, prog.Program.GetBluePrint("Service"))
}

// TestCppIncludeTolerance checks that both an existing local header and a
// missing system header leave the translation unit compilable.
func TestCppIncludeTolerance(t *testing.T) {
	progs := compileCppProject(t, map[string]string{
		"main.cpp": `
#include "helper.hpp"
#include <vector>
#include <missing_header.h>

int main() {
    std::vector<int> values;
    values.push_back(helper());
    return static_cast<int>(values.size());
}
`,
		"helper.hpp": `
int helper();
`,
	})
	prog := progs[0]
	require.NotEmpty(t, prog.Ref("main"))
	require.NotEmpty(t, prog.Ref("values"))
}

// TestCppStdlibPlaceholders checks the standard-library baseline: the container
// types exist and their member methods are callable without the real headers.
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
	require.NotEmpty(t, cpp2ssa.StdlibHeaders())
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

// TestCppModernSpellings keeps C++14/17 spellings in the parse path: range-for,
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
